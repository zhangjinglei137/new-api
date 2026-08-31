package controller

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// 富模型元数据响应（/v1/models OpenAI 分支，开关开启时附加在基础字段之后）
// 所有字段 omitempty：未配置/未知时不输出，保持与基础兼容输出的最小差异。

// OpenAIModelProvider provider 扩展段（当前仅 npm 生态映射）
type OpenAIModelProvider struct {
	NPM string `json:"npm,omitempty"`
}

// OpenAIModelLimit 模型限额（单位 tokens）
type OpenAIModelLimit struct {
	Context int `json:"context,omitempty"`
	Output  int `json:"output,omitempty"`
}

// OpenAIModelModalities 输入/输出模态
type OpenAIModelModalities struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

// OpenAIModelCapabilityGroup 能力分组：一组模态/限额/推理选项。
// 顶层 modalities/limit/reasoning_options 是 groups[0] 的投影，groups 按存储顺序完整输出。
type OpenAIModelCapabilityGroup struct {
	Name             string                  `json:"name"`
	Modalities       *OpenAIModelModalities  `json:"modalities,omitempty"`
	Limits           *OpenAIModelLimit       `json:"limits,omitempty"`
	ReasoningOptions []map[string]any        `json:"reasoning_options,omitempty"`
}

// OpenAIModelCost 公开定价（单位 USD/1M tokens）。数值来自 tiered_expr 精确
// 系数或 price/ratio 换算估算，配合 cost_source 字段说明来源。
// 使用指针类型：nil 表示未推导出该维度（省略），显式 0 表示免费，避免
// omitempty 把合法的零价格吞掉。
type OpenAIModelCost struct {
	Input      *float64 `json:"input,omitempty"`
	Output     *float64 `json:"output,omitempty"`
	CacheRead  *float64 `json:"cache_read,omitempty"`
	CacheWrite *float64 `json:"cache_write,omitempty"`
}

// CostSource 枚举
const (
	CostSourceExact     = "exact"     // tiered_expr 表达式的真实 $/1M 系数
	CostSourceEstimated = "estimated" // 由 price/ratio 换算的估算值
	CostSourceUnknown   = "unknown"   // 无法推导，cost 省略
)

// OpenAIModelsExtended 富模型对象：嵌入基础 OpenAI 模型对象（内联展开），
// 追加扩展元数据。定义在根模块，避免污染 relaykit 的独立 DTO。
type OpenAIModelsExtended struct {
	dto.OpenAIModels
	Name             string                       `json:"name,omitempty"`
	Description      string                       `json:"description,omitempty"`
	Provider         *OpenAIModelProvider         `json:"provider,omitempty"`
	Attachment       *bool                        `json:"attachment,omitempty"`
	Reasoning        *bool                        `json:"reasoning,omitempty"`
	ReasoningOptions []map[string]any             `json:"reasoning_options,omitempty"`
	ToolCall         *bool                        `json:"tool_call,omitempty"`
	StructuredOutput *bool                        `json:"structured_output,omitempty"`
	Temperature      *bool                        `json:"temperature,omitempty"`
	OpenWeights      *bool                        `json:"open_weights,omitempty"`
	ReleaseDate      string                       `json:"release_date,omitempty"`
	LastUpdated      string                       `json:"last_updated,omitempty"`
	Modalities       *OpenAIModelModalities       `json:"modalities,omitempty"`
	Limit            *OpenAIModelLimit            `json:"limit,omitempty"`
	Cost             *OpenAIModelCost             `json:"cost,omitempty"`
	CostSource       string                       `json:"cost_source,omitempty"`
	Groups           []OpenAIModelCapabilityGroup `json:"groups,omitempty"`
}

