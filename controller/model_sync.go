package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// 上游地址
const (
	upstreamModelsURL  = "https://basellm.github.io/llm-metadata/api/newapi/models.json"
	upstreamVendorsURL = "https://basellm.github.io/llm-metadata/api/newapi/vendors.json"
	// opencode-go 官方仓库（llm-metadata）无 opencode 条目，独立拉取 models.opencode.ai
	openCodeGoSyncModelsURL = "https://models.opencode.ai/api.json"
	openCodeGoSyncVendor    = "OpenCode Go"
	// upstreamFetchCacheLimit 限制 etagCache/bodyCache 条目数（超过后整体清空）。
	// fetchJSON 按调用方提供的任意 URL 累积缓存，URL 来自不可信输入时缓存会
	// 无界增长，故设置简单容量上限防止内存泄漏。
	upstreamFetchCacheLimit = 64
)

// openCodeGoProviderVendors 将 api.json 的 provider.npm 值映射为供应商名
// （次要手段，仅当模型 ID 正则未命中时使用）。仅收录明确一对一的主流 SDK
// 包；@ai-sdk/openai-compatible 等泛化包不在此映射，交由模型 ID 正则/兜底
// 处理，避免误判。
// 命名与 DB 既有供应商保持一致（OpenAI、Z.AI、Hunyuan、Qwen、DeepSeek、
// MiniMax、xAI、Nvidia、Moonshot、OpenRouter、OpenCode Go 等），避免同一
// 厂商建出多个供应商；未收录的厂商按规范英文名新建。
var openCodeGoProviderVendors = map[string]string{
	"@ai-sdk/openai":          "OpenAI",
	"@ai-sdk/anthropic":       "Anthropic",
	"@ai-sdk/google":          "Google",
	"@ai-sdk/google-vertex":   "Google",
	"@ai-sdk/mistral":         "Mistral",
	"@ai-sdk/xai":             "xAI",
	"@ai-sdk/azure":           "Azure OpenAI",
	"@ai-sdk/amazon-bedrock":  "AWS Bedrock",
	"@ai-sdk/aws-bedrock":     "AWS Bedrock",
	"@ai-sdk/cerebras":        "Cerebras",
	"@ai-sdk/deepseek":        "DeepSeek",
	"@ai-sdk/groq":            "Groq",
	"@ai-sdk/perplexity":      "Perplexity",
	"@ai-sdk/togetherai":      "Together AI",
	"@ai-sdk/cohere":          "Cohere",
	"@ai-sdk/fireworks":       "Fireworks AI",
	"@ai-sdk/moonshot":        "Moonshot",
	"@ai-sdk/qwen":            "Qwen",
	"@ai-sdk/nvidia":          "Nvidia",
	"@ai-sdk/ollama":          "Ollama",
	"@ai-sdk/huggingface":     "Hugging Face",
	"@ai-sdk/replicate":       "Replicate",
	"@ai-sdk/modal":           "Modal",
	"@ai-sdk/luma":            "Luma",
	"@ai-sdk/elevenlabs":      "ElevenLabs",
	"@ai-sdk/minimax":         "MiniMax",
	"@ai-sdk/tencent-hunyuan": "Hunyuan",
	"@ai-sdk/zhipu":           "Z.AI",
	"@ai-sdk/kling":           "Kling",
	"@ai-sdk/openrouter":      "OpenRouter",
	"@ai-sdk/baidu":           "Baidu",
	"@ai-sdk/baichuan":        "Baichuan",
	"@ai-sdk/sambanova":       "SambaNova",
	"@ai-sdk/upstage":         "Upstage",
	"@ai-sdk/watsonx":         "IBM watsonx",
	"@ai-sdk/portkey":         "Portkey",
}

// openCodeGoVendorForProvider 返回 npm 包对应的供应商名；无映射返回空串。
func openCodeGoVendorForProvider(npm string) string {
	return openCodeGoProviderVendors[npm]
}

