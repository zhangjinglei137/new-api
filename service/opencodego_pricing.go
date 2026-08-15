package service

import (
	"fmt"
	"io"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// TODO(opencode-go 额度系数)：额度系数（model_ratio ×(60/总额度)，总额度
// 15/60 来自 opencode 官方定价文档 go.md）已于本次移除，价格现直接采用
// api.json 原价（model_ratio = input × USD/1000）。如需恢复：
//   1. 从 git 历史取回该逻辑（首次引入于 commit bdf7bb94 的
//      service/opencodego_pricing.go，精度调整见 db783104），恢复
//      normalizeOpenCodeModelName / parseOpenCodeGoUsageCaps /
//      FetchOpenCodeGoUsageCaps 三个函数与 openCodeGoUsageCapsURL /
//      openCodeGoDefaultCap 两个常量；
//   2. 在 convertOpenCodeGoRatioData 中恢复 factor 计算
//      （model_ratio = input × usd/1000 × (60/cap)，cap 缺失默认 60）。

// ConvertOpenCodeGoToRatioData 解析 models.opencode.ai api.json 并转换为本地
// ratio 格式（model_ratio/completion_ratio/cache_ratio），价格直接采用
// api.json 原价（USD/1M tokens）。
func ConvertOpenCodeGoToRatioData(reader io.Reader) (map[string]any, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return convertOpenCodeGoRatioData(data, float64(ratio_setting.USD))
}

// convertOpenCodeGoRatioData 将 api.json 定价（USD/1M tokens）转换为本地 ratio：
// model_ratio = input × USD/1000
// completion_ratio = output/input；cache_ratio = cache_read/input。
// 免费模型（input 为 0）的 ratio 为 0，不产生 completion/cache 比值。
func convertOpenCodeGoRatioData(data []byte, usd float64) (map[string]any, error) {
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

	for name, m := range models {
		input := 0.0
		if m.Cost.Input != nil {
			input = *m.Cost.Input
		}
		if math.IsNaN(input) || math.IsInf(input, 0) || input < 0 {
			continue
		}

		if input == 0 {
			// 免费模型（含 -free 合并条目）
			modelRatioMap[name] = 0.0
			continue
		}

		modelRatioMap[name] = roundOpenCodeGoRatioValue(input * usd / 1000.0)

		if m.Cost.Output != nil && isValidOpenCodeGoCost(*m.Cost.Output) {
			completionRatioMap[name] = roundOpenCodeGoRatioValue(*m.Cost.Output / input)
		}
		if m.Cost.CacheRead != nil && isValidOpenCodeGoCost(*m.Cost.CacheRead) {
			cacheRatioMap[name] = roundOpenCodeGoRatioValue(*m.Cost.CacheRead / input)
		}
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

// roundOpenCodeGoRatioValue 保留 15 位小数（float64 在 15 位内一般可精确表示）。
// 注意精度：completion/cache 是相对 model_ratio 的比值（如 0.003625/0.435 =
// 0.00833333... 循环小数），前端用 model_ratio × 比值还原实际价格。若舍入到
// 6 位（models.dev 惯例），还原价会产生可见误差（1.74×0.008333 = 0.01449942
// 而非 0.0145）；舍入到 12 位则误差 5.8e-10 超出前端 snapFloatDrift 的 1e-12
// 容差，仍显示长尾。15 位舍入的误差 < 1e-15，前端可干净还原为 0.0145。
func roundOpenCodeGoRatioValue(value float64) float64 {
	return math.Round(value*1e15) / 1e15
}

func isValidOpenCodeGoCost(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}
