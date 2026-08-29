package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 订阅计费统计窗口（固定周期桶，非自然月/周）。
const (
	SubscriptionWindow5hSeconds         = 5 * 3600       // 5h
	SubscriptionWindow7dSeconds         = 7 * 24 * 3600  // 7d
	SubscriptionWindow31dSeconds        = 31 * 24 * 3600 // 31d
	SubscriptionDefaultMonthlyUSD       = 60             // 默认月额度（USD）
	SubscriptionDefaultFiveHourRatioBps = 2000           // 默认 5h 占比 20%
	SubscriptionDefaultWeeklyRatioBps   = 5000           // 默认周占比 50%
)

// 订阅计费展示窗口 key
const (
	SubscriptionWindowName5h  = "5h"
	SubscriptionWindowName7d  = "7d"
	SubscriptionWindowName31d = "31d"
)

// SubscriptionUsageWindow 单窗口用量展示。
type SubscriptionUsageWindow struct {
	WindowSizeSeconds int64   `json:"window_size_seconds"`
	UsedQuota         int64   `json:"used_quota"`
	LimitQuota        int64   `json:"limit_quota"`
	UsedPercent       float64 `json:"used_percent"`    // 允许 >100
	DisplayPercent    float64 `json:"display_percent"` // = min(used_percent, 100)
	OverLimit         bool    `json:"over_limit"`
	ResetAt           int64   `json:"reset_at"`            // 桶结束时间
	ResetAfterSeconds int64   `json:"reset_after_seconds"` // 距重置的剩余秒数
}

// SubscriptionPerModelUsage per-model 月度用量展示。
type SubscriptionPerModelUsage struct {
	Model       string  `json:"model"`
	UsedQuota   int64   `json:"used_quota"`
	LimitQuota  int64   `json:"limit_quota"`
	UsedPercent float64 `json:"used_percent"` // 允许 >100
	OverLimit   bool    `json:"over_limit"`
}

// SubscriptionUsageData 用量接口返回体。
type SubscriptionUsageData struct {
	UpdatedAt int64                              `json:"updated_at"`
	Partial   bool                               `json:"partial"`
	Windows   map[string]SubscriptionUsageWindow `json:"windows"`
	PerModel  []SubscriptionPerModelUsage        `json:"per_model"`
}

