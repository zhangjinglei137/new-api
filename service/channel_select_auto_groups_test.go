package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectAutoGroupsTest(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRetryTimes := common.RetryTimes
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RetryTimes = 0

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RetryTimes = originalRetryTimes
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", originalMaxTokenAutoGroups)))

		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	return db
}

func createChannelSelectAutoGroupsChannel(t *testing.T, db *gorm.DB, id int, group, modelName string) {
	t.Helper()
	priority := int64(0)
	weight := uint(100)
	require.NoError(t, db.Create(&model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Models:   modelName,
		Group:    group,
		Priority: &priority,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func TestCacheGetRandomSatisfiedChannelUsesTokenAutoGroupsWhenGlobalAutoIsEmpty(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-runtime-model"
	createChannelSelectAutoGroupsChannel(t, db, 2101, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2102, "default", modelName)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2101, first.Id)
	assert.Equal(t, "vip", selectedGroup)
	assert.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	assert.Empty(t, setting.GetAutoGroups(), "the selection must not depend on the global Auto list")

	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2102, second.Id)
	assert.Equal(t, "default", selectedGroup)
	assert.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestAffinityAdjustedRetry(t *testing.T) {
	// 未命中亲和 → 不偏移
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r"})
	require.Equal(t, 1, AffinityAdjustedRetry(ctx, 1))
	require.Equal(t, 7, AffinityAdjustedRetry(ctx, 7))

	// 亲和命中 → retry>=1 时偏移 -1，retry=0 不偏移（0 为首轮快捷路径，不进入此分支）
	ctx2 := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r2"})
	MarkChannelAffinityUsed(ctx2, "default", 6)
	require.Equal(t, 0, AffinityAdjustedRetry(ctx2, 1))
	require.Equal(t, 6, AffinityAdjustedRetry(ctx2, 7))
	require.Equal(t, 0, AffinityAdjustedRetry(ctx2, 0))

	// nil ctx → 不偏移
	require.Equal(t, 1, AffinityAdjustedRetry(nil, 1))
}

func TestAffinityRetrySelectsTopPriorityChannel(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	// 构造渠道：priority 20（渠道 1）与 19（渠道 2），同一 model/group
	channels := []model.Channel{
		{Id: 1, Type: 1, Key: "k1", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(20)},
		{Id: 2, Type: 1, Key: "k2", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(19)},
	}
	require.NoError(t, model.DB.Create(&channels).Error)
	// InitChannelCache 用 Ability 表的 group 集合初始化候选池的二级 map，
	// 测试环境需先为该 group 建立一条 Ability 记录，否则 nil map 赋值会 panic。
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "default", Model: "m1", ChannelId: 1, Enabled: true, Priority: int64Ptr(20),
	}).Error)
	model.InitChannelCache()

	// 未命中亲和：retry=1 选中层 1 → 渠道 2（priority 19）
	param1 := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "m1", Retry: intPtr(1)}
	ch1, _, err1 := CacheGetRandomSatisfiedChannel(param1)
	require.NoError(t, err1)
	require.NotNil(t, ch1)
	require.Equal(t, 2, ch1.Id, "无亲和时 retry=1 应选中层 1 的渠道 2")

	// 亲和命中：缓存渠道 6（无实际 6 渠道影响，仅设置标志）→ retry=1 偏移为 0 → 选中最高优先级渠道 1
	affCtx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r"})
	MarkChannelAffinityUsed(affCtx, "default", 6)
	effective := AffinityAdjustedRetry(affCtx, 1)
	require.Equal(t, 0, effective, "亲和命中时 retry=1 应偏移到 0")
	// 与 controller/relay.go getChannel 一致：偏移值经 SetRetry 注入后再选择渠道
	param2 := &RetryParam{Ctx: affCtx, TokenGroup: "default", ModelName: "m1", Retry: intPtr(1)}
	param2.SetRetry(effective)
	ch2, _, err2 := CacheGetRandomSatisfiedChannel(param2)
	require.NoError(t, err2)
	require.NotNil(t, ch2)
	require.Equal(t, 1, ch2.Id, "亲和命中时重试应能选中最高优先级渠道 1")
}

