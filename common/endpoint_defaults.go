package common

import (
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/constant"
)

// EndpointInfo 描述单个端点的默认请求信息
// path: 上游路径
// method: HTTP 请求方式，例如 POST/GET
// 目前均为 POST，后续可扩展
//
// json 标签用于直接序列化到 API 输出
// 例如：{"path":"/v1/chat/completions","method":"POST"}

type EndpointInfo struct {
	Path   string `json:"path"`
	Method string `json:"method"`
}

// EndpointDefinitionsOptionKey 端点定义配置在 option 表中的键。
// 该键是「配置覆盖内置」的单一数据源：管理端 PUT 后持久化在此，
// 读取侧（GetEndpointDefinitions / GetDefaultEndpointInfo / npm 推导）统一从此消费。
const EndpointDefinitionsOptionKey = "model_setting.endpoint_definitions"

// EndpointDefinition 描述单个端点类型的完整定义。
// 端点类型集合固定（10 种，不可增删），仅可修改各类型的值。
type EndpointDefinition struct {
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	Path        string `json:"path"`
	Method      string `json:"method"`
	NPM         string `json:"npm,omitempty"`
}

// defaultEndpointDefinitions 内置 10 种端点类型的定义：path/method 沿用原有
// defaultEndpointInfoMap 的值（openai-video 无默认 path，保持空）。
// npm 仅填有据可查的两个 SDK 包，其余留空，不臆造。
var defaultEndpointDefinitions = []EndpointDefinition{
	{Type: string(constant.EndpointTypeOpenAI), DisplayName: "OpenAI Chat Completions", Path: "/v1/chat/completions", Method: "POST"},
	{Type: string(constant.EndpointTypeOpenAIResponse), DisplayName: "OpenAI Responses", Path: "/v1/responses", Method: "POST", NPM: "@ai-sdk/openai"},
	{Type: string(constant.EndpointTypeOpenAIResponseCompact), DisplayName: "OpenAI Responses Compact", Path: "/v1/responses/compact", Method: "POST"},
	{Type: string(constant.EndpointTypeOpenAIAlphaSearch), DisplayName: "OpenAI Alpha Search", Path: "/v1/alpha/search", Method: "POST"},
	{Type: string(constant.EndpointTypeAnthropic), DisplayName: "Anthropic Messages", Path: "/v1/messages", Method: "POST", NPM: "@ai-sdk/anthropic"},
	{Type: string(constant.EndpointTypeGemini), DisplayName: "Gemini Generate Content", Path: "/v1beta/models/{model}:generateContent", Method: "POST"},
	{Type: string(constant.EndpointTypeJinaRerank), DisplayName: "Jina Rerank", Path: "/v1/rerank", Method: "POST"},
	{Type: string(constant.EndpointTypeImageGeneration), DisplayName: "Image Generation", Path: "/v1/images/generations", Method: "POST"},
	{Type: string(constant.EndpointTypeEmbeddings), DisplayName: "Embeddings", Path: "/v1/embeddings", Method: "POST"},
	{Type: string(constant.EndpointTypeOpenAIVideo), DisplayName: "OpenAI Video", Path: "", Method: "POST"},
}

// defaultEndpointInfoMap 保留内置端点的默认 Path 与 Method（openai-video 无默认路径）。
var defaultEndpointInfoMap = map[constant.EndpointType]EndpointInfo{
	constant.EndpointTypeOpenAI:                {Path: "/v1/chat/completions", Method: "POST"},
	constant.EndpointTypeOpenAIResponse:        {Path: "/v1/responses", Method: "POST"},
	constant.EndpointTypeOpenAIResponseCompact: {Path: "/v1/responses/compact", Method: "POST"},
	constant.EndpointTypeOpenAIAlphaSearch:     {Path: "/v1/alpha/search", Method: "POST"},
	constant.EndpointTypeAnthropic:             {Path: "/v1/messages", Method: "POST"},
	constant.EndpointTypeGemini:                {Path: "/v1beta/models/{model}:generateContent", Method: "POST"},
	constant.EndpointTypeJinaRerank:            {Path: "/v1/rerank", Method: "POST"},
	constant.EndpointTypeImageGeneration:       {Path: "/v1/images/generations", Method: "POST"},
	constant.EndpointTypeEmbeddings:            {Path: "/v1/embeddings", Method: "POST"},
}

// endpointDefsOverride 缓存 option 表里的配置覆盖值；nil 表示未配置（回退内置）。
var (
	endpointDefsMu       sync.RWMutex
	endpointDefsOverride []EndpointDefinition
)

// mergedEndpointDefinitions 合并配置覆盖与内置默认，按内置顺序返回。
// 覆盖里缺失的类型用内置定义补齐。
func mergedEndpointDefinitions() []EndpointDefinition {
	endpointDefsMu.RLock()
	defer endpointDefsMu.RUnlock()
	if len(endpointDefsOverride) == 0 {
		return defaultEndpointDefinitions
	}
	byType := make(map[string]EndpointDefinition, len(endpointDefsOverride))
	for _, d := range endpointDefsOverride {
		byType[d.Type] = d
	}
	out := make([]EndpointDefinition, 0, len(defaultEndpointDefinitions))
	for _, def := range defaultEndpointDefinitions {
		if ov, ok := byType[def.Type]; ok {
			out = append(out, ov)
			continue
		}
		out = append(out, def)
	}
	return out
}

