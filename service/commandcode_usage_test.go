package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commandCodeCreditsCamelFixture 对应新版 /internal/billing/credits 响应
// （camelCase、windowLimits 置于顶层、resetAt 为毫秒）。
const commandCodeCreditsCamelFixture = `{
  "credits": {
    "monthlyCredits": 8.7784,
    "purchasedCredits": 0,
    "premiumMonthlyCredits": 0,
    "opensourceMonthlyCredits": 8.7784
  },
  "windowLimits": {
    "limited": true,
    "exceeded": null,
    "fiveHour": {"used": 1.2216, "cap": 3, "exceeded": false, "resetAt": 1786700000000},
    "weekly": {"used": 1.2216, "cap": 6, "exceeded": false, "resetAt": 1787000000000}
  }
}`

// commandCodeCreditsSnakeFixture 兼容 snake_case 字段名。
const commandCodeCreditsSnakeFixture = `{
  "credits": {
    "monthly_credits": 6.5,
    "purchased_credits": 2.5,
    "premium_monthly_credits": 0,
    "opensource_monthly_credits": 6.5
  },
  "window_limits": {
    "limited": true,
    "five_hour": {"used": 2.0, "cap": 4, "exceeded": false, "reset_at": 1786700},
    "weekly": {"used": 5.0, "cap": 10, "exceeded": true, "reset_at": 1787000}
  }
}`

// commandCodeCreditsNestedFixture 兼容旧版：windowLimits 内嵌在 credits 中。
const commandCodeCreditsNestedFixture = `{
  "credits": {
    "monthlyCredits": 5.0,
    "purchasedCredits": 0,
    "windowLimits": {
      "fiveHour": {"used": 0.5, "cap": 1, "exceeded": false, "resetAt": 0},
      "weekly": {"used": 1.0, "cap": 2, "exceeded": false, "resetAt": 1787000000}
    }
  }
}`

// commandCodeSubsFixture 对应新版 /internal/billing/subscriptions 响应。
const commandCodeSubsFixture = `{
  "success": true,
  "data": {
    "id": "sub_123",
    "status": "active",
    "planId": "individual-go",
    "currentPeriodStart": "2026-08-15T04:42:16.000Z",
    "currentPeriodEnd": "2026-09-15T04:42:16.000Z",
    "metadata": {"commandCodeUserId": "u_1"},
    "userId": "u_1"
  }
}`

func TestParseCommandCodeCreditsCamelCase(t *testing.T) {
	credits, err := parseCommandCodeCredits([]byte(commandCodeCreditsCamelFixture))
	require.NoError(t, err)
	assert.InDelta(t, 8.7784, credits.MonthlyCredits, 1e-9)
	assert.InDelta(t, 0.0, credits.PurchasedCredits, 1e-9)
	require.NotNil(t, credits.WindowLimits)
	require.NotNil(t, credits.WindowLimits.FiveHour)
	require.NotNil(t, credits.WindowLimits.Weekly)
	assert.InDelta(t, 1.2216, credits.WindowLimits.FiveHour.Used, 1e-9)
	assert.InDelta(t, 3, credits.WindowLimits.FiveHour.Cap, 1e-9)
	assert.Equal(t, int64(1786700000000), credits.WindowLimits.FiveHour.ResetAt)
	assert.InDelta(t, 6, credits.WindowLimits.Weekly.Cap, 1e-9)
	assert.Equal(t, int64(1787000000000), credits.WindowLimits.Weekly.ResetAt)
}

func TestParseCommandCodeCreditsSnakeCase(t *testing.T) {
	credits, err := parseCommandCodeCredits([]byte(commandCodeCreditsSnakeFixture))
	require.NoError(t, err)
	assert.InDelta(t, 6.5, credits.MonthlyCredits, 1e-9)
	assert.InDelta(t, 2.5, credits.PurchasedCredits, 1e-9)
	require.NotNil(t, credits.WindowLimits)
	require.NotNil(t, credits.WindowLimits.FiveHour)
	assert.InDelta(t, 4, credits.WindowLimits.FiveHour.Cap, 1e-9)
	assert.Equal(t, int64(1786700), credits.WindowLimits.FiveHour.ResetAt)
	assert.True(t, credits.WindowLimits.Weekly.Exceeded)
}

