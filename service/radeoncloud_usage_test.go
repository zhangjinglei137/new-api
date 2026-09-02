package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// radeonTestStandardBody 构造一段贴近官方 /api/v1/usage 实测响应的 JSON，
// 数字字段同时覆盖 number 与 string 两种类型，reset_epoch 取动态时间。
func radeonTestStandardBody(t *testing.T, resetEpoch int64) string {
	t.Helper()
	return fmt.Sprintf(`{
		"organization_id": "org-1",
		"project_id": "proj-1",
		"rpm_limit": 20,
		"daily_cost_limit_usd": 1,
		"daily_cost_used_usd": 0.00018652000289876014,
		"daily_cost_remaining_usd": 0.9998134799971012,
		"daily_reset_at": "%s",
		"daily_reset_epoch": %d,
		"period_started_at": "2026-09-02T06:37:37.282Z",
		"today": {"requests": 17, "errors": 0, "total_tokens": 615, "prompt_tokens": 373,
			"prefill_tokens": 373, "completion_tokens": 242, "reasoning_tokens": 96,
			"cached_tokens": 0, "cost": 0.00018652000289876014, "last_request_at": "2026-09-02 06:40:06.206049"},
		"last_24_hours": {"requests": 17, "errors": 0, "total_tokens": 615,
			"last_request_at": "2026-09-02 06:40:06.206049"}
	}`, time.Unix(resetEpoch, 0).UTC().Format(time.RFC3339), resetEpoch)
}

func TestParseRadeonCloudUsageStandardResponse(t *testing.T) {
	resetEpoch := time.Now().Add(3 * time.Hour).Unix()
	info, err := parseRadeonCloudUsage([]byte(radeonTestStandardBody(t, resetEpoch)))
	require.NoError(t, err)

	assert.Equal(t, 20, info.RpmLimit)
	// 1 USD = 1e6 points
	assert.Equal(t, 1000000.0, info.DailyLimitPoints)
	assert.Equal(t, 186.52, info.DailyUsedPoints)
	assert.Equal(t, 999813.48, info.DailyRemainingPoints)
	assert.Equal(t, 0.0002, info.DailyUsedPercent)

	assert.Equal(t, time.Unix(resetEpoch, 0).UTC().Format(time.RFC3339), info.DailyResetAt)
	assert.InDelta(t, 3*3600, info.DailyResetInSec, 60)

	assert.Equal(t, 17, info.TodayRequests)
	assert.Equal(t, int64(615), info.TodayTokens)
	assert.Equal(t, 17, info.Last24hRequests)
	assert.Equal(t, int64(615), info.Last24hTokens)
	assert.Equal(t, "2026-09-02 06:40:06.206049", info.Last24hLastRequestAt)
	assert.Equal(t, "2026-09-02T06:37:37.282Z", info.PeriodStartedAt)
}

func TestParseRadeonCloudUsagePointsConversionPrecision(t *testing.T) {
	// 上游 cost 是 float64 精度的 USD 值：0.00018652000289876014 * 1e6
	// 应精确保留两位小数 186.52，而不是被截断成整数 186。
	body := []byte(`{
		"daily_cost_limit_usd": 1,
		"daily_cost_used_usd": 0.00018652000289876014,
		"daily_cost_remaining_usd": 0.9998134799971012
	}`)
	info, err := parseRadeonCloudUsage(body)
	require.NoError(t, err)
	assert.Equal(t, 186.52, info.DailyUsedPoints)
	assert.Equal(t, 999813.48, info.DailyRemainingPoints)
	assert.Equal(t, 1000000.0, info.DailyLimitPoints)
	assert.Equal(t, 0.0002, info.DailyUsedPercent)
}

func TestParseRadeonCloudUsageMissingFieldsDefaultZero(t *testing.T) {
	// 字段缺失或结构不完整时全部兜底为 0，不报错。
	assertAllZero := func(t *testing.T, info *RadeonCloudUsageInfo) {
		t.Helper()
		assert.Zero(t, info.RpmLimit)
		assert.Zero(t, info.DailyLimitPoints)
		assert.Zero(t, info.DailyUsedPoints)
		assert.Zero(t, info.DailyRemainingPoints)
		assert.Zero(t, info.DailyUsedPercent)
		assert.Zero(t, info.DailyResetInSec)
		assert.Empty(t, info.DailyResetAt)
		assert.Zero(t, info.TodayRequests)
		assert.Zero(t, info.TodayTokens)
		assert.Zero(t, info.Last24hRequests)
		assert.Zero(t, info.Last24hTokens)
		assert.Empty(t, info.Last24hLastRequestAt)
		assert.Empty(t, info.PeriodStartedAt)
	}

	info, err := parseRadeonCloudUsage([]byte(`{}`))
	require.NoError(t, err)
	assertAllZero(t, info)

	// 窗口对象存在但字段缺失，或类型不兼容（数组/字符串），同样兜底为 0。
	for _, body := range []string{
		`{"today": {}, "last_24_hours": {}}`,
		`{"today": [], "last_24_hours": "x"}`,
	} {
		info, err := parseRadeonCloudUsage([]byte(body))
		require.NoError(t, err, "body: %s", body)
		assert.Zero(t, info.TodayRequests)
		assert.Zero(t, info.TodayTokens)
		assert.Zero(t, info.Last24hRequests)
		assert.Zero(t, info.Last24hTokens)
		assert.Empty(t, info.Last24hLastRequestAt)
	}

	// daily_cost_used_usd 为 null：used/remaining 兜底为 0，percent 不计算，
	// 仅 limit 有值时 limit_points 照常解析。
	info, err = parseRadeonCloudUsage([]byte(`{"daily_cost_limit_usd": 1, "daily_cost_used_usd": null}`))
	require.NoError(t, err)
	assert.Equal(t, 1000000.0, info.DailyLimitPoints)
	assert.Zero(t, info.DailyUsedPoints)
	assert.Zero(t, info.DailyRemainingPoints)
	assert.Zero(t, info.DailyUsedPercent)
}