// openCodeGoVendorRules 按模型 id 前缀正则归属供应商（主要手段）：本地已存在
// 则复用（ensureVendorID 先查 DB），不存在则创建；匹配不到时回退 provider
// 映射，再兜底 openCodeGoSyncVendor。规则按序匹配，命中最特异的放前面；
// 本规则优先于 Provider(npm) 映射（见 openCodeGoVendorForModelAndProvider）。
// 供应商名经上游 api.json 查证（name 字段/独立分组名），与 DB 既有命名一致。
var openCodeGoVendorRules = []struct {
	re     *regexp.Regexp
	vendor string
}{
	{regexp.MustCompile(`^grok-`), "xAI"},
	{regexp.MustCompile(`^gpt-`), "OpenAI"},
	{regexp.MustCompile(`^glm-`), "Z.AI"},
	{regexp.MustCompile(`^kimi-`), "Moonshot"},
	{regexp.MustCompile(`^mimo-`), "Xiaomi"},
	{regexp.MustCompile(`^minimax-`), "MiniMax"},
	{regexp.MustCompile(`^qwen`), "Qwen"},
	{regexp.MustCompile(`^deepseek-`), "DeepSeek"},
	{regexp.MustCompile(`^hy`), "Hunyuan"},
	{regexp.MustCompile(`^nemotron`), "Nvidia"},
	{regexp.MustCompile(`^laguna`), "Poolside"},
	// 补充主流厂商前缀（放已有规则之后，避免与更特异的规则冲突）
	{regexp.MustCompile(`^claude-`), "Anthropic"},
	{regexp.MustCompile(`^llama-`), "Meta"},
	{regexp.MustCompile(`^mistral-`), "Mistral"},
	{regexp.MustCompile(`^gemma-`), "Google"},
	{regexp.MustCompile(`^gemini-`), "Google"},
	{regexp.MustCompile(`^phi-`), "Microsoft"},
	{regexp.MustCompile(`^command-`), "Cohere"},
	{regexp.MustCompile(`^doubao-`), "Volcengine"},
	{regexp.MustCompile(`^ernie-`), "Baidu"},
	{regexp.MustCompile(`^spark-`), "iFlytek"},
	{regexp.MustCompile(`^o1-`), "OpenAI"},
	{regexp.MustCompile(`^o3-`), "OpenAI"},
	{regexp.MustCompile(`^o4-`), "OpenAI"},
	// opencode-go 分组前缀补全（供应商名经上游 api.json 查证）：
	// longcat 有独立分组 name="LongCat"；ling/ring 为蚂蚁集团 InclusionAI
	// 产品线（llmgateway 分组 name="InclusionAI Ling 3.0 Flash"），用户要求
	// 供应商名显示为 AntGroup；north 为 Cohere 产品（cohere 分组
	// north-mini-code-1-0）；trinity 为 Arcee 产品（arcee 分组
	// trinity-large-thinking）；muse-spark 在各聚合分组均挂 meta/ 前缀
	// （openrouter 分组 meta/muse-spark-*）；ox-alpha 仅存在于
	// opencode-go/opencode 分组（OpenCode 生态自家模型）。
	{regexp.MustCompile(`^longcat-`), "LongCat"},
	{regexp.MustCompile(`^ling-`), "AntGroup"},
	{regexp.MustCompile(`^ring-`), "AntGroup"},
	{regexp.MustCompile(`^north-`), "Cohere"},
	{regexp.MustCompile(`^trinity-`), "Arcee"},
	{regexp.MustCompile(`^muse-`), "Meta"},
	{regexp.MustCompile(`^ox-`), "OpenCode Go"},
	{regexp.MustCompile(`^big-pickle`), "OpenCode Go"},
	{regexp.MustCompile(`^x-preview`), "OpenCode Go"},
}

func openCodeGoVendorForModel(modelID string) string {
	for _, r := range openCodeGoVendorRules {
		if r.re.MatchString(modelID) {
			return r.vendor
		}
	}
	return openCodeGoSyncVendor
}

