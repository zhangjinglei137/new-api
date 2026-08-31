package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const openCodeGoModelsURL = "https://models.opencode.ai/api.json"

// openCodeGoProvider 是 models.opencode.ai api.json 中单个 provider 段的结构。
type openCodeGoProvider struct {
	Models map[string]openCodeGoModel `json:"models"`
}

// openCodeGoProviderRef 是 api.json 模型条目中 provider 对象（如 {"npm": "..."}）。
type openCodeGoProviderRef struct {
	NPM string `json:"npm"`
}

type openCodeGoModel struct {
	ID               string                `json:"id"`
	Name             string                `json:"name"`
	Description      string                `json:"description"`
	Status           string                `json:"status"`
	Family           string                `json:"family"`
	Attachment       *bool                 `json:"attachment"`
	Reasoning        *bool                 `json:"reasoning"`
	ReasoningOptions []map[string]any      `json:"reasoning_options"`
	ToolCall         *bool                 `json:"tool_call"`
	StructuredOutput *bool                 `json:"structured_output"`
	Temperature      *bool                 `json:"temperature"`
	OpenWeights      *bool                 `json:"open_weights"`
	ReleaseDate      string                `json:"release_date"`
	LastUpdated      string                `json:"last_updated"`
	Modalities       *openCodeGoModalities `json:"modalities"`
	Limit            *openCodeGoLimit      `json:"limit"`
	Cost             openCodeGoCost        `json:"cost"`
	Provider         openCodeGoProviderRef `json:"provider"`
}

type openCodeGoModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type openCodeGoLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

type openCodeGoCost struct {
	Input     *float64 `json:"input"`
	Output    *float64 `json:"output"`
	CacheRead *float64 `json:"cache_read"`
}

// collectOpenCodeGoModels 合并 opencode-go 段全部非 deprecated 模型与
// opencode 段中免费的 -free 模型（cost 全 0 且非 deprecated）；同 id 时以
// 免费模型覆盖（后写入者胜）。
func collectOpenCodeGoModels(upstream map[string]openCodeGoProvider) map[string]openCodeGoModel {
	models := make(map[string]openCodeGoModel)
	if provider, ok := upstream["opencode-go"]; ok {
		for name, m := range provider.Models {
			if m.Status != "deprecated" {
				models[name] = m
			}
		}
	}
	if provider, ok := upstream["opencode"]; ok {
		for name, m := range provider.Models {
			if m.Status == "deprecated" || !strings.HasSuffix(name, "-free") {
				continue
			}
			if isOpenCodeGoZeroCost(m.Cost) {
				// 免费模型覆盖同 id 条目；缺失的元数据字段回退原条目的值，
				// 避免免费条目（通常只含价格信息）抹掉付费条目的富元数据。
				if existing, ok := models[name]; ok {
					if m.Name == "" {
						m.Name = existing.Name
					}
					if m.Description == "" {
						m.Description = existing.Description
					}
					if m.Provider.NPM == "" {
						m.Provider = existing.Provider
					}
					if m.Family == "" {
						m.Family = existing.Family
					}
					if m.Attachment == nil {
						m.Attachment = existing.Attachment
					}
					if m.Reasoning == nil {
						m.Reasoning = existing.Reasoning
					}
					if len(m.ReasoningOptions) == 0 {
						m.ReasoningOptions = existing.ReasoningOptions
					}
					if m.ToolCall == nil {
						m.ToolCall = existing.ToolCall
					}
					if m.StructuredOutput == nil {
						m.StructuredOutput = existing.StructuredOutput
					}
					if m.Temperature == nil {
						m.Temperature = existing.Temperature
					}
					if m.OpenWeights == nil {
						m.OpenWeights = existing.OpenWeights
					}
					if m.ReleaseDate == "" {
						m.ReleaseDate = existing.ReleaseDate
					}
					if m.LastUpdated == "" {
						m.LastUpdated = existing.LastUpdated
					}
					if m.Modalities == nil {
						m.Modalities = existing.Modalities
					}
					if m.Limit == nil {
						m.Limit = existing.Limit
					}
				}
				models[name] = m
			}
		}
	}
	return models
}

func isOpenCodeGoZeroCost(cost openCodeGoCost) bool {
	input, output := 0.0, 0.0
	if cost.Input != nil {
		input = *cost.Input
	}
	if cost.Output != nil {
		output = *cost.Output
	}
	return input == 0 && output == 0
}

