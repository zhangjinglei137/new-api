package service

import (
	"math"
	"testing"

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

func TestApplyBucketRollover(t *testing.T) {
	now := int64(1000000)
	usage := &model.ChannelSubscriptionUsage{
		BucketStart5h: now, UsedQuota5h: 100,
		BucketStart7d: now, UsedQuota7d: 100,
		BucketStart31d: now, UsedQuota31d: 100,
	}
	// 未跨周期：不清零
	applyBucketRollover(usage, now+SubscriptionWindow5hSeconds-1)
	assert.Equal(t, int64(100), usage.UsedQuota5h)
	assert.Equal(t, now, usage.BucketStart5h)
	// 恰跨 5h 周期：只清零 5h
	applyBucketRollover(usage, now+SubscriptionWindow5hSeconds)
	assert.Equal(t, int64(0), usage.UsedQuota5h)
	assert.Equal(t, now+SubscriptionWindow5hSeconds, usage.BucketStart5h)
	assert.Equal(t, int64(100), usage.UsedQuota7d)
	assert.Equal(t, int64(100), usage.UsedQuota31d)
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
		ChannelId:        int64(channelId),
		LastCheckpointAt: now - 100,
		LastRefreshAt:    now - 100,
		BucketStart5h:    now - 100,
		BucketStart7d:    now - 100,
		BucketStart31d:   now - 100,
		UpdatedAt:        now - 100,
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
	assert.Equal(t, int64(300), usage2.UsedQuota7d)
	assert.Equal(t, int64(300), usage2.UsedQuota31d)
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
	// 首次初始化回填：桶起点 = now - 窗口（无日志时用量为 0）
	assert.GreaterOrEqual(t, usage.BucketStart5h, before-SubscriptionWindow5hSeconds)
	assert.LessOrEqual(t, usage.BucketStart5h, after-SubscriptionWindow5hSeconds)
	assert.Zero(t, usage.UsedQuota5h)
	assert.Zero(t, usage.UsedQuota31d)
	assert.GreaterOrEqual(t, usage.LastCheckpointAt, before)
}

func TestRefreshChannelSubscriptionUsageRolloverSkipsOldBucket(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 7
	now := common.GetTimestamp()
	oldStart := now - SubscriptionWindow5hSeconds - 1000 // 距 5h 桶起点 19000s
	checkpoint := now - 2000

	usage := &model.ChannelSubscriptionUsage{
		ChannelId:        int64(channelId),
		LastCheckpointAt: checkpoint,
		LastRefreshAt:    checkpoint,
		BucketStart5h:    oldStart, UsedQuota5h: 1000,
		BucketStart7d: oldStart, UsedQuota7d: 1000,
		BucketStart31d: oldStart, UsedQuota31d: 1000,
		UpdatedAt: checkpoint,
	}
	require.NoError(t, model.UpsertChannelSubscriptionUsage(usage))

	// now-1500 位于旧 5h 桶（< 新桶起点 now-1000），now-500 位于新桶
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: now - 1500, ModelName: "m", Quota: 500},
		{CreatedAt: now - 500, ModelName: "m", Quota: 250},
	})
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage2, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)

	assert.Equal(t, oldStart+SubscriptionWindow5hSeconds, usage2.BucketStart5h)
	assert.Equal(t, int64(250), usage2.UsedQuota5h) // 旧桶日志被跳过
	assert.Equal(t, oldStart, usage2.BucketStart7d) // 7d 未翻转
	assert.Equal(t, int64(1750), usage2.UsedQuota7d)
	assert.Equal(t, int64(1750), usage2.UsedQuota31d)
}

func TestFullRecalibrateChannelSubscriptionUsage(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 9
	now := common.GetTimestamp()
	bucketStart := now - 3600
	usage := &model.ChannelSubscriptionUsage{
		ChannelId:        int64(channelId),
		LastCheckpointAt: now,
		LastRefreshAt:    now,
		BucketStart5h:    bucketStart, UsedQuota5h: 999,
		BucketStart7d: bucketStart, UsedQuota7d: 999,
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
	assert.Equal(t, int64(150), usage2.UsedQuota5h)
	assert.Equal(t, int64(150), usage2.UsedQuota7d)
	assert.Equal(t, int64(150), usage2.UsedQuota31d)
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
		ChannelId:        int64(channelId),
		LastCheckpointAt: now - 3600,
		LastRefreshAt:    now - 3600,
		BucketStart5h:    now - 3600,
		BucketStart7d:    now - 3600,
		BucketStart31d:   now - 3600,
		UpdatedAt:        now - 3600,
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
		ChannelId:        int64(channelId),
		LastCheckpointAt: now - 3600,
		LastRefreshAt:    now - 3600,
		BucketStart5h:    now - 3600,
		BucketStart7d:    now - 3600,
		BucketStart31d:   now - 3600,
		UpdatedAt:        now - 3600,
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
// 手动基线（百分比）相关测试
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
	assert.Equal(t, int64(300), usage.UsedQuota7d)
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
	require.NoError(t, SetChannelSubscriptionBaseline(channelId, 3000, baselineAt))

	// 基线时刻之后的日志才会被增量累加
	insertConsumeLogs(t, channelId, []consumeLogSeed{
		{CreatedAt: baselineAt + 10, ModelName: "a", Quota: 50},
		{CreatedAt: now - 20, ModelName: "b", Quota: 60},
	})
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))

	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.True(t, usage.ManualInitialized)
	assert.Equal(t, 3000, usage.BaselineBps31d)
	assert.Equal(t, baselineAt, usage.BaselineSetAt)
	assert.Equal(t, baselineAt, usage.BucketStart31d)
	// UsedQuota 为纯增量，不含基线
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
	require.NoError(t, SetChannelSubscriptionBaseline(channelId, 3000, baselineAt))
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
		ChannelId:         int64(channelId),
		LastCheckpointAt:  now,
		LastRefreshAt:     now,
		ManualInitialized: true,
		BaselineBps31d:    3000,
		BaselineSetAt:     now - SubscriptionWindow31dSeconds - 7200,
		BucketStart5h:     now - 100, UsedQuota5h: 111,
		BucketStart7d: now - 100, UsedQuota7d: 222,
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
	assert.Greater(t, usage2.BucketStart31d, old31d)
	// 5h/7d 未跨周期不受影响
	assert.Equal(t, int64(111), usage2.UsedQuota5h)
	assert.Equal(t, int64(222), usage2.UsedQuota7d)
	// 手动基线标志保留（仍走增量分支）
	assert.True(t, usage2.ManualInitialized)
}