// GetEndpointDefinitions 返回合并配置覆盖与内置默认的完整端点定义列表
// （10 条，按内置顺序）。返回副本，调用方修改不影响缓存。
func GetEndpointDefinitions() []EndpointDefinition {
	src := mergedEndpointDefinitions()
	out := make([]EndpointDefinition, len(src))
	copy(out, src)
	return out
}

// GetEndpointNPM 返回端点类型在当前生效定义中的 npm 包名；未配置返回空串。
func GetEndpointNPM(et string) string {
	for _, d := range mergedEndpointDefinitions() {
		if d.Type == et {
			return d.NPM
		}
	}
	return ""
}

// GetDefaultEndpointInfo 返回指定端点类型的默认信息以及是否存在。
// 配置覆盖优先于内置默认；某类型路径为空（含内置 openai-video）时仍返回
// (info, false)，保持既有 ok=false 语义，消费方逐分支等价、零改动。
func GetDefaultEndpointInfo(et constant.EndpointType) (EndpointInfo, bool) {
	for _, d := range mergedEndpointDefinitions() {
		if d.Type != string(et) {
			continue
		}
		if d.Path == "" {
			return EndpointInfo{Path: d.Path, Method: d.Method}, false
		}
		return EndpointInfo{Path: d.Path, Method: d.Method}, true
	}
	info, ok := defaultEndpointInfoMap[et]
	return info, ok
}

// ValidateEndpointDefinitions 校验端点定义列表（option 值与管理端 PUT 共用）：
// 每个 type 必须属于内置 10 种且不重复、display_name 非空、非 openai-video
// 的 path 非空、method ∈ GET/POST/PUT/DELETE/PATCH、npm 为字符串（由 JSON 类型
// 解析保证）。不做词表类校验。
func ValidateEndpointDefinitions(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("端点定义不能为空")
	}
	var defs []EndpointDefinition
	if err := Unmarshal([]byte(raw), &defs); err != nil {
		return fmt.Errorf("端点定义 JSON 解析失败: %w", err)
	}
	validTypes := make(map[string]struct{}, len(defaultEndpointDefinitions))
	for _, d := range defaultEndpointDefinitions {
		validTypes[d.Type] = struct{}{}
	}
	seen := make(map[string]struct{}, len(defs))
	for _, d := range defs {
		if _, ok := validTypes[d.Type]; !ok {
			return fmt.Errorf("未知的端点类型: %s", d.Type)
		}
		if _, dup := seen[d.Type]; dup {
			return fmt.Errorf("端点类型重复: %s", d.Type)
		}
		seen[d.Type] = struct{}{}
		if strings.TrimSpace(d.DisplayName) == "" {
			return fmt.Errorf("端点类型 %s 的 display_name 不能为空", d.Type)
		}
		if d.Type != string(constant.EndpointTypeOpenAIVideo) && strings.TrimSpace(d.Path) == "" {
			return fmt.Errorf("端点类型 %s 的 path 不能为空", d.Type)
		}
		switch strings.ToUpper(d.Method) {
		case "GET", "POST", "PUT", "DELETE", "PATCH":
		default:
			return fmt.Errorf("端点类型 %s 的 method 非法: %s", d.Type, d.Method)
		}
	}
	return nil
}

// SetEndpointDefinitions 用 option 值刷新配置覆盖缓存。
// 值损坏/校验失败时清空覆盖回退内置默认，并以 SysError 告警（与项目惯例一致）。
func SetEndpointDefinitions(raw string) error {
	if err := ValidateEndpointDefinitions(raw); err != nil {
		SysError("endpoint definitions option 校验失败: " + err.Error())
		endpointDefsMu.Lock()
		endpointDefsOverride = nil
		endpointDefsMu.Unlock()
		return err
	}
	var defs []EndpointDefinition
	if err := Unmarshal([]byte(raw), &defs); err != nil {
		SysError("endpoint definitions option JSON 解析失败: " + err.Error())
		endpointDefsMu.Lock()
		endpointDefsOverride = nil
		endpointDefsMu.Unlock()
		return err
	}
	endpointDefsMu.Lock()
	endpointDefsOverride = defs
	endpointDefsMu.Unlock()
	return nil
}

// ResetEndpointDefinitions 清除配置覆盖，回退内置默认（供测试清理使用）。
func ResetEndpointDefinitions() {
	endpointDefsMu.Lock()
	endpointDefsOverride = nil
	endpointDefsMu.Unlock()
}

// EndpointDefinitionsDefaultJSON 返回内置默认端点定义的 JSON 字符串，用于
// InitOptionMap 注册该 option 键的代码默认值。
func EndpointDefinitionsDefaultJSON() string {
	data, err := Marshal(defaultEndpointDefinitions)
	if err != nil {
		return "[]"
	}
	return string(data)
}
