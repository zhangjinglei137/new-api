package controller

import (
	"math"
	"sort"
	"strings"
	"sync"

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
	// Tiers 只含非 base 档（size 升序）；多档提取失败/单档时省略。
	Tiers []OpenAIModelCostTier `json:"tiers,omitempty"`
	// ContextOver200K 为按 size 升序第一个 size>200000 的档位的四价；
	// 不存在任何 size>200000 的档时省略。
	ContextOver200K *OpenAICostValues `json:"context_over_200k,omitempty"`
}

// OpenAICostValues 单个价格集合（与 OpenAIModelCost 前四字段同构），
// 用于 tier 与 context_over_200k 的嵌套输出。
type OpenAICostValues struct {
	Input      *float64 `json:"input,omitempty"`
	Output     *float64 `json:"output,omitempty"`
	CacheRead  *float64 `json:"cache_read,omitempty"`
	CacheWrite *float64 `json:"cache_write,omitempty"`
}

// OpenAIModelCostTier 一个分档的价格（OpenAICostValues 内联）与档位元信息。
type OpenAIModelCostTier struct {
	OpenAICostValues
	Tier OpenAIModelTierInfo `json:"tier"`
}

// OpenAIModelTierInfo 档位元信息：size 语义为「上下文超过 size token 时本档
// 价格生效」（size = 探测出的最小生效 len - 1）。
type OpenAIModelTierInfo struct {
	Type string `json:"type"` // 恒为 "context"
	Size int    `json:"size"`
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
	//    先尝试分级价格提取（多档成功 → base 档价 + tiers）；
	//    任何回退情形（单档/无 tier()/探测失败/非法）落回单档提取，保证
	//    与改造前行为逐字段一致。
	if billing_setting.GetBillingMode(modelName) == billing_setting.BillingModeTieredExpr {
		expr, ok := billing_setting.GetBillingExpr(modelName)
		if ok && strings.TrimSpace(expr) != "" {
			if cost, source, ok := deriveTieredCostFromExpr(expr); ok {
				return cost, source
			}
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
	// 1M token 向量（Len=1M）下提取单档价格；与 tiered 路径共用同一提取器，
	// 保证两种形态的数值语义逐字段一致。
	vals, _, ok := extractTierValues(expr, usedVars, 1_000_000)
	if !ok {
		return nil, CostSourceUnknown
	}
	cost := &OpenAIModelCost{
		Input:      vals.Input,
		Output:     vals.Output,
		CacheRead:  vals.CacheRead,
		CacheWrite: vals.CacheWrite,
	}
	source := CostSourceExact
	// 表达式含档位切换（?）或请求规则时，1M 向量只代表其中一档。
	// 请求规则的存储形态是 (cond ? multiplier : 1) 三元（编译期被
	// requestRulePatcher 插桩），探测其 RequestRules 数量即可确定。
	if strings.Contains(expr, "?") || hasRequestRuleTraces(expr) {
		source = CostSourceEstimated
	}
	return cost, source
}

// hasRequestRuleTraces 探测表达式是否含请求规则（形如
// (param/header 条件 ? 乘数 : 1) 的三元，编译期被插桩为 RequestRules）。
// 空 RequestInput 下乘数为 1，不影响数值；仅用于 cost_source 判定。
func hasRequestRuleTraces(expr string) bool {
	_, trace, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{
		P:   1_000_000,
		C:   0,
		Len: 1_000_000,
	})
	return err == nil && len(trace.RequestRules) > 0
}

// extractTierValues 在指定 evalLen 下用单位向量组求值表达式，提取一个价格
// 集合（$/1M）：P:1M（input）、C:1M（output），并按 UsedVars 决定是否单独
// 计价 cr/cc/cc1h。返回 matchedTier（input run 命中的档名）与 ok。
// 语义与原有 deriveCostFromExpr 完全一致：
//   - input/output 的 raw 值任一非法/运行出错 → ok=false（整体失败）；
//   - 未引用 cr 时 cache_read = input；引用 cc 或 cc1h 且成功时单独计价，
//     否则该字段省略（nil）。
func extractTierValues(expr string, usedVars map[string]bool, evalLen float64) (*OpenAICostValues, string, bool) {
	input, trace, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{
		P:   1_000_000,
		C:   0,
		Len: evalLen,
	})
	if err != nil {
		return nil, "", false
	}
	output, _, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{
		P:   0,
		C:   1_000_000,
		Len: evalLen,
	})
	if err != nil {
		return nil, "", false
	}
	if !allFiniteNonNegative(input, output) {
		return nil, "", false
	}
	vals := &OpenAICostValues{
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
			Len: evalLen,
		}); err == nil && allFiniteNonNegative(crCost) {
			vals.CacheRead = floatPtr(roundCost(crCost / 1_000_000))
		}
	} else {
		vals.CacheRead = vals.Input
	}
	// 缓存写入系数：表达式引用 cc 或 cc1h 时单独计价（cc 优先）。
	if usedVars["cc"] {
		if ccCost, _, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{
			P:   0,
			C:   0,
			CC:  1_000_000,
			Len: evalLen,
		}); err == nil && allFiniteNonNegative(ccCost) {
			vals.CacheWrite = floatPtr(roundCost(ccCost / 1_000_000))
		}
	} else if usedVars["cc1h"] {
		if cc1hCost, _, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{
			P:     0,
			C:     0,
			CC1h:  1_000_000,
			Len:   evalLen,
		}); err == nil && allFiniteNonNegative(cc1hCost) {
			vals.CacheWrite = floatPtr(roundCost(cc1hCost / 1_000_000))
		}
	}
	return vals, trace.MatchedTier, true
}

