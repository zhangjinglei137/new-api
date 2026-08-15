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

type openCodeGoModel struct {
	Status string         `json:"status"`
	Cost   openCodeGoCost `json:"cost"`
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

// FetchOpenCodeGoModels 拉取 models.opencode.ai 的官方模型列表。
func FetchOpenCodeGoModels() ([]string, error) {
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseOpenCodeGoModels(body)
}