// SubscriptionUSDToQuota 将 USD 金额换算为 quota。
// 订阅额度属于展示/统计口径（不参与单次扣费），因此使用钱包域
// （JavaScript 安全整数，int53）做严格换算，而非单次扣费的 int32 边界。
func SubscriptionUSDToQuota(usd float64) (int64, error) {
	if math.IsNaN(usd) || math.IsInf(usd, 0) || usd < 0 {
		return 0, fmt.Errorf("invalid usd amount: %v", usd)
	}
	quota, err := common.WalletQuotaFromDecimalStrict(
		decimal.NewFromFloat(usd).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
	if err != nil {
		return 0, err
	}
	return int64(quota), nil
}

// SubscriptionQuotaToUSD 将 quota 换算为 USD 展示值。
func SubscriptionQuotaToUSD(quota int64) float64 {
	if quota <= 0 {
		return 0
	}
	return decimal.NewFromInt(quota).Div(decimal.NewFromFloat(common.QuotaPerUnit)).InexactFloat64()
}

// SubscriptionRatioBpsFromPercent 将百分比（0-100）换算为 basis points。
func SubscriptionRatioBpsFromPercent(percent float64) (int, error) {
	if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > 100 {
		return 0, fmt.Errorf("invalid ratio percent: %v", percent)
	}
	return int(math.Round(percent * 100)), nil
}

// SubscriptionBaselineBpsFromPercent 将基线百分比换算为 basis points。
// 与 SubscriptionRatioBpsFromPercent 不同，基线允许 >100（表示渠道已超限），
// 因此不设上界，仅拒绝 NaN/±Inf/负数。
func SubscriptionBaselineBpsFromPercent(percent float64) (int, error) {
	if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 {
		return 0, fmt.Errorf("invalid baseline percent: %v", percent)
	}
	return int(math.Round(percent * 100)), nil
}

// ValidateSubscriptionBillingConfig 校验订阅计费配置：
// 月额度>0、0≤占比≤100（bps 0-10000）、"*" 至多一个。
func ValidateSubscriptionBillingConfig(cfg *dto.SubscriptionBillingConfig) error {
	if cfg == nil {
		return errors.New("subscription billing config is required")
	}
	if cfg.BillingMode != dto.SubscriptionBillingModePayAsYouGo &&
		cfg.BillingMode != dto.SubscriptionBillingModeSubscribe {
		return fmt.Errorf("invalid billing_mode: %d", cfg.BillingMode)
	}
	if cfg.MonthlyTotalQuota <= 0 {
		return errors.New("monthly_total_quota must be greater than 0")
	}
	if cfg.FiveHourRatioBps < 0 || cfg.FiveHourRatioBps > 10000 {
		return fmt.Errorf("five_hour_ratio must be between 0%% and 100%%, got %d bps", cfg.FiveHourRatioBps)
	}
	if cfg.WeeklyRatioBps < 0 || cfg.WeeklyRatioBps > 10000 {
		return fmt.Errorf("weekly_ratio must be between 0%% and 100%%, got %d bps", cfg.WeeklyRatioBps)
	}
	// 与前端校验对齐：5h 与每周是同一总额度的子窗口，相加不得超过 100%。
	if cfg.FiveHourRatioBps+cfg.WeeklyRatioBps > 10000 {
		return fmt.Errorf("five_hour_ratio + weekly_ratio must not exceed 100%%, got %d bps", cfg.FiveHourRatioBps+cfg.WeeklyRatioBps)
	}
	wildcard := 0
	for _, tier := range cfg.ModelTiers {
		if strings.TrimSpace(tier.Model) == "" {
			return errors.New("model tier model must not be empty")
		}
		if tier.MonthlyQuota < 0 {
			return fmt.Errorf("model tier %s monthly_quota must not be negative", tier.Model)
		}
		if tier.Model == "*" {
			wildcard++
			if wildcard > 1 {
				return errors.New("model tier wildcard \"*\" may appear at most once")
			}
		}
	}
	return nil
}

// subscriptionWindowLimits 推导三窗口上限：
// limit5h=WalletRound(T×r5bps/10000)、limit7d=WalletRound(T×rWbps/10000)、limit31d=T。
// 订阅额度属展示/统计口径，使用钱包域（int53）保持一致，避免大额配置
// 在单次扣费的 int32 边界饱和导致窗口百分比失真。
func subscriptionWindowLimits(cfg *dto.SubscriptionBillingConfig) (limit5h, limit7d, limit31d int64) {
	if cfg == nil {
		return 0, 0, 0
	}
	total := cfg.MonthlyTotalQuota
	if total <= 0 {
		return 0, 0, 0
	}
	limit5h = walletRoundRatio(total, cfg.FiveHourRatioBps)
	limit7d = walletRoundRatio(total, cfg.WeeklyRatioBps)
	limit31d = total
	return limit5h, limit7d, limit31d
}

// walletRoundRatio 按钱包域（int53）计算 total×bps/10000，四舍五入到整数。
func walletRoundRatio(total int64, bps int) int64 {
	q, err := common.WalletQuotaFromDecimalStrict(
		decimal.NewFromInt(total).Mul(decimal.NewFromInt(int64(bps))).Div(decimal.NewFromInt(10000)),
	)
	if err != nil {
		// 配置经校验后 total>0 且 0<=bps<=10000，理论上不会溢出；溢出时回退到
		// total（即按 100% 计算），保证展示不因饱和失真。
		common.SysError(fmt.Sprintf("subscription window limit overflow: total=%d bps=%d err=%v", total, bps, err))
		return total
	}
	return int64(q)
}

// bpsToQuota 将基线百分比（bps）换算为该窗口的 quota：limit×bps/10000。
// 走钱包域（int53）严格换算，避免裸 int(float64*ratio) 转换导致溢出失真。
func bpsToQuota(bps int, limit int64) int64 {
	if bps <= 0 || limit <= 0 {
		return 0
	}
	q, err := common.WalletQuotaFromDecimalStrict(
		decimal.NewFromInt(limit).Mul(decimal.NewFromInt(int64(bps))).Div(decimal.NewFromInt(10000)),
	)
	if err != nil {
		// 溢出时回退到 limit（即按 100% 处理），保证展示不因饱和失真。
		common.SysError(fmt.Sprintf("baseline quota overflow: bps=%d limit=%d err=%v", bps, limit, err))
		return limit
	}
	return int64(q)
}

// bucketStartFor 返回 now 所在桶的起点（保留相位）。
// bucketStart <= 0 时按首次初始化处理，返回 now。
func bucketStartFor(bucketStart int64, now int64, period int64) int64 {
	if bucketStart <= 0 || period <= 0 {
		return now
	}
	elapsed := now - bucketStart
	if elapsed < period {
		return bucketStart
	}
	return bucketStart + elapsed/period*period
}

// currentUTCMondayStart 返回 now 所在 UTC 自然周的周一 0 点（unix 秒）。
// 对齐 opencode Go 官方 getWeekBounds 语义：周一起点、UTC 0 点、严格 7×24h。
func currentUTCMondayStart(now int64) int64 {
	t := time.Unix(now, 0).UTC()
	// time.Weekday: Sunday=0 ... Saturday=6；周一=1
	daysSinceMonday := (int(t.Weekday()) + 6) % 7
	weekStart := time.Date(t.Year(), t.Month(), t.Day()-daysSinceMonday, 0, 0, 0, 0, time.UTC)
	return weekStart.Unix()
}

// applyWindowRollover 对三窗口执行周期判定：
//   - 5h：真滑窗，过期判定在 Refresh 内按 windowStart 做（此处不做事）。
//   - 7d：UTC 自然周，跨过本周一 0 点则清周增量（基线保留）。
//   - 31d：固定 31d 桶，翻转时清月增量并清月基线（额度重置）。
//
// 5h/7d 基线是用户声明的"已用百分比"底数，不随窗口过期/跨周清空；
// 只有 31d 月度翻转时基线失效（额度重置）。
func applyWindowRollover(usage *model.ChannelSubscriptionUsage, now int64) {
	weekStart := currentUTCMondayStart(now)
	if usage.TimeWeeklyUpdated != 0 && usage.TimeWeeklyUpdated < weekStart {
		usage.UsedQuota7d = 0 // 跨自然周：周增量归 0，基线保留
	}
	if newStart := bucketStartFor(usage.BucketStart31d, now, SubscriptionWindow31dSeconds); newStart != usage.BucketStart31d {
		usage.BucketStart31d = newStart
		usage.UsedQuota31d = 0
		usage.BaselineBps31d = 0 // 月度重置：基线失效
		usage.BaselineSetAt31d = 0
	}
}

// fetchChannelConsumeLogs 增量拉取日志：半开区间 (lastCheckpointAt, now]，
// 防同秒重复计数。
func fetchChannelConsumeLogs(channelId int, lastCheckpointAt int64, now int64) ([]struct {
	CreatedAt int64
	ModelName string
	Quota     int
}, error) {
	var rows []struct {
		CreatedAt int64
		ModelName string
		Quota     int
	}
	err := model.LOG_DB.Model(&model.Log{}).
		Select("created_at", "model_name", "quota").
		Where("channel_id = ? AND type = ? AND created_at > ? AND created_at <= ?",
			channelId, model.LogTypeConsume, lastCheckpointAt, now).
		Find(&rows).Error
	return rows, err
}

// RefreshChannelSubscriptionUsage 增量刷新单个渠道的订阅统计（手动与定时共用）。
// 幂等：checkpoint 只前进，已处理的日志不会重复累加。
// 窗口语义对齐 opencode Go 官方：5h 真滑窗、7d UTC 自然周、31d 固定桶。
func RefreshChannelSubscriptionUsage(channelId int) error {
	lock := model.GetChannelPollingLock(channelId)
	lock.Lock()
	defer lock.Unlock()

	now := common.GetTimestamp()
	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if usage == nil {
		usage = &model.ChannelSubscriptionUsage{ChannelId: int64(channelId)}
	}

	// 首次初始化且未手动设置基线：回填历史日志到三窗口（修复预览首次显示为 0 的问题）。
	// 时间戳锚点一律取 now：滑窗/周窗"最近更新时间"与 31d 桶起点都用 now，
	// 避免第二次刷新因相位差恰好等于一个周期而被误判翻转清零。
	if usage.LastCheckpointAt == 0 && !usage.ManualInitialized {
		usage.TimeRollingUpdated = now
		usage.TimeWeeklyUpdated = now
		usage.BucketStart31d = now
		if usage.UsedQuota5h, err = sumChannelConsumeQuota(channelId, now-SubscriptionWindow5hSeconds, now); err != nil {
			usage.LastError = err.Error()
			usage.UpdatedAt = now
			_ = model.UpsertChannelSubscriptionUsage(usage)
			return err
		}
		if usage.UsedQuota7d, err = sumChannelConsumeQuota(channelId, currentUTCMondayStart(now), now); err != nil {
			usage.LastError = err.Error()
			usage.UpdatedAt = now
			_ = model.UpsertChannelSubscriptionUsage(usage)
			return err
		}
		if usage.UsedQuota31d, err = sumChannelConsumeQuota(channelId, now-SubscriptionWindow31dSeconds, now); err != nil {
			usage.LastError = err.Error()
			usage.UpdatedAt = now
			_ = model.UpsertChannelSubscriptionUsage(usage)
			return err
		}
		usage.LastCheckpointAt = now
		usage.LastRefreshAt = now
		usage.LastError = ""
		usage.Partial = probeSubscriptionLogPartial(channelId, now)
		usage.UpdatedAt = now
		return model.UpsertChannelSubscriptionUsage(usage)
	}

	applyWindowRollover(usage, now)

	// 5h 滑窗过期判定：最近累计时刻落在滚窗外 → 增量归 0（基线保留）。
	windowStart5h := now - SubscriptionWindow5hSeconds
	if usage.TimeRollingUpdated == 0 || usage.TimeRollingUpdated < windowStart5h {
		usage.UsedQuota5h = 0
	}
	weekStart := currentUTCMondayStart(now)

	rows, err := fetchChannelConsumeLogs(channelId, usage.LastCheckpointAt, now)
	if err != nil {
		usage.LastError = err.Error()
		usage.UpdatedAt = now
		_ = model.UpsertChannelSubscriptionUsage(usage)
		return err
	}
	accumulated := false
	for _, row := range rows {
		if row.Quota <= 0 {
			continue // quota<=0 忽略
		}
		accumulated = true
		if row.CreatedAt >= windowStart5h {
			usage.UsedQuota5h += int64(row.Quota)
		}
		if row.CreatedAt >= weekStart {
			usage.UsedQuota7d += int64(row.Quota)
		}
		if row.CreatedAt >= usage.BucketStart31d {
			usage.UsedQuota31d += int64(row.Quota)
		}
	}
	// 仅当本轮有新日志时推进"最近更新时间"，保证滚窗/周窗按真实活动过期重置。
	if accumulated {
		usage.TimeRollingUpdated = now
		usage.TimeWeeklyUpdated = now
	}
	usage.LastCheckpointAt = now
	usage.LastRefreshAt = now
	usage.LastError = ""
	usage.UpdatedAt = now
	return model.UpsertChannelSubscriptionUsage(usage)
}

// sumChannelConsumeQuota 全量求和：created_at ∈ [start, now]。
func sumChannelConsumeQuota(channelId int, start int64, now int64) (int64, error) {
	var total int64
	err := model.LOG_DB.Model(&model.Log{}).
		Select("COALESCE(SUM(quota), 0)").
		Where("channel_id = ? AND type = ? AND quota > 0 AND created_at >= ? AND created_at <= ?",
			channelId, model.LogTypeConsume, start, now).
		Scan(&total).Error
	return total, err
}

// probeSubscriptionLogPartial 探测月窗口是否不完整：
// 该渠道最早的消费日志距今不足 31 天（含日志保留策略短于 31 天的情况）。
func probeSubscriptionLogPartial(channelId int, now int64) bool {
	var earliest int64
	err := model.LOG_DB.Model(&model.Log{}).
		Select("MIN(created_at)").
		Where("channel_id = ? AND type = ?", channelId, model.LogTypeConsume).
		Scan(&earliest).Error
	if err != nil || earliest <= 0 {
		return false // 无日志，无从判断不完整
	}
	return now-earliest < SubscriptionWindow31dSeconds
}

// FullRecalibrateChannelSubscriptionUsage 每日全量校正：
// 三窗口 Sum = SUM(quota)（窗口内），同时探测 partial。基线字段不重算。
func FullRecalibrateChannelSubscriptionUsage(channelId int) error {
	lock := model.GetChannelPollingLock(channelId)
	lock.Lock()
	defer lock.Unlock()

	now := common.GetTimestamp()
	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if usage == nil {
		usage = &model.ChannelSubscriptionUsage{ChannelId: int64(channelId)}
	}
	// 首次初始化：与增量首刷一致，锚点=now（保持相位），回填求和覆盖过去一个周期；
	// 否则按当前窗口语义进行全量求和校正。
	firstInit := usage.LastCheckpointAt == 0
	if firstInit {
		usage.TimeRollingUpdated = now
		usage.TimeWeeklyUpdated = now
		usage.BucketStart31d = now
	}
	applyWindowRollover(usage, now)

	windowStart5h := now - SubscriptionWindow5hSeconds
	weekStart := currentUTCMondayStart(now)

	if usage.UsedQuota5h, err = sumChannelConsumeQuota(channelId, windowStart5h, now); err != nil {
		usage.LastError = err.Error()
		usage.UpdatedAt = now
		_ = model.UpsertChannelSubscriptionUsage(usage)
		return err
	}
	if usage.UsedQuota7d, err = sumChannelConsumeQuota(channelId, weekStart, now); err != nil {
		usage.LastError = err.Error()
		usage.UpdatedAt = now
		_ = model.UpsertChannelSubscriptionUsage(usage)
		return err
	}
	if usage.UsedQuota31d, err = sumChannelConsumeQuota(channelId, usage.BucketStart31d, now); err != nil {
		usage.LastError = err.Error()
		usage.UpdatedAt = now
		_ = model.UpsertChannelSubscriptionUsage(usage)
		return err
	}

	usage.TimeRollingUpdated = now
	usage.TimeWeeklyUpdated = now
	usage.LastCheckpointAt = now
	usage.LastRefreshAt = now
	usage.LastError = ""
	usage.Partial = probeSubscriptionLogPartial(channelId, now)
	usage.UpdatedAt = now
	return model.UpsertChannelSubscriptionUsage(usage)
}

// SubscriptionBaselineInput 三窗口各自独立的手动基线输入。
// 每个窗口的 percent（百分比，允许 >100 表示超限）与 at（起始时间 unix 秒）均可独立设置；
// nil 表示该窗口基线保持不变（部分更新）。at 缺省 = 设置时刻 now。
type SubscriptionBaselineInput struct {
	UsedPercent5h  *float64 `json:"used_percent_5h"`
	UsedPercent7d  *float64 `json:"used_percent_7d"`
	UsedPercent31d *float64 `json:"used_percent_31d"`
	BaselineAt5h   *int64   `json:"baseline_at_5h"`
	BaselineAt7d   *int64   `json:"baseline_at_7d"`
	BaselineAt31d  *int64   `json:"baseline_at_31d"`
}

// SetChannelSubscriptionBaseline 手动设置渠道订阅用量基线（用户在 baseline tab 配置
// 三窗口各自独立的"已用百分比 + 起始时间"）。保存时记录各窗口起点：
// LastCheckpointAt 只前进到所有已设置窗口起始时间中的最大值（避免回退 checkpoint
// 导致重读历史日志），各窗口增量 UsedQuota*=0，从该窗口起始时间起重新累计。
func SetChannelSubscriptionBaseline(channelId int, input SubscriptionBaselineInput) error {
	var err error
	now := common.GetTimestamp()

	if input.UsedPercent5h == nil && input.UsedPercent7d == nil && input.UsedPercent31d == nil {
		return errors.New("至少设置一个窗口的基线")
	}

	// 校验各窗口：百分比合法、起始时间不晚于当前。
	parse := func(percent *float64, at *int64, name string) (int, int64, error) {
		bps, err := SubscriptionBaselineBpsFromPercent(*percent)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid %s percent: %w", name, err)
		}
		baselineAt := now
		if at != nil {
			baselineAt = *at
		}
		if baselineAt < 0 || baselineAt > now {
			return 0, 0, fmt.Errorf("invalid %s baseline_at: %d", name, baselineAt)
		}
		return bps, baselineAt, nil
	}
	bps5h := 0
	at5h := int64(0)
	if input.UsedPercent5h != nil {
		bps5h, at5h, err = parse(input.UsedPercent5h, input.BaselineAt5h, "5h")
		if err != nil {
			return err
		}
	}
	bps7d := 0
	at7d := int64(0)
	if input.UsedPercent7d != nil {
		bps7d, at7d, err = parse(input.UsedPercent7d, input.BaselineAt7d, "7d")
		if err != nil {
			return err
		}
	}
	bps31d := 0
	at31d := int64(0)
	if input.UsedPercent31d != nil {
		bps31d, at31d, err = parse(input.UsedPercent31d, input.BaselineAt31d, "31d")
		if err != nil {
			return err
		}
	}

	lock := model.GetChannelPollingLock(channelId)
	lock.Lock()
	defer lock.Unlock()

	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		return err
	}
	cfg := channel.GetOtherSettings().SubscriptionBilling
	if cfg == nil || cfg.BillingMode != dto.SubscriptionBillingModeSubscribe {
		return errors.New("渠道未启用订阅计费")
	}

	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if usage == nil {
		usage = &model.ChannelSubscriptionUsage{ChannelId: int64(channelId)}
	}

	// 每个被设置的窗口：写入独立基线 + 起点，清该窗口增量，锚点对齐到起点。
	newCheckpoint := usage.LastCheckpointAt
	if bps5h != 0 {
		usage.BaselineBps5h = bps5h
		usage.BaselineSetAt5h = at5h
		usage.UsedQuota5h = 0
		usage.TimeRollingUpdated = at5h
		if at5h > newCheckpoint {
			newCheckpoint = at5h
		}
	}
	if bps7d != 0 {
		usage.BaselineBps7d = bps7d
		usage.BaselineSetAt7d = at7d
		usage.UsedQuota7d = 0
		usage.TimeWeeklyUpdated = at7d
		if at7d > newCheckpoint {
			newCheckpoint = at7d
		}
	}
	if bps31d != 0 {
		usage.BaselineBps31d = bps31d
		usage.BaselineSetAt31d = at31d
		usage.UsedQuota31d = 0
		usage.BucketStart31d = at31d
		if at31d > newCheckpoint {
			newCheckpoint = at31d
		}
	}
	usage.LastCheckpointAt = newCheckpoint
	usage.ManualInitialized = true
	usage.LastRefreshAt = now
	usage.LastError = ""
	usage.Partial = false
	usage.UpdatedAt = now
	return model.UpsertChannelSubscriptionUsage(usage)
}