// 分档价格探测常量
const (
	// tierProbeCap 是档位探测的 len 上限（2^24）。超过该 len 的阈值无法
	// 探测到，视为单档回退。
	tierProbeCap = 1 << 24
	// maxTierCount 是档位（非 base 档）总数上限；超过则整体回退。
	maxTierCount = 16
	// tierOver200K 是 context_over_200k 的 size 阈值（严格大于）。
	tierOver200K = 200000
)

// tierTransition 记录一个档位切换点：len 达到 S 时档位切换到 name。
type tierTransition struct {
	S    int    // 该档最小生效 len；size = S - 1
	name string // 该档名
}

// tieredCostCacheEntry 缓存一次提取结果（含回退结论 ok=false）。
type tieredCostCacheEntry struct {
	cost   *OpenAIModelCost
	source string
	ok     bool
}

// tieredCostCache 以 billingexpr.ExprHashString(expr) 为 key 缓存提取结果。
var (
	tieredCostCacheMu sync.RWMutex
	tieredCostCache  = make(map[string]tieredCostCacheEntry)
)

// resetTieredCostCache 清空分档价格缓存（供测试使用）。
func resetTieredCostCache() {
	tieredCostCacheMu.Lock()
	tieredCostCache = make(map[string]tieredCostCacheEntry)
	tieredCostCacheMu.Unlock()
}

// deriveTieredCostFromExpr 探测表达式中的档位结构并提取每档价格。
// 返回 (cost, source, true) 表示多档提取成功（顶层 cost = base 档价格，
// cost.Tiers 含非 base 档、size 升序）；否则返回 (nil, "", false)，
// 调用方回退到 deriveCostFromExpr（与改造前行为逐字段一致）。
func deriveTieredCostFromExpr(expr string) (*OpenAIModelCost, string, bool) {
	hash := billingexpr.ExprHashString(expr)

	tieredCostCacheMu.RLock()
	if e, ok := tieredCostCache[hash]; ok {
		tieredCostCacheMu.RUnlock()
		return e.cost, e.source, e.ok
	}
	tieredCostCacheMu.RUnlock()

	cost, source, ok := deriveTieredCostFromExprUncached(expr)

	tieredCostCacheMu.Lock()
	if len(tieredCostCache) >= 256 {
		// 容量上限：满则整体重置
		tieredCostCache = make(map[string]tieredCostCacheEntry)
	}
	tieredCostCache[hash] = tieredCostCacheEntry{cost: cost, source: source, ok: ok}
	tieredCostCacheMu.Unlock()
	return cost, source, ok
}

