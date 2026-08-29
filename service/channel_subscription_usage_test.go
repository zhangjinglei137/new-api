package service

import (
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本测试文件复用 service 包 task_billing_test.go 中的 TestMain（sqlite 内存库）。

func setupSubscriptionUsageTest(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelSubscriptionUsage{}))
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM channel_subscription_usages")
		model.DB.Exec("DELETE FROM logs")
	})
}

type consumeLogSeed struct {
	CreatedAt int64
	ModelName string
	Quota     int
}

func insertConsumeLogs(t *testing.T, channelId int, seeds []consumeLogSeed) {
	t.Helper()
	for _, s := range seeds {
		log := &model.Log{
			UserId:    1,
			Username:  "u",
			CreatedAt: s.CreatedAt,
			Type:      model.LogTypeConsume,
			ModelName: s.ModelName,
			Quota:     s.Quota,
			ChannelId: channelId,
		}
		require.NoError(t, model.LOG_DB.Create(log).Error)
	}
}

// weekElapsedOk 判断 now 距本周一 0 点是否已过去至少 needed 秒。
// 用于守卫依赖"日志落在本周内"的 7d 断言，避免在 UTC 周一凌晨运行测试时因
// 跨周边界导致断言随机失败。
func weekElapsedOk(now int64, needed int64) bool {
	return now-currentUTCMondayStart(now) >= needed
}

// f64p 构造 *float64 测试辅助。
func f64p(v float64) *float64 { return &v }

// i64p 构造 *int64 测试辅助。
func i64p(v int64) *int64 { return &v }

func TestSubscriptionUSDToQuota(t *testing.T) {
	t.Run("60 usd maps to 30,000,000 quota", func(t *testing.T) {
		quota, err := SubscriptionUSDToQuota(60)
		require.NoError(t, err)
		assert.Equal(t, int64(30000000), quota)
	})

	t.Run("zero is zero", func(t *testing.T) {
		quota, err := SubscriptionUSDToQuota(0)
		require.NoError(t, err)
		assert.Equal(t, int64(0), quota)
	})

	t.Run("negative rejected", func(t *testing.T) {
		_, err := SubscriptionUSDToQuota(-1)
		require.Error(t, err)
	})

	t.Run("nan rejected", func(t *testing.T) {
		_, err := SubscriptionUSDToQuota(math.NaN())
		require.Error(t, err)
	})

	t.Run("large usd accepted within wallet domain", func(t *testing.T) {
		// 订阅额度属展示口径，使用钱包域（int53），不再受单次扣费的
		// int32 MaxQuota 限制：60×500000=3e7、4295 USD 等大额均可换算。
		quota, err := SubscriptionUSDToQuota(4295)
		require.NoError(t, err)
		assert.Equal(t, int64(2147500000), quota)

		// 仅当超过 int53 钱包上限时才严格报错。
		_, err = SubscriptionUSDToQuota(float64((1<<53)/500000 + 1))
		require.Error(t, err)
	})
}

func TestSubscriptionQuotaToUSD(t *testing.T) {
	assert.InDelta(t, 60.0, SubscriptionQuotaToUSD(30000000), 1e-9)
	assert.InDelta(t, 0.0, SubscriptionQuotaToUSD(0), 1e-9)
	assert.InDelta(t, 0.2, SubscriptionQuotaToUSD(100000), 1e-9)
}

func TestSubscriptionRatioBpsFromPercent(t *testing.T) {
	bps, err := SubscriptionRatioBpsFromPercent(20)
	require.NoError(t, err)
	assert.Equal(t, 2000, bps)
	bps, err = SubscriptionRatioBpsFromPercent(50)
	require.NoError(t, err)
	assert.Equal(t, 5000, bps)
	bps, err = SubscriptionRatioBpsFromPercent(99.99)
	require.NoError(t, err)
	assert.Equal(t, 9999, bps)
	_, err = SubscriptionRatioBpsFromPercent(-1)
	require.Error(t, err)
	_, err = SubscriptionRatioBpsFromPercent(101)
	require.Error(t, err)
}

func TestValidateSubscriptionBillingConfig(t *testing.T) {
	t.Run("valid subscription config", func(t *testing.T) {
		cfg := &dto.SubscriptionBillingConfig{
			BillingMode:       dto.SubscriptionBillingModeSubscribe,
			MonthlyTotalQuota: 30000000,
			FiveHourRatioBps:  2000,
			WeeklyRatioBps:    5000,
		}
		require.NoError(t, ValidateSubscriptionBillingConfig(cfg))
	})

	t.Run("nil rejected", func(t *testing.T) {
		require.Error(t, ValidateSubscriptionBillingConfig(nil))
	})

	t.Run("invalid billing mode rejected", func(t *testing.T) {
		cfg := &dto.SubscriptionBillingConfig{BillingMode: 2, MonthlyTotalQuota: 100}
		require.Error(t, ValidateSubscriptionBillingConfig(cfg))
	})

	t.Run("monthly total must be positive", func(t *testing.T) {
		cfg := &dto.SubscriptionBillingConfig{BillingMode: 1, MonthlyTotalQuota: 0}
		require.Error(t, ValidateSubscriptionBillingConfig(cfg))
	})

	t.Run("ratios must be within 0-100 percent", func(t *testing.T) {
		cfg := &dto.SubscriptionBillingConfig{BillingMode: 1, MonthlyTotalQuota: 100, FiveHourRatioBps: 10001}
		require.Error(t, ValidateSubscriptionBillingConfig(cfg))
		cfg = &dto.SubscriptionBillingConfig{BillingMode: 1, MonthlyTotalQuota: 100, WeeklyRatioBps: -1}
		require.Error(t, ValidateSubscriptionBillingConfig(cfg))
	})

	t.Run("at most one wildcard tier", func(t *testing.T) {
		cfg := &dto.SubscriptionBillingConfig{
			BillingMode: 1, MonthlyTotalQuota: 100,
			ModelTiers: []dto.SubscriptionModelTier{
				{Model: "*", MonthlyQuota: 10},
				{Model: "*", MonthlyQuota: 20},
			},
		}
		require.Error(t, ValidateSubscriptionBillingConfig(cfg))
	})

	t.Run("empty tier model rejected", func(t *testing.T) {
		cfg := &dto.SubscriptionBillingConfig{
			BillingMode: 1, MonthlyTotalQuota: 100,
			ModelTiers: []dto.SubscriptionModelTier{{Model: "  ", MonthlyQuota: 10}},
		}
		require.Error(t, ValidateSubscriptionBillingConfig(cfg))
	})

	t.Run("negative tier quota rejected", func(t *testing.T) {
		cfg := &dto.SubscriptionBillingConfig{
			BillingMode: 1, MonthlyTotalQuota: 100,
			ModelTiers: []dto.SubscriptionModelTier{{Model: "m", MonthlyQuota: -1}},
		}
		require.Error(t, ValidateSubscriptionBillingConfig(cfg))
	})
}

