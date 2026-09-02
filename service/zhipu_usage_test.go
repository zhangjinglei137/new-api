package service

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func zhipuTestResetMs(t *testing.T, fromNow time.Duration) int64 {
	t.Helper()
	return time.Now().Add(fromNow).UnixMilli()
}

func TestParseZhipuCodingPlanUsageStandardResponse(t *testing.T) {
	resetSession := zhipuTestResetMs(t, 1*time.Hour)
	resetWeekly := zhipuTestResetMs(t, 3*time.Hour)
	resetMonthly := zhipuTestResetMs(t, 2*time.Hour)
	body := []byte(fmt.Sprintf(`{
		"code": 200,
		"msg": "操作成功",
		"data": {
			"level": "pro",
			"limits": [
				{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 1, "nextResetTime": %d},
				{"type": "TOKENS_LIMIT", "unit": 6, "number": 1, "percentage": 44, "nextResetTime": %d},
				{"type": "TIME_LIMIT", "unit": 5, "usage": 100, "currentValue": 0, "remaining": 100, "percentage": 0, "nextResetTime": %d}
			]
		},
		"success": true
	}`, resetSession, resetWeekly, resetMonthly))

	info, err := parseZhipuCodingPlanUsage(body)
	require.NoError(t, err)
	assert.Equal(t, "pro", info.Level)
	// 窗口顺序固定为 session -> weekly -> monthly
	require.Len(t, info.Windows, 3)
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, float64(1), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(99), info.Windows[0].RemainingPercent)
	assert.Equal(t, time.UnixMilli(resetSession).UTC().Format(time.RFC3339), info.Windows[0].ResetAt)
	assert.InDelta(t, 3600, info.Windows[0].ResetInSec, 60)

	assert.Equal(t, "weekly", info.Windows[1].Period)
	assert.Equal(t, float64(44), info.Windows[1].UsedPercent)
	assert.Equal(t, float64(56), info.Windows[1].RemainingPercent)
	assert.Equal(t, time.UnixMilli(resetWeekly).UTC().Format(time.RFC3339), info.Windows[1].ResetAt)
	assert.InDelta(t, 3*3600, info.Windows[1].ResetInSec, 60)

	assert.Equal(t, "monthly", info.Windows[2].Period)
	assert.Equal(t, float64(0), info.Windows[2].UsedPercent)
	assert.Equal(t, float64(100), info.Windows[2].RemainingPercent)
	assert.Equal(t, time.UnixMilli(resetMonthly).UTC().Format(time.RFC3339), info.Windows[2].ResetAt)
	assert.InDelta(t, 2*3600, info.Windows[2].ResetInSec, 60)
}

func TestParseZhipuCodingPlanUsageCreditLimitNoTimeLimit(t *testing.T) {
	// 新套餐 level=pro 用 CREDIT_LIMIT 且无 TIME_LIMIT：只识别 session + weekly。
	body := []byte(`{
		"data": {
			"level": "pro",
			"limits": [
				{"type": "CREDIT_LIMIT", "unit": 3, "number": 5, "percentage": 20},
				{"type": "CREDIT_LIMIT", "unit": 6, "number": 1, "percentage": 30}
			]
		}
	}`)
	info, err := parseZhipuCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 2)
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, float64(20), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(80), info.Windows[0].RemainingPercent)
	assert.Equal(t, "weekly", info.Windows[1].Period)
	assert.Equal(t, float64(30), info.Windows[1].UsedPercent)
	assert.Equal(t, float64(70), info.Windows[1].RemainingPercent)
}

func TestParseZhipuCodingPlanUsageOnlySession(t *testing.T) {
	// 旧套餐可能只有 session 无 weekly；缺失窗口不出现，不补 0% 假窗口。
	body := []byte(`{"data": {"limits": [
		{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 10}
	]}}`)
	info, err := parseZhipuCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, float64(10), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(90), info.Windows[0].RemainingPercent)
}

func TestParseZhipuCodingPlanUsagePercentageFallback(t *testing.T) {
	// percentage 缺失时用 currentValue/usage*100 兜底（usage 为分母）。
	body := []byte(`{"data": {"limits": [
		{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "usage": "100", "currentValue": "25"}
	]}}`)
	info, err := parseZhipuCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, float64(25), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(75), info.Windows[0].RemainingPercent)
}

func TestParseZhipuCodingPlanUsageTimeLimitNotMisclassified(t *testing.T) {
	// TIME_LIMIT 的 unit 也可能是 5，不能仅凭 unit 误分类；unit==3 但 number!=5
	// 的 TOKENS_LIMIT 行同样被忽略。
	body := []byte(`{"data": {"limits": [
		{"type": "TIME_LIMIT", "unit": 5, "number": 5, "usage": 100, "currentValue": 50},
		{"type": "TOKENS_LIMIT", "unit": 3, "number": 6, "percentage": 90},
		{"type": "TOKENS_LIMIT", "unit": 5, "number": 5, "percentage": 80},
		{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 10}
	]}}`)
	info, err := parseZhipuCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 2)
	// session 由 unit=3 number=5 命中；TIME_LIMIT(unit=5) 归为 monthly 而非 session
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, float64(10), info.Windows[0].UsedPercent)
	assert.Equal(t, "monthly", info.Windows[1].Period)
	assert.Equal(t, float64(50), info.Windows[1].UsedPercent)
	assert.Equal(t, float64(50), info.Windows[1].RemainingPercent)
}