// openCodeGoVendorForModelAndProvider 按判定顺序归属供应商：
// 模型 ID 正则 > Provider(npm) 映射 > 兜底 "OpenCode Go"。
// 理由：上游 api.json 的 provider 字段已被证实会错标（如 minimax-m3 的
// provider 竟是 @ai-sdk/anthropic、grok-4.6 是 @ai-sdk/openai），而模型 ID
// 前缀命名规律是稳定可离线维护的信号，故正则优先；provider 映射作为正则
// 未命中时的兜底（如 muse- 无正则时 provider @ai-sdk/openai → OpenAI，
// 至少比掉到 "OpenCode Go" 有意义）。
func openCodeGoVendorForModelAndProvider(modelID, providerNpm string) string {
	if v := openCodeGoVendorForModel(modelID); v != openCodeGoSyncVendor {
		return v
	}
	if v := openCodeGoVendorForProvider(providerNpm); v != "" {
		return v
	}
	return openCodeGoSyncVendor
}

// openCodeGoEndpointForProvider 将 api.json 的 provider.npm 映射为端点类型：
// @ai-sdk/anthropic → anthropic、@ai-sdk/openai → openai-response、
// 其余（含空、@ai-sdk/openai-compatible 等）→ openai。
func openCodeGoEndpointForProvider(providerNpm string) string {
	switch providerNpm {
	case "@ai-sdk/anthropic":
		return string(constant.EndpointTypeAnthropic)
	case "@ai-sdk/openai":
		return string(constant.EndpointTypeOpenAIResponse)
	default:
		return string(constant.EndpointTypeOpenAI)
	}
}

// endpointsJSON 组装模型端点 JSON（map 形态，如
// {"openai":{"path":"/v1/chat/completions","method":"POST"}}，与模型库端点
// 配置 UI 及 model/pricing.go 的解析格式一致）；path/method 统一取自 common
// 端点定义配置层（与前端/后端同一数据源）；marshal 失败返回 nil。
func endpointsJSON(providerNpm string) json.RawMessage {
	endpoint := openCodeGoEndpointForProvider(providerNpm)
	info, ok := common.GetDefaultEndpointInfo(constant.EndpointType(endpoint))
	if !ok {
		return nil
	}
	bytes, err := common.Marshal(map[string]any{endpoint: map[string]any{"path": info.Path, "method": info.Method}})
	if err != nil {
		return nil
	}
	return json.RawMessage(bytes)
}

// jsonEndpointsEqual 语义比较端点 JSON（键序/缩进无关），解析失败回退字符串比较
// buildUpstreamCapabilities 将 opencode-go 条目的子结构与富能力字段组装为
// model.Model.Capabilities JSON（新 groups 格式，与响应解析格式一致，见
// model_rich.go）：{"groups":[{"name":"chat","modalities":{...},"limits":{...},
// "reasoning_options":[...]}]}。仅当有子结构时才输出对应字段；全部缺失返回
// nil（写入空）。
func buildUpstreamCapabilities(entry service.OpenCodeGoModelEntry) json.RawMessage {
	group := map[string]any{"name": "chat"}
	if entry.Modalities != nil && (len(entry.Modalities.Input) > 0 || len(entry.Modalities.Output) > 0) {
		group["modalities"] = map[string]any{
			"input":  entry.Modalities.Input,
			"output": entry.Modalities.Output,
		}
	}
	if entry.Limit != nil && (entry.Limit.Context > 0 || entry.Limit.Output > 0) {
		group["limits"] = map[string]any{
			"context": entry.Limit.Context,
			"output":  entry.Limit.Output,
		}
	}
	if len(entry.ReasoningOptions) > 0 {
		group["reasoning_options"] = entry.ReasoningOptions
	}
	// 仅 name 无任何子结构 → 全部缺失，返回 nil 保持现状
	if len(group) == 1 {
		return nil
	}
	bytes, err := common.Marshal(map[string]any{"groups": []any{group}})
	if err != nil {
		return nil
	}
	return json.RawMessage(bytes)
}

func jsonEndpointsEqual(local string, upstream json.RawMessage) bool {
	if len(upstream) == 0 {
		return true
	}
	if strings.TrimSpace(local) == "" {
		return false
	}
	var localMap, upstreamMap map[string]any
	if err := common.Unmarshal([]byte(local), &localMap); err != nil {
		return local == string(upstream)
	}
	if err := common.Unmarshal(upstream, &upstreamMap); err != nil {
		return local == string(upstream)
	}
	return reflect.DeepEqual(localMap, upstreamMap)
}

