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
	// 优先选择 monthly 窗口
	assert.Equal(t, "monthly", info.Period)
	assert.Equal(t, float64(30), info.UsedPercent)
	assert.Equal(t, float64(70), info.RemainingPercent)
	assert.Equal(t, resetAt.UTC().Format(time.RFC3339), info.ResetAt)
	// 解析发生在 resetAt 之前，remaining 为 resetAt 与 now 的差值（上限 3 小时）
	assert.InDelta(t, 3*3600, info.ResetInSec, 60)
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
	assert.Equal(t, "monthly", info.Period)
	assert.Equal(t, float64(10), info.UsedPercent)
	assert.Equal(t, float64(90), info.RemainingPercent)
}

func TestParseVolcCodingPlanUsagePrefersMonthlyOverWeekly(t *testing.T) {
	body := []byte(`{
		"Result": {
			"QuotaUsage": [
				{"Level": "weekly", "Percent": 80},
				{"Level": "session", "Percent": 99},
				{"Level": "monthly", "Percent": 25}
			]
		}
	}`)

	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	assert.Equal(t, "monthly", info.Period)
	assert.Equal(t, float64(25), info.UsedPercent)
	assert.Equal(t, float64(75), info.RemainingPercent)
}

func TestParseVolcCodingPlanUsageFallsBackToWeeklyWhenNoMonthly(t *testing.T) {
	body := []byte(`{
		"Result": {"QuotaUsage": [{"Level": "session", "Percent": 60}, {"Level": "weekly", "Percent": 40}]}
	}`)

	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	assert.Equal(t, "weekly", info.Period)
	assert.Equal(t, float64(40), info.UsedPercent)
	assert.Equal(t, float64(60), info.RemainingPercent)
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
				strconv.FormatFloat(tc.percent, 'f', -1, 64) + `, "Cap": 100}]}}`)
			info, err := ParseVolcCodingPlanUsage(body)
			require.NoError(t, err)
			assert.Equal(t, tc.wantRemaining, info.RemainingPercent)
		})
	}
}

func TestParseVolcCodingPlanUsageMissingCapDefaultsTo100(t *testing.T) {
	body := []byte(`{"Result": {"QuotaUsage": [{"Level": "monthly", "Percent": 30}]}}`)
	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	assert.Equal(t, float64(70), info.RemainingPercent)
}

func TestParseVolcCodingPlanUsageResetTimestampMilliseconds(t *testing.T) {
	// 毫秒级时间戳（>=1e12）自动换算为秒
	resetMs := time.Now().Add(2 * time.Hour).UnixMilli()
	body := []byte(`{"Result": {"QuotaUsage": [{"Level": "monthly", "Percent": 10, "ResetTimestamp": ` +
		strconv.FormatInt(resetMs, 10) + `}]}}`)
	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	assert.Equal(t, int64(7200), info.ResetInSec)
	assert.Equal(t, time.Unix(resetMs/1000, 0).UTC().Format(time.RFC3339), info.ResetAt)
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
