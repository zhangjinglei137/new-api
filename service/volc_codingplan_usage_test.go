package service

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVolcCodingPlanUsageStandardResponse(t *testing.T) {
	resetAt := time.Now().Add(3 * time.Hour).Truncate(time.Second)
	body := []byte(`{
		"ResponseMetadata": {"RequestId": "req-1"},
		"Result": {
			"Status": "Running",
			"UpdateTimestamp": 1700000000,
			"QuotaUsage": [
				{"Level": "session", "Percent": 50, "Cap": 100, "ResetTimestamp": ` + strconv.FormatInt(resetAt.Unix(), 10) + `},
				{"Level": "weekly", "Percent": 20, "Cap": 100, "ResetTimestamp": ` + strconv.FormatInt(resetAt.Unix(), 10) + `},
				{"Level": "monthly", "Percent": 30, "Cap": 100, "ResetTimestamp": ` + strconv.FormatInt(resetAt.Unix(), 10) + `}
			]
		}
	}`)

	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	assert.Equal(t, "Running", info.Status)
	require.Len(t, info.Windows, 3)
	// 全部窗口返回，顺序固定为 session -> weekly -> monthly
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, float64(50), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(50), info.Windows[0].RemainingPercent)
	assert.Equal(t, "weekly", info.Windows[1].Period)
	assert.Equal(t, float64(20), info.Windows[1].UsedPercent)
	assert.Equal(t, float64(80), info.Windows[1].RemainingPercent)
	assert.Equal(t, "monthly", info.Windows[2].Period)
	assert.Equal(t, float64(30), info.Windows[2].UsedPercent)
	assert.Equal(t, float64(70), info.Windows[2].RemainingPercent)
	assert.Equal(t, resetAt.UTC().Format(time.RFC3339), info.Windows[2].ResetAt)
	// 解析发生在 resetAt 之前，remaining 为 resetAt 与 now 的差值（上限 3 小时）
	assert.InDelta(t, 3*3600, info.Windows[2].ResetInSec, 60)
}

func TestParseVolcCodingPlanUsageAliasResponse(t *testing.T) {
	// 别名回退：Usages/Details、UsedPercent/UsagePercent、Type/Period、ResetTime
	body := []byte(`{
		"Result": {
			"Status": "Active",
			"Usages": [
				{"Type": "monthly", "UsedPercent": 10, "ResetTime": 1700004000}
			]
		}
	}`)

	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	assert.Equal(t, "Active", info.Status)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, "monthly", info.Windows[0].Period)
	assert.Equal(t, float64(10), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(90), info.Windows[0].RemainingPercent)
}

func TestParseVolcCodingPlanUsageOnlyMonthly(t *testing.T) {
	// 缺失的窗口不出现，不补默认
	body := []byte(`{"Result": {"QuotaUsage": [{"Level": "monthly", "Percent": 25}]}}`)

	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, "monthly", info.Windows[0].Period)
	assert.Equal(t, float64(25), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(75), info.Windows[0].RemainingPercent)
}

func TestParseVolcCodingPlanUsageSkipsUnknownLevel(t *testing.T) {
	// 未识别 Level 条目被跳过，不影响其余窗口
	body := []byte(`{
		"Result": {"QuotaUsage": [
			{"Level": "daily", "Percent": 90},
			{"Level": "yearly", "Percent": 5},
			{"Level": "", "Percent": 50},
			{"Level": "weekly", "Percent": 40}
		]}
	}`)

	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, "weekly", info.Windows[0].Period)
	assert.Equal(t, float64(40), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(60), info.Windows[0].RemainingPercent)
}

func TestParseVolcCodingPlanUsageAllUnknownLevelsReturnsError(t *testing.T) {
	body := []byte(`{"Result": {"QuotaUsage": [{"Level": "daily", "Percent": 90}]}}`)

	_, err := ParseVolcCodingPlanUsage(body)
	require.Error(t, err)
}

func TestParseVolcCodingPlanUsageRoundsPercentToTwoDecimals(t *testing.T) {
	body := []byte(`{
		"Result": {"QuotaUsage": [
			{"Level": "session", "Percent": 33.33333},
			{"Level": "weekly", "Percent": 66.66666},
			{"Level": "monthly", "Percent": 12.34567}
		]}
	}`)

	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 3)
	assert.Equal(t, 33.33, info.Windows[0].UsedPercent)
	assert.Equal(t, 66.67, info.Windows[0].RemainingPercent)
	assert.Equal(t, 66.67, info.Windows[1].UsedPercent)
	assert.Equal(t, 33.33, info.Windows[1].RemainingPercent)
	assert.Equal(t, 12.35, info.Windows[2].UsedPercent)
	assert.Equal(t, 87.65, info.Windows[2].RemainingPercent)
}

func TestParseVolcCodingPlanUsageClampsRemaining(t *testing.T) {
	for _, tc := range []struct {
		name          string
		percent       float64
		wantRemaining float64
	}{
		{name: "percent zero", percent: 0, wantRemaining: 100},
		{name: "percent full", percent: 100, wantRemaining: 0},
		{name: "percent over cap", percent: 150, wantRemaining: 0},
		{name: "percent negative", percent: -10, wantRemaining: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"Result": {"QuotaUsage": [{"Level": "monthly", "Percent": ` +
				strconv.FormatFloat(tc.percent, 'f', -1, 64) + `}]}}`)
			info, err := ParseVolcCodingPlanUsage(body)
			require.NoError(t, err)
			require.Len(t, info.Windows, 1)
			assert.Equal(t, tc.wantRemaining, info.Windows[0].RemainingPercent)
		})
	}
}

func TestParseVolcCodingPlanUsageResetTimestampMilliseconds(t *testing.T) {
	// 毫秒级时间戳（>=1e12）自动换算为秒
	resetMs := time.Now().Add(2 * time.Hour).UnixMilli()
	body := []byte(`{"Result": {"QuotaUsage": [{"Level": "monthly", "Percent": 10, "ResetTimestamp": ` +
		strconv.FormatInt(resetMs, 10) + `}]}}`)
	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, int64(7200), info.Windows[0].ResetInSec)
	assert.Equal(t, time.Unix(resetMs/1000, 0).UTC().Format(time.RFC3339), info.Windows[0].ResetAt)
}

func TestParseVolcCodingPlanUsageNoWindowsReturnsError(t *testing.T) {
	for _, body := range []string{
		`{"Result": {}}`,
		`{"Result": {"QuotaUsage": []}}`,
		`{"ResponseMetadata": {}}`,
		`not-json`,
	} {
		_, err := ParseVolcCodingPlanUsage([]byte(body))
		require.Error(t, err)
	}
}

func TestRegionFromVolcBaseURL(t *testing.T) {
	for _, tc := range []struct {
		baseURL string
		region  string
		ok      bool
	}{
		{baseURL: "https://ark.cn-beijing.volces.com/api/coding", region: "cn-beijing", ok: true},
		{baseURL: "ark.cn-beijing.volces.com", region: "cn-beijing", ok: true},
		{baseURL: "https://ark.ap-southeast.bytepluses.com/api/coding", region: "ap-southeast", ok: true},
		{baseURL: "ark.ap-southeast.bytepluses.com", region: "ap-southeast", ok: true},
		{baseURL: "https://api.openai.com", ok: false},
		{baseURL: "https://ark.cn-hangzhou.volces.com/api/coding", ok: false},
		{baseURL: "", ok: false},
	} {
		region, ok := RegionFromVolcBaseURL(tc.baseURL)
		assert.Equal(t, tc.ok, ok)
		assert.Equal(t, tc.region, region)
	}
}
