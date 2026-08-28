package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

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

// SubscriptionUSDToQuota 将 USD 金额换算为 quota，使用严格转换
// （超出 common.MaxQuota 报错，不静默饱和）。
func SubscriptionUSDToQuota(usd float64) (int64, error) {
	if math.IsNaN(usd) || math.IsInf(usd, 0) || usd < 0 {
		return 0, fmt.Errorf("invalid usd amount: %v", usd)
	}
	quota, err := common.QuotaFromDecimalStrict(
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
// limit5h=QuotaRound(T×r5bps/10000)、limit7d=QuotaRound(T×rWbps/10000)、limit31d=T。
func subscriptionWindowLimits(cfg *dto.SubscriptionBillingConfig) (limit5h, limit7d, limit31d int64) {
	if cfg == nil {
		return 0, 0, 0
	}
	total := cfg.MonthlyTotalQuota
	if total <= 0 {
		return 0, 0, 0
	}
	limit5h = int64(common.QuotaRound(float64(total) * float64(cfg.FiveHourRatioBps) / 10000.0))
	limit7d = int64(common.QuotaRound(float64(total) * float64(cfg.WeeklyRatioBps) / 10000.0))
	limit31d = total
	return limit5h, limit7d, limit31d
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

// applyBucketRollover 对三个窗口执行桶翻转：窗口进入新周期则清零对应 Sum。
func applyBucketRollover(usage *model.ChannelSubscriptionUsage, now int64) {
	if newStart := bucketStartFor(usage.BucketStart5h, now, SubscriptionWindow5hSeconds); newStart != usage.BucketStart5h {
		usage.BucketStart5h = newStart
		usage.UsedQuota5h = 0
	}
	if newStart := bucketStartFor(usage.BucketStart7d, now, SubscriptionWindow7dSeconds); newStart != usage.BucketStart7d {
		usage.BucketStart7d = newStart
		usage.UsedQuota7d = 0
	}
	if newStart := bucketStartFor(usage.BucketStart31d, now, SubscriptionWindow31dSeconds); newStart != usage.BucketStart31d {
		usage.BucketStart31d = newStart
		usage.UsedQuota31d = 0
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

	// 首次初始化：三窗口 bucketStart=now、Sum=0、LastCheckpointAt=now。
	if usage.LastCheckpointAt == 0 {
		usage.BucketStart5h = now
		usage.BucketStart7d = now
		usage.BucketStart31d = now
		usage.LastCheckpointAt = now
		usage.LastRefreshAt = now
		usage.LastError = ""
		usage.UpdatedAt = now
		return model.UpsertChannelSubscriptionUsage(usage)
	}

	applyBucketRollover(usage, now)

	rows, err := fetchChannelConsumeLogs(channelId, usage.LastCheckpointAt, now)
	if err != nil {
		usage.LastError = err.Error()
		usage.UpdatedAt = now
		_ = model.UpsertChannelSubscriptionUsage(usage)
		return err
	}
	for _, row := range rows {
		if row.Quota <= 0 {
			continue // quota<=0 忽略
		}
		if row.CreatedAt >= usage.BucketStart5h {
			usage.UsedQuota5h += int64(row.Quota)
		}
		if row.CreatedAt >= usage.BucketStart7d {
			usage.UsedQuota7d += int64(row.Quota)
		}
		if row.CreatedAt >= usage.BucketStart31d {
			usage.UsedQuota31d += int64(row.Quota)
		}
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
// 三窗口 Sum = SUM(quota)（窗口内），同时探测 partial。
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
	if usage.LastCheckpointAt == 0 {
		// 未初始化则按首次初始化处理，再执行全量求和。
		usage.BucketStart5h = now
		usage.BucketStart7d = now
		usage.BucketStart31d = now
	}
	applyBucketRollover(usage, now)

	if usage.UsedQuota5h, err = sumChannelConsumeQuota(channelId, usage.BucketStart5h, now); err != nil {
		usage.LastError = err.Error()
		usage.UpdatedAt = now
		_ = model.UpsertChannelSubscriptionUsage(usage)
		return err
	}
	if usage.UsedQuota7d, err = sumChannelConsumeQuota(channelId, usage.BucketStart7d, now); err != nil {
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

	usage.LastCheckpointAt = now
	usage.LastRefreshAt = now
	usage.LastError = ""
	usage.Partial = probeSubscriptionLogPartial(channelId, now)
	usage.UpdatedAt = now
	return model.UpsertChannelSubscriptionUsage(usage)
}

// ResetChannelSubscriptionUsage 配置变更（PUT 保存）时重置统计：
// 三窗口 Sum=0、bucketStart=当前桶起点、LastCheckpointAt=now。
// 无既有记录时按首次初始化处理。
func ResetChannelSubscriptionUsage(channelId int) error {
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
		usage.BucketStart5h = now
		usage.BucketStart7d = now
		usage.BucketStart31d = now
	} else {
		usage.BucketStart5h = bucketStartFor(usage.BucketStart5h, now, SubscriptionWindow5hSeconds)
		usage.BucketStart7d = bucketStartFor(usage.BucketStart7d, now, SubscriptionWindow7dSeconds)
		usage.BucketStart31d = bucketStartFor(usage.BucketStart31d, now, SubscriptionWindow31dSeconds)
	}
	usage.UsedQuota5h = 0
	usage.UsedQuota7d = 0
	usage.UsedQuota31d = 0
	usage.LastCheckpointAt = now
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
	buildWindow := func(windowSize int64, used int64, limit int64, bucketStart int64) SubscriptionUsageWindow {
		usedPercent := subscriptionPercent(used, limit)
		resetAt := bucketStart + windowSize
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

	windows := map[string]SubscriptionUsageWindow{
		SubscriptionWindowName5h:  buildWindow(SubscriptionWindow5hSeconds, usage.UsedQuota5h, limit5h, usage.BucketStart5h),
		SubscriptionWindowName7d:  buildWindow(SubscriptionWindow7dSeconds, usage.UsedQuota7d, limit7d, usage.BucketStart7d),
		SubscriptionWindowName31d: buildWindow(SubscriptionWindow31dSeconds, usage.UsedQuota31d, limit31d, usage.BucketStart31d),
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