func TestSubscriptionWindowLimits(t *testing.T) {
	cfg := &dto.SubscriptionBillingConfig{MonthlyTotalQuota: 30000000, FiveHourRatioBps: 2000, WeeklyRatioBps: 5000}
	limit5h, limit7d, limit31d := subscriptionWindowLimits(cfg)
	assert.Equal(t, int64(6000000), limit5h)
	assert.Equal(t, int64(15000000), limit7d)
	assert.Equal(t, int64(30000000), limit31d)

	t.Run("half-away-from-zero rounding via QuotaRound", func(t *testing.T) {
		cfg := &dto.SubscriptionBillingConfig{MonthlyTotalQuota: 3, FiveHourRatioBps: 5000}
		limit5h, _, _ := subscriptionWindowLimits(cfg)
		assert.Equal(t, int64(2), limit5h) // 3*5000/10000 = 1.5 → round → 2
	})

	t.Run("zero total yields zero limits", func(t *testing.T) {
		limit5h, limit7d, limit31d := subscriptionWindowLimits(&dto.SubscriptionBillingConfig{})
		assert.Zero(t, limit5h)
		assert.Zero(t, limit7d)
		assert.Zero(t, limit31d)
	})
}

func TestBucketStartFor(t *testing.T) {
	const period = int64(18000)
	assert.Equal(t, int64(1000), bucketStartFor(1000, 1000+17999, period))
	assert.Equal(t, int64(19000), bucketStartFor(1000, 1000+18000, period))
	assert.Equal(t, int64(37000), bucketStartFor(1000, 1000+2*18000+100, period))
	assert.Equal(t, int64(5000), bucketStartFor(0, 5000, period)) // 未初始化 → now
}

func TestCurrentUTCMondayStart(t *testing.T) {
	// 2026-08-17 是周一（UTC）。验证回退到周一 0 点。
	monday := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC).Unix()
	wednesday := time.Date(2026, 8, 19, 12, 30, 45, 0, time.UTC).Unix()
	sunday := time.Date(2026, 8, 23, 23, 59, 59, 0, time.UTC).Unix()
	assert.Equal(t, monday, currentUTCMondayStart(wednesday))
	assert.Equal(t, monday, currentUTCMondayStart(sunday))
	// 周一当天任何时刻都属于该周
	assert.Equal(t, monday, currentUTCMondayStart(monday))
	// 下一周一 0 点属于新周
	nextMonday := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC).Unix()
	assert.Equal(t, nextMonday, currentUTCMondayStart(nextMonday))
}

func TestApplyWindowRollover(t *testing.T) {
	now := int64(1000000)
	weekStart := currentUTCMondayStart(now)
	usage := &model.ChannelSubscriptionUsage{
		TimeWeeklyUpdated: weekStart, UsedQuota7d: 100,
		TimeRollingUpdated: now, UsedQuota5h: 100,
		BucketStart31d: now, UsedQuota31d: 100,
	}
	// 同一自然周内：不跨周，不清零
	applyWindowRollover(usage, now+60)
	assert.Equal(t, int64(100), usage.UsedQuota7d)
	assert.Equal(t, weekStart, usage.TimeWeeklyUpdated)
	// 7d/31d 均未跨周期
	assert.Equal(t, int64(100), usage.UsedQuota31d)
	assert.Equal(t, int64(100), usage.UsedQuota5h) // 5h 滑窗在 rollover 内不做任何事
	assert.Equal(t, now, usage.TimeRollingUpdated)

	// 跨自然周：上周累计清零（基线保留），31d 未翻转
	nextWeek := currentUTCMondayStart(now) + SubscriptionWindow7dSeconds + 60
	usage.TimeWeeklyUpdated = weekStart // 上次累计仍在上一周
	applyWindowRollover(usage, nextWeek)
	assert.Equal(t, int64(0), usage.UsedQuota7d)
	assert.Equal(t, int64(100), usage.UsedQuota31d)

	// 31d 桶翻转：增量清零 + 月基线失效
	usage.BucketStart31d = now
	usage.BaselineBps31d = 3000
	usage.BaselineSetAt31d = now - 100
	usage.UsedQuota31d = 100
	applyWindowRollover(usage, now+SubscriptionWindow31dSeconds)
	assert.Equal(t, int64(0), usage.UsedQuota31d)
	assert.Equal(t, 0, usage.BaselineBps31d)
	assert.Equal(t, int64(0), usage.BaselineSetAt31d)
	assert.NotEqual(t, now, usage.BucketStart31d)
}