func TestParseCommandCodeCreditsWindowLimitsNested(t *testing.T) {
	credits, err := parseCommandCodeCredits([]byte(commandCodeCreditsNestedFixture))
	require.NoError(t, err)
	assert.InDelta(t, 5.0, credits.MonthlyCredits, 1e-9)
	require.NotNil(t, credits.WindowLimits)
	require.NotNil(t, credits.WindowLimits.FiveHour)
	assert.InDelta(t, 1, credits.WindowLimits.FiveHour.Cap, 1e-9)
	// 内嵌窗口 resetAt 为 0（未触碰）
	assert.Equal(t, int64(0), credits.WindowLimits.FiveHour.ResetAt)
}

func TestParseCommandCodeCreditsErrors(t *testing.T) {
	for _, body := range []string{
		`not-json`,
		`{"windowLimits": {}}`,
		`{"credits": null}`,
		`{}`,
	} {
		_, err := parseCommandCodeCredits([]byte(body))
		require.Error(t, err)
	}
}

func TestCommandCodeResetAt(t *testing.T) {
	tests := []struct {
		name  string
		epoch int64
		want  string
	}{
		{name: "zero means no reset", epoch: 0, want: ""},
		{name: "negative ignored", epoch: -5, want: ""},
		{name: "milliseconds", epoch: 1786700000000, want: time.Unix(1786700000, 0).UTC().Format(time.RFC3339)},
		{name: "seconds", epoch: 1786700000, want: time.Unix(1786700000, 0).UTC().Format(time.RFC3339)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, commandCodeResetAt(tt.epoch))
		})
	}
}

func TestCommandCodeLimitWindowToWindow(t *testing.T) {
	t.Run("valid cap computes percent", func(t *testing.T) {
		w := commandCodeLimitWindowToWindow("session", &commandCodeLimitWindow{Used: 1.2216, Cap: 3, ResetAt: 1786700000000})
		assert.Equal(t, "session", w.Period)
		assert.Equal(t, "ok", w.Status)
		assert.True(t, w.Metered)
		assert.InDelta(t, 40.72, w.UsedPercent, 1e-9)
		assert.InDelta(t, 59.28, w.RemainingPercent, 1e-9)
		assert.InDelta(t, 1.2216, w.Used, 1e-9)
		assert.InDelta(t, 3, w.Limit, 1e-9)
		assert.Equal(t, time.Unix(1786700000, 0).UTC().Format(time.RFC3339), w.ResetAt)
	})

	t.Run("exceeded window marks status", func(t *testing.T) {
		w := commandCodeLimitWindowToWindow("weekly", &commandCodeLimitWindow{Used: 6, Cap: 6, Exceeded: true})
		assert.Equal(t, "weekly", w.Period)
		assert.Equal(t, "exceeded", w.Status)
		assert.InDelta(t, 100, w.UsedPercent, 1e-9)
		assert.InDelta(t, 0, w.RemainingPercent, 1e-9)
	})

	t.Run("zero cap uses -1 sentinel", func(t *testing.T) {
		w := commandCodeLimitWindowToWindow("session", &commandCodeLimitWindow{Used: 1, Cap: 0})
		assert.Equal(t, float64(-1), w.UsedPercent)
		assert.Equal(t, float64(-1), w.RemainingPercent)
		assert.InDelta(t, 1, w.Used, 1e-9)
		assert.InDelta(t, 0, w.Limit, 1e-9)
	})

	t.Run("percent clamped to 100", func(t *testing.T) {
		w := commandCodeLimitWindowToWindow("session", &commandCodeLimitWindow{Used: 9, Cap: 3})
		assert.InDelta(t, 100, w.UsedPercent, 1e-9)
		assert.InDelta(t, 0, w.RemainingPercent, 1e-9)
	})
}