// openCodeGoStatusToModelStatus 将 api.json 的 status 字符串映射为模型库状态。
// deprecated 已被 service 层过滤，其余状态（空、beta、preview 等）一律启用。
func openCodeGoStatusToModelStatus(status string) int {
	return 1
}

func normalizeLocale(locale string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case "", "zh", "zh-cn":
		return "zh", true
	case "en":
		return "en", true
	case "ja":
		return "ja", true
	default:
		return "", false
	}
}

func getUpstreamBase() string {
	return common.GetEnvOrDefaultString("SYNC_UPSTREAM_BASE", "https://basellm.github.io/llm-metadata")
}

func getUpstreamURLs(locale string) (modelsURL, vendorsURL string) {
	base := strings.TrimRight(getUpstreamBase(), "/")
	if l, ok := normalizeLocale(locale); ok && l != "" {
		return fmt.Sprintf("%s/api/i18n/%s/newapi/models.json", base, l),
			fmt.Sprintf("%s/api/i18n/%s/newapi/vendors.json", base, l)
	}
	return fmt.Sprintf("%s/api/newapi/models.json", base), fmt.Sprintf("%s/api/newapi/vendors.json", base)
}

type upstreamEnvelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    []T    `json:"data"`
}

type upstreamModel struct {
	Description string          `json:"description"`
	Endpoints   json.RawMessage `json:"endpoints"`
	Icon        string          `json:"icon"`
	ModelName   string          `json:"model_name"`
	NameRule    int             `json:"name_rule"`
	Status      int             `json:"status"`
	Tags        string          `json:"tags"`
	VendorName  string          `json:"vendor_name"`

	// 富模型元数据（仅 opencode-go 源提供，llm-metadata 源为缺省值）
	DisplayName      string          `json:"display_name"`
	Family           string          `json:"family"`
	ProviderNpm      string          `json:"provider_npm"`
	ReleaseDate      string          `json:"release_date"`
	LastUpdated      string          `json:"last_updated"`
	OpenWeights      *bool           `json:"open_weights"`
	CapAttachment    *bool           `json:"cap_attachment"`
	CapReasoning     *bool           `json:"cap_reasoning"`
	CapToolCall      *bool           `json:"cap_tool_call"`
	CapStructuredOut *bool           `json:"cap_structured_output"`
	CapTemperature   *bool           `json:"cap_temperature"`
	Capabilities     json.RawMessage `json:"capabilities"`
}

type upstreamVendor struct {
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Name        string `json:"name"`
	Status      int    `json:"status"`
}

var (
	etagCache  = make(map[string]string)
	bodyCache  = make(map[string][]byte)
	cacheMutex sync.RWMutex
)

// metadataSyncSource 描述一次同步的目录来源与版本指纹（与前端 MetadataSyncSource 对应）。
type metadataSyncSource struct {
	Locale     string `json:"locale"`
	ModelsURL  string `json:"models_url"`
	VendorsURL string `json:"vendors_url"`
	Version    string `json:"version"`
}

// metadataSyncField 是单个元数据字段的本地/上游差异（与前端 field 结构对应）。
type metadataSyncField struct {
	Field    string `json:"field"`
	Local    any    `json:"local"`
	Upstream any    `json:"upstream"`
}

// metadataSyncCandidate 描述一个模型的同步候选（与前端 MetadataSyncCandidate 对应）。
type metadataSyncCandidate struct {
	ModelName      string                `json:"model_name"`
	Kind           string                `json:"kind"`
	Scope          string                `json:"scope"`
	RecordVersion  string                `json:"record_version"`
	Fields         []metadataSyncField   `json:"fields"`
	Upstream       *model.MetadataValues `json:"upstream,omitempty"`
	VendorToCreate string                `json:"vendor_to_create,omitempty"`
}

type syncRequest struct {
	Source        string                        `json:"source"`
	Locale        string                        `json:"locale"`
	SourceVersion string                        `json:"source_version"`
	Selections    []model.MetadataSyncSelection `json:"selections"`
}