func TestSubscriptionPercent(t *testing.T) {
	assert.InDelta(t, 50.0, subscriptionPercent(50, 100), 1e-9)
	assert.InDelta(t, 150.0, subscriptionPercent(150, 100), 1e-9) // 允许 >100
	assert.InDelta(t, 100.0, subscriptionPercent(5, 0), 1e-9)     // 无上限且有用量
	assert.InDelta(t, 0.0, subscriptionPercent(0, 0), 1e-9)
}

func TestRefreshChannelSubscriptionUsageIncrementalAndIdempotent(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 42
	now := common.GetTimestamp()

	usage := &model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   now - 100,
		LastRefreshAt:      now - 100,
		TimeRollingUpdated: now - 100,
		TimeWeeklyUpdated:  now - 100,
		BucketStart31d:     now - 100,
		UpdatedAt:          now - 100,
	}
	require.NoError(t, model.UpsertChannelSubscriptionUsage(usage))

	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: now - 90, ModelName: "model-a", Quota: 100},
		{CreatedAt: now - 80, ModelName: "model-b", Quota: 200},
		{CreatedAt: now - 70, ModelName: "model-a", Quota: -50}, // quota<=0 忽略
	})
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage2, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage2.UsedQuota5h)
	assert.Equal(t, int64(300), usage2.UsedQuota31d)
	if weekElapsedOk(now, 100) {
		assert.Equal(t, int64(300), usage2.UsedQuota7d)
	}
	assert.True(t, usage2.LastCheckpointAt >= now)

	// 幂等：无新日志时重复刷新不重复累加
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage3, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage3.UsedQuota31d)

	// 半开区间：created_at == checkpoint 的日志不计入（防同秒重复）
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: usage3.LastCheckpointAt, ModelName: "model-c", Quota: 999},
	})
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage4, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage4.UsedQuota31d)
}

func TestRefreshChannelSubscriptionUsageFirstInitWithNoLogs(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 43
	before := common.GetTimestamp()
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	after := common.GetTimestamp()
	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	// 首次初始化回填：滑窗/周窗锚点 = now（与语义一致），回填窗口覆盖过去一个周期
	assert.GreaterOrEqual(t, usage.TimeRollingUpdated, before)
	assert.LessOrEqual(t, usage.TimeRollingUpdated, after)
	assert.Zero(t, usage.UsedQuota5h)
	assert.Zero(t, usage.UsedQuota31d)
	assert.GreaterOrEqual(t, usage.LastCheckpointAt, before)
}

// TestRefreshSecondPassKeepsBackfilledUsage 回归：首次回填历史后，紧接着的
// 第二次刷新不得清零（预览始终显示 0 的 bug 修复）。
func TestRefreshSecondPassKeepsBackfilledUsage(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 54
	now := common.GetTimestamp()
	// 1 小时前的历史日志（落在 5h/7d/31d 三窗口内）
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: now - 3600, ModelName: "a", Quota: 300},
	})
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage.UsedQuota5h)
	assert.Equal(t, int64(300), usage.UsedQuota31d)
	if weekElapsedOk(now, 3600+100) {
		assert.Equal(t, int64(300), usage.UsedQuota7d)
	}

	// 第二次刷新：无新日志，窗口用量必须保持，锚点不得被误推进/清零
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage2, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage2.UsedQuota5h)
	assert.Equal(t, int64(300), usage2.UsedQuota31d)
	if weekElapsedOk(now, 3600+100) {
		assert.Equal(t, int64(300), usage2.UsedQuota7d)
	}
	// 无新日志时滑窗/周窗最近更新时间保持（不因"刷新动作"而推进）
	assert.Equal(t, usage.TimeRollingUpdated, usage2.TimeRollingUpdated)
	assert.Equal(t, usage.TimeWeeklyUpdated, usage2.TimeWeeklyUpdated)
	assert.Equal(t, usage.BucketStart31d, usage2.BucketStart31d)
}

// TestRefreshSkipsLogsOutsideRolling5hWindow 验证窗口边界语义：
// 31d 桶翻转后，翻转前旧桶内的日志被 31d 跳过；同一时段日志因落在 5h 滑窗 /
// 7d 自然周内仍被计入。同时验证 5h 滑窗外（>5h 前）的日志不计入 5h。
func TestRefreshSkipsLogsOutsideRolling5hWindow(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 7
	now := common.GetTimestamp()
	// 31d 桶起点：31d+2h 前；翻转移位后新起点 = now-2h
	old31d := now - SubscriptionWindow31dSeconds - 2*3600
	// checkpoint 设在翻转前旧桶内、新桶起点之前
	checkpoint := now - 3*3600
	usage := &model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   checkpoint,
		LastRefreshAt:      checkpoint,
		TimeRollingUpdated: now - 2000, // 滑窗内（最近累计在 5h 内）
		UsedQuota5h:        1000,
		TimeWeeklyUpdated:  now - 2000, UsedQuota7d: 1000,
		BucketStart31d: old31d, UsedQuota31d: 1000,
		UpdatedAt: now - 2000,
	}
	require.NoError(t, model.UpsertChannelSubscriptionUsage(usage))

	// 日志 A：翻转前旧桶内（checkpoint 之后、31d 新桶起点 now-2h 之前）
	// 日志 B：31d 新桶内
	// 日志 C：5h 滑窗外（5h 之前）→ 5h 不计入；且早于 31d 新桶起点 → 31d 也不计入
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: now - 9500, ModelName: "old", Quota: 500},       // A
		{CreatedAt: now - 500, ModelName: "new", Quota: 250},        // B
		{CreatedAt: now - 6*3600, ModelName: "expired", Quota: 400}, // C
	})
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage2, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)

	// 5h 滑窗：日志 A(now-9500) 与 B(now-500) 在滑窗内 → 累计；C(now-6h) 在窗外 → 不计
	assert.Equal(t, int64(1000+500+250), usage2.UsedQuota5h)
	// 31d 桶：翻转清零后，旧桶日志 A 与新桶起点之前的 C 均被跳过；仅新桶内 B 计入
	assert.Equal(t, int64(250), usage2.UsedQuota31d)
}

