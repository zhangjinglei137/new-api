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
      "gpt-5.6-luna": {"id": "gpt-5.6-luna", "name": "GPT 5.6 Luna", "description": "GPT 5.6 Luna official model", "provider": {"npm": "@ai-sdk/openai"}, "status": "", "cost": {"input": 15, "output": 60, "cache_read": 7.5}},
      "no-cap-model": {"description": "A model without usage cap", "status": "", "cost": {"input": 1, "output": 2, "cache_read": 0.5}},
      "deepseek-v4-pro": {"name": "DeepSeek V4 Pro", "description": "DeepSeek V4 Pro model", "status": "", "cost": {"input": 0.435, "output": 0.87, "cache_read": 0.003625}},
      "shared-free": {"name": "Shared", "description": "shared paid description", "provider": {"npm": "@ai-sdk/openai"}, "status": "", "cost": {"input": 10, "output": 20}},
      "deprecated-model": {"name": "Deprecated", "description": "deprecated model", "status": "deprecated", "cost": {"input": 1, "output": 2}}
    }
  },
  "opencode": {
    "models": {
      "hy3-free": {"name": "Hy3 Free", "description": "Hy3 free tier", "provider": {"npm": "@ai-sdk/anthropic"}, "status": "", "cost": {"input": 0, "output": 0}},
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
		assert.Equal(t, "@ai-sdk/openai", entries[1].Provider)
		// -free 模型描述优先取 -free 条目
		assert.Equal(t, "hy3-free", entries[2].ID)
		assert.Equal(t, "Hy3 Free", entries[2].Name)
		assert.Equal(t, "Hy3 free tier", entries[2].Description)
		assert.Equal(t, "@ai-sdk/anthropic", entries[2].Provider)
		// name 缺失时回退 id；provider 缺失时为空
		assert.Equal(t, "no-cap-model", entries[3].ID)
		assert.Equal(t, "no-cap-model", entries[3].Name)
		assert.Equal(t, "A model without usage cap", entries[3].Description)
		assert.Equal(t, "", entries[3].Provider)
		// 同 id 时 -free 免费条目覆盖（含 name/description）；provider 缺失时回退原条目值
		assert.Equal(t, "shared-free", entries[4].ID)
		assert.Equal(t, "Shared Free", entries[4].Name)
		assert.Equal(t, "shared free description", entries[4].Description)
		assert.Equal(t, "@ai-sdk/openai", entries[4].Provider)
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

func TestConvertOpenCodeGoRatioData(t *testing.T) {
	converted, err := convertOpenCodeGoRatioData([]byte(openCodeGoAPIJSONFixture), 500)
	require.NoError(t, err)

	modelRatios, ok := converted["model_ratio"].(map[string]any)
	require.True(t, ok, "model_ratio missing")
	completionRatios, ok := converted["completion_ratio"].(map[string]any)
	require.True(t, ok, "completion_ratio missing")
	cacheRatios, ok := converted["cache_ratio"].(map[string]any)
	require.True(t, ok, "cache_ratio missing")

	t.Run("no cap multiplier, api.json price used as-is", func(t *testing.T) {
		// input 15 × USD/1000 = 15 × 0.5 = 7.5（无额度系数）
		assert.InDelta(t, 7.5, modelRatios["gpt-5.6-luna"].(float64), 1e-9)
		assert.InDelta(t, 4.0, completionRatios["gpt-5.6-luna"].(float64), 1e-9)
		assert.InDelta(t, 0.5, cacheRatios["gpt-5.6-luna"].(float64), 1e-9)
	})

	t.Run("cache ratio precision preserves restored price", func(t *testing.T) {
		// deepseek-v4-pro: cache_read 0.003625 / input 0.435 = 0.00833333...(循环小数)
		// 前端还原缓存价 = model_ratio × cache_ratio × 1000，必须接近
		// 0.003625 × USD/1000 × 1000 = 1.8125，不能因比值舍入产生可见误差
		modelRatio := modelRatios["deepseek-v4-pro"].(float64)
		assert.InDelta(t, 0.2175, modelRatio, 1e-9)
		cacheRatio := cacheRatios["deepseek-v4-pro"].(float64)
		restored := modelRatio * cacheRatio * 1000
		assert.InDelta(t, 1.8125, restored, 1e-6)
		// 若按 6 位舍入，restored 会差 2.9e-7 < 1e-6 仍通过，
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
		_, err := convertOpenCodeGoRatioData([]byte("{not json"), 500)
		require.Error(t, err)
	})
}

// openCodeGoBalancePageFixture 对应当前 opencode 工作区页面（2026-08 抓取）：
// usagePercent 为浮点、status 为 "ok"、rewards 段以 ]}) 结尾（位于 referral 对象内）。
const openCodeGoBalancePageFixture = `
<script>
const workspace = {mine:!0,useBalance:!0,allowTraining:!1,region:$R[32]=["us","eu","sg","cn"],rollingUsage:$R[33]={status:"ok",resetInSec:3811,usagePercent:23.6,usage:283070516,limit:1200000000},weeklyUsage:$R[34]={status:"ok",resetInSec:397146,usagePercent:81.2,usage:2435686434,limit:3000000000},monthlyUsage:$R[35]={status:"ok",resetInSec:1615573,usagePercent:65.8,usage:3950635351,limit:6000000000}});
const referral = {referralCode:"Z70E54QECG",hasReferral:!0,rewardAmount:500,rewards:$R[40]=[{id:"ref_01M0BRG1FNH1FAPQGX92FW6YCA",source:"inviter",status:"available",email:"a@example.com",amount:500,timeCreated:new Date("2026-08-19T01:07:50.000Z"),timeApplied:null},{id:"ref_01KZZ1C8WKB0W7C2YMX76M562F",source:"invitee",status:"available",email:"b@example.com",amount:500,timeCreated:new Date("2026-08-14T02:27:08.000Z"),timeApplied:null}]});
</script>
`

// openCodeGoLegacyBalancePageFixture 对应旧版工作区页面：usagePercent 为整数、
// status 为 "active"、rewards 段以 ]) 结尾，用于兼容性回归。
const openCodeGoLegacyBalancePageFixture = `
<script>
const usage = {monthlyUsage:$R[0]={status:"active",resetInSec:86400,usagePercent:40}};
const rewardAmount:500;
const rewards:$R[1]=[{id:"1",source:"invite",status:"applied",email:"a@b.com",amount:500},{id:"2",source:"signup",status:"pending",email:"b@c.com",amount:500}]);
</script>
`

func TestParseOpenCodeGoBalancePage(t *testing.T) {
	t.Run("valid current page", func(t *testing.T) {
		balance, err := parseOpenCodeGoBalancePage(openCodeGoBalancePageFixture)
		require.NoError(t, err)
		// 60 × (1 − 0.658) + 2 × 500/100 = 20.52 + 10 = 30.52
		assert.InDelta(t, 30.52, balance, 1e-9)
	})

	t.Run("legacy page format", func(t *testing.T) {
		balance, err := parseOpenCodeGoBalancePage(openCodeGoLegacyBalancePageFixture)
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
		html := `{monthlyUsage:$R[0]={status:"ok",resetInSec:1615573,usagePercent:65.8}};`
		balance, err := parseOpenCodeGoBalancePage(html)
		require.NoError(t, err)
		assert.InDelta(t, 20.52, balance, 1e-9)
	})

	t.Run("empty rewards list", func(t *testing.T) {
		html := `{monthlyUsage:$R[0]={status:"ok",resetInSec:1615573,usagePercent:65.8}};rewards:$R[1]=[]);`
		balance, err := parseOpenCodeGoBalancePage(html)
		require.NoError(t, err)
		assert.InDelta(t, 20.52, balance, 1e-9)
	})

	t.Run("reward amount defaults to 500", func(t *testing.T) {
		html := strings.Replace(openCodeGoBalancePageFixture, "rewardAmount:500,", "", 1)
		balance, err := parseOpenCodeGoBalancePage(html)
		require.NoError(t, err)
		assert.InDelta(t, 30.52, balance, 1e-9)
	})

	t.Run("custom reward amount", func(t *testing.T) {
		html := strings.Replace(openCodeGoBalancePageFixture, "rewardAmount:500,", "rewardAmount:1000,", 1)
		balance, err := parseOpenCodeGoBalancePage(html)
		require.NoError(t, err)
		// 60 × (1 − 0.658) + 2 × 1000/100 = 20.52 + 20 = 40.52
		assert.InDelta(t, 40.52, balance, 1e-9)
	})

	t.Run("integer usage percent", func(t *testing.T) {
		// 整数百分比（旧格式）也应由浮点正则解析
		html := `{monthlyUsage:$R[0]={status:"active",resetInSec:86400,usagePercent:40}};`
		balance, err := parseOpenCodeGoBalancePage(html)
		require.NoError(t, err)
		assert.InDelta(t, 36.0, balance, 1e-9)
	})

	t.Run("usage percent clamped to 100", func(t *testing.T) {
		html := `{monthlyUsage:$R[0]={status:"ok",resetInSec:1615573,usagePercent:123.7}};`
		balance, err := parseOpenCodeGoBalancePage(html)
		require.NoError(t, err)
		assert.InDelta(t, 0.0, balance, 1e-9)
	})
}

func TestParseOpenCodeGoUsagePage(t *testing.T) {
	tests := []struct {
		name            string
		html            string
		wantUsage       float64
		wantRemaining   float64
		wantBalance     float64
		wantResetInSec  int64
		wantErrContains string
	}{
		{
			name:           "current page float percent + resetInSec",
			html:           openCodeGoBalancePageFixture,
			wantUsage:      65.8,
			wantRemaining:  34.2,
			wantBalance:    30.52,
			wantResetInSec: 1615573,
		},
		{
			name:           "legacy page integer percent",
			html:           openCodeGoLegacyBalancePageFixture,
			wantUsage:      40,
			wantRemaining:  60,
			wantBalance:    41,
			wantResetInSec: 86400,
		},
		{
			name:           "missing rewards still returns usage",
			html:           `{monthlyUsage:$R[0]={status:"ok",resetInSec:1615573,usagePercent:65.8}};`,
			wantUsage:      65.8,
			wantRemaining:  34.2,
			wantBalance:    20.52,
			wantResetInSec: 1615573,
		},
		{
			name:           "usage percent clamped to 100",
			html:           `{monthlyUsage:$R[0]={status:"ok",resetInSec:1615573,usagePercent:123.7}};`,
			wantUsage:      100,
			wantRemaining:  0,
			wantBalance:    0,
			wantResetInSec: 1615573,
		},
		{
			name:           "usage percent clamped to 0",
			html:           `{monthlyUsage:$R[0]={status:"ok",resetInSec:123,usagePercent:-5.2}};`,
			wantUsage:      0,
			wantRemaining:  100,
			wantBalance:    60,
			wantResetInSec: 123,
		},
		{
			name:            "missing usage data",
			html:            "<html>no usage data</html>",
			wantErrContains: "无法从 opencode 页面解析用量数据",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parseOpenCodeGoUsagePage(tt.html)
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tt.wantUsage, info.UsagePercent, 1e-9)
			assert.InDelta(t, tt.wantRemaining, info.RemainingPercent, 1e-9)
			assert.InDelta(t, tt.wantBalance, info.Balance, 1e-9)
			assert.Equal(t, tt.wantResetInSec, info.ResetInSec)
			assert.InDelta(t, openCodeGoMonthlyCapUSD, info.MonthlyCapUSD, 1e-9)
		})
	}
}