// matchSubscriptionModelTier 匹配模型月度上限：精确模型名 → "*" 兜底。
// 未匹配时返回 (0, false)，调用方以渠道月上限兜底。
func matchSubscriptionModelTier(tiers []dto.SubscriptionModelTier, modelName string) (int64, bool) {
	for _, tier := range tiers {
		if tier.Model == modelName {
			return tier.MonthlyQuota, true
		}
	}
	for _, tier := range tiers {
		if tier.Model == "*" {
			return tier.MonthlyQuota, true
		}
	}
	return 0, false
}

// subscriptionModelUsageRow 按 model_name 分组当月用量（31d 窗口）。
type subscriptionModelUsageRow struct {
	ChannelId int64
	ModelName string
	Total     int64
}

// fetchSubscriptionPerModelUsage 批量读取多个渠道的 per-model 当月用量。
// 主库即日志库时用单条 JOIN 查询避免 N+1；日志库独立（如 ClickHouse）时
// 退化为按渠道查询（订阅渠道通常数量有限）。
func fetchSubscriptionPerModelUsage(channelIds []int64) (map[int64]map[string]int64, error) {
	result := make(map[int64]map[string]int64, len(channelIds))
	if len(channelIds) == 0 {
		return result, nil
	}

	var rows []subscriptionModelUsageRow
	if model.LOG_DB == model.DB {
		err := model.LOG_DB.Table("logs l").
			Select("l.channel_id AS channel_id, l.model_name AS model_name, SUM(l.quota) AS total").
			Joins("JOIN channel_subscription_usages u ON u.channel_id = l.channel_id").
			Where("l.channel_id IN ? AND l.type = ? AND l.quota > 0 AND l.created_at >= u.bucket_start31d AND l.created_at <= u.updated_at",
				channelIds, model.LogTypeConsume).
			Group("l.channel_id, l.model_name").
			Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if result[row.ChannelId] == nil {
				result[row.ChannelId] = make(map[string]int64)
			}
			result[row.ChannelId][row.ModelName] = row.Total
		}
		return result, nil
	}

	usages, err := model.GetChannelSubscriptionUsagesByIds(channelIds)
	if err != nil {
		return nil, err
	}
	for _, channelId := range channelIds {
		usage := usages[channelId]
		if usage == nil || usage.BucketStart31d <= 0 {
			continue
		}
		var perChannel []struct {
			ModelName string
			Total     int64
		}
		err := model.LOG_DB.Model(&model.Log{}).
			Select("model_name, SUM(quota) AS total").
			Where("channel_id = ? AND type = ? AND quota > 0 AND created_at >= ? AND created_at <= ?",
				channelId, model.LogTypeConsume, usage.BucketStart31d, usage.UpdatedAt).
			Group("model_name").
			Scan(&perChannel).Error
		if err != nil {
			return nil, err
		}
		if len(perChannel) == 0 {
			continue
		}
		result[channelId] = make(map[string]int64, len(perChannel))
		for _, row := range perChannel {
			result[channelId][row.ModelName] = row.Total
		}
	}
	return result, nil
}