// deriveTieredCostFromExprUncached 是未缓存的实际提取逻辑。
func deriveTieredCostFromExprUncached(expr string) (*OpenAIModelCost, string, bool) {
	usedVars := billingexpr.UsedVars(expr)

	baseTierName, transitions, ok := detectTierTransitions(expr)
	if !ok || len(transitions) == 0 {
		// 探测失败或单档 → 回退（今天行为）
		return nil, "", false
	}

	// base 档（首档）价格：区间起点 0
	baseVals, matched, ok := extractTierValues(expr, usedVars, 0)
	if !ok || matched != baseTierName {
		return nil, "", false
	}

	// 各非 base 档价格：区间起点 = 该档最小生效 len S
	tiers := make([]OpenAIModelCostTier, 0, len(transitions))
	for _, tr := range transitions {
		vals, matched, ok := extractTierValues(expr, usedVars, float64(tr.S))
		if !ok || matched != tr.name {
			return nil, "", false
		}
		tiers = append(tiers, OpenAIModelCostTier{
			OpenAICostValues: *vals,
			Tier:             OpenAIModelTierInfo{Type: "context", Size: tr.S - 1},
		})
	}

	cost := &OpenAIModelCost{
		Input:      baseVals.Input,
		Output:     baseVals.Output,
		CacheRead:  baseVals.CacheRead,
		CacheWrite: baseVals.CacheWrite,
		Tiers:      tiers,
	}
	// context_over_200k：按 size 升序第一个 size>200000 的档位的四价；
	// 不存在任何 size>200000 的档则省略。
	for i := range tiers {
		if tiers[i].Tier.Size > tierOver200K {
			cost.ContextOver200K = &tiers[i].OpenAICostValues
			break
		}
	}

	source := CostSourceExact
	// 多档提取成功：请求规则存在（存储形态为被插桩的请求三元，空
	// RequestInput 乘数为 1 不影响数值）或遗留 ||| 标记 → estimated；
	// 否则 exact。
	if hasRequestRuleTraces(expr) || strings.Contains(expr, "|||") {
		source = CostSourceEstimated
	}
	return cost, source, true
}

// probeMatchedTier 在给定 len 下求值表达式，返回命中的档名。
// P 固定 1M（tier 条件按设计应基于 len；p/c 驱动隐藏条件由档内系数断言拦截）。
func probeMatchedTier(expr string, l int) (string, error) {
	_, trace, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{
		P:   1_000_000,
		C:   0,
		Len: float64(l),
	})
	if err != nil {
		return "", err
	}
	return trace.MatchedTier, nil
}

// detectTierTransitions 用几何探测序列（len = 0,1,2,4,...,2^24）找出所有
// 档位切换点。相邻探测点档位不同时，对区间做完整二分收集该区间内的全部
// 阈值（防同一倍增区间多个阈值漏档）。返回 base 档名（len=0 命中的档）、
// 全部切换点（S 升序）与是否成功。档位总数超过 maxTierCount 回退。
func detectTierTransitions(expr string) (string, []tierTransition, bool) {
	base, err := probeMatchedTier(expr, 0)
	if err != nil {
		return "", nil, false
	}
	transitions := []tierTransition{}
	prev := 0
	prevTier := base
	for next := 1; next <= tierProbeCap; next *= 2 {
		cur, err := probeMatchedTier(expr, next)
		if err != nil {
			return base, nil, false
		}
		if cur == prevTier {
			prev = next
			continue
		}
		found, ok := collectTransitions(expr, prev, prevTier, next, cur)
		if !ok {
			return base, nil, false
		}
		if len(transitions)+len(found) > maxTierCount {
			return base, nil, false
		}
		transitions = append(transitions, found...)
		prev = next
		prevTier = cur
	}
	if len(transitions) == 0 {
		return base, nil, true
	}
	// 按 S 升序（collectTransitions 内部顺序不保证）
	sort.Slice(transitions, func(i, j int) bool { return transitions[i].S < transitions[j].S })
	return base, transitions, true
}

// collectTransitions 收集区间 (lo, hi] 内的全部档位切换点，其中
// probe(lo)=loTier、probe(hi)=hiTier 且二者不同。通过反复对半拆分，
// 把区间内每一个档位变化都作为相邻两点间的切换点找出。
func collectTransitions(expr string, lo int, loTier string, hi int, hiTier string) ([]tierTransition, bool) {
	type interval struct {
		lo, hi             int
		loTier, hiTier    string
	}
	pending := []interval{{lo: lo, hi: hi, loTier: loTier, hiTier: hiTier}}
	out := []tierTransition{}
	for len(pending) > 0 {
		cur := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if cur.hi-cur.lo <= 1 {
			// 相邻两点档位不同：切换点位于 hi（该档从 hi 起生效）
			out = append(out, tierTransition{S: cur.hi, name: cur.hiTier})
			continue
		}
		mid := cur.lo + (cur.hi-cur.lo)/2
		midTier, err := probeMatchedTier(expr, mid)
		if err != nil {
			return nil, false
		}
		if midTier != cur.loTier {
			pending = append(pending, interval{lo: cur.lo, hi: mid, loTier: cur.loTier, hiTier: midTier})
		}
		if midTier != cur.hiTier {
			pending = append(pending, interval{lo: mid, hi: cur.hi, loTier: midTier, hiTier: cur.hiTier})
		}
	}
	return out, true
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