func TestFullRecalibrateChannelSubscriptionUsage(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 9
	now := common.GetTimestamp()
	bucketStart := now - 3600
	usage := &model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   now,
		LastRefreshAt:      now,
		TimeRollingUpdated: now, UsedQuota5h: 999,
		TimeWeeklyUpdated: now, UsedQuota7d: 999,
		BucketStart31d: bucketStart, UsedQuota31d: 999,
		UpdatedAt: now,
	}
	require.NoError(t, model.UpsertChannelSubscriptionUsage(usage))

	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: bucketStart + 10, ModelName: "a", Quota: 100},
		{CreatedAt: bucketStart + 20, ModelName: "b", Quota: 50},
		{CreatedAt: bucketStart + 30, ModelName: "a", Quota: -10}, // 忽略
	})
	require.NoError(t, FullRecalibrateChannelSubscriptionUsage(channelId))
	usage2, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.Equal(t, int64(150), usage2.UsedQuota5h) // 滑窗 [now-5h, now]
	assert.Equal(t, int64(150), usage2.UsedQuota31d)
	if weekElapsedOk(now, 3600+100) {
		assert.Equal(t, int64(150), usage2.UsedQuota7d) // 自然周 [周一0点, now]
	}
	assert.True(t, usage2.Partial) // 最早日志距今 <31 天

	// 出现早于 31 天的日志后 partial=false，且不参与窗口求和
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: now - SubscriptionWindow31dSeconds - 1000, ModelName: "old", Quota: 1},
	})
	require.NoError(t, FullRecalibrateChannelSubscriptionUsage(channelId))
	usage2, err = model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.False(t, usage2.Partial)
	assert.Equal(t, int64(150), usage2.UsedQuota31d)
}

func TestBuildSubscriptionPerModelUsage(t *testing.T) {
	t.Run("exact model beats wildcard", func(t *testing.T) {
		cfg := &dto.SubscriptionBillingConfig{
			MonthlyTotalQuota: 10000,
			ModelTiers: []dto.SubscriptionModelTier{
				{Model: "model-a", MonthlyQuota: 4000},
				{Model: "*", MonthlyQuota: 2000},
			},
		}
		used := map[string]int64{"model-a": 5000, "model-b": 3000, "model-c": 500}
		perModel := buildSubscriptionPerModelUsage(cfg, used)
		require.Len(t, perModel, 3)
		byModel := make(map[string]SubscriptionPerModelUsage, len(perModel))
		for _, pm := range perModel {
			byModel[pm.Model] = pm
		}
		assert.Equal(t, int64(4000), byModel["model-a"].LimitQuota) // 精确档位优先
		assert.True(t, byModel["model-a"].OverLimit)
		assert.Equal(t, int64(2000), byModel["model-b"].LimitQuota) // 兜底档
		assert.True(t, byModel["model-b"].OverLimit)
		assert.Equal(t, int64(2000), byModel["model-c"].LimitQuota)
		assert.False(t, byModel["model-c"].OverLimit)
	})

	t.Run("unconfigured model falls back to channel total", func(t *testing.T) {
		cfg := &dto.SubscriptionBillingConfig{
			MonthlyTotalQuota: 10000,
			ModelTiers:        []dto.SubscriptionModelTier{{Model: "model-a", MonthlyQuota: 4000}},
		}
		used := map[string]int64{"model-b": 3000}
		perModel := buildSubscriptionPerModelUsage(cfg, used)
		require.Len(t, perModel, 2) // 已配置档位即使无用量也展示
		byModel := make(map[string]SubscriptionPerModelUsage, len(perModel))
		for _, pm := range perModel {
			byModel[pm.Model] = pm
		}
		assert.Equal(t, int64(10000), byModel["model-b"].LimitQuota)
		assert.Zero(t, byModel["model-a"].UsedQuota)
	})
}

func TestBuildSubscriptionUsageData(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 12
	cfg := &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModeSubscribe,
		MonthlyTotalQuota: 10000,
		FiveHourRatioBps:  2000,
		WeeklyRatioBps:    5000,
		ModelTiers:        []dto.SubscriptionModelTier{{Model: "model-a", MonthlyQuota: 4000}},
	}
	now := common.GetTimestamp()
	require.NoError(t, model.UpsertChannelSubscriptionUsage(&model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   now - 3600,
		LastRefreshAt:      now - 3600,
		TimeRollingUpdated: now - 3600,
		TimeWeeklyUpdated:  now - 3600,
		BucketStart31d:     now - 3600,
		UpdatedAt:          now - 3600,
	}))
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: now - 3500, ModelName: "model-a", Quota: 3000},
		{CreatedAt: now - 3000, ModelName: "model-b", Quota: 500},
	})
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))

	data, err := BuildSubscriptionUsageData(channelId, cfg, common.GetTimestamp())
	require.NoError(t, err)
	assert.Equal(t, int64(3500), data.Windows["31d"].UsedQuota)
	assert.Equal(t, int64(10000), data.Windows["31d"].LimitQuota)
	assert.InDelta(t, 35.0, data.Windows["31d"].UsedPercent, 1e-6)
	assert.InDelta(t, 35.0, data.Windows["31d"].DisplayPercent, 1e-6)
	assert.False(t, data.Windows["31d"].OverLimit)
	assert.Equal(t, int64(18000), data.Windows["5h"].WindowSizeSeconds)
	assert.Equal(t, int64(2000), data.Windows["5h"].LimitQuota) // 10000*2000/10000
	assert.Equal(t, int64(5000), data.Windows["7d"].LimitQuota) // 10000*5000/10000
	assert.True(t, data.Windows["31d"].ResetAt > data.UpdatedAt)

	require.Len(t, data.PerModel, 2)
	byModel := make(map[string]SubscriptionPerModelUsage, len(data.PerModel))
	for _, pm := range data.PerModel {
		byModel[pm.Model] = pm
	}
	assert.Equal(t, int64(3000), byModel["model-a"].UsedQuota)
	assert.Equal(t, int64(4000), byModel["model-a"].LimitQuota)
	assert.Equal(t, int64(500), byModel["model-b"].UsedQuota)
	assert.Equal(t, int64(10000), byModel["model-b"].LimitQuota)
}