func TestBuildCommandCodeUsageInfoCatalogMetered(t *testing.T) {
	credits, err := parseCommandCodeCredits([]byte(commandCodeCreditsCamelFixture))
	require.NoError(t, err)
	sub, err := parseCommandCodeSubscriptions([]byte(commandCodeSubsFixture))
	require.NoError(t, err)

	info := buildCommandCodeUsageInfo(credits, sub)
	require.Len(t, info.Windows, 3) // session, weekly, monthly（purchasedCredits=0 无 topup）
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, "weekly", info.Windows[1].Period)

	monthly := info.Windows[2]
	assert.Equal(t, "monthly", monthly.Period)
	assert.True(t, monthly.Metered)
	// individual-go: monthly=10，used = 10 - 8.7784 = 1.2216，percent = 12.216%
	assert.InDelta(t, 1.2216, monthly.Used, 1e-9)
	assert.InDelta(t, 10, monthly.Limit, 1e-9)
	assert.InDelta(t, 12.216, monthly.UsedPercent, 1e-9)
	assert.InDelta(t, 87.784, monthly.RemainingPercent, 1e-9)
	assert.Equal(t, "2026-09-15T04:42:16.000Z", monthly.ResetAt)
}

func TestBuildCommandCodeUsageInfoCatalogFallback(t *testing.T) {
	subMetered, err := parseCommandCodeSubscriptions([]byte(commandCodeSubsFixture))
	require.NoError(t, err)

	tests := []struct {
		name         string
		credits      *commandCodeCredits
		sub          *commandCodeSubscription
		wantMetered  bool
		wantLimit    float64
		wantUsed     float64
		wantUsedPct  float64
	}{
		{
			name:        "unknown plan id falls back to money-only with zero limit",
			credits:     &commandCodeCredits{MonthlyCredits: 3.0, WindowLimits: &commandCodeWindowLimits{FiveHour: &commandCodeLimitWindow{Cap: 3}, Weekly: &commandCodeLimitWindow{Cap: 6}}},
			sub:         &commandCodeSubscription{PlanID: "individual-provider", HasData: true},
			wantMetered: false,
			wantLimit:   0,
			wantUsed:    3.0,
			wantUsedPct: -1,
		},
		{
			name:        "cap mismatch falls back to money-only with catalog limit",
			credits:     &commandCodeCredits{MonthlyCredits: 3.0, WindowLimits: &commandCodeWindowLimits{FiveHour: &commandCodeLimitWindow{Cap: 99}, Weekly: &commandCodeLimitWindow{Cap: 6}}},
			sub:         subMetered,
			wantMetered: false,
			wantLimit:   10, // individual-go 目录月度额度仍用于展示
			wantUsed:    3.0,
			wantUsedPct: -1,
		},
		{
			name:        "remaining exceeds catalog monthly falls back",
			credits:     &commandCodeCredits{MonthlyCredits: 25.0, WindowLimits: &commandCodeWindowLimits{FiveHour: &commandCodeLimitWindow{Cap: 3}, Weekly: &commandCodeLimitWindow{Cap: 6}}},
			sub:         subMetered,
			wantMetered: false,
			wantLimit:   10,
			wantUsed:    25.0,
			wantUsedPct: -1,
		},
		{
			name:        "missing windowLimits falls back",
			credits:     &commandCodeCredits{MonthlyCredits: 3.0},
			sub:         subMetered,
			wantMetered: false,
			wantLimit:   10,
			wantUsed:    3.0,
			wantUsedPct: -1,
		},
		{
			name:        "free user no subscription falls back with zero limit",
			credits:     &commandCodeCredits{MonthlyCredits: 3.0, WindowLimits: &commandCodeWindowLimits{FiveHour: &commandCodeLimitWindow{Cap: 3}, Weekly: &commandCodeLimitWindow{Cap: 6}}},
			sub:         &commandCodeSubscription{},
			wantMetered: false,
			wantLimit:   0,
			wantUsed:    3.0,
			wantUsedPct: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := buildCommandCodeUsageInfo(tt.credits, tt.sub)
			var monthly CommandCodeWindow
			for _, w := range info.Windows {
				if w.Period == "monthly" {
					monthly = w
				}
			}
			assert.Equal(t, tt.wantMetered, monthly.Metered)
			assert.InDelta(t, tt.wantLimit, monthly.Limit, 1e-9)
			assert.InDelta(t, tt.wantUsed, monthly.Used, 1e-9)
			assert.InDelta(t, tt.wantUsedPct, monthly.UsedPercent, 1e-9)
			assert.Equal(t, float64(-1), monthly.RemainingPercent)
		})
	}
}