func Test5hRolloverKeepsBaseline(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 48
	now := common.GetTimestamp()
	usage := &model.ChannelSubscriptionUsage{
		ChannelId:         int64(channelId),
		LastCheckpointAt:  now,
		LastRefreshAt:     now,
		ManualInitialized: true,
		BaselineBps31d:    3000,
		BaselineSetAt:     now - 100,
		BucketStart5h:     now - SubscriptionWindow5hSeconds - 3600, // 已过 5h 翻转点
		UsedQuota5h:       111,
		BucketStart7d:     now - 100, UsedQuota7d: 222,
		BucketStart31d: now - 100, UsedQuota31d: 333,
		UpdatedAt: now,
	}
	require.NoError(t, model.UpsertChannelSubscriptionUsage(usage))

	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage2, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	// 5h 翻转只清增量，不清基线
	assert.Equal(t, int64(0), usage2.UsedQuota5h)
	assert.Equal(t, 3000, usage2.BaselineBps31d)

	// Build 时 5h 窗口展示 = 派生基线 quota（BaselineBps31d × FiveHourRatioBps/10000）
	cfg := &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModeSubscribe,
		MonthlyTotalQuota: 30000000, // 60 USD
		FiveHourRatioBps:  2000,     // 20%
		WeeklyRatioBps:    5000,     // 50%
	}
	data, err := BuildSubscriptionUsageData(channelId, cfg, now)
	require.NoError(t, err)
	// limit5h = 30000000×2000/10000 = 6000000；
	// baselineBps5h = 3000×2000/10000 = 600 bps；
	// baselineQuota5h = 6000000×600/10000 = 360000；增量 0
	assert.Equal(t, int64(360000), data.Windows["5h"].UsedQuota)
	// 31d 窗口 = 基线 9000000 + 增量 333
	assert.Equal(t, int64(9000333), data.Windows["31d"].UsedQuota)
}

func TestFullRecalibratePreservesBaseline(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 49
	now := common.GetTimestamp()
	bucketStart := now - 3600
	usage := &model.ChannelSubscriptionUsage{
		ChannelId:         int64(channelId),
		LastCheckpointAt:  now,
		LastRefreshAt:     now,
		ManualInitialized: true,
		BaselineBps31d:    3000,
		BaselineSetAt:     now - 7200,
		BucketStart5h:     bucketStart, UsedQuota5h: 1,
		BucketStart7d: bucketStart, UsedQuota7d: 1,
		BucketStart31d: bucketStart, UsedQuota31d: 1,
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
	// 全量校正只重算增量；基线字段原样保留
	assert.Equal(t, 3000, usage2.BaselineBps31d)
	assert.Equal(t, now-7200, usage2.BaselineSetAt)
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
				ChannelId:         int64(channelId),
				LastCheckpointAt:  now,
				LastRefreshAt:     now,
				ManualInitialized: true,
				BaselineBps31d:    tc.percent * 100,
				BaselineSetAt:     now,
				BucketStart5h:     now, UsedQuota5h: 0,
				BucketStart7d: now, UsedQuota7d: 0,
				BucketStart31d: now, UsedQuota31d: 0,
				UpdatedAt: now,
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
	// 基线 30% + 增量 1000（请求：改月额度后基线 quota 跟随配置，增量不清零）
	require.NoError(t, model.UpsertChannelSubscriptionUsage(&model.ChannelSubscriptionUsage{
		ChannelId:         int64(channelId),
		LastCheckpointAt:  now,
		LastRefreshAt:     now,
		ManualInitialized: true,
		BaselineBps31d:    3000,
		BaselineSetAt:     now,
		BucketStart5h:     now, UsedQuota5h: 1000,
		BucketStart7d: now, UsedQuota7d: 1000,
		BucketStart31d: now, UsedQuota31d: 1000,
		UpdatedAt: now,
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
	err := SetChannelSubscriptionBaseline(channelId, 3000, now+100)
	require.Error(t, err)
	// 负数百分比拒绝
	err = SetChannelSubscriptionBaseline(channelId, -1, now)
	require.Error(t, err)
	// 超限百分比（150%）接受
	require.NoError(t, SetChannelSubscriptionBaseline(channelId, 15000, now))
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

	err := SetChannelSubscriptionBaseline(channelId, 3000, common.GetTimestamp())
	require.Error(t, err)
}