func TestBuildChannelSubscriptionListSnapshots(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 13
	cfg := &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModeSubscribe,
		MonthlyTotalQuota: 10000,
		FiveHourRatioBps:  2000,
		WeeklyRatioBps:    5000,
		ModelTiers:        []dto.SubscriptionModelTier{{Model: "model-a", MonthlyQuota: 4000}},
	}
	channel := &model.Channel{Id: channelId}
	channel.SetOtherSettings(dto.ChannelOtherSettings{SubscriptionBilling: cfg})

	// 未启用订阅计费的渠道不应出现在快照中
	payAsYouGo := &model.Channel{Id: 99}
	payAsYouGo.SetOtherSettings(dto.ChannelOtherSettings{SubscriptionBilling: &dto.SubscriptionBillingConfig{
		BillingMode: dto.SubscriptionBillingModePayAsYouGo,
	}})

	now := common.GetTimestamp()
	require.NoError(t, model.UpsertChannelSubscriptionUsage(&model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   now - 3600,
		LastRefreshAt:      now - 3600,
		TimeRollingUpdated: now - 3600,
		TimeWeeklyUpdated:  now - 3600,
		BucketStart31d:     now - 3600,
		UpdatedAt:          now - 3600,
	}))
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: now - 3500, ModelName: "model-a", Quota: 5000}, // 超档位 4000
		{CreatedAt: now - 3000, ModelName: "model-b", Quota: 500},
	})
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))

	snapshots, err := BuildChannelSubscriptionListSnapshots([]*model.Channel{channel, payAsYouGo})
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	snapshot, ok := snapshots[int64(channelId)]
	require.True(t, ok)
	assert.Equal(t, int64(5500), snapshot.MonthlyUsed)
	assert.Equal(t, int64(10000), snapshot.MonthlyLimit)
	assert.True(t, snapshot.ModelOverLimit) // model-a 5000 > 4000
	assert.True(t, snapshot.UpdatedAt > 0)
}

// ---------------------------------------------------------------------------
// 手动基线（三窗口独立）相关测试
// ---------------------------------------------------------------------------

// createSubscriptionChannel 创建已启用订阅计费的渠道（供 SetChannelSubscriptionBaseline 等使用）。
func createSubscriptionChannel(t *testing.T, channelId int) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Id:    channelId,
		Type:  1,
		Name:  "sub-channel",
		Key:   "sk-subscription-billing-test",
		Group: "default",
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{SubscriptionBilling: &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModeSubscribe,
		MonthlyTotalQuota: 30000000, // 60 USD
		FiveHourRatioBps:  2000,     // 20%
		WeeklyRatioBps:    5000,     // 50%
	}})
	require.NoError(t, model.DB.Create(channel).Error)
	return channel
}

func TestSubscriptionBaselineBpsFromPercent(t *testing.T) {
	bps, err := SubscriptionBaselineBpsFromPercent(30)
	require.NoError(t, err)
	assert.Equal(t, 3000, bps)

	// 允许 >100（超限基线）
	bps, err = SubscriptionBaselineBpsFromPercent(200)
	require.NoError(t, err)
	assert.Equal(t, 20000, bps)

	_, err = SubscriptionBaselineBpsFromPercent(-10)
	require.Error(t, err)
	_, err = SubscriptionBaselineBpsFromPercent(math.NaN())
	require.Error(t, err)
	_, err = SubscriptionBaselineBpsFromPercent(math.Inf(1))
	require.Error(t, err)
}

func TestRefreshFirstInitBackfillsHistory(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 44
	now := common.GetTimestamp()
	// 5h 内 1 条、7d 内 5h 外 1 条、31d 内 7d 外 1 条
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: now - 60, ModelName: "a", Quota: 100},
		{CreatedAt: now - SubscriptionWindow5hSeconds - 60, ModelName: "b", Quota: 200},
		{CreatedAt: now - SubscriptionWindow7dSeconds - 60, ModelName: "c", Quota: 400},
	})
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.Equal(t, int64(100), usage.UsedQuota5h)
	// 7d 自然周：[周一0点, now]；此刻距离周一起点需覆盖 7d 窗口内日志
	if weekElapsedOk(now, SubscriptionWindow5hSeconds+120) {
		assert.Equal(t, int64(300), usage.UsedQuota7d)
	}
	assert.Equal(t, int64(700), usage.UsedQuota31d)
	assert.GreaterOrEqual(t, usage.LastCheckpointAt, now)
}

