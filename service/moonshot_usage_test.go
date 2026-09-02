package service

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func moonshotTestResetAt(t *testing.T, fromNow time.Duration) string {
	t.Helper()
	return time.Now().Add(fromNow).UTC().Format(time.RFC3339)
}

func TestParseMoonshotCodingPlanUsageStandardResponse(t *testing.T) {
	resetWeekly := moonshotTestResetAt(t, 3*time.Hour)
	resetSession := moonshotTestResetAt(t, 1*time.Hour)
	body := []byte(fmt.Sprintf(`{
		"usage": {"used": "40", "limit": "1000", "remaining": "960", "resetTime": "%s"},
		"limits": [
			{"window": {"duration": 300, "timeUnit": "TIME_UNIT_MINUTE"}, "detail": {"used": "1", "limit": "100", "remaining": "99", "resetTime": "%s"}}
		],
		"boosterWallet": {"balance": {"amount": "100", "amountLeft": "50"}}
	}`, resetWeekly, resetSession))

	info, err := parseMoonshotCodingPlanUsage(body)
	require.NoError(t, err)
	// 窗口顺序固定为 session -> weekly
	require.Len(t, info.Windows, 2)
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, float64(1), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(99), info.Windows[0].RemainingPercent)
	assert.Equal(t, resetSession, info.Windows[0].ResetAt)
	assert.InDelta(t, 3600, info.Windows[0].ResetInSec, 60)

	assert.Equal(t, "weekly", info.Windows[1].Period)
	assert.Equal(t, float64(4), info.Windows[1].UsedPercent)
	assert.Equal(t, float64(96), info.Windows[1].RemainingPercent)
	assert.Equal(t, resetWeekly, info.Windows[1].ResetAt)
	assert.InDelta(t, 3*3600, info.Windows[1].ResetInSec, 60)
}

func TestParseMoonshotCodingPlanUsageAliasAndFallback(t *testing.T) {
	// used 缺失时用 limit-remaining 兜底；reset_at 别名；数字可能为 number 而非 string。
	resetAt := moonshotTestResetAt(t, 2*time.Hour)
	body := []byte(fmt.Sprintf(`{
		"usage": {"limit": 200, "remaining": 150, "reset_at": "%s"},
		"limits": [
			{"window": {"duration": 300, "timeUnit": "TIME_UNIT_MINUTE"}, "detail": {"usage": 30, "quota": 100, "quotaRemaining": 70, "reset_time": "%s"}}
		]
	}`, resetAt, resetAt))

	info, err := parseMoonshotCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 2)
	// weekly: used = limit - remaining = 200 - 150 = 50 → 25%
	assert.Equal(t, "weekly", info.Windows[1].Period)
	assert.Equal(t, float64(25), info.Windows[1].UsedPercent)
	assert.Equal(t, float64(75), info.Windows[1].RemainingPercent)
	assert.Equal(t, resetAt, info.Windows[1].ResetAt)
	// session: used=30, limit=100 → 30%
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, float64(30), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(70), info.Windows[0].RemainingPercent)
	assert.InDelta(t, 2*3600, info.Windows[0].ResetInSec, 60)
}

func TestParseMoonshotCodingPlanUsageOnlyWeekly(t *testing.T) {
	body := []byte(`{"usage": {"used": "10", "limit": "100"}}`)
	info, err := parseMoonshotCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, "weekly", info.Windows[0].Period)
	assert.Equal(t, float64(10), info.Windows[0].UsedPercent)
	assert.Equal(t, float64(90), info.Windows[0].RemainingPercent)
}

func TestParseMoonshotCodingPlanUsageSkipsUnknownWindow(t *testing.T) {
	// duration != 300 的 limits 窗口被跳过，不影响 usage 周配额。
	body := []byte(`{
		"usage": {"used": "50", "limit": "100"},
		"limits": [
			{"window": {"duration": 60, "timeUnit": "TIME_UNIT_MINUTE"}, "detail": {"used": "1", "limit": "100"}},
			{"window": {"duration": 300, "timeUnit": "TIME_UNIT_MINUTE"}, "detail": {"used": "5", "limit": "10"}}
		]
	}`)
	info, err := parseMoonshotCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 2)
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, float64(50), info.Windows[0].UsedPercent)
	assert.Equal(t, "weekly", info.Windows[1].Period)
}

func TestParseMoonshotCodingPlanUsageClampsPercent(t *testing.T) {
	for _, tc := range []struct {
		name          string
		used          string
		limit         string
		wantUsed      float64
		wantRemaining float64
	}{
		{name: "zero used", used: "0", limit: "100", wantUsed: 0, wantRemaining: 100},
		{name: "full used", used: "100", limit: "100", wantUsed: 100, wantRemaining: 0},
		{name: "over limit", used: "150", limit: "100", wantUsed: 100, wantRemaining: 0},
		{name: "rounding", used: "33", limit: "100", wantUsed: 33, wantRemaining: 67},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"usage": {"used": "` + tc.used + `", "limit": "` + tc.limit + `"}}`)
			info, err := parseMoonshotCodingPlanUsage(body)
			require.NoError(t, err)
			require.Len(t, info.Windows, 1)
			assert.Equal(t, tc.wantUsed, info.Windows[0].UsedPercent)
			assert.Equal(t, tc.wantRemaining, info.Windows[0].RemainingPercent)
		})
	}
}

func TestParseMoonshotCodingPlanUsageResetNumericTimestamp(t *testing.T) {
	resetSec := time.Now().Add(2 * time.Hour).Unix()
	body := []byte(`{"usage": {"used": "10", "limit": "100", "resetTime": ` + strconv.FormatInt(resetSec, 10) + `}}`)
	info, err := parseMoonshotCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, time.Unix(resetSec, 0).UTC().Format(time.RFC3339), info.Windows[0].ResetAt)
	assert.InDelta(t, 2*3600, info.Windows[0].ResetInSec, 60)

	// 毫秒级时间戳（>=1e12）自动换算为秒
	resetMs := time.Now().Add(1 * time.Hour).UnixMilli()
	body = []byte(`{"usage": {"used": "10", "limit": "100", "resetTime": ` + strconv.FormatInt(resetMs, 10) + `}}`)
	info, err = parseMoonshotCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, time.Unix(resetMs/1000, 0).UTC().Format(time.RFC3339), info.Windows[0].ResetAt)
	assert.InDelta(t, 3600, info.Windows[0].ResetInSec, 60)
}

func TestParseMoonshotCodingPlanUsageInvalidResetIgnored(t *testing.T) {
	// reset 无法解析时窗口仍返回，ResetAt/ResetInSec 保持零值。
	body := []byte(`{"usage": {"used": "10", "limit": "100", "resetTime": "not-a-time"}}`)
	info, err := parseMoonshotCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, "weekly", info.Windows[0].Period)
	assert.Empty(t, info.Windows[0].ResetAt)
	assert.Equal(t, int64(0), info.Windows[0].ResetInSec)
}

func TestParseMoonshotCodingPlanUsageNoWindowsReturnsError(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"usage": {}}`,
		`{"usage": {"limit": "0"}}`,
		`{"limits": [{"window": {"duration": 60}}]}`,
		`{"limits": []}`,
		`not-json`,
	} {
		_, err := parseMoonshotCodingPlanUsage([]byte(body))
		require.Error(t, err, "body: %s", body)
	}
}