// subscriptionPercent 计算 used/limit 百分比；limit<=0 时按 0（无上限）处理，
// used>0 且无上限视为超限（over_limit 由调用方单独判断）。
func subscriptionPercent(used int64, limit int64) float64 {
	if limit <= 0 {
		if used > 0 {
			return 100
		}
		return 0
	}
	return float64(used) * 100 / float64(limit)
}

// BuildSubscriptionUsageData 组装用量接口响应体（增量刷新已由调用方执行）。
// 展示值 = 各窗口独立基线 quota（实时换算）+ 增量 quota（UsedQuota* 为纯增量）。
// 窗口语义对齐 opencode Go 官方：5h 真滑窗、7d UTC 自然周、31d 固定桶。
func BuildSubscriptionUsageData(channelId int, cfg *dto.SubscriptionBillingConfig, now int64) (*SubscriptionUsageData, error) {
	if cfg == nil {
		return nil, errors.New("subscription billing is not configured")
	}
	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 理论上刷新已创建记录；缺失时按全零窗口返回。
			usage = &model.ChannelSubscriptionUsage{ChannelId: int64(channelId), UpdatedAt: now}
		} else {
			return nil, err
		}
	}

	limit5h, limit7d, limit31d := subscriptionWindowLimits(cfg)
	baselineQuota5h := bpsToQuota(usage.BaselineBps5h, limit5h)
	baselineQuota7d := bpsToQuota(usage.BaselineBps7d, limit7d)
	baselineQuota31d := bpsToQuota(usage.BaselineBps31d, limit31d)

	buildWindow := func(windowSize int64, used int64, limit int64, resetAt int64) SubscriptionUsageWindow {
		usedPercent := subscriptionPercent(used, limit)
		resetAfter := resetAt - now
		if resetAfter < 0 {
			resetAfter = 0
		}
		return SubscriptionUsageWindow{
			WindowSizeSeconds: windowSize,
			UsedQuota:         used,
			LimitQuota:        limit,
			UsedPercent:       usedPercent,
			DisplayPercent:    math.Min(usedPercent, 100),
			OverLimit:         used > limit && limit > 0,
			ResetAt:           resetAt,
			ResetAfterSeconds: resetAfter,
		}
	}

	// 5h 滑窗：最近累计时刻在滚窗外则增量归 0（基线保留）；reset 锚定 timeRollingUpdated+5h。
	windowStart5h := now - SubscriptionWindow5hSeconds
	used5h := baselineQuota5h
	resetAt5h := now + SubscriptionWindow5hSeconds
	if usage.TimeRollingUpdated != 0 && usage.TimeRollingUpdated >= windowStart5h {
		used5h += usage.UsedQuota5h
		resetAt5h = usage.TimeRollingUpdated + SubscriptionWindow5hSeconds
	}

	// 7d 自然周：跨过本周一 0 点则增量归 0（基线保留）；reset 锚定下周一 0 点。
	weekStart := currentUTCMondayStart(now)
	used7d := baselineQuota7d
	if usage.TimeWeeklyUpdated != 0 && usage.TimeWeeklyUpdated >= weekStart {
		used7d += usage.UsedQuota7d
	}
	resetAt7d := weekStart + SubscriptionWindow7dSeconds

	// 31d 桶：用量=基线+桶内增量；reset 锚定桶结束。
	used31d := baselineQuota31d + usage.UsedQuota31d
	resetAt31d := usage.BucketStart31d + SubscriptionWindow31dSeconds

	windows := map[string]SubscriptionUsageWindow{
		SubscriptionWindowName5h:  buildWindow(SubscriptionWindow5hSeconds, used5h, limit5h, resetAt5h),
		SubscriptionWindowName7d:  buildWindow(SubscriptionWindow7dSeconds, used7d, limit7d, resetAt7d),
		SubscriptionWindowName31d: buildWindow(SubscriptionWindow31dSeconds, used31d, limit31d, resetAt31d),
	}

	perModelUsage, err := fetchSubscriptionPerModelUsage([]int64{int64(channelId)})
	if err != nil {
		return nil, err
	}
	usedByModel := perModelUsage[int64(channelId)]
	perModel := buildSubscriptionPerModelUsage(cfg, usedByModel)

	return &SubscriptionUsageData{
		UpdatedAt: usage.UpdatedAt,
		Partial:   usage.Partial,
		Windows:   windows,
		PerModel:  perModel,
	}, nil
}