func TestSetBaselineThenRefreshIncremental(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 45
	channel := createSubscriptionChannel(t, channelId)
	defer func() { _ = channel.Delete() }()

	now := common.GetTimestamp()
	baselineAt := now - 100
	require.NoError(t, SetChannelSubscriptionBaseline(channelId, SubscriptionBaselineInput{
		UsedPercent31d: f64p(30),
		BaselineAt31d:  i64p(baselineAt),
	}))

	// 基线时刻之后的日志才会被增量累加（基线时刻之前的历史已由基线声明承载）
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: baselineAt + 10, ModelName: "a", Quota: 50},
		{CreatedAt: now - 20, ModelName: "b", Quota: 60},
	})
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))

	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.True(t, usage.ManualInitialized)
	assert.Equal(t, 3000, usage.BaselineBps31d)
	assert.Equal(t, baselineAt, usage.BaselineSetAt31d)
	assert.Equal(t, baselineAt, usage.BucketStart31d)
	// UsedQuota 为纯增量，不含基线（基线在 Build 时叠加）
	assert.Equal(t, int64(110), usage.UsedQuota31d)
	assert.Equal(t, int64(110), usage.UsedQuota5h)
	// 增量起点 = baselineAt
	assert.LessOrEqual(t, usage.LastCheckpointAt, now)
}

func TestRefreshIdempotentAfterBaseline(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 46
	channel := createSubscriptionChannel(t, channelId)
	defer func() { _ = channel.Delete() }()

	now := common.GetTimestamp()
	baselineAt := now - 100
	require.NoError(t, SetChannelSubscriptionBaseline(channelId, SubscriptionBaselineInput{
		UsedPercent31d: f64p(30),
		BaselineAt31d:  i64p(baselineAt),
	}))
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: baselineAt + 10, ModelName: "a", Quota: 50},
	})
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.Equal(t, int64(50), usage.UsedQuota31d)

	// 无新日志时重复刷新不重复累加
	cp := usage.LastCheckpointAt
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage2, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.Equal(t, int64(50), usage2.UsedQuota31d)
	assert.GreaterOrEqual(t, usage2.LastCheckpointAt, cp)
}

func TestMonthlyRolloverClearsBaseline(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 47
	now := common.GetTimestamp()
	old31d := now - SubscriptionWindow31dSeconds - 3600 // 已过月度翻转点
	usage := &model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   now,
		LastRefreshAt:      now,
		ManualInitialized:  true,
		BaselineBps31d:     3000,
		BaselineSetAt31d:   now - SubscriptionWindow31dSeconds - 7200,
		TimeRollingUpdated: now - 100, UsedQuota5h: 111,
		TimeWeeklyUpdated: now - 100, UsedQuota7d: 222,
		BucketStart31d: old31d, UsedQuota31d: 333,
		UpdatedAt: now,
	}
	require.NoError(t, model.UpsertChannelSubscriptionUsage(usage))

	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage2, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	// 月度翻转：增量清零 + 基线失效
	assert.Equal(t, int64(0), usage2.UsedQuota31d)
	assert.Equal(t, 0, usage2.BaselineBps31d)
	assert.Equal(t, int64(0), usage2.BaselineSetAt31d)
	assert.Greater(t, usage2.BucketStart31d, old31d)
	// 5h 滑窗未过期不受影响；7d 不跨周不受影响
	assert.Equal(t, int64(111), usage2.UsedQuota5h)
	if weekElapsedOk(now, 100) {
		assert.Equal(t, int64(222), usage2.UsedQuota7d)
	}
	// 手动基线标志保留（仍走增量分支）
	assert.True(t, usage2.ManualInitialized)
}

// TestRolling5hExpiryClearsIncrementButKeepsBaseline 验证 5h 滑窗过期：
// 增量归 0、该窗口独立基线保留；Build 时 5h 展示 = 基线 quota。
func TestRolling5hExpiryClearsIncrementButKeepsBaseline(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 48
	now := common.GetTimestamp()
	usage := &model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   now,
		LastRefreshAt:      now,
		ManualInitialized:  true,
		BaselineBps5h:      3000,
		BaselineSetAt5h:    now - SubscriptionWindow5hSeconds - 7200,
		TimeRollingUpdated: now - SubscriptionWindow5hSeconds - 3600, // 已过期
		UsedQuota5h:        111,
		BaselineBps7d:      3000,
		BaselineSetAt7d:    now - 100,
		TimeWeeklyUpdated:  now - 100, UsedQuota7d: 222,
		BaselineBps31d:   3000,
		BaselineSetAt31d: now - 100,
		BucketStart31d:   now - 100, UsedQuota31d: 333,
		UpdatedAt: now,
	}
	require.NoError(t, model.UpsertChannelSubscriptionUsage(usage))

	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage2, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	// 5h 滑窗过期：增量清零、基线保留
	assert.Equal(t, int64(0), usage2.UsedQuota5h)
	assert.Equal(t, 3000, usage2.BaselineBps5h)

	// Build 时 5h 窗口展示 = 独立基线 quota（BaselineBps5h × limit5h/10000）
	cfg := &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModeSubscribe,
		MonthlyTotalQuota: 30000000, // 60 USD
		FiveHourRatioBps:  2000,     // 20%
		WeeklyRatioBps:    5000,     // 50%
	}
	data, err := BuildSubscriptionUsageData(channelId, cfg, now)
	require.NoError(t, err)
	// limit5h = 30000000×2000/10000 = 6000000；
	// baselineQuota5h = 6000000×3000/10000 = 1800000；增量已过期 → 0
	assert.Equal(t, int64(1800000), data.Windows["5h"].UsedQuota)
	// 31d 窗口 = 独立月基线 9000000 + 增量 333
	assert.Equal(t, int64(9000333), data.Windows["31d"].UsedQuota)
}

