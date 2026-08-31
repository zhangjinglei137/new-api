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
		CapReasoning: boolPtr(true),
		Endpoints:    `{"anthropic":{"path":"/v1/messages","method":"POST"}}`,
		Capabilities: `{"limits":{"context":1000000},"modalities":{"input":["text"],"output":["text"]}}`,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: "rich-test-model", ChannelId: 1, Enabled: true,
	}).Error)

	old := operation_setting.ModelMetadataExtendedEnabled
	operation_setting.ModelMetadataExtendedEnabled = true
	t.Cleanup(func() { operation_setting.ModelMetadataExtendedEnabled = old })
	t.Cleanup(common.ResetEndpointDefinitions)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Request = req
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	ListModels(c, constant.ChannelTypeOpenAI)

	body := w.Body.String()
	assert.True(t, strings.Contains(body, `"name":"Rich Test"`), "body=%s", body)
	assert.True(t, strings.Contains(body, `"reasoning":true`), "body=%s", body)
	assert.True(t, strings.Contains(body, `"provider":{"npm":"@ai-sdk/anthropic"}`), "body=%s", body)
	assert.True(t, strings.Contains(body, `"limit":{"context":1000000}`), "body=%s", body)
	assert.False(t, strings.Contains(body, `"family"`), "family must not be output, body=%s", body)
}

// TestBuildRichOpenAIModelFields 验证富字段组装：三态 *bool、capabilities
// 子结构、provider（由 endpoints 推导）、cost_source。
func TestBuildRichOpenAIModelFields(t *testing.T) {
	t.Cleanup(common.ResetEndpointDefinitions)
	meta := &model.Model{
		ModelName:           "rich-test",
		DisplayName:         "Rich Test Model",
		Endpoints:           `{"anthropic":{"path":"/v1/messages","method":"POST"}}`,
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
	require.Len(t, rich.Groups, 1)
	assert.Equal(t, "chat", rich.Groups[0].Name)
}

// TestBuildRichOpenAIModelNilMeta 验证 meta 为 nil 时富字段全部省略，仅基础字段。
func TestBuildRichOpenAIModelNilMeta(t *testing.T) {
	base := dto.OpenAIModels{Id: "m", Object: "model", Created: 1, OwnedBy: "x"}
	rich := buildRichOpenAIModel(base, nil)
	assert.Equal(t, "m", rich.Id)
	assert.Nil(t, rich.Provider)
	assert.Nil(t, rich.Cost)
	assert.Empty(t, rich.CostSource)
	assert.Nil(t, rich.Groups)
}

// TestParseModelCapabilities 验证 capabilities 解析的健壮性与双格式归一化。
func TestParseModelCapabilities(t *testing.T) {
	// 旧格式：无 groups 但含旧顶层字段 → 归一化为单组 chat，旧字段保留
	caps := parseModelCapabilities(`{"limits":{"context":10}}`)
	require.NotNil(t, caps)
	require.NotNil(t, caps.Limits)
	assert.Equal(t, 10, caps.Limits.Context)
	require.Len(t, caps.Groups, 1)
	assert.Equal(t, "chat", caps.Groups[0].Name)
	require.NotNil(t, caps.Groups[0].Limits)
	assert.Equal(t, 10, caps.Groups[0].Limits.Context)

	// 新格式：有 groups 直接用
	caps = parseModelCapabilities(`{"groups":[{"name":"reasoning","limits":{"context":20},"modalities":{"input":["text"],"output":["text"]}}]}`)
	require.NotNil(t, caps)
	require.Len(t, caps.Groups, 1)
	assert.Equal(t, "reasoning", caps.Groups[0].Name)
	require.NotNil(t, caps.Groups[0].Limits)
	assert.Equal(t, 20, caps.Groups[0].Limits.Context)

	// 空串 / 坏 JSON / 空对象 / 空 groups 数组 → nil
	assert.Nil(t, parseModelCapabilities(""))
	assert.Nil(t, parseModelCapabilities("{not-json"))
	assert.Nil(t, parseModelCapabilities(`{}`))
	assert.Nil(t, parseModelCapabilities(`{"groups":[]}`))
}

// TestBuildRichOpenAIModelCapabilitiesGolden 表驱动 golden 测试：capabilities
// 双格式归一化在 buildRichOpenAIModel 的输出表现（顶层投影 + groups 全量）。
func TestBuildRichOpenAIModelCapabilitiesGolden(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		// 顶层投影期望（nil 表示对应字段省略）
		wantModalities *OpenAIModelModalities
		wantLimit      *OpenAIModelLimit
		wantReasoning  bool
		wantGroups     []string // groups 的 name 顺序
	}{
		{
			name:           "legacy single group",
			raw:            `{"modalities":{"input":["text","image"],"output":["text"]},"limits":{"context":1000000,"output":131072},"reasoning_options":[{"type":"effort","values":["low","high"]}]}`,
			wantModalities: &OpenAIModelModalities{Input: []string{"text", "image"}, Output: []string{"text"}},
			wantLimit:      &OpenAIModelLimit{Context: 1000000, Output: 131072},
			wantReasoning:  true,
			wantGroups:     []string{"chat"},
		},
		{
			name:           "new single group",
			raw:            `{"groups":[{"name":"chat","modalities":{"input":["text"],"output":["text"]},"limits":{"context":1000000},"reasoning_options":[{"type":"effort","values":["low","high"]}]}]}`,
			wantModalities: &OpenAIModelModalities{Input: []string{"text"}, Output: []string{"text"}},
			wantLimit:      &OpenAIModelLimit{Context: 1000000},
			wantReasoning:  true,
			wantGroups:     []string{"chat"},
		},
		{
			name: "new multiple groups projection from first",
			raw:  `{"groups":[{"name":"chat","limits":{"context":1000000}},{"name":"reasoning","limits":{"context":200000},"modalities":{"input":["text"],"output":["text"]}}]}`,
			// 顶层 = groups[0] 投影：仅 limit
			wantLimit:  &OpenAIModelLimit{Context: 1000000},
			wantGroups: []string{"chat", "reasoning"},
		},
		{
			name:       "name only group",
			raw:        `{"groups":[{"name":"chat"}]}`,
			wantGroups: []string{"chat"},
		},
		{
			name: "empty groups",
			raw:  `{"groups":[]}`,
		},
		{
			name: "bad json",
			raw:  `{not-json`,
		},
		{
			name: "empty string",
			raw:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta := &model.Model{
				ModelName:    "golden-cap",
				DisplayName:  "Golden Cap",
				Capabilities: tc.raw,
			}
			rich := buildRichOpenAIModel(dto.OpenAIModels{Id: "golden-cap"}, meta)

			if tc.wantModalities == nil {
				assert.Nil(t, rich.Modalities, "modalities should be omitted")
			} else {
				require.NotNil(t, rich.Modalities)
				assert.Equal(t, tc.wantModalities.Input, rich.Modalities.Input)
				assert.Equal(t, tc.wantModalities.Output, rich.Modalities.Output)
			}
			if tc.wantLimit == nil {
				assert.Nil(t, rich.Limit, "limit should be omitted")
			} else {
				require.NotNil(t, rich.Limit)
				assert.Equal(t, tc.wantLimit.Context, rich.Limit.Context)
				assert.Equal(t, tc.wantLimit.Output, rich.Limit.Output)
			}
			if !tc.wantReasoning {
				assert.Nil(t, rich.ReasoningOptions, "reasoning_options should be omitted")
			} else {
				require.Len(t, rich.ReasoningOptions, 1)
			}
			var names []string
			for _, g := range rich.Groups {
				names = append(names, g.Name)
			}
			assert.Equal(t, tc.wantGroups, names, "groups should be output in stored order")
		})
	}
}

