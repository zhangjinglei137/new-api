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

func TestRefreshChannelSubscriptionUsageFirstInit(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 43
	before := common.GetTimestamp()
	require.NoError(t, RefreshChannelSubscriptionUsage(channelId))
	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.True(t, usage.BucketStart5h >= before)
	assert.True(t, usage.BucketStart31d >= before)
	assert.Zero(t, usage.UsedQuota31d)
	assert.True(t, usage.LastCheckpointAt >= before)
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

func TestResetChannelSubscriptionUsage(t *testing.T) {
	setupSubscriptionUsageTest(t)
	channelId := 5
	now := common.GetTimestamp()
	oldBucket := now - 10000
	usage := &model.ChannelSubscriptionUsage{
		ChannelId:        int64(channelId),
		LastCheckpointAt: now - 5000,
		LastRefreshAt:    now - 5000,
		BucketStart5h:    oldBucket, UsedQuota5h: 500,
		BucketStart7d: oldBucket, UsedQuota7d: 500,
		BucketStart31d: oldBucket, UsedQuota31d: 500,
		UpdatedAt: now - 5000,
	}
	require.NoError(t, model.UpsertChannelSubscriptionUsage(usage))

	require.NoError(t, ResetChannelSubscriptionUsage(channelId))
	usage2, err := model.GetChannelSubscriptionUsage(int64(channelId))
	require.NoError(t, err)
	assert.Zero(t, usage2.UsedQuota5h)
	assert.Zero(t, usage2.UsedQuota7d)
	assert.Zero(t, usage2.UsedQuota31d)
	// 桶未跨周期时保持原起点；LastCheckpointAt 前移到当前时间
	assert.Equal(t, oldBucket, usage2.BucketStart5h)
	assert.True(t, usage2.LastCheckpointAt >= now)
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
