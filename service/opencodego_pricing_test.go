package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertOpenCodeGoRatioDataSkipsPathologicalPrices 验证计费换算的输入
// 防御不变式：病态价格（NaN/±Inf/负值/超 maxRatioBound）不得进入真实计费。
// input 异常 → 整个模型被跳过；output/cache_read 异常 → 仅跳过对应比值，
// model_ratio 仍按合法 input 正常换算。
func TestConvertOpenCodeGoRatioDataSkipsPathologicalPrices(t *testing.T) {
	const usd = 1000.0
	apiJSON := `{
  "opencode-go": {
    "models": {
      "norm-model": {
        "id": "norm-model",
        "cost": {"input": 1.0, "output": 4.0, "cache_read": 0.5}
      },
      "free-model": {
        "id": "free-model",
        "cost": {"input": 0.0, "output": 0.0}
      },
      "huge-input-model": {
        "id": "huge-input-model",
        "cost": {"input": 2000000.0, "output": 1.0}
      },
      "negative-input-model": {
        "id": "negative-input-model",
        "cost": {"input": -1.0}
      },
      "nil-input-model": {
        "id": "nil-input-model",
        "cost": {"output": 2.0}
      },
      "huge-output-model": {
        "id": "huge-output-model",
        "cost": {"input": 1.0, "output": 3000000.0, "cache_read": 0.5}
      },
      "huge-cache-model": {
        "id": "huge-cache-model",
        "cost": {"input": 2.0, "output": 1.0, "cache_read": 3000000.0}
      }
    }
  }
}`

	converted, err := convertOpenCodeGoRatioData([]byte(apiJSON), usd)
	require.NoError(t, err)

	modelRatios, ok := converted["model_ratio"].(map[string]any)
	require.True(t, ok, "model_ratio must be present")

	// 正常模型：model_ratio = input × usd/1000，completion/cache 为相对比值
	completionRatios, ok := converted["completion_ratio"].(map[string]any)
	require.True(t, ok, "completion_ratio must be present")
	cacheRatios, ok := converted["cache_ratio"].(map[string]any)
	require.True(t, ok, "cache_ratio must be present")
	assert.Equal(t, 1.0, modelRatios["norm-model"])
	assert.Equal(t, 4.0, completionRatios["norm-model"])
	assert.Equal(t, 0.5, cacheRatios["norm-model"])

	// 免费模型：model_ratio = 0，不产生 completion/cache 比值
	assert.Equal(t, 0.0, modelRatios["free-model"])
	assert.NotContains(t, completionRatios, "free-model")
	assert.NotContains(t, cacheRatios, "free-model")

	// input 病态（超上界 / 负值）→ 整个模型跳过，即使其它价格正常
	assert.NotContains(t, modelRatios, "huge-input-model")
	assert.NotContains(t, modelRatios, "negative-input-model")
	assert.NotContains(t, completionRatios, "huge-input-model")

	// input 缺失（nil）→ 视为免费模型
	assert.Equal(t, 0.0, modelRatios["nil-input-model"])

	// output 病态 → 仅 completion_ratio 不含该模型，model_ratio/cache 正常
	assert.Equal(t, 1.0, modelRatios["huge-output-model"])
	assert.NotContains(t, completionRatios, "huge-output-model")
	assert.Equal(t, 0.5, cacheRatios["huge-output-model"])

	// cache_read 病态 → 仅 cache_ratio 不含该模型，model_ratio/completion 正常
	assert.Equal(t, 2.0, modelRatios["huge-cache-model"])
	assert.Equal(t, 0.5, completionRatios["huge-cache-model"])
	assert.NotContains(t, cacheRatios, "huge-cache-model")
}

// TestIsValidOpenCodeGoCostBounds 锁定 isValidOpenCodeGoCost 的边界：合法价格
// 必须是有限、非负且不超 maxRatioBound；NaN/±Inf/负值/超上界一律拒绝。
func TestIsValidOpenCodeGoCostBounds(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  bool
	}{
		{name: "zero", value: 0, want: true},
		{name: "normal", value: 1.25, want: true},
		{name: "at upper bound", value: maxRatioBound, want: true},
		{name: "above upper bound", value: maxRatioBound + 1, want: false},
		{name: "huge", value: 1e12, want: false},
		{name: "negative", value: -0.1, want: false},
		{name: "NaN", value: math.NaN(), want: false},
		{name: "+Inf", value: math.Inf(1), want: false},
		{name: "-Inf", value: math.Inf(-1), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isValidOpenCodeGoCost(tt.value))
		})
	}
}