package service

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
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

func TestSignVolcOpenAPIV4Golden(t *testing.T) {
	// 固定 now / AK / SK / region / action 的签名 golden 测试。
	// 期望值用独立实现（Python hmac/sha256 逐步推演）核对后写死，
	// 覆盖 canonical headers 固定顺序、SignedHeaders 非字母序、SK 不加
	// AWS4 前缀等与 AWS SigV4 的致命差异点。
	now := time.Date(2025, 9, 1, 12, 0, 0, 0, time.UTC)
	authorization, xDate, xContentSha256 := signVolcOpenAPIV4(
		"AKLTtesttesttesttesttesttesttest",
		"sk-test-secret-key-0000000000000000",
		"cn-beijing",
		"GetCodingPlanUsage",
		now,
	)

	assert.Equal(t, "20250901T120000Z", xDate)
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", xContentSha256)
	assert.Equal(t, "HMAC-SHA256 Credential=AKLTtesttesttesttesttesttesttest/20250901/cn-beijing/ark/request, "+
		"SignedHeaders=host;x-date;x-content-sha256;content-type, "+
		"Signature=75d9ef90b001d3150f8b0c5f25a97d722354dbe0b50775174013ac9c7c1e560d", authorization)
}

func TestSignVolcOpenAPIV4SignatureIs64Hex(t *testing.T) {
	authorization, _, _ := signVolcOpenAPIV4("ak", "sk", "cn-beijing", "GetCodingPlanUsage", time.Now())
	require.Contains(t, authorization, "SignedHeaders=host;x-date;x-content-sha256;content-type")
	parts := strings.SplitN(authorization, "Signature=", 2)
	require.Len(t, parts, 2)
	assert.Len(t, parts[1], 64)
	_, err := regexp.MatchString("^[0-9a-f]{64}$", parts[1])
	require.NoError(t, err)
}

func TestFetchVolcCodingPlanUsageByAkSkRejectsInvalidParams(t *testing.T) {
	validClient := &http.Client{}
	for _, tc := range []struct {
		name                       string
		client                     *http.Client
		region, accessKeyID, apiSK string
	}{
		{name: "nil client", client: nil, region: "cn-beijing", accessKeyID: "ak", apiSK: "sk"},
		{name: "empty region", client: validClient, region: "  ", accessKeyID: "ak", apiSK: "sk"},
		{name: "empty access key id", client: validClient, region: "cn-beijing", accessKeyID: "  ", apiSK: "sk"},
		{name: "empty secret access key", client: validClient, region: "cn-beijing", accessKeyID: "ak", apiSK: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := FetchVolcCodingPlanUsageByAkSk(context.Background(), tc.client, tc.region, tc.accessKeyID, tc.apiSK)
			require.Error(t, err)
		})
	}
}

func TestFetchVolcCodingPlanUsageByAkSkSetsSignedHeaders(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/", r.URL.Path)
		assert.Equal(t, "GetCodingPlanUsage", r.URL.Query().Get("Action"))
		assert.Equal(t, "cn-beijing", r.URL.Query().Get("Region"))
		assert.Equal(t, "2024-01-01", r.URL.Query().Get("Version"))

		auth := r.Header.Get("Authorization")
		require.True(t, strings.HasPrefix(auth, "HMAC-SHA256 Credential="), "authorization prefix: %s", auth)
		assert.Contains(t, auth, "SignedHeaders=host;x-date;x-content-sha256;content-type")

		xDate := r.Header.Get("X-Date")
		assert.Regexp(t, `^\d{8}T\d{6}Z$`, xDate)
		assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", r.Header.Get("X-Content-Sha256"))
		assert.Equal(t, "application/json; charset=utf-8", r.Header.Get("Content-Type"))
		rawBody, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Empty(t, rawBody)

		// 用请求携带的 X-Date 独立重算签名，验证 Authorization 与请求头一致。
		expectedAuth, _, _ := signVolcOpenAPIV4(
			"AKLTtesttesttesttesttesttesttest",
			"sk-test-secret-key-0000000000000000",
			"cn-beijing",
			"GetCodingPlanUsage",
			parseVolcTestXDate(t, xDate),
		)
		assert.Equal(t, expectedAuth, auth)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Result":{"Status":"Running","QuotaUsage":[{"Level":"session","Percent":50}]}}`))
	}))
	defer server.Close()

	// server.Client() 对 SNI 为 open.volcengineapi.com（非 example.com）会校验失败，
	// 这里用自定义 transport：DialContext 指向本地 TLS server，测试信任自签证书。
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, server.Listener.Addr().String())
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // 仅测试：信任本地自签证书
	}
	client := &http.Client{Transport: transport}
	statusCode, body, err := FetchVolcCodingPlanUsageByAkSk(
		context.Background(),
		client,
		"cn-beijing",
		"AKLTtesttesttesttesttesttesttest",
		"sk-test-secret-key-0000000000000000",
	)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	assert.Equal(t, "Running", info.Status)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, "session", info.Windows[0].Period)
}

func parseVolcTestXDate(t *testing.T, xDate string) time.Time {
	t.Helper()
	parsed, err := time.Parse("20060102T150405Z", xDate)
	require.NoError(t, err)
	return parsed
}

// TestParseVolcCodingPlanUsageSessionResetMinusOne 覆盖 session 窗口
// ResetTimestamp 为 -1 的场景（AK/SK OpenAPI 实测返回），剩余时间不解析，
// ResetAt/ResetInSec 保持零值。
func TestParseVolcCodingPlanUsageSessionResetMinusOne(t *testing.T) {
	body := []byte(`{"Result": {"QuotaUsage": [{"Level": "session", "Percent": 30, "ResetTimestamp": -1}]}}`)
	info, err := ParseVolcCodingPlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, float64(30), info.Windows[0].UsedPercent)
	assert.Empty(t, info.Windows[0].ResetAt)
	assert.Equal(t, int64(0), info.Windows[0].ResetInSec)
}