// modelCapabilities 是 model.Model.Capabilities 的 JSON 结构。支持双格式：
//   - 新格式：{"groups":[{"name":"chat","modalities":{...},"limits":{...},"reasoning_options":[...]}]}
//   - 旧格式：{"modalities":{...},"limits":{...},"reasoning_options":[...]}
//
// parseModelCapabilities 会把旧格式归一化为单组（name=chat），内存转换、不回写 DB。
// 任何子结构缺失/解析失败时对应响应字段省略，不报错。
type modelCapabilities struct {
	Groups           []OpenAIModelCapabilityGroup `json:"groups"`
	Modalities       *OpenAIModelModalities       `json:"modalities"`
	Limits           *OpenAIModelLimit            `json:"limits"`
	ReasoningOptions []map[string]any             `json:"reasoning_options"`
}

// costDerivation 返回推导出的公开定价与来源标记。
func deriveOpenAIModelCost(modelName string) (*OpenAIModelCost, string) {
	// 1) tiered_expr：表达式系数即真实 $/1M 价格，用 1M token 向量求值提取。
	//    含三元条件（? 档位切换）或请求规则（|||）时只能得到默认档，标记为估算。
	if billing_setting.GetBillingMode(modelName) == billing_setting.BillingModeTieredExpr {
		expr, ok := billing_setting.GetBillingExpr(modelName)
		if ok && strings.TrimSpace(expr) != "" {
			cost, source := deriveCostFromExpr(expr)
			if cost != nil {
				return cost, source
			}
		}
	}

	// 2) 按次计费（per-request price）：是固定请求价而非 token 价，
	//    无法可靠换算为 input/output USD/1M，返回 unknown（不输出 cost）。
	if _, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		return nil, CostSourceUnknown
	}

	// 3) 倍率模式：ratio 是配额倍率，$1 = QuotaPerUnit 配额，
	//    1M token 按倍率 r 扣 1M*r 配额 = $ (1M*r/QuotaPerUnit)。
	//    QuotaPerUnit = 500000（见 common），即 1M 输入 ≈ $2r。
	ratio, success, _ := ratio_setting.GetModelRatio(modelName)
	// 未配置倍率时 GetModelRatio 返回回退值 37.5（success=SelfUseModeEnabled）。
	// 非自用模式下该回退值是猜测值，不应展示为估算价格。
	if !success && !operation_setting.SelfUseModeEnabled {
		return nil, CostSourceUnknown
	}
	completion := ratio_setting.GetCompletionRatio(modelName)
	inputTokenPrice := ratio * 2
	outputTokenPrice := inputTokenPrice * completion
	cacheRead := inputTokenPrice
	if cr, ok := ratio_setting.GetCacheRatio(modelName); ok {
		cacheRead = inputTokenPrice * cr
	}
	cacheWrite := inputTokenPrice * 1.25
	if cc, ok := ratio_setting.GetCreateCacheRatio(modelName); ok {
		cacheWrite = inputTokenPrice * cc
	}
	if !allFiniteNonNegative(inputTokenPrice, outputTokenPrice, cacheRead, cacheWrite) {
		return nil, CostSourceUnknown
	}
	return &OpenAIModelCost{
		Input:      floatPtr(roundCost(inputTokenPrice)),
		Output:     floatPtr(roundCost(outputTokenPrice)),
		CacheRead:  floatPtr(roundCost(cacheRead)),
		CacheWrite: floatPtr(roundCost(cacheWrite)),
	}, CostSourceEstimated
}

