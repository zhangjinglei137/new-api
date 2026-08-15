package service

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const openCodeGoAPIJSONFixture = `{
  "opencode-go": {
    "models": {
      "gpt-5.6-luna": {"id": "gpt-5.6-luna", "name": "GPT 5.6 Luna", "description": "GPT 5.6 Luna official model", "status": "", "cost": {"input": 15, "output": 60, "cache_read": 7.5}},
      "no-cap-model": {"description": "A model without usage cap", "status": "", "cost": {"input": 1, "output": 2, "cache_read": 0.5}},
      "deepseek-v4-pro": {"name": "DeepSeek V4 Pro", "description": "DeepSeek V4 Pro model", "status": "", "cost": {"input": 0.435, "output": 0.87, "cache_read": 0.003625}},
      "shared-free": {"name": "Shared", "description": "shared paid description", "status": "", "cost": {"input": 10, "output": 20}},
      "deprecated-model": {"name": "Deprecated", "description": "deprecated model", "status": "deprecated", "cost": {"input": 1, "output": 2}}
    }
  },
  "opencode": {
    "models": {
      "hy3-free": {"name": "Hy3 Free", "description": "Hy3 free tier", "status": "", "cost": {"input": 0, "output": 0}},
      "shared-free": {"name": "Shared Free", "description": "shared free description", "status": "", "cost": {"input": 0, "output": 0}},
      "paid-x": {"name": "Paid X", "description": "paid model", "status": "", "cost": {"input": 5, "output": 10}},
      "dep-free": {"name": "Dep Free", "description": "deprecated free", "status": "deprecated", "cost": {"input": 0, "output": 0}}
    }
  }
}`

func TestParseOpenCodeGoModels(t *testing.T) {
	t.Run("valid api.json", func(t *testing.T) {
		ids, err := parseOpenCodeGoModels([]byte(openCodeGoAPIJSONFixture))
		require.NoError(t, err)
		// deprecated 剔除、非 -free 付费模型剔除、-free 免费模型合并
		assert.Equal(t, []string{"deepseek-v4-pro", "gpt-5.6-luna", "hy3-free", "no-cap-model", "shared-free"}, ids)
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := parseOpenCodeGoModels([]byte("{not json"))
		require.Error(t, err)
	})

	t.Run("no valid models", func(t *testing.T) {
		_, err := parseOpenCodeGoModels([]byte(`{"opencode-go": {"models": {"x": {"status": "deprecated", "cost": {"input": 1, "output": 1}}}}}`))
		require.Error(t, err)
	})
}

func TestParseOpenCodeGoModelEntries(t *testing.T) {
	t.Run("valid api.json", func(t *testing.T) {
		entries, err := parseOpenCodeGoModelEntries([]byte(openCodeGoAPIJSONFixture))
		require.NoError(t, err)
		require.Len(t, entries, 5)
		// 排序后：deepseek-v4-pro, gpt-5.6-luna, hy3-free, no-cap-model, shared-free
		assert.Equal(t, "deepseek-v4-pro", entries[0].ID)
		assert.Equal(t, "DeepSeek V4 Pro", entries[0].Name)
		assert.Equal(t, "DeepSeek V4 Pro model", entries[0].Description)
		assert.Equal(t, "gpt-5.6-luna", entries[1].ID)
		assert.Equal(t, "GPT 5.6 Luna", entries[1].Name)
		assert.Equal(t, "GPT 5.6 Luna official model", entries[1].Description)
		// -free 模型描述优先取 -free 条目
		assert.Equal(t, "hy3-free", entries[2].ID)
		assert.Equal(t, "Hy3 Free", entries[2].Name)
		assert.Equal(t, "Hy3 free tier", entries[2].Description)
		// name 缺失时回退 id
		assert.Equal(t, "no-cap-model", entries[3].ID)
		assert.Equal(t, "no-cap-model", entries[3].Name)
		assert.Equal(t, "A model without usage cap", entries[3].Description)
		// 同 id 时 -free 免费条目覆盖（含 name/description）
		assert.Equal(t, "shared-free", entries[4].ID)
		assert.Equal(t, "Shared Free", entries[4].Name)
		assert.Equal(t, "shared free description", entries[4].Description)
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := parseOpenCodeGoModelEntries([]byte("{not json"))
		require.Error(t, err)
	})

	t.Run("no valid models", func(t *testing.T) {
		_, err := parseOpenCodeGoModelEntries([]byte(`{"opencode-go": {"models": {"x": {"status": "deprecated", "cost": {"input": 1, "output": 1}}}}}`))
		require.Error(t, err)
	})
}