// buildSubscriptionPerModelUsage 组装 per-model 月度明细：
// 覆盖所有已配置档位 + 有用量但未配置的模型；匹配优先级
// 精确模型名 → "*" 兜底 → 未配置（分母=渠道月上限）。
func buildSubscriptionPerModelUsage(cfg *dto.SubscriptionBillingConfig, usedByModel map[string]int64) []SubscriptionPerModelUsage {
	modelSet := make(map[string]struct{}, len(cfg.ModelTiers)+len(usedByModel))
	for _, tier := range cfg.ModelTiers {
		if tier.Model != "*" {
			modelSet[tier.Model] = struct{}{}
		}
	}
	for modelName := range usedByModel {
		modelSet[modelName] = struct{}{}
	}
	models := make([]string, 0, len(modelSet))
	for modelName := range modelSet {
		models = append(models, modelName)
	}
	sort.Strings(models)

	perModel := make([]SubscriptionPerModelUsage, 0, len(models))
	for _, modelName := range models {
		used := usedByModel[modelName]
		limit, matched := matchSubscriptionModelTier(cfg.ModelTiers, modelName)
		if !matched {
			limit = cfg.MonthlyTotalQuota // 未配置：分母=渠道月上限
		}
		perModel = append(perModel, SubscriptionPerModelUsage{
			Model:       modelName,
			UsedQuota:   used,
			LimitQuota:  limit,
			UsedPercent: subscriptionPercent(used, limit),
			OverLimit:   used > limit && limit > 0,
		})
	}
	return perModel
}