// deriveCostFromExpr 用 billingexpr 在 1M token 向量下求值表达式，提取
// input/output 系数。表达式系数是 $/1M tokens；含档位/请求规则时为默认档估算。
func deriveCostFromExpr(expr string) (*OpenAIModelCost, string) {
	usedVars := billingexpr.UsedVars(expr)
	// 1M tokens 输入（无输出）
	input, _, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{
		P:   1_000_000,
		C:   0,
		Len: 1_000_000,
	})
	if err != nil {
		return nil, CostSourceUnknown
	}
	// 1M tokens 输出（无输入）
	output, _, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{
		P:   0,
		C:   1_000_000,
		Len: 1_000_000,
	})
	if err != nil {
		return nil, CostSourceUnknown
	}
	if !allFiniteNonNegative(input, output) {
		return nil, CostSourceUnknown
	}
	// 表达式结果 = 系数 × 1M token，除以 1M 还原为 $/1M 单价
	cost := &OpenAIModelCost{
		Input:  floatPtr(roundCost(input / 1_000_000)),
		Output: floatPtr(roundCost(output / 1_000_000)),
	}
	// 缓存读取系数：仅当表达式引用 cr 时单独计价；未引用时缓存 token 按
	// 输入价计费（与倍率分支语义一致）。
	if usedVars["cr"] {
		if crCost, _, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{
			P:   0,
			C:   0,
			CR:  1_000_000,
			Len: 1_000_000,
		}); err == nil && allFiniteNonNegative(crCost) {
			cost.CacheRead = floatPtr(roundCost(crCost / 1_000_000))
		}
	} else {
		cost.CacheRead = cost.Input
	}
	// 缓存写入系数：表达式引用 cc 或 cc1h 时单独计价（cc 优先）。
	if usedVars["cc"] {
		if ccCost, _, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{
			P:   0,
			C:   0,
			CC:  1_000_000,
			Len: 1_000_000,
		}); err == nil && allFiniteNonNegative(ccCost) {
			cost.CacheWrite = floatPtr(roundCost(ccCost / 1_000_000))
		}
	} else if usedVars["cc1h"] {
		if cc1hCost, _, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{
			P:     0,
			C:     0,
			CC1h:  1_000_000,
			Len:   1_000_000,
		}); err == nil && allFiniteNonNegative(cc1hCost) {
			cost.CacheWrite = floatPtr(roundCost(cc1hCost / 1_000_000))
		}
	}
	source := CostSourceExact
	// 表达式含档位切换（?）或请求规则（|||）时，1M 向量只代表其中一档
	if strings.Contains(expr, "?") || strings.Contains(expr, "|||") {
		source = CostSourceEstimated
	}
	return cost, source
}

// maxDisplayCostBound 是展示价格的上界（$/1M tokens）。超过该值的输入视为
// 病态配置（如 ratio 被误写为 1e999 解析出的 +Inf 或超常大数），拒绝输出，
// 防止 /v1/models 暴露荒谬或溢出的价格。
const maxDisplayCostBound = 1e9

func allFiniteNonNegative(values ...float64) bool {
	for _, v := range values {
		// 严格拒绝 NaN / ±Inf / 负值 / 超出展示合理范围的值
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > maxDisplayCostBound {
			return false
		}
	}
	return true
}

func roundCost(v float64) float64 {
	// 保留 4 位小数展示，避免浮点噪音；使用 math.Round 避免 int64 溢出
	return math.Round(v*10000) / 10000
}

func floatPtr(v float64) *float64 { return &v }

// parseModelCapabilities 解析 model.Model.Capabilities JSON，双格式归一化：
//   - 有 groups（非空数组）→ 直接用，顶层投影取 groups[0]；
//   - 无 groups 但含任一旧顶层字段（modalities/limits/reasoning_options）→
//     归一化为 groups:[{name:"chat", ...出现的字段}]（内存转换，不回写 DB），
//     旧字段保留以兼容读取；
//   - 两者皆无 / 坏 JSON / 空串 → 返回 nil（各字段省略）。
func parseModelCapabilities(raw string) *modelCapabilities {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var caps modelCapabilities
	if err := common.Unmarshal([]byte(raw), &caps); err != nil {
		return nil
	}
	if len(caps.Groups) == 0 {
		if caps.Modalities == nil && caps.Limits == nil && len(caps.ReasoningOptions) == 0 {
			return nil
		}
		// 旧格式归一化为单组 chat
		caps.Groups = []OpenAIModelCapabilityGroup{{
			Name:             "chat",
			Modalities:       caps.Modalities,
			Limits:           caps.Limits,
			ReasoningOptions: caps.ReasoningOptions,
		}}
	}
	return &caps
}