func intPtr(v int) *int {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}

func TestCacheGetRandomWithPriorityKeepsCrossGroupRetryState(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	// 组 default：渠道 1(pri 20)、渠道 2(pri 19)；组 vip：渠道 3(pri 18)、渠道 4(pri 17)
	channels := []model.Channel{
		{Id: 1, Type: 1, Key: "k1", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(20)},
		{Id: 2, Type: 1, Key: "k2", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(19)},
		{Id: 3, Type: 1, Key: "k3", Models: "m1", Group: "vip", Status: common.ChannelStatusEnabled, Priority: int64Ptr(18)},
		{Id: 4, Type: 1, Key: "k4", Models: "m1", Group: "vip", Status: common.ChannelStatusEnabled, Priority: int64Ptr(17)},
	}
	require.NoError(t, model.DB.Create(&channels).Error)
	// 为两个组播种 Ability（group 集合）
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "m1", ChannelId: 1, Enabled: true, Priority: int64Ptr(20)}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "vip", Model: "m1", ChannelId: 3, Enabled: true, Priority: int64Ptr(18)}).Error)
	model.InitChannelCache()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))

	param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "m1", Retry: intPtr(1)}
	// 未命中亲和：WithPriority(param, 1) 与 CacheGetRandomSatisfiedChannel(param) 等价
	ch1, _, err1 := CacheGetRandomSatisfiedChannelWithPriority(param, 1)
	require.NoError(t, err1)
	require.NotNil(t, ch1)
	require.Equal(t, 2, ch1.Id, "未命中亲和 retry=1 应选中 default 组层 1 的渠道 2")

	// 关键断言：priorityRetry 偏移只影响层索引、不写回 param；内部跨组状态机变异
	// （SetRetry(0) + ResetRetryNextTry）仍作用在原始 param 上（复现既有行为）。
	// fixture 中 common.RetryTimes=0 且 crossGroupRetry=true，auto 命中路径
	// priorityRetry(=1) >= RetryTimes(=0) 恒成立，因此调用后 param.Retry 被状态机
	// 置 0、resetNextTry 置位——这正是 clone 方案会丢失的变异（回归根因）。
	// 对照实证：原入口 CacheGetRandomSatisfiedChannel 同一场景下事后状态完全相同
	// （GetRetry()=0, resetNextTry=true），二者等价。
	require.Equal(t, 0, param.GetRetry(), "跨组状态机 SetRetry(0) 必须作用在原始 param 上")
	require.True(t, param.resetNextTry, "ResetRetryNextTry 必须在原始 param 上置位")
}

func TestCacheGetWithPriorityAffinityOffsetKeepsStateMachine(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	channels := []model.Channel{
		{Id: 1, Type: 1, Key: "k1", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(20)},
		{Id: 2, Type: 1, Key: "k2", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(19)},
	}
	require.NoError(t, model.DB.Create(&channels).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "m1", ChannelId: 1, Enabled: true, Priority: int64Ptr(20)}).Error)
	model.InitChannelCache()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))

	// 亲和命中：retry=1 偏移为 0 → 选中 highest priority（渠道 1）
	affCtx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r"})
	MarkChannelAffinityUsed(affCtx, "default", 6)
	effective := AffinityAdjustedRetry(affCtx, 1)
	require.Equal(t, 0, effective)
	param := &RetryParam{Ctx: affCtx, TokenGroup: "default", ModelName: "m1", Retry: intPtr(1)}
	ch2, _, err2 := CacheGetRandomSatisfiedChannelWithPriority(param, effective)
	require.NoError(t, err2)
	require.NotNil(t, ch2)
	require.Equal(t, 1, ch2.Id, "亲和命中偏移后应选中最高优先级渠道 1")
	require.Equal(t, 1, param.GetRetry(), "循环计数不被偏移污染")
}