func TestBuildCommandCodeUsageInfoTopUp(t *testing.T) {
	creditsNoTopUp, err := parseCommandCodeCredits([]byte(commandCodeCreditsCamelFixture))
	require.NoError(t, err)
	info := buildCommandCodeUsageInfo(creditsNoTopUp, &commandCodeSubscription{})
	for _, w := range info.Windows {
		assert.NotEqual(t, "topup", w.Period, "purchasedCredits=0 时不应出现 topup 窗口")
	}

	creditsWithTopUp := &commandCodeCredits{MonthlyCredits: 1.0, PurchasedCredits: 4.5}
	info = buildCommandCodeUsageInfo(creditsWithTopUp, &commandCodeSubscription{})
	require.Len(t, info.Windows, 2) // monthly + topup
	topup := info.Windows[1]
	assert.Equal(t, "topup", topup.Period)
	assert.False(t, topup.Metered)
	assert.InDelta(t, 4.5, topup.Used, 1e-9)
	assert.Equal(t, float64(-1), topup.UsedPercent)
	assert.Equal(t, "", topup.ResetAt)
}

func TestParseCommandCodeSubscriptions(t *testing.T) {
	t.Run("active subscription", func(t *testing.T) {
		sub, err := parseCommandCodeSubscriptions([]byte(commandCodeSubsFixture))
		require.NoError(t, err)
		assert.True(t, sub.HasData)
		assert.Equal(t, "individual-go", sub.PlanID)
		assert.Equal(t, "2026-09-15T04:42:16.000Z", sub.CurrentPeriodEnd)
	})

	t.Run("free user data null is not an error", func(t *testing.T) {
		sub, err := parseCommandCodeSubscriptions([]byte(`{"success": true, "data": null}`))
		require.NoError(t, err)
		assert.False(t, sub.HasData)
	})

	t.Run("data object present with snake_case fields", func(t *testing.T) {
		sub, err := parseCommandCodeSubscriptions([]byte(`{"success": true, "data": {"plan_id": "teams-pro", "current_period_end": "2026-10-01T00:00:00.000Z"}}`))
		require.NoError(t, err)
		assert.True(t, sub.HasData)
		assert.Equal(t, "teams-pro", sub.PlanID)
		assert.Equal(t, "2026-10-01T00:00:00.000Z", sub.CurrentPeriodEnd)
	})

	t.Run("invalid json errors", func(t *testing.T) {
		_, err := parseCommandCodeSubscriptions([]byte(`not-json`))
		require.Error(t, err)
	})
}

func TestNormalizeCommandCodeCookie(t *testing.T) {
	t.Run("extracts only whitelisted session cookies", func(t *testing.T) {
		cookie := "foo=bar; __Secure-commandcode_prod_.session_token=opaque1; __Secure-commandcode_prod_.session_data=base641; other=zzz; commandcode_prod_.session_token=opaque2"
		got := normalizeCommandCodeCookie(cookie)
		assert.Contains(t, got, "__Secure-commandcode_prod_.session_token=opaque1")
		assert.Contains(t, got, "__Secure-commandcode_prod_.session_data=base641")
		assert.Contains(t, got, "commandcode_prod_.session_token=opaque2")
		assert.NotContains(t, got, "foo")
		assert.NotContains(t, got, "other")
		assert.NotContains(t, got, "zzz")
	})

	t.Run("no whitelisted cookie returns empty", func(t *testing.T) {
		assert.Equal(t, "", normalizeCommandCodeCookie("foo=bar; baz=qux"))
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		assert.Equal(t, "", normalizeCommandCodeCookie(""))
	})

	t.Run("missing value entries dropped", func(t *testing.T) {
		assert.Equal(t, "", normalizeCommandCodeCookie("__Secure-commandcode_prod_.session_token"))
	})
}

// commandCodeMockTransport 是测试用 RoundTripper：按路径返回不同的
// credits/subscriptions 响应，并捕获请求携带的 Cookie 头。
type commandCodeMockTransport struct {
	creditsStatus int
	creditsBody   string
	subsStatus    int
	subsBody      string
	cookieHeader  string
}