func newHTTPClient() *http.Client {
	timeoutSec := common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", 10)
	dialer := &net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}
	transport := &http.Transport{
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   time.Duration(timeoutSec) * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: time.Duration(timeoutSec) * time.Second,
	}
	if common.TLSInsecureSkipVerify {
		transport.TLSClientConfig = common.InsecureTLSConfig
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		if strings.HasSuffix(host, "github.io") {
			if conn, err := dialer.DialContext(ctx, "tcp4", addr); err == nil {
				return conn, nil
			}
			return dialer.DialContext(ctx, "tcp6", addr)
		}
		return dialer.DialContext(ctx, network, addr)
	}
	return &http.Client{Transport: transport}
}

var (
	httpClientOnce sync.Once
	httpClient     *http.Client
)

func getHTTPClient() *http.Client {
	httpClientOnce.Do(func() {
		httpClient = newHTTPClient()
	})
	return httpClient
}

func fetchJSON[T any](ctx context.Context, url string, out *upstreamEnvelope[T]) error {
	var lastErr error
	attempts := common.GetEnvOrDefault("SYNC_HTTP_RETRY", 3)
	if attempts < 1 {
		attempts = 1
	}
	baseDelay := 200 * time.Millisecond
	maxMB := common.GetEnvOrDefault("SYNC_HTTP_MAX_MB", 10)
	maxBytes := int64(maxMB) << 20
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		// ETag conditional request
		cacheMutex.RLock()
		if et := etagCache[url]; et != "" {
			req.Header.Set("If-None-Match", et)
		}
		cacheMutex.RUnlock()

		resp, err := getHTTPClient().Do(req)
		if err != nil {
			lastErr = err
			// backoff with jitter
			sleep := baseDelay * time.Duration(1<<attempt)
			jitter := time.Duration(rand.Intn(150)) * time.Millisecond
			time.Sleep(sleep + jitter)
			continue
		}
		func() {
			defer resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusOK:
				// read body into buffer for caching and flexible decode
				limited := io.LimitReader(resp.Body, maxBytes)
				buf, err := io.ReadAll(limited)
				if err != nil {
					lastErr = err
					return
				}
				// cache body and ETag
				cacheMutex.Lock()
				if len(bodyCache) >= upstreamFetchCacheLimit {
					// 缓存满：整体清空（简单淘汰），防止不可信上游 URL 使缓存无界增长
					etagCache = make(map[string]string)
					bodyCache = make(map[string][]byte)
				}
				if et := resp.Header.Get("ETag"); et != "" {
					etagCache[url] = et
				}
				bodyCache[url] = buf
				cacheMutex.Unlock()

				// Try decode as envelope first
				if err := common.Unmarshal(buf, out); err != nil {
					// Try decode as pure array
					var arr []T
					if err2 := common.Unmarshal(buf, &arr); err2 != nil {
						lastErr = err
						return
					}
					out.Success = true
					out.Data = arr
					out.Message = ""
				} else {
					if !out.Success && len(out.Data) == 0 && out.Message == "" {
						out.Success = true
					}
				}
				lastErr = nil
			case http.StatusNotModified:
				// use cache
				cacheMutex.RLock()
				buf := bodyCache[url]
				cacheMutex.RUnlock()
				if len(buf) == 0 {
					lastErr = errors.New("cache miss for 304 response")
					return
				}
				if err := common.Unmarshal(buf, out); err != nil {
					var arr []T
					if err2 := common.Unmarshal(buf, &arr); err2 != nil {
						lastErr = err
						return
					}
					out.Success = true
					out.Data = arr
					out.Message = ""
				} else {
					if !out.Success && len(out.Data) == 0 && out.Message == "" {
						out.Success = true
					}
				}
				lastErr = nil
			default:
				lastErr = errors.New(resp.Status)
			}
		}()
		if lastErr == nil {
			return nil
		}
		sleep := baseDelay * time.Duration(1<<attempt)
		jitter := time.Duration(rand.Intn(150)) * time.Millisecond
		time.Sleep(sleep + jitter)
	}
	return lastErr
}

