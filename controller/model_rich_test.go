package controller

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(v bool) *bool { return &v }

// TestListModelsExtendedNoSwitchOutputStaysMinimal 验证开关关闭时 /v1/models
// OpenAI 分支输出与基础结构一致（不附加富字段），保持向后兼容。
func TestListModelsExtendedNoSwitchOutputStaysMinimal(t *testing.T) {
	withSelfUseModeEnabled(t)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Model{
		ModelName:    "rich-test-model",
		DisplayName:  "Rich Test",
		Family:       "test",
		CapReasoning: boolPtr(true),
		Capabilities: `{"limits":{"context":1000000}}`,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: "rich-test-model", ChannelId: 1, Enabled: true,
	}).Error)

	old := operation_setting.ModelMetadataExtendedEnabled
	operation_setting.ModelMetadataExtendedEnabled = false
	t.Cleanup(func() { operation_setting.ModelMetadataExtendedEnabled = old })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Request = req
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	ListModels(c, constant.ChannelTypeOpenAI)

	body := w.Body.String()
	assert.True(t, strings.Contains(body, `"id":"rich-test-model"`), "body=%s", body)
	assert.False(t, strings.Contains(body, `"family":"test"`), "family should not be present without switch, body=%s", body)
}

// TestListModelsExtendedSwitchOutputIncludesRichFields 验证开关开启时
// /v1/models OpenAI 分支输出富元数据。
func TestListModelsExtendedSwitchOutputIncludesRichFields(t *testing.T) {
	withSelfUseModeEnabled(t)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Model{
		ModelName:    "rich-test-model",
		DisplayName:  "Rich Test",
		Family:       "test",
		CapReasoning: boolPtr(true),
		ProviderNpm:  "@ai-sdk/anthropic",
		Capabilities: `{"limits":{"context":1000000},"modalities":{"input":["text"],"output":["text"]}}`,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: "rich-test-model", ChannelId: 1, Enabled: true,
	}).Error)

	old := operation_setting.ModelMetadataExtendedEnabled
	operation_setting.ModelMetadataExtendedEnabled = true
	t.Cleanup(func() { operation_setting.ModelMetadataExtendedEnabled = old })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Request = req
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	ListModels(c, constant.ChannelTypeOpenAI)

	body := w.Body.String()
	assert.True(t, strings.Contains(body, `"name":"Rich Test"`), "body=%s", body)
	assert.True(t, strings.Contains(body, `"family":"test"`), "body=%s", body)
	assert.True(t, strings.Contains(body, `"reasoning":true`), "body=%s", body)
	assert.True(t, strings.Contains(body, `"provider":{"npm":"@ai-sdk/anthropic"}`), "body=%s", body)
	assert.True(t, strings.Contains(body, `"limit":{"context":1000000}`), "body=%s", body)
}

// TestBuildRichOpenAIModelFields 验证富字段组装：三态 *bool、capabilities
// 子结构、provider、cost_source。
func TestBuildRichOpenAIModelFields(t *testing.T) {
	meta := &model.Model{
		ModelName:           "rich-test",
		DisplayName:         "Rich Test Model",
		Family:              "glm",
		ProviderNpm:         "@ai-sdk/anthropic",
		ReleaseDate:         "2026-08-26",
		LastUpdated:         "2026-08-26",
		OpenWeights:         boolPtr(false),
		CapAttachment:       boolPtr(true),
		CapReasoning:        boolPtr(true),
		CapToolCall:         boolPtr(true),
		CapStructuredOutput: boolPtr(false),
		CapTemperature:      boolPtr(false),
		Capabilities:        `{"modalities":{"input":["text","image"],"output":["text"]},"limits":{"context":1000000,"output":131072},"reasoning_options":[{"type":"effort","values":["low","high"]}]}`,
	}

	base := dto.OpenAIModels{Id: "rich-test", Object: "model", Created: 1710000000, OwnedBy: "glm"}
	rich := buildRichOpenAIModel(base, meta)

	assert.Equal(t, "Rich Test Model", rich.Name)
	assert.Equal(t, "glm", rich.Family)
	require.NotNil(t, rich.Provider)
	assert.Equal(t, "@ai-sdk/anthropic", rich.Provider.NPM)
	require.NotNil(t, rich.Attachment)
	assert.True(t, *rich.Attachment)
	require.NotNil(t, rich.StructuredOutput)
	assert.False(t, *rich.StructuredOutput)
	require.NotNil(t, rich.OpenWeights)
	assert.False(t, *rich.OpenWeights)
	assert.Equal(t, "2026-08-26", rich.ReleaseDate)

	require.NotNil(t, rich.Modalities)
	assert.Equal(t, []string{"text", "image"}, rich.Modalities.Input)
	require.NotNil(t, rich.Limit)
	assert.Equal(t, 1000000, rich.Limit.Context)
	require.Len(t, rich.ReasoningOptions, 1)
}

// TestBuildRichOpenAIModelNilMeta 验证 meta 为 nil 时富字段全部省略，仅基础字段。
func TestBuildRichOpenAIModelNilMeta(t *testing.T) {
	base := dto.OpenAIModels{Id: "m", Object: "model", Created: 1, OwnedBy: "x"}
	rich := buildRichOpenAIModel(base, nil)
	assert.Equal(t, "m", rich.Id)
	assert.Empty(t, rich.Family)
	assert.Nil(t, rich.Provider)
	assert.Nil(t, rich.Cost)
	assert.Empty(t, rich.CostSource)
}