func TestParseGoogleTranslateResponse(t *testing.T) {
	t.Run("single segment", func(t *testing.T) {
		body := []byte(`[[["这是翻译后的文本","original text",null,null,10]],null,"en",null,null,null,1]`)
		got, err := parseGoogleTranslateResponse(body)
		require.NoError(t, err)
		assert.Equal(t, "这是翻译后的文本", got)
	})

	t.Run("multi segment concatenation", func(t *testing.T) {
		body := []byte(`[[["第一段","a",null,null,10],["第二段","b",null,null,10]],null,"en"]`)
		got, err := parseGoogleTranslateResponse(body)
		require.NoError(t, err)
		assert.Equal(t, "第一段第二段", got)
	})

	t.Run("empty body", func(t *testing.T) {
		_, err := parseGoogleTranslateResponse(nil)
		require.Error(t, err)
	})

	t.Run("bad json", func(t *testing.T) {
		_, err := parseGoogleTranslateResponse([]byte("not json"))
		require.Error(t, err)
	})

	t.Run("no translated segments", func(t *testing.T) {
		_, err := parseGoogleTranslateResponse([]byte(`[[],null,"en"]`))
		require.Error(t, err)
	})
}

func TestOpenCodeGoTranslateTextNoNetwork(t *testing.T) {
	// 不涉及 HTTP 的分支：空文本、非中/日语言、空语言直接返回原文
	assert.Equal(t, "", TranslateText("", "zh-CN"))
	assert.Equal(t, "", TranslateText("   ", "ja"))
	assert.Equal(t, "Hello World", TranslateText("Hello World", "en"))
	assert.Equal(t, "Hello World", TranslateText("Hello World", ""))
}

func TestNormalizeOpenCodeModelName(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{name: "Grok 4.5", want: "grok-4.5"},
		{name: "GPT 5.6 Luna (≤ 272K tokens)", want: "gpt-5.6-luna"},
		{name: "Kimi K2.7 Code", want: "kimi-k2.7-code"},
		{name: "MiMo V2.5 Pro", want: "mimo-v2.5-pro"},
		{name: "Hy3", want: "hy3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeOpenCodeModelName(tc.name))
		})
	}
}

const openCodeGoUsageCapsFixture = `# Go pricing
| Model | Input | Output | Cached read | Cached write | Usage |
|-------|-------|--------|-------------|--------------|-------|
| GPT 5.6 Luna (≤ 272K tokens) | $15 | $60 | $7.50 | $15.00 | $15 |
| Grok 4.5 | $15 | $60 | $7.50 | $15.00 | $60 |
| Kimi K2.7 Code | $15 | $60 | $7.50 | $15.00 | $60 |
| MiMo V2.5 Pro | $15 | $60 | $7.50 | $15.00 | $60 |
| Hy3 | $15 | $60 | $7.50 | $15.00 | $15 |
`

func TestParseOpenCodeGoUsageCaps(t *testing.T) {
	t.Run("pricing table", func(t *testing.T) {
		caps, err := parseOpenCodeGoUsageCaps([]byte(openCodeGoUsageCapsFixture))
		require.NoError(t, err)
		assert.Equal(t, map[string]int{
			"gpt-5.6-luna":   15,
			"grok-4.5":       60,
			"kimi-k2.7-code": 60,
			"mimo-v2.5-pro":  60,
			"hy3":            15,
		}, caps)
	})

	t.Run("no table", func(t *testing.T) {
		caps, err := parseOpenCodeGoUsageCaps([]byte("no pricing table here"))
		require.NoError(t, err)
		assert.Empty(t, caps)
	})
}