// fetchMetadataCatalog 拉取上游模型/供应商目录并计算版本指纹。
// source == "opencode-go" 时使用 models.opencode.ai（本地 vendor 判定规则 +
// 富元数据），否则使用 llm-metadata 仓库（locale 对应 i18n URL）。
// 返回 source 元信息、模型目录、供应商目录、以及 opencode-go 来源的富元数据
// 映射（modelName -> 模型列名 map；llm-metadata 来源为空 map）。
func fetchMetadataCatalog(c *gin.Context, locale, source string) (metadataSyncSource, map[string]model.MetadataValues, map[string]model.Vendor, map[string]map[string]any, error) {
	resolved, valid := normalizeLocale(locale)
	if !valid {
		return metadataSyncSource{}, nil, nil, nil, errors.New("unsupported metadata language")
	}
	if source == "opencode-go" {
		return fetchOpenCodeGoCatalog(resolved)
	}
	return fetchLLMMetadataCatalog(c, resolved)
}

// fetchLLMMetadataCatalog 拉取 llm-metadata 仓库的模型/供应商目录。
func fetchLLMMetadataCatalog(c *gin.Context, locale string) (metadataSyncSource, map[string]model.MetadataValues, map[string]model.Vendor, map[string]map[string]any, error) {
	modelsURL, vendorsURL := getUpstreamURLs(locale)
	source := metadataSyncSource{Locale: locale, ModelsURL: modelsURL, VendorsURL: vendorsURL}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", 15))*time.Second)
	defer cancel()
	var modelsEnv upstreamEnvelope[upstreamModel]
	var vendorsEnv upstreamEnvelope[upstreamVendor]
	var modelsErr, vendorsErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); modelsErr = fetchJSON(ctx, modelsURL, &modelsEnv) }()
	go func() { defer wg.Done(); vendorsErr = fetchJSON(ctx, vendorsURL, &vendorsEnv) }()
	wg.Wait()
	if modelsErr != nil {
		return source, nil, nil, nil, fmt.Errorf("fetch models (%s, %s): %w", locale, modelsURL, modelsErr)
	}
	if vendorsErr != nil {
		return source, nil, nil, nil, fmt.Errorf("fetch vendors (%s, %s): %w", locale, vendorsURL, vendorsErr)
	}
	if !modelsEnv.Success || !vendorsEnv.Success {
		return source, nil, nil, nil, errors.New("upstream metadata source reported failure")
	}
	models := make(map[string]model.MetadataValues)
	vendors := make(map[string]model.Vendor)
	for _, vendor := range vendorsEnv.Data {
		vendor.Name = strings.TrimSpace(vendor.Name)
		if vendor.Name == "" {
			continue
		}
		vendors[vendor.Name] = model.Vendor{Name: vendor.Name, Description: vendor.Description, Icon: vendor.Icon, Status: vendor.Status}
	}
	for _, item := range modelsEnv.Data {
		if strings.TrimSpace(item.ModelName) == "" {
			continue
		}
		endpoints := ""
		if len(item.Endpoints) > 0 && string(item.Endpoints) != "null" {
			if err := common.Unmarshal(item.Endpoints, &endpoints); err != nil {
				endpoints = string(item.Endpoints)
			}
		}
		values := model.MetadataValues{Description: item.Description, Icon: item.Icon, Tags: item.Tags, Vendor: strings.TrimSpace(item.VendorName), Endpoints: endpoints, NameRule: item.NameRule, Status: item.Status}
		if err := model.ValidateMetadataValues(values); err != nil {
			return source, nil, nil, nil, fmt.Errorf("model %s: %w", item.ModelName, err)
		}
		if _, duplicate := models[item.ModelName]; duplicate {
			return source, nil, nil, nil, fmt.Errorf("duplicate upstream model: %s", item.ModelName)
		}
		models[item.ModelName] = values
	}
	encoded, err := common.Marshal([]any{source.Locale, models, vendors})
	if err != nil {
		return source, nil, nil, nil, err
	}
	source.Version = fmt.Sprintf("%x", sha256.Sum256(encoded))
	return source, models, vendors, map[string]map[string]any{}, nil
}