// TestBuildRichOpenAIModelBoolFieldsOnlyNoCaps 验证仅布尔字段、无 capabilities
// 时输出布尔字段且不输出任何能力结构。
func TestBuildRichOpenAIModelBoolFieldsOnlyNoCaps(t *testing.T) {
	meta := &model.Model{
		ModelName:     "bool-only",
		CapAttachment: boolPtr(true),
		CapReasoning:  boolPtr(false),
	}
	rich := buildRichOpenAIModel(dto.OpenAIModels{Id: "bool-only"}, meta)
	require.NotNil(t, rich.Attachment)
	assert.True(t, *rich.Attachment)
	require.NotNil(t, rich.Reasoning)
	assert.False(t, *rich.Reasoning)
	assert.Nil(t, rich.Modalities)
	assert.Nil(t, rich.Limit)
	assert.Nil(t, rich.ReasoningOptions)
	assert.Nil(t, rich.Groups)
}

// TestDeriveProviderNpmPriority 验证 provider.npm 推导的确定性优先级：
// 固定优先级取「模型已配置且该类型 npm 非空」的第一个值。
func TestDeriveProviderNpmPriority(t *testing.T) {
	t.Cleanup(common.ResetEndpointDefinitions)
	cases := []struct {
		name      string
		endpoints string
		want      string
	}{
		{
			name:      "anthropic wins over openai",
			endpoints: `{"openai":{"path":"/v1/chat/completions","method":"POST"},"anthropic":{"path":"/v1/messages","method":"POST"}}`,
			want:      "@ai-sdk/anthropic",
		},
		{
			name:      "openai-response npm",
			endpoints: `{"openai-response":{"path":"/v1/responses","method":"POST"}}`,
			want:      "@ai-sdk/openai",
		},
		{
			name:      "openai only no npm",
			endpoints: `{"openai":{"path":"/v1/chat/completions","method":"POST"}}`,
			want:      "",
		},
		{
			name:      "empty endpoints",
			endpoints: ``,
			want:      "",
		},
		{
			name:      "bad json",
			endpoints: `{not-json`,
			want:      "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta := &model.Model{ModelName: "npm-test", Endpoints: tc.endpoints}
			assert.Equal(t, tc.want, deriveProviderNpm(meta))
		})
	}
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