// endpointNpmPriority 是 provider.npm 推导的固定遍历优先级。
// 模型 endpoints 是 JSON map，遍历无序，必须按此优先级取
// 「模型已配置且该类型 npm 非空」的第一个值。
var endpointNpmPriority = []string{
	string(constant.EndpointTypeAnthropic),
	string(constant.EndpointTypeGemini),
	string(constant.EndpointTypeOpenAIResponse),
	string(constant.EndpointTypeOpenAIResponseCompact),
	string(constant.EndpointTypeOpenAIAlphaSearch),
	string(constant.EndpointTypeOpenAI),
	string(constant.EndpointTypeOpenAIVideo),
	string(constant.EndpointTypeJinaRerank),
	string(constant.EndpointTypeImageGeneration),
	string(constant.EndpointTypeEmbeddings),
}

// deriveProviderNpm 从模型 endpoints 配置推导 npm 包名（确定性）：
// npm 映射来自 common 的端点定义配置层（非内置硬编码），无命中返回空串。
func deriveProviderNpm(meta *model.Model) string {
	if strings.TrimSpace(meta.Endpoints) == "" {
		return ""
	}
	var raw map[string]any
	if err := common.Unmarshal([]byte(meta.Endpoints), &raw); err != nil {
		return ""
	}
	for _, et := range endpointNpmPriority {
		if _, ok := raw[et]; !ok {
			continue
		}
		if npm := common.GetEndpointNPM(et); npm != "" {
			return npm
		}
	}
	return ""
}

// buildRichOpenAIModel 把基础 OpenAI 模型对象与数据库元数据合并为富对象。
// meta 为 nil 时仅返回基础字段（富字段省略）。
func buildRichOpenAIModel(base dto.OpenAIModels, meta *model.Model) OpenAIModelsExtended {
	rich := OpenAIModelsExtended{OpenAIModels: base}
	if meta == nil {
		return rich
	}
	rich.Name = meta.DisplayName
	if rich.Name == "" {
		rich.Name = meta.ModelName
	}
	rich.Description = meta.Description
	if npm := deriveProviderNpm(meta); npm != "" {
		rich.Provider = &OpenAIModelProvider{NPM: npm}
	}
	rich.Attachment = meta.CapAttachment
	rich.Reasoning = meta.CapReasoning
	rich.ToolCall = meta.CapToolCall
	rich.StructuredOutput = meta.CapStructuredOutput
	rich.Temperature = meta.CapTemperature
	rich.OpenWeights = meta.OpenWeights
	rich.ReleaseDate = meta.ReleaseDate
	rich.LastUpdated = meta.LastUpdated

	// capabilities：顶层 modalities/limit/reasoning_options 是 groups[0] 的投影；
	// 无组（旧格式）时等于旧字段原值；groups[0] 缺某子结构 → 对应顶层字段省略。
	if caps := parseModelCapabilities(meta.Capabilities); caps != nil {
		rich.Groups = caps.Groups
		rich.Modalities = caps.Groups[0].Modalities
		rich.Limit = caps.Groups[0].Limits
		rich.ReasoningOptions = caps.Groups[0].ReasoningOptions
	}

	if cost, source := deriveOpenAIModelCost(meta.ModelName); cost != nil {
		rich.Cost = cost
		rich.CostSource = source
	}
	return rich
}

// modelMetadataExtendedEnabled 判断 /v1/models OpenAI 分支是否输出富元数据：
// 全局开关开启，或请求携带 ?extended=1（true 亦可）。
func modelMetadataExtendedEnabled(c interface {
	Query(string) string
}) bool {
	if operation_setting.ModelMetadataExtendedEnabled {
		return true
	}
	switch c.Query("extended") {
	case "1", "true":
		return true
	}
	return false
}