func TestFullRecalibratePreservesBaseline(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 49
	now := common.GetTimestamp()
	bucketStart := now - 3600
	usage := &model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   now,
		LastRefreshAt:      now,
		ManualInitialized:  true,
		BaselineBps5h:      2000,
		BaselineSetAt5h:    now - 7200,
		TimeRollingUpdated: now, UsedQuota5h: 1,
		BaselineBps7d:     3000,
		BaselineSetAt7d:   now - 7200,
		TimeWeeklyUpdated: now, UsedQuota7d: 1,
		BaselineBps31d:   4000,
		BaselineSetAt31d: now - 7200,
		BucketStart31d:   bucketStart, UsedQuota31d: 1,
		UpdatedAt: now,
	}
	require.NoError(t, model.UpsertChannelSubscriptionUsage(usage))
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: bucketStart + 10, ModelName: "a", Quota: 100},
		{CreatedAt: bucketStart + 20, ModelName: "b", Quota: 50},
	})
	require.NoError(t, FullRecalibrateChannelSubscriptionUsage(channelId))
	usage2, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	// 全量校正只重算增量；三窗口基线字段原样保留
	assert.Equal(t, 2000, usage2.BaselineBps5h)
	assert.Equal(t, 3000, usage2.BaselineBps7d)
	assert.Equal(t, 4000, usage2.BaselineBps31d)
	assert.Equal(t, now-7200, usage2.BaselineSetAt5h)
	assert.Equal(t, now-7200, usage2.BaselineSetAt7d)
	assert.Equal(t, now-7200, usage2.BaselineSetAt31d)
	assert.True(t, usage2.ManualInitialized)
	assert.Equal(t, int64(150), usage2.UsedQuota31d)
	assert.Equal(t, int64(150), usage2.UsedQuota5h)
}

func TestBuildBaselinePercentToQuota(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 50
	now := common.GetTimestamp()
	cfg := &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModeSubscribe,
		MonthlyTotalQuota: 30000000, // 60 USD
		FiveHourRatioBps:  2000,
		WeeklyRatioBps:    5000,
	}
	cases := []struct {
		name      string
		percent   int
		wantQuota int64 // 月窗口 used（无增量）
		wantPct   float64
		wantOver  bool
	}{
		{name: "zero baseline", percent: 0, wantQuota: 0, wantPct: 0},
		{name: "30 percent", percent: 30, wantQuota: 9000000, wantPct: 30}, // 60×30%
		{name: "100 percent", percent: 100, wantQuota: 30000000, wantPct: 100},
		{name: "150 percent over limit", percent: 150, wantQuota: 45000000, wantPct: 150, wantOver: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, model.UpsertChannelSubscriptionUsage(&model.ChannelSubscriptionUsage{
				ChannelId:          int64(channelId),
				LastCheckpointAt:   now,
				LastRefreshAt:      now,
				ManualInitialized:  true,
				TimeRollingUpdated: now, UsedQuota5h: 0,
				TimeWeeklyUpdated: now, UsedQuota7d: 0,
				BucketStart31d: now, UsedQuota31d: 0,
				BaselineBps31d:   tc.percent * 100,
				BaselineSetAt31d: now,
				UpdatedAt:        now,
			}))
			data, err := BuildSubscriptionUsageData(channelId, cfg, now)
			require.NoError(t, err)
			w := data.Windows["31d"]
			assert.Equal(t, tc.wantQuota, w.UsedQuota)
			assert.InDelta(t, tc.wantPct, w.UsedPercent, 1e-6)
			assert.Equal(t, tc.wantOver, w.OverLimit)
		})
	}
}

func TestConfigChangeRecomputesBaselineQuota(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 51
	now := common.GetTimestamp()
	// 月基线 30% + 增量 1000（请求：改月额度后基线 quota 跟随配置，增量不清零）
	require.NoError(t, model.UpsertChannelSubscriptionUsage(&model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   now,
		LastRefreshAt:      now,
		ManualInitialized:  true,
		TimeRollingUpdated: now, UsedQuota5h: 1000,
		TimeWeeklyUpdated: now, UsedQuota7d: 1000,
		BucketStart31d: now, UsedQuota31d: 1000,
		BaselineBps31d:   3000,
		BaselineSetAt31d: now,
		UpdatedAt:        now,
	}))

	// 60 USD 配置：基线 quota = 60×30% = 18 USD
	cfg60 := &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModeSubscribe,
		MonthlyTotalQuota: 30000000,
		FiveHourRatioBps:  2000,
		WeeklyRatioBps:    5000,
	}
	data60, err := BuildSubscriptionUsageData(channelId, cfg60, now)
	require.NoError(t, err)
	assert.Equal(t, int64(9000000+1000), data60.Windows["31d"].UsedQuota)

	// 100 USD 配置：基线 quota 跟随 = 100×30% = 30 USD；增量 1000 保留
	cfg100 := &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModeSubscribe,
		MonthlyTotalQuota: 50000000, // 100 USD
		FiveHourRatioBps:  2000,
		WeeklyRatioBps:    5000,
	}
	data100, err := BuildSubscriptionUsageData(channelId, cfg100, now)
	require.NoError(t, err)
	assert.Equal(t, int64(15000000+1000), data100.Windows["31d"].UsedQuota)
	assert.InDelta(t, 30.002, data100.Windows["31d"].UsedPercent, 1e-3)
}