// fetchOpenCodeGoCatalog 拉取 models.opencode.ai 目录并组装七字段契约 + 富元数据。
func fetchOpenCodeGoCatalog(locale string) (metadataSyncSource, map[string]model.MetadataValues, map[string]model.Vendor, map[string]map[string]any, error) {
	entries, err := service.FetchOpenCodeGoModelEntries()
	if err != nil {
		return metadataSyncSource{}, nil, nil, nil, err
	}
	source := metadataSyncSource{Locale: locale, ModelsURL: openCodeGoSyncModelsURL}
	models := make(map[string]model.MetadataValues)
	richByModel := make(map[string]map[string]any)
	// 供应商列表：OpenCode Go + 规则表全部目标供应商 + provider 映射厂商（去重）
	vendorSet := map[string]struct{}{openCodeGoSyncVendor: {}}
	for _, r := range openCodeGoVendorRules {
		vendorSet[r.vendor] = struct{}{}
	}
	for _, v := range openCodeGoProviderVendors {
		vendorSet[v] = struct{}{}
	}
	vendors := make(map[string]model.Vendor, len(vendorSet))
	for name := range vendorSet {
		description := ""
		if name == openCodeGoSyncVendor {
			description = "opencode.ai zen/go 官方模型"
		}
		vendors[name] = model.Vendor{Name: name, Description: description, Status: 1}
	}
	for _, entry := range entries {
		vendorName := openCodeGoVendorForModelAndProvider(entry.ID, entry.Provider)
		values := model.MetadataValues{
			Description: entry.Description,
			Vendor:      vendorName,
			Endpoints:   string(endpointsJSON(entry.Provider)),
			NameRule:    0,
			Status:      openCodeGoStatusToModelStatus(entry.Status),
		}
		if err := model.ValidateMetadataValues(values); err != nil {
			return metadataSyncSource{}, nil, nil, nil, fmt.Errorf("model %s: %w", entry.ID, err)
		}
		models[entry.ID] = values
		richByModel[entry.ID] = openCodeGoRichFields(entry)
	}
	encoded, err := common.Marshal([]any{locale, models, vendors, richByModel})
	if err != nil {
		return metadataSyncSource{}, nil, nil, nil, err
	}
	source.Version = fmt.Sprintf("%x", sha256.Sum256(encoded))
	return source, models, vendors, richByModel, nil
}

// openCodeGoRichFields 组装 opencode-go 条目的富元数据为模型列名 map。
func openCodeGoRichFields(entry service.OpenCodeGoModelEntry) map[string]any {
	fields := map[string]any{
		"display_name":           entry.Name,
		"provider_npm":           entry.Provider,
		"release_date":           entry.ReleaseDate,
		"last_updated":           entry.LastUpdated,
		"open_weights":           entry.OpenWeights,
		"cap_attachment":         entry.Attachment,
		"cap_reasoning":          entry.Reasoning,
		"cap_tool_call":          entry.ToolCall,
		"cap_structured_output":  entry.StructuredOutput,
		"cap_temperature":        entry.Temperature,
	}
	if entry.Family != "" {
		fields["family"] = entry.Family
	}
	if caps := buildUpstreamCapabilities(entry); len(caps) > 0 {
		fields["capabilities"] = string(caps)
	}
	// 剔除 nil 指针，避免写入 NULL 覆盖默认值
	for key, value := range fields {
		if value == nil {
			delete(fields, key)
		}
	}
	return fields
}