// parseOpenCodeGoModels 解析 models.opencode.ai api.json 的模型 id 列表。
func parseOpenCodeGoModels(data []byte) ([]string, error) {
	var upstream map[string]openCodeGoProvider
	if err := common.Unmarshal(data, &upstream); err != nil {
		return nil, fmt.Errorf("invalid opencode api.json response: %w", err)
	}
	models := collectOpenCodeGoModels(upstream)
	if len(models) == 0 {
		return nil, fmt.Errorf("opencode api.json response contains no valid models")
	}
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// fetchOpenCodeGoAPIJSON 拉取 models.opencode.ai api.json 原始字节。
func fetchOpenCodeGoAPIJSON() ([]byte, error) {
	client, err := GetHttpClientWithProxy("")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openCodeGoModelsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// FetchOpenCodeGoModels 拉取 models.opencode.ai 的官方模型列表。
func FetchOpenCodeGoModels() ([]string, error) {
	body, err := fetchOpenCodeGoAPIJSON()
	if err != nil {
		return nil, err
	}
	return parseOpenCodeGoModels(body)
}

// OpenCodeGoModelEntry 是 opencode-go 模型库同步使用的结构化条目。
type OpenCodeGoModelEntry struct {
	ID          string
	Name        string
	Description string
	// Status 是 api.json 条目中的 status 字段（""、beta、preview 等；deprecated 已被过滤）。
	Status string
	// Provider 是 api.json 条目中 provider.npm 值（如 "@ai-sdk/openai"），无则空。
	Provider         string
	Family           string
	Attachment       *bool
	Reasoning        *bool
	ReasoningOptions []map[string]any
	ToolCall         *bool
	StructuredOutput *bool
	Temperature      *bool
	OpenWeights      *bool
	ReleaseDate      string
	LastUpdated      string
	Modalities       *ModalitiesEntry
	Limit            *LimitEntry
}

// ModalitiesEntry 输入/输出模态
type ModalitiesEntry struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// LimitEntry 上下文/输出限额（tokens）
type LimitEntry struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

// parseOpenCodeGoModelEntries 解析 models.opencode.ai api.json 的结构化条目
// 列表，集合规则与 parseOpenCodeGoModels 完全一致（同 id 时 -free 免费条目覆盖）。
func parseOpenCodeGoModelEntries(data []byte) ([]OpenCodeGoModelEntry, error) {
	var upstream map[string]openCodeGoProvider
	if err := common.Unmarshal(data, &upstream); err != nil {
		return nil, fmt.Errorf("invalid opencode api.json response: %w", err)
	}
	models := collectOpenCodeGoModels(upstream)
	if len(models) == 0 {
		return nil, fmt.Errorf("opencode api.json response contains no valid models")
	}
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]OpenCodeGoModelEntry, 0, len(names))
	for _, name := range names {
		m := models[name]
		entry := OpenCodeGoModelEntry{ID: name, Description: m.Description, Status: m.Status, Provider: m.Provider.NPM}
		entry.Name = m.Name
		if entry.Name == "" {
			entry.Name = name
		}
		entry.Family = m.Family
		entry.Attachment = m.Attachment
		entry.Reasoning = m.Reasoning
		entry.ReasoningOptions = m.ReasoningOptions
		entry.ToolCall = m.ToolCall
		entry.StructuredOutput = m.StructuredOutput
		entry.Temperature = m.Temperature
		entry.OpenWeights = m.OpenWeights
		entry.ReleaseDate = m.ReleaseDate
		entry.LastUpdated = m.LastUpdated
		if m.Modalities != nil {
			entry.Modalities = &ModalitiesEntry{Input: m.Modalities.Input, Output: m.Modalities.Output}
		}
		if m.Limit != nil {
			entry.Limit = &LimitEntry{Context: m.Limit.Context, Output: m.Limit.Output}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// FetchOpenCodeGoModelEntries 拉取 models.opencode.ai 的结构化模型条目。
func FetchOpenCodeGoModelEntries() ([]OpenCodeGoModelEntry, error) {
	body, err := fetchOpenCodeGoAPIJSON()
	if err != nil {
		return nil, err
	}
	return parseOpenCodeGoModelEntries(body)
}