func TestParseRadeonCloudUsageStringNumbers(t *testing.T) {
	// 数字字段可能以字符串返回（"20"、"0.00018652"），必须兼容解析。
	body := []byte(`{
		"rpm_limit": "20",
		"daily_cost_limit_usd": "1",
		"daily_cost_used_usd": "0.00018652",
		"daily_cost_remaining_usd": "0.99981348",
		"today": {"requests": "7", "total_tokens": "123"},
		"last_24_hours": {"requests": "9", "total_tokens": "456"}
	}`)
	info, err := parseRadeonCloudUsage(body)
	require.NoError(t, err)
	assert.Equal(t, 20, info.RpmLimit)
	assert.Equal(t, 186.52, info.DailyUsedPoints)
	assert.Equal(t, 999813.48, info.DailyRemainingPoints)
	assert.Equal(t, 7, info.TodayRequests)
	assert.Equal(t, int64(123), info.TodayTokens)
	assert.Equal(t, 9, info.Last24hRequests)
	assert.Equal(t, int64(456), info.Last24hTokens)
}

func TestParseRadeonCloudUsageResetInSecFromAt(t *testing.T) {
	// daily_reset_epoch 缺失时回退解析 daily_reset_at（RFC3339）。
	resetAt := time.Now().Add(2 * time.Hour).Truncate(time.Second).UTC()
	body := []byte(fmt.Sprintf(`{"daily_reset_at": "%s"}`, resetAt.Format(time.RFC3339)))
	info, err := parseRadeonCloudUsage(body)
	require.NoError(t, err)
	assert.Equal(t, resetAt.Format(time.RFC3339), info.DailyResetAt)
	assert.InDelta(t, 2*3600, info.DailyResetInSec, 60)

	// 已过期的 reset 时刻剩余秒数置 0。
	expired := time.Now().Add(-time.Hour).UTC()
	body = []byte(fmt.Sprintf(`{"daily_reset_at": "%s"}`, expired.Format(time.RFC3339)))
	info, err = parseRadeonCloudUsage(body)
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.DailyResetInSec)
}

func TestParseRadeonCloudUsageInvalidJSON(t *testing.T) {
	_, err := parseRadeonCloudUsage([]byte(`not-json`))
	require.Error(t, err)
}

// radeonTestChannel 构造一个 BaseURL 指向测试 server 的 RadeonCloud 渠道。
func radeonTestChannel(serverURL string) *model.Channel {
	baseURL := serverURL
	return &model.Channel{
		Type:    constant.ChannelTypeRadeonCloud,
		Key:     "test-key",
		BaseURL: &baseURL,
	}
}

func TestFetchRadeonCloudUsageSuccess(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/usage", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(radeonTestStandardBody(t, time.Now().Add(3*time.Hour).Unix())))
	}))
	defer server.Close()

	info, err := FetchRadeonCloudUsage(radeonTestChannel(server.URL))
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Equal(t, 20, info.RpmLimit)
	assert.Equal(t, 186.52, info.DailyUsedPoints)
	assert.Equal(t, 17, info.TodayRequests)
}

func TestFetchRadeonCloudUsageUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := FetchRadeonCloudUsage(radeonTestChannel(server.URL))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRadeonCloudUnauthorized), "err: %v", err)
	assert.False(t, errors.Is(err, ErrRadeonCloudFetchFailed))
}

func TestFetchRadeonCloudUsageFetchFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := FetchRadeonCloudUsage(radeonTestChannel(server.URL))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRadeonCloudFetchFailed), "err: %v", err)
	assert.False(t, errors.Is(err, ErrRadeonCloudUnauthorized))
}

func TestFetchRadeonCloudUsageMissingKey(t *testing.T) {
	ch := &model.Channel{Type: constant.ChannelTypeRadeonCloud, Key: "  "}
	_, err := FetchRadeonCloudUsage(ch)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrRadeonCloudUnauthorized))

	_, err = FetchRadeonCloudUsage(nil)
	require.Error(t, err)
}

func TestFetchRadeonCloudUsageSchemaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	_, err := FetchRadeonCloudUsage(radeonTestChannel(server.URL))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRadeonCloudSchema), "err: %v", err)
}

func TestRadeonResetInSecWithEpochPreference(t *testing.T) {
	// daily_reset_epoch 优先于 daily_reset_at。
	resetEpoch := time.Now().Add(time.Hour).Unix()
	body := []byte(fmt.Sprintf(`{
		"daily_reset_epoch": %d,
		"daily_reset_at": %q
	}`, resetEpoch, strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)))
	info, err := parseRadeonCloudUsage(body)
	require.NoError(t, err)
	assert.InDelta(t, 3600, info.DailyResetInSec, 60)
}