// SyncUpstreamModels 应用用户从预览中选中的同步变更（selections）。
// 校验目录版本指纹与预览一致后，调 model.ApplyMetadataSync 创建/更新模型与供应商。
func SyncUpstreamModels(c *gin.Context) {
	var req syncRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || len(req.Selections) == 0 || req.SourceVersion == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Preview and select metadata changes before applying"})
		return
	}
	src, upstream, vendors, richByModel, err := fetchMetadataCatalog(c, req.Locale, req.Source)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if src.Version != req.SourceVersion {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Upstream metadata changed; preview again"})
		return
	}
	updates := make([]model.MetadataSyncUpdate, 0, len(req.Selections))
	for _, selection := range req.Selections {
		values, exists := upstream[selection.ModelName]
		if !exists {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Selected upstream model is no longer available"})
			return
		}
		update := model.MetadataSyncUpdate{MetadataSyncSelection: selection, Values: values}
		if selection.Create {
			update.RichFields = richByModel[selection.ModelName]
		}
		updates = append(updates, update)
	}
	result, err := model.ApplyMetadataSync(updates, vendors)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrMetadataSyncConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.ApiSuccess(c, result)
}

// SyncUpstreamPreview 预览上游与本地的差异，生成可选择的候选列表
// （candidates 契约与前端 MetadataSyncPreview 对应）。
func SyncUpstreamPreview(c *gin.Context) {
	locale := c.Query("locale")
	source := c.Query("source")
	src, upstream, upstreamVendors, _, err := fetchMetadataCatalog(c, locale, source)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	locals, vendors, err := model.GetMetadataSyncState(model.DB)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	missing, err := model.GetMissingModels()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	siteNames := make(map[string]bool)
	allNames := make(map[string]bool)
	for name := range locals {
		siteNames[name] = true
		allNames[name] = true
	}
	for _, name := range missing {
		siteNames[name] = true
		allNames[name] = true
	}
	for name := range upstream {
		allNames[name] = true
	}
	names := make([]string, 0, len(allNames))
	for name := range allNames {
		names = append(names, name)
	}
	sort.Strings(names)
	vendorByID := make(map[int]*model.Vendor)
	for _, vendor := range vendors {
		vendorByID[vendor.Id] = vendor
	}
	candidates := make([]metadataSyncCandidate, 0, len(names))
	for _, name := range names {
		candidate := metadataSyncCandidate{ModelName: name, Scope: "catalog", Kind: "create", Fields: []metadataSyncField{}}
		if siteNames[name] {
			candidate.Scope = "site"
		}
		local := locals[name]
		up, found := upstream[name]
		if !found {
			candidate.Kind = "missing_upstream"
			candidates = append(candidates, candidate)
			continue
		}
		candidate.Upstream = &up
		var localVendor *model.Vendor
		if local != nil {
			localVendor = vendorByID[local.VendorID]
		}
		candidate.RecordVersion = model.MetadataRecordVersion(local, localVendor, model.FindMetadataVendor(vendors, up.Vendor))
		if local != nil && local.SyncOfficial == 0 {
			candidate.Kind = "blocked"
			candidates = append(candidates, candidate)
			continue
		}
		if up.Vendor != "" && model.FindMetadataVendor(vendors, up.Vendor) == nil {
			if _, exists := upstreamVendors[up.Vendor]; !exists {
				candidate.Kind = "missing_vendor"
				candidates = append(candidates, candidate)
				continue
			}
			candidate.VendorToCreate = up.Vendor
		}
		localValues := model.MetadataValues{}
		if local != nil {
			candidate.Kind = "update"
			localValues = model.MetadataValues{Description: local.Description, Icon: local.Icon, Tags: local.Tags, Endpoints: local.Endpoints, NameRule: local.NameRule, Status: local.Status}
			if localVendor != nil {
				localValues.Vendor = localVendor.Name
			}
		}
		localRaw, _ := common.Marshal(localValues)
		upRaw, _ := common.Marshal(up)
		var localFields, upFields map[string]any
		_ = common.Unmarshal(localRaw, &localFields)
		_ = common.Unmarshal(upRaw, &upFields)
		for _, field := range model.MetadataSyncFields {
			if local == nil || localFields[field] != upFields[field] {
				candidate.Fields = append(candidate.Fields, metadataSyncField{Field: field, Local: localFields[field], Upstream: upFields[field]})
			}
		}
		if local != nil && len(candidate.Fields) == 0 {
			candidate.Kind = "unchanged"
		}
		candidates = append(candidates, candidate)
	}
	common.ApiSuccess(c, gin.H{"source": src, "candidates": candidates})
}