func TestParseZhipuCodingPlanUsageResetMillisecondTimestamp(t *testing.T) {
	resetMs := time.Now().Add(2 * time.Hour).UnixMilli()
	body := []byte(`{"data": {"limits": [
		{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 10, "nextResetTime": ` +
		strconv.FormatInt(resetMs, 10) + `}
	]}}`)
	info, err := parseZhipuCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, time.UnixMilli(resetMs).UTC().Format(time.RFC3339), info.Windows[0].ResetAt)
	assert.InDelta(t, 2*3600, info.Windows[0].ResetInSec, 60)

	// 毫秒时间戳也可为字符串
	body = []byte(`{"data": {"limits": [
		{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 10, "nextResetTime": "` +
		strconv.FormatInt(resetMs, 10) + `"}
	]}}`)
	info, err = parseZhipuCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, time.UnixMilli(resetMs).UTC().Format(time.RFC3339), info.Windows[0].ResetAt)
}

func TestParseZhipuCodingPlanUsageResetExpiredIgnored(t *testing.T) {
	// 已过期的 nextResetTime：ResetAt 保留格式化值，ResetInSec 置 0。
	expiredMs := time.Now().Add(-1 * time.Hour).UnixMilli()
	body := []byte(`{"data": {"limits": [
		{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 10, "nextResetTime": ` +
		strconv.FormatInt(expiredMs, 10) + `}
	]}}`)
	info, err := parseZhipuCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, time.UnixMilli(expiredMs).UTC().Format(time.RFC3339), info.Windows[0].ResetAt)
	assert.Equal(t, int64(0), info.Windows[0].ResetInSec)

	// nextResetTime 无效（<=0 或不可解析）时窗口仍返回，ResetAt/ResetInSec 保持零值。
	for _, reset := range []string{"0", "-1", `"not-a-time"`} {
		body := []byte(`{"data": {"limits": [
			{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 10, "nextResetTime": ` + reset + `}
		]}}`)
		info, err := parseZhipuCodingPlanUsage(body)
		require.NoError(t, err, "reset: %s", reset)
		require.Len(t, info.Windows, 1)
		assert.Empty(t, info.Windows[0].ResetAt)
		assert.Equal(t, int64(0), info.Windows[0].ResetInSec)
	}
}

func TestParseZhipuCodingPlanUsageSkipsUnusableWindow(t *testing.T) {
	// percentage 缺失且无法用 usage 兜底（usage 缺失或为 0）的窗口被跳过。
	body := []byte(`{"data": {"limits": [
		{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 0},
		{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "currentValue": 5},
		{"type": "TIME_LIMIT", "unit": 5, "usage": 0, "currentValue": 5}
	]}}`)
	info, err := parseZhipuCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, float64(0), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(100), info.Windows[0].RemainingPercent)
}

func TestParseZhipuCodingPlanUsageClampsPercent(t *testing.T) {
	for _, tc := range []struct {
		name          string
		percentage    float64
		wantUsed      float64
		wantRemaining float64
	}{
		{name: "percent zero", percentage: 0, wantUsed: 0, wantRemaining: 100},
		{name: "percent full", percentage: 100, wantUsed: 100, wantRemaining: 0},
		{name: "percent over cap", percentage: 150, wantUsed: 100, wantRemaining: 0},
		{name: "percent negative", percentage: -10, wantUsed: 0, wantRemaining: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"data": {"limits": [
				{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": ` +
				strconv.FormatFloat(tc.percentage, 'f', -1, 64) + `}
			]}}`)
			info, err := parseZhipuCodingPlanUsage(body)
			require.NoError(t, err)
			require.Len(t, info.Windows, 1)
			assert.Equal(t, tc.wantUsed, info.Windows[0].UsedPercent)
			assert.Equal(t, tc.wantRemaining, info.Windows[0].RemainingPercent)
		})
	}
}

func TestParseZhipuCodingPlanUsageNoWindowsReturnsError(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"data": {}}`,
		`{"data": {"limits": []}}`,
		`{"data": {"limits": [{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "currentValue": 5}]}}`,
		`{"data": {"limits": [{"type": "FOO_LIMIT", "unit": 3, "number": 5, "percentage": 10}]}}`,
		`not-json`,
	} {
		_, err := parseZhipuCodingPlanUsage([]byte(body))
		require.Error(t, err, "body: %s", body)
	}
}

func TestZhipuCodingPlanUsageEndpointRegionAuth(t *testing.T) {
	// 国内 bigmodel：无 Bearer 前缀；国际 api.z.ai：Bearer 前缀。
	url, useBearer, ok := zhipuCodingPlanUsageEndpoint("glm-coding-plan")
	require.True(t, ok)
	assert.Equal(t, "https://open.bigmodel.cn/api/monitor/usage/quota/limit", url)
	assert.False(t, useBearer)

	url, useBearer, ok = zhipuCodingPlanUsageEndpoint("glm-coding-plan-international")
	require.True(t, ok)
	assert.Equal(t, "https://api.z.ai/api/monitor/usage/quota/limit", url)
	assert.True(t, useBearer)

	_, _, ok = zhipuCodingPlanUsageEndpoint("kimi-coding-plan")
	assert.False(t, ok)
	_, _, ok = zhipuCodingPlanUsageEndpoint("")
	assert.False(t, ok)
}