func TestConvertOpenCodeGoRatioData(t *testing.T) {
	caps := map[string]int{"gpt-5.6-luna": 15, "deepseek-v4-pro": 15}
	converted, err := convertOpenCodeGoRatioData([]byte(openCodeGoAPIJSONFixture), caps, 500)
	require.NoError(t, err)

	modelRatios, ok := converted["model_ratio"].(map[string]any)
	require.True(t, ok, "model_ratio missing")
	completionRatios, ok := converted["completion_ratio"].(map[string]any)
	require.True(t, ok, "completion_ratio missing")
	cacheRatios, ok := converted["cache_ratio"].(map[string]any)
	require.True(t, ok, "cache_ratio missing")

	t.Run("cap 15 multiplies ratio by 4", func(t *testing.T) {
		// input 15 × USD/1000 × (60/15) = 15 × 0.5 × 4 = 30
		assert.InDelta(t, 30.0, modelRatios["gpt-5.6-luna"].(float64), 1e-9)
		assert.InDelta(t, 4.0, completionRatios["gpt-5.6-luna"].(float64), 1e-9)
		assert.InDelta(t, 0.5, cacheRatios["gpt-5.6-luna"].(float64), 1e-9)
	})

	t.Run("missing cap defaults to 60", func(t *testing.T) {
		// input 1 × 0.5 × 1 = 0.5
		assert.InDelta(t, 0.5, modelRatios["no-cap-model"].(float64), 1e-9)
		assert.InDelta(t, 2.0, completionRatios["no-cap-model"].(float64), 1e-9)
		assert.InDelta(t, 0.5, cacheRatios["no-cap-model"].(float64), 1e-9)
	})

	t.Run("cache ratio precision preserves restored price", func(t *testing.T) {
		// deepseek-v4-pro: cache_read 0.003625 / input 0.435 = 0.00833333...(循环小数)
		// 前端还原缓存价 = model_ratio × cache_ratio × 1000，必须接近
		// 0.003625 × 4(系数) × USD/1000 × 1000 = 7.25，不能因比值舍入产生可见误差
		modelRatio := modelRatios["deepseek-v4-pro"].(float64)
		cacheRatio := cacheRatios["deepseek-v4-pro"].(float64)
		restored := modelRatio * cacheRatio * 1000
		assert.InDelta(t, 7.25, restored, 1e-6)
		// 若按 6 位舍入，restored 会是 7.24999971，差 2.9e-7 < 1e-6 仍通过，
		// 因此再断言比值本身保留 12 位精度（0.008333333333 而非 0.008333）
		assert.InDelta(t, 0.008333333333, cacheRatio, 1e-11)
	})

	t.Run("free model ratio is zero without NaN", func(t *testing.T) {
		// -free 合并模型与免费覆盖条目均应为 0
		assert.InDelta(t, 0.0, modelRatios["hy3-free"].(float64), 1e-9)
		assert.InDelta(t, 0.0, modelRatios["shared-free"].(float64), 1e-9)
		_, hasCompletion := completionRatios["hy3-free"]
		assert.False(t, hasCompletion)
		_, hasCache := cacheRatios["hy3-free"]
		assert.False(t, hasCache)
	})

	t.Run("no NaN or Inf values", func(t *testing.T) {
		for _, m := range []map[string]any{modelRatios, completionRatios, cacheRatios} {
			for _, v := range m {
				f, ok := v.(float64)
				require.True(t, ok)
				assert.False(t, math.IsNaN(f), "unexpected NaN")
				assert.False(t, math.IsInf(f, 0), "unexpected Inf")
			}
		}
	})

	t.Run("deprecated and non-free entries excluded", func(t *testing.T) {
		assert.NotContains(t, modelRatios, "deprecated-model")
		assert.NotContains(t, modelRatios, "paid-x")
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := convertOpenCodeGoRatioData([]byte("{not json"), caps, 500)
		require.Error(t, err)
	})
}

const openCodeGoBalancePageFixture = `
<script>
const usage = {monthlyUsage:$R[0]={status:"active",resetInSec:86400,usagePercent:40}};
const rewardAmount:500;
const rewards:$R[1]=[{id:"1",source:"invite",status:"applied",email:"a@b.com",amount:500},{id:"2",source:"signup",status:"pending",email:"b@c.com",amount:500}]);
</script>
`

func TestParseOpenCodeGoBalancePage(t *testing.T) {
	t.Run("valid page", func(t *testing.T) {
		balance, err := parseOpenCodeGoBalancePage(openCodeGoBalancePageFixture)
		require.NoError(t, err)
		// 60 × (1 − 0.4) + 1 × 500/100 = 36 + 5 = 41
		assert.InDelta(t, 41.0, balance, 1e-9)
	})

	t.Run("missing usage percent", func(t *testing.T) {
		_, err := parseOpenCodeGoBalancePage("<html>no usage data</html>")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "无法从 opencode 页面解析用量数据")
	})

	t.Run("missing rewards", func(t *testing.T) {
		// 页面无奖励段（用户无邀请奖励）时按 0 个奖励计算，不报错
		html := `{monthlyUsage:$R[0]={status:"active",resetInSec:86400,usagePercent:40}};`
		balance, err := parseOpenCodeGoBalancePage(html)
		require.NoError(t, err)
		assert.InDelta(t, 36.0, balance, 1e-9)
	})

	t.Run("empty rewards list", func(t *testing.T) {
		html := `{monthlyUsage:$R[0]={status:"active",resetInSec:86400,usagePercent:40}};rewards:$R[1]=[]);`
		balance, err := parseOpenCodeGoBalancePage(html)
		require.NoError(t, err)
		assert.InDelta(t, 36.0, balance, 1e-9)
	})

	t.Run("reward amount defaults to 500", func(t *testing.T) {
		html := strings.Replace(openCodeGoBalancePageFixture, "const rewardAmount:500;", "", 1)
		balance, err := parseOpenCodeGoBalancePage(html)
		require.NoError(t, err)
		assert.InDelta(t, 41.0, balance, 1e-9)
	})

	t.Run("custom reward amount", func(t *testing.T) {
		html := strings.Replace(openCodeGoBalancePageFixture, "const rewardAmount:500;", "const rewardAmount:1000;", 1)
		balance, err := parseOpenCodeGoBalancePage(html)
		require.NoError(t, err)
		// 60 × (1 − 0.4) + 1 × 1000/100 = 36 + 10 = 46
		assert.InDelta(t, 46.0, balance, 1e-9)
	})
}