// TestParseModelCapabilities 验证 capabilities 解析的健壮性。
func TestParseModelCapabilities(t *testing.T) {
	caps := parseModelCapabilities(`{"limits":{"context":10}}`)
	require.NotNil(t, caps)
	require.NotNil(t, caps.Limits)
	assert.Equal(t, 10, caps.Limits.Context)

	assert.Nil(t, parseModelCapabilities(""))
	assert.Nil(t, parseModelCapabilities("{not-json"))
}

// TestDeriveOpenAIModelCostFallbackRatio 验证非自用模式下未配置倍率返回
// unknown（不做猜测性展示）；自用模式下回退倍率视为有效，标 estimated。
func TestDeriveOpenAIModelCostFallbackRatio(t *testing.T) {
	original := operation_setting.SelfUseModeEnabled

	operation_setting.SelfUseModeEnabled = false
	cost, source := deriveOpenAIModelCost("some-unconfigured-model")
	assert.Nil(t, cost, "non-self-use mode must not surface a guessed price")
	assert.Equal(t, CostSourceUnknown, source)

	operation_setting.SelfUseModeEnabled = true
	cost, source = deriveOpenAIModelCost("some-unconfigured-model")
	require.NotNil(t, cost)
	assert.Equal(t, CostSourceEstimated, source)
	require.NotNil(t, cost.Input)
	require.NotNil(t, cost.Output)
	assert.True(t, *cost.Input > 0)
	assert.True(t, *cost.Output > 0)

	operation_setting.SelfUseModeEnabled = original
}

// TestModelMetadataExtendedEnabledQuery 验证请求级 ?extended=1 覆盖开关关闭。
// TestAllFiniteNonNegative 验证数值守卫对 NaN / ±Inf / 负值 的拒绝。
func TestAllFiniteNonNegative(t *testing.T) {
	nan := math.NaN()
	posInf := math.Inf(1)
	negInf := math.Inf(-1)

	assert.False(t, allFiniteNonNegative(nan), "NaN must be rejected")
	assert.False(t, allFiniteNonNegative(posInf), "+Inf must be rejected")
	assert.False(t, allFiniteNonNegative(negInf), "-Inf must be rejected")
	assert.False(t, allFiniteNonNegative(-1), "negative must be rejected")
	assert.True(t, allFiniteNonNegative(0, 1.5, 1e6), "finite non-negative within bound must pass")
	assert.False(t, allFiniteNonNegative(1e300), "absurdly large finite value should be rejected by display bound")
	assert.False(t, allFiniteNonNegative(maxDisplayCostBound+1), "value above display bound must be rejected")
}

// TestRoundCost 验证 roundCost 对极大值的稳定性（不应溢出为负数）。
func TestRoundCost(t *testing.T) {
	// 极大有限值：math.Round 不应产生 int64 溢出导致的负数
	assert.GreaterOrEqual(t, roundCost(1e300), 0.0)
	assert.GreaterOrEqual(t, roundCost(1e308), 0.0)
	assert.InDelta(t, 1.2346, roundCost(1.23456), 0.0001)
	assert.InDelta(t, 0.0, roundCost(0), 0.0001)
}

// TestDeriveCostFromExprBasic 验证 tiered_expr 系数还原：p*2.5 + c*15。
func TestDeriveCostFromExprBasic(t *testing.T) {
	cost, source := deriveCostFromExpr(`tier("base", p * 2.5 + c * 15)`)
	require.NotNil(t, cost)
	assert.Equal(t, CostSourceExact, source)
	require.NotNil(t, cost.Input)
	require.NotNil(t, cost.Output)
	assert.InDelta(t, 2.5, *cost.Input, 0.001)
	assert.InDelta(t, 15.0, *cost.Output, 0.001)
}

// TestDeriveCostFromExprWithCache 验证引用 cr/cc 时缓存成本被单独推导。
func TestDeriveCostFromExprWithCache(t *testing.T) {
	cost, source := deriveCostFromExpr(`tier("base", p * 3 + c * 15 + cr * 0.3 + cc * 3.75)`)
	require.NotNil(t, cost)
	assert.Equal(t, CostSourceExact, source)
	require.NotNil(t, cost.CacheRead)
	require.NotNil(t, cost.CacheWrite)
	assert.InDelta(t, 0.3, *cost.CacheRead, 0.001)
	assert.InDelta(t, 3.75, *cost.CacheWrite, 0.001)
}

// TestDeriveCostFromExprTieredEstimated 验证含档位表达式标为 estimated。
func TestDeriveCostFromExprTieredEstimated(t *testing.T) {
	_, source := deriveCostFromExpr(`len <= 200000 ? tier("a", p * 3 + c * 15) : tier("b", p * 6 + c * 22.5)`)
	assert.Equal(t, CostSourceEstimated, source)
}

// TestDeriveOpenAIModelCostPerRequestUnknown 验证按次计费模型 cost 为 unknown。
func TestDeriveOpenAIModelCostPerRequestUnknown(t *testing.T) {
	original := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		_ = ratio_setting.UpdateModelPriceByJSONString(original)
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"zz-per-request": 1.5}`))
	cost, source := deriveOpenAIModelCost("zz-per-request")
	assert.Nil(t, cost)
	assert.Equal(t, CostSourceUnknown, source)
}

func TestModelMetadataExtendedEnabledQuery(t *testing.T) {
	old := operation_setting.ModelMetadataExtendedEnabled
	operation_setting.ModelMetadataExtendedEnabled = false
	t.Cleanup(func() { operation_setting.ModelMetadataExtendedEnabled = old })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req, _ := http.NewRequest(http.MethodGet, "/v1/models?extended=1", nil)
	c.Request = req
	assert.True(t, modelMetadataExtendedEnabled(c))

	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	req2, _ := http.NewRequest(http.MethodGet, "/v1/models", nil)
	c2.Request = req2
	assert.False(t, modelMetadataExtendedEnabled(c2))
}
