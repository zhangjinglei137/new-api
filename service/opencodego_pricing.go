package service

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const (
	openCodeGoUsageCapsURL = "https://opencode.ai/docs/go.md"
	openCodeGoDefaultCap   = 60
)

// normalizeOpenCodeModelName 将 markdown 定价表中的展示名归一化为 api.json 的 id
// 风格：小写 → 删除括号及其内容 → 连续空白折叠为单个 '-'
// （例如 "GPT 5.6 Luna (≤ 272K tokens)" → "gpt-5.6-luna"，"." 保留）。
func normalizeOpenCodeModelName(name string) string {
	name = strings.ToLower(name)
	if idx := strings.Index(name, "("); idx >= 0 {
		name = name[:idx]
	}
	return strings.Join(strings.Fields(name), "-")
}

// parseOpenCodeGoUsageCaps 解析 go.md 定价表（6 列：Model | Input | Output |
// Cached read | Cached write | Usage），按列位置提取模型名与 Usage 档位（$N）。
// 表头与分隔行自动跳过，无法解析的行忽略。
func parseOpenCodeGoUsageCaps(data []byte) (map[string]int, error) {
	caps := make(map[string]int)
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|-") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 7 {
			continue
		}
		modelName := strings.TrimSpace(cells[1])
		usageCell := strings.TrimSpace(cells[6])
		usageCell = strings.TrimSpace(strings.TrimPrefix(usageCell, "$"))
		if modelName == "" || usageCell == "" {
			continue
		}
		usage, err := strconv.Atoi(usageCell)
		if err != nil {
			continue // 表头或不可解析的行
		}
		caps[normalizeOpenCodeModelName(modelName)] = usage
	}
	return caps, nil
}

// FetchOpenCodeGoUsageCaps 拉取 opencode 官方 go.md 定价表，返回
// 归一化模型 id → 月额度（美元）映射。
func FetchOpenCodeGoUsageCaps() (map[string]int, error) {
	client, err := GetHttpClientWithProxy("")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openCodeGoUsageCapsURL, nil)
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
	return parseOpenCodeGoUsageCaps(body)
}

// ConvertOpenCodeGoToRatioData 解析 models.opencode.ai api.json 并转换为本地
// ratio 格式（model_ratio/completion_ratio/cache_ratio），额度档位来自
// opencode.ai/docs/go.md；拉取失败时降级为默认档位 60。
func ConvertOpenCodeGoToRatioData(reader io.Reader) (map[string]any, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	caps, err := FetchOpenCodeGoUsageCaps()
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to fetch opencode go usage caps, fallback to default cap %d: %v", openCodeGoDefaultCap, err))
		caps = nil
	}
	return convertOpenCodeGoRatioData(data, caps, float64(ratio_setting.USD))
}

// convertOpenCodeGoRatioData 将 api.json 定价（USD/1M tokens）转换为本地 ratio：
// model_ratio = input × USD/1000 × 系数（系数 = 60/cap，cap 缺失默认 60）
// completion_ratio = output/input；cache_ratio = cache_read/input。
// 免费模型（input 为 0）的 ratio 为 0，不产生 completion/cache 比值。
func convertOpenCodeGoRatioData(data []byte, caps map[string]int, usd float64) (map[string]any, error) {
	var upstream map[string]openCodeGoProvider
	if err := common.Unmarshal(data, &upstream); err != nil {
		return nil, fmt.Errorf("failed to decode opencode api.json response: %w", err)
	}
	if len(upstream) == 0 {
		return nil, fmt.Errorf("empty opencode api.json response")
	}

	models := collectOpenCodeGoModels(upstream)
	if len(models) == 0 {
		return nil, fmt.Errorf("no valid opencode pricing entries found")
	}

	modelRatioMap := make(map[string]any)
	completionRatioMap := make(map[string]any)
	cacheRatioMap := make(map[string]any)

	// caps 拉取成功但个别模型在 go.md 表中没有对应行时记一条告警；
	// caps 整体拉取失败（nil）时已在 ConvertOpenCodeGoToRatioData 中告警，不重复统计。
	var missingCaps []string

	for name, m := range models {
		input := 0.0
		if m.Cost.Input != nil {
			input = *m.Cost.Input
		}
		if math.IsNaN(input) || math.IsInf(input, 0) || input < 0 {
			continue
		}

		if input == 0 {
			// 免费模型（含 -free 合并条目），不需要 cap
			modelRatioMap[name] = 0.0
			continue
		}

		cap := openCodeGoDefaultCap
		if v, ok := caps[name]; ok && v > 0 {
			cap = v
		} else if caps != nil {
			missingCaps = append(missingCaps, name)
		}
		factor := float64(openCodeGoDefaultCap) / float64(cap)

		modelRatioMap[name] = roundOpenCodeGoRatioValue(input * usd / 1000.0 * factor)

		if m.Cost.Output != nil && isValidOpenCodeGoCost(*m.Cost.Output) {
			completionRatioMap[name] = roundOpenCodeGoRatioValue(*m.Cost.Output / input)
		}
		if m.Cost.CacheRead != nil && isValidOpenCodeGoCost(*m.Cost.CacheRead) {
			cacheRatioMap[name] = roundOpenCodeGoRatioValue(*m.Cost.CacheRead / input)
		}
	}

	if len(missingCaps) > 0 {
		sort.Strings(missingCaps)
		display := missingCaps
		if len(display) > 10 {
			display = display[:10]
		}
		common.SysLog(fmt.Sprintf("opencode-go ratio: %d models missing usage caps from go.md, defaulted to %d: %s",
			len(missingCaps), openCodeGoDefaultCap, strings.Join(display, ",")))
	}

	converted := make(map[string]any)
	if len(modelRatioMap) > 0 {
		converted["model_ratio"] = modelRatioMap
	}
	if len(completionRatioMap) > 0 {
		converted["completion_ratio"] = completionRatioMap
	}
	if len(cacheRatioMap) > 0 {
		converted["cache_ratio"] = cacheRatioMap
	}
	return converted, nil
}

func roundOpenCodeGoRatioValue(value float64) float64 {
	return math.Round(value*1e6) / 1e6
}

func isValidOpenCodeGoCost(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}