func TestSetBaselineValidation(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 52
	now := common.GetTimestamp()
	channel := createSubscriptionChannel(t, channelId)
	defer func() { _ = channel.Delete() }()

	// 未来时间拒绝
	err := SetChannelSubscriptionBaseline(channelId, SubscriptionBaselineInput{
		UsedPercent31d: f64p(30),
		BaselineAt31d:  i64p(now + 100),
	})
	require.Error(t, err)
	// 负数百分比拒绝
	err = SetChannelSubscriptionBaseline(channelId, SubscriptionBaselineInput{
		UsedPercent31d: f64p(-1),
	})
	require.Error(t, err)
	// 未设置任何窗口拒绝
	err = SetChannelSubscriptionBaseline(channelId, SubscriptionBaselineInput{})
	require.Error(t, err)
	// 超限百分比（150%）接受
	require.NoError(t, SetChannelSubscriptionBaseline(channelId, SubscriptionBaselineInput{
		UsedPercent31d: f64p(150),
	}))
	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.Equal(t, 15000, usage.BaselineBps31d)
}

func TestSetBaselineRequiresSubscribeMode(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 53
	// 按量计费渠道不允许设置基线
	channel := &model.Channel{Id: channelId, Type: 1, Name: "payg", Key: "sk-payg", Group: "default"}
	channel.SetOtherSettings(dto.ChannelOtherSettings{SubscriptionBilling: &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModePayAsYouGo,
		MonthlyTotalQuota: 30000000,
	}})
	require.NoError(t, model.DB.Create(channel).Error)
	defer func() { _ = channel.Delete() }()

	err := SetChannelSubscriptionBaseline(channelId, SubscriptionBaselineInput{
		UsedPercent31d: f64p(30),
	})
	require.Error(t, err)
}

// TestThreeWindowIndependentBaselines 验证三窗口基线各自独立互不影响：
// 只设置 5h 与 31d 基线，7d 保持未设置；Build 时各窗口展示各自的基线 quota。
func TestThreeWindowIndependentBaselines(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 55
	now := common.GetTimestamp()

	require.NoError(t, model.UpsertChannelSubscriptionUsage(&model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   now,
		LastRefreshAt:      now,
		ManualInitialized:  true,
		BaselineBps5h:      2000,      // 5h 基线 20%
		BaselineSetAt5h:    now - 100, // 5h 窗口已过期（>5h 前）→ 仅展示基线
		TimeRollingUpdated: now - SubscriptionWindow5hSeconds - 3600,
		UsedQuota5h:        0,
		BaselineBps31d:     4000,      // 31d 基线 40%
		BaselineSetAt31d:   now - 100, // 月窗口未过期
		BucketStart31d:     now - 100,
		UsedQuota31d:       500,
		UpdatedAt:          now,
	}))

	cfg := &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModeSubscribe,
		MonthlyTotalQuota: 30000000, // 60 USD
		FiveHourRatioBps:  2000,
		WeeklyRatioBps:    5000,
	}
	data, err := BuildSubscriptionUsageData(channelId, cfg, now)
	require.NoError(t, err)
	// 5h：基线 20% × limit5h(6000000) = 1200000；增量过期 → 0
	assert.Equal(t, int64(1200000), data.Windows["5h"].UsedQuota)
	// 31d：基线 40% × 30000000 = 12000000 + 增量 500
	assert.Equal(t, int64(12000500), data.Windows["31d"].UsedQuota)
	// 7d：未设置基线 → 仅增量（0）
	assert.Equal(t, int64(0), data.Windows["7d"].UsedQuota)
	assert.InDelta(t, 20.0, data.Windows["5h"].UsedPercent, 1e-6)
}

// TestRolling5hResetAnchorsToUpdateTime 验证滑窗 reset 语义：
// 最近累计时刻 + 5h 为窗口终点（对齐 opencode analyzeRollingUsage）。
func TestRolling5hResetAnchorsToUpdateTime(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 56
	now := common.GetTimestamp()
	usage := &model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   now,
		LastRefreshAt:      now,
		TimeRollingUpdated: now - 3600, UsedQuota5h: 500,
		TimeWeeklyUpdated: now, UsedQuota7d: 0,
		BucketStart31d: now, UsedQuota31d: 0,
		UpdatedAt: now,
	}
	require.NoError(t, model.UpsertChannelSubscriptionUsage(usage))
	cfg := &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModeSubscribe,
		MonthlyTotalQuota: 30000000,
		FiveHourRatioBps:  2000,
		WeeklyRatioBps:    5000,
	}
	data, err := BuildSubscriptionUsageData(channelId, cfg, now)
	require.NoError(t, err)
	// 5h 滑窗有效（timeRollingUpdated=now-1h 在窗内）：Used=增量 500
	// resetAt = timeRollingUpdated + 5h = now + 4h
	assert.Equal(t, int64(500), data.Windows["5h"].UsedQuota)
	assert.Equal(t, usage.TimeRollingUpdated+SubscriptionWindow5hSeconds, data.Windows["5h"].ResetAt)

	// 滑窗过期（timeRollingUpdated 在 5h 外）：增量归 0，resetAt = now + 5h
	usage2 := &model.ChannelSubscriptionUsage{
		ChannelId:          int64(channelId),
		LastCheckpointAt:   now,
		LastRefreshAt:      now,
		TimeRollingUpdated: now - SubscriptionWindow5hSeconds - 60, UsedQuota5h: 500,
		TimeWeeklyUpdated: now, UsedQuota7d: 0,
		BucketStart31d: now, UsedQuota31d: 0,
		UpdatedAt: now,
	}
	require.NoError(t, model.UpsertChannelSubscriptionUsage(usage2))
	data2, err := BuildSubscriptionUsageData(channelId, cfg, now)
	require.NoError(t, err)
	assert.Equal(t, int64(0), data2.Windows["5h"].UsedQuota)
	assert.Equal(t, now+SubscriptionWindow5hSeconds, data2.Windows["5h"].ResetAt)
}