func (t *commandCodeMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.cookieHeader = req.Header.Get("Cookie")
	status := t.creditsStatus
	body := t.creditsBody
	if strings.Contains(req.URL.Path, "subscriptions") {
		status = t.subsStatus
		body = t.subsBody
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func TestFetchCommandCodeUsageAuthFailure(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		transport := &commandCodeMockTransport{creditsStatus: status, creditsBody: `{}`}
		client := &http.Client{Transport: transport}
		_, err := fetchCommandCodeUsageWithClient(client, "__Secure-commandcode_prod_.session_token=opaque")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "commandcode cookie 无效或已过期")
		assert.Contains(t, transport.cookieHeader, "__Secure-commandcode_prod_.session_token=opaque")
	}
}

func TestFetchCommandCodeUsageServerError(t *testing.T) {
	transport := &commandCodeMockTransport{creditsStatus: http.StatusInternalServerError, creditsBody: `{}`}
	client := &http.Client{Transport: transport}
	_, err := fetchCommandCodeUsageWithClient(client, "session_token=opaque")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status code: 500")
}

func TestFetchCommandCodeUsageEndToEnd(t *testing.T) {
	// credits 带 top-up 余额（>0），subscriptions 为 individual-go
	creditsBody := `{"credits": {"monthlyCredits": 8.7784, "purchasedCredits": 2.0},
	  "windowLimits": {"fiveHour": {"used": 1.2216, "cap": 3, "resetAt": 1786700000000},
	  "weekly": {"used": 1.2216, "cap": 6, "resetAt": 1787000000000}}}`
	transport := &commandCodeMockTransport{
		creditsStatus: http.StatusOK,
		creditsBody:   creditsBody,
		subsStatus:    http.StatusOK,
		subsBody:      commandCodeSubsFixture,
	}
	client := &http.Client{Transport: transport}
	info, err := fetchCommandCodeUsageWithClient(client, "__Secure-commandcode_prod_.session_token=opaque1; __Secure-commandcode_prod_.session_data=base641")
	require.NoError(t, err)

	// 顺序固定：session -> weekly -> monthly -> topup
	require.Len(t, info.Windows, 4)
	assert.Equal(t, []string{"session", "weekly", "monthly", "topup"},
		[]string{info.Windows[0].Period, info.Windows[1].Period, info.Windows[2].Period, info.Windows[3].Period})

	monthly := info.Windows[2]
	assert.True(t, monthly.Metered)
	assert.InDelta(t, 1.2216, monthly.Used, 1e-9)
	assert.InDelta(t, 10, monthly.Limit, 1e-9)
	assert.Equal(t, "2026-09-15T04:42:16.000Z", monthly.ResetAt)

	topup := info.Windows[3]
	assert.False(t, topup.Metered)
	assert.InDelta(t, 2.0, topup.Used, 1e-9)

	// 发送给上游的 Cookie 只含白名单会话 cookie
	assert.Equal(t, "__Secure-commandcode_prod_.session_token=opaque1; __Secure-commandcode_prod_.session_data=base641", transport.cookieHeader)
}

func TestFetchCommandCodeUsageSubscriptionsFailureIgnored(t *testing.T) {
	transport := &commandCodeMockTransport{
		creditsStatus: http.StatusOK,
		creditsBody:   commandCodeCreditsCamelFixture,
		subsStatus:    http.StatusInternalServerError,
		subsBody:      `{"error": "boom"}`,
	}
	client := &http.Client{Transport: transport}
	info, err := fetchCommandCodeUsageWithClient(client, "session_token=opaque")
	require.NoError(t, err)
	// 无订阅数据 → monthly 降级为金额模式，但 credits 窗口仍完整返回
	require.Len(t, info.Windows, 3)
	assert.Equal(t, "session", info.Windows[0].Period)
	assert.Equal(t, "weekly", info.Windows[1].Period)
	assert.False(t, info.Windows[2].Metered)
	assert.InDelta(t, 8.7784, info.Windows[2].Used, 1e-9)
}