// ChannelSubscriptionListSnapshot 渠道列表所需的订阅统计快照（单渠道）。
type ChannelSubscriptionListSnapshot struct {
	MonthlyUsed    int64
	MonthlyLimit   int64
	UpdatedAt      int64
	ModelOverLimit bool
}

// BuildChannelSubscriptionListSnapshots 为渠道列表批量补充订阅统计快照
// （checkpoint 批量读取 + per-model 批量查询，避免 N+1）。
// 仅返回启用订阅计费（billing_mode=1）且存在统计记录的渠道。
func BuildChannelSubscriptionListSnapshots(channels []*model.Channel) (map[int64]ChannelSubscriptionListSnapshot, error) {
	result := make(map[int64]ChannelSubscriptionListSnapshot)
	if len(channels) == 0 {
		return result, nil
	}

	// 先解析配置，收集订阅渠道。
	ids := make([]int64, 0, len(channels))
	configs := make(map[int64]*dto.SubscriptionBillingConfig, len(channels))
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		cfg := ch.GetOtherSettings().SubscriptionBilling
		if cfg == nil || cfg.BillingMode != dto.SubscriptionBillingModeSubscribe {
			continue
		}
		configs[int64(ch.Id)] = cfg
		ids = append(ids, int64(ch.Id))
	}
	if len(ids) == 0 {
		return result, nil
	}

	usages, err := model.GetChannelSubscriptionUsagesByIds(ids)
	if err != nil {
		return nil, err
	}
	perModelUsage, err := fetchSubscriptionPerModelUsage(ids)
	if err != nil {
		return nil, err
	}

	for _, channelId := range ids {
		usage := usages[channelId]
		cfg := configs[channelId]
		snapshot := ChannelSubscriptionListSnapshot{
			MonthlyLimit: cfg.MonthlyTotalQuota,
		}
		if usage != nil {
			snapshot.MonthlyUsed = usage.UsedQuota31d
			snapshot.UpdatedAt = usage.UpdatedAt
			// per-model 超限探测：任一模型用量 > 其档位上限（未配置档位分母=渠道月上限）。
			for modelName, used := range perModelUsage[channelId] {
				limit, matched := matchSubscriptionModelTier(cfg.ModelTiers, modelName)
				if !matched {
					limit = cfg.MonthlyTotalQuota
				}
				if used > limit && limit > 0 {
					snapshot.ModelOverLimit = true
					break
				}
			}
		}
		result[channelId] = snapshot
	}
	return result, nil
}
