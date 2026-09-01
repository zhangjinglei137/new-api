package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// senseNovaPoolUsageFixture 是 pool-usage 接口的真实响应（数值全部为字符串，
// reset_at/nearest_grant_expiry 为秒级 epoch）。
const senseNovaPoolUsageFixture = `{"plan":{"id":"free", "name":"Free Plan", "type":"TOKEN_PLAN_PLAN_TYPE_FREE"}, "pools":[{"id":"pool_f2f9196e-3009-42dd-a044-46d30cc9143a", "name":"通用积分池", "model_ids":["deepseek-v4-flash", "deepseek-v4-pro", "glm-5.1", "glm-5.2", "kimi-k2.6", "kimi-k3", "minimax-m2.7", "qwen3.6-27b", "sensenova-6.7-flash-lite", "sensenova-6.8-flash", "sensenova-6.8-flash-lite", "sensenova-u1-fast", "sensenova-u1-pro-preview", "sensenova-u1.5-lite"], "window_5h":{"limit":"60000", "used":"0.464", "remaining":"59999.536", "reset_at":"1788248127"}, "window_7d":{"limit":"600000", "used":"11166.4256", "remaining":"588833.5744", "reset_at":"1788474927"}, "grant_balance":"114679.3388", "nearest_grant_expiry":"1790499600", "nearest_grant_expiring_balance":"4.636", "pool_type":"default"}, {"id":"pool_2afaf48e-1974-4f3e-a466-9b61f297d177", "name":"Flash-Lite积分池", "model_ids":["sensenova-6.7-flash-lite", "sensenova-6.8-flash-lite"], "window_5h":{"limit":"60000", "used":"82.832", "remaining":"59917.168", "reset_at":"1788248127"}, "window_7d":{"limit":"600000", "used":"114679.3388", "remaining":"485320.6612", "reset_at":"1788474927"}, "grant_balance":"0", "nearest_grant_expiry":"0", "nearest_grant_expiring_balance":"0", "pool_type":"dedicated"}]}`

// senseNovaMockChallenge / senseNovaMockCSRF 是登录流程各步骤的固定模拟值。
const (
	senseNovaMockChallenge = "0123456789abcdef0123456789abcdef"
	senseNovaMockCSRF      = "csrf-token-123"
	senseNovaMockRedirect  = "https://platform.sensenova.cn/oauth2/auth?client_id=nova&login_verifier=verifier-abc"
	senseNovaMockCode      = "auth-code-123"
	senseNovaMockTokenBody = `{"access_token":"access-123","refresh_token":"refresh-456","token_type":"Bearer","expires_in":7200}`
)

func TestNewSenseNovaPKCE(t *testing.T) {
	pkce, err := newSenseNovaPKCE()
	require.NoError(t, err)

	// verifier：43 随机字节 → base64url 无填充
	verifierRaw, err := base64.RawURLEncoding.DecodeString(pkce.verifier)
	require.NoError(t, err)
	assert.Len(t, verifierRaw, 43, "verifier 必须由 43 字节随机数 base64url 编码而来")

	// challenge = base64url(sha256(verifier))
	sum := sha256.Sum256([]byte(pkce.verifier))
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(sum[:]), pkce.challenge, "challenge 必须等于 base64url(sha256(verifier))")

	// state：16 字节 hex（32 字符）
	stateRaw, err := hex.DecodeString(pkce.state)
	require.NoError(t, err)
	assert.Len(t, stateRaw, 16, "state 必须为 16 字节 hex")

	for _, s := range []string{pkce.verifier, pkce.challenge} {
		assert.NotContains(t, s, "=", "base64url 不允许填充符")
		assert.NotContains(t, s, "+", "base64url 不允许 +")
		assert.NotContains(t, s, "/", "base64url 不允许 /")
	}

	pkce2, err := newSenseNovaPKCE()
	require.NoError(t, err)
	assert.NotEqual(t, pkce.verifier, pkce2.verifier, "每次生成必须随机")
}

// senseNovaMockTransport 记录请求并按 handler 模拟 SenseNova 各接口响应。
type senseNovaMockTransport struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   []string
	handler  func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error)
}

func (m *senseNovaMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := ""
	if req.Body != nil {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(strings.NewReader(string(raw)))
		body = string(raw)
	}
	m.mu.Lock()
	m.requests = append(m.requests, req)
	m.bodies = append(m.bodies, body)
	m.mu.Unlock()
	return m.handler(m, req)
}

func (m *senseNovaMockTransport) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

func (m *senseNovaMockTransport) request(i int) *http.Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests[i]
}

func (m *senseNovaMockTransport) body(i int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bodies[i]
}

func senseNovaMockResponse(status int, headers map[string]string, body string) *http.Response {
	h := make(http.Header)
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func readSenseNovaReqBody(req *http.Request) string {
	if req.Body == nil {
		return ""
	}
	raw, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	req.Body = io.NopCloser(strings.NewReader(string(raw)))
	return string(raw)
}

// senseNovaLoginSteps 描述登录流程各步骤的模拟响应（空值字段表示该步骤不生效）。
type senseNovaLoginSteps struct {
	aStatus          int    // 步骤 A 状态码
	aLocation        string // 步骤 A 302 Location
	bStatus          int    // 步骤 B 状态码
	bBody            string // 步骤 B 响应体（含 redirect 字段）
	cStatus          int    // 步骤 C 第一跳状态码
	cLocation        string // 步骤 C 第一跳 Location（无 consent 时直接携带 code）
	cConsentStatus   int    // 步骤 C 第二跳（consent 页）状态码
	cConsentLocation string // 步骤 C 第二跳 Location（consent → 授权页；空 = 无 consent 步骤）
	cFinalStatus     int    // 步骤 C 最后一跳状态码（携带 code）
	cFinalLocation   string // 步骤 C 最后一跳 Location（携带 code；空 = 无 consent 步骤）
	dStatus          int    // 步骤 D 状态码
	dBody            string // 步骤 D 响应体
	refreshStatus    int    // 续期接口状态码
	refreshBody      string // 续期接口响应体
}

// senseNovaMockConsentChallenge / senseNovaMockConsentVerifier 是 consent 链路的固定模拟值。
const (
	senseNovaMockConsentChallenge = "cccccccccccccccccccccccccccccccc"
	senseNovaMockConsentVerifier  = "vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv"
)

func senseNovaSuccessSteps() *senseNovaLoginSteps {
	return &senseNovaLoginSteps{
		aStatus:   http.StatusFound,
		aLocation: "https://platform.sensenova.cn/login?login_challenge=" + senseNovaMockChallenge,
		bStatus:   http.StatusOK,
		bBody:     `{"redirect":"` + senseNovaMockRedirect + `"}`,
		// 步骤 C 默认模拟 consent 3 跳链：redirect → consent 页 → 授权页 → code
		cStatus:          http.StatusFound,
		cLocation:        "https://iam.sensecoreapi.cn/iam/authn/v1/auth/consent?consent_challenge=" + senseNovaMockConsentChallenge,
		cConsentStatus:   http.StatusFound,
		cConsentLocation: "https://platform.sensenova.cn/oauth2/auth?client_id=nova&consent_verifier=" + senseNovaMockConsentVerifier + "&redirect_uri=https%3A%2F%2Fplatform.sensenova.cn&response_type=code&scope=openid+offline+offline_access&state=state-123",
		cFinalStatus:     http.StatusSeeOther,
		cFinalLocation:   "https://platform.sensenova.cn/?code=" + senseNovaMockCode + "&scope=openid+offline+offline_access&state=state-123",
		dStatus:          http.StatusOK,
		dBody:            senseNovaMockTokenBody,
		refreshStatus:    http.StatusOK,
		refreshBody:      `{"access_token":"refreshed-789","refresh_token":"refresh-999","expires_in":7200}`,
	}
}

// senseNovaLoginHandler 按登录步骤分发模拟响应。
func senseNovaLoginHandler(steps *senseNovaLoginSteps) func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error) {
	return func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodGet && req.URL.String() == steps.cConsentLocation:
			// 步骤 C 第三跳：consent 后回到授权页 → 303 携带 code 的回调地址
			return senseNovaMockResponse(steps.cFinalStatus, map[string]string{"Location": steps.cFinalLocation}, ""), nil
		case req.Method == http.MethodGet && req.URL.String() == steps.cLocation:
			// 步骤 C 第二跳：consent 同意页 → 302 回到授权页（并种下 iam 域 cookie）
			return senseNovaMockResponse(steps.cConsentStatus, map[string]string{
				"Location":   steps.cConsentLocation,
				"Set-Cookie": "iam_session=iam-cookie-1",
			}, ""), nil
		case req.Method == http.MethodGet && req.URL.Host == "platform.sensenova.cn" &&
			strings.HasPrefix(req.URL.Path, "/oauth2/auth") && req.URL.Query().Get("login_verifier") != "":
			// 步骤 C 第一跳：授权重定向（种下 platform 域会话 cookie，验证跨跳传递）
			return senseNovaMockResponse(steps.cStatus, map[string]string{
				"Location":   steps.cLocation,
				"Set-Cookie": "platform_session=plat-1",
			}, ""), nil
		case req.Method == http.MethodGet && req.URL.Host == "platform.sensenova.cn" &&
			strings.HasPrefix(req.URL.Path, "/oauth2/auth"):
			// 步骤 A：授权页，302 + CSRF cookie
			return senseNovaMockResponse(steps.aStatus, map[string]string{
				"Location":   steps.aLocation,
				"Set-Cookie": senseNovaCSRFCookieName + "=" + senseNovaMockCSRF,
			}, ""), nil
		case req.Method == http.MethodPost && req.URL.Host == "iam.sensecoreapi.cn":
			// 步骤 B：账号密码登录
			return senseNovaMockResponse(steps.bStatus, nil, steps.bBody), nil
		case req.Method == http.MethodPost && req.URL.Host == "signin.sensecore.cn" &&
			strings.HasPrefix(req.URL.Path, "/oauth2/token"):
			// 步骤 D 或续期（按 grant_type 区分）
			if strings.Contains(readSenseNovaReqBody(req), "grant_type=refresh_token") {
				return senseNovaMockResponse(steps.refreshStatus, nil, steps.refreshBody), nil
			}
			return senseNovaMockResponse(steps.dStatus, nil, steps.dBody), nil
		}
		return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL.String())
	}
}

func TestLoginSenseNovaFlow(t *testing.T) {
	steps := senseNovaSuccessSteps()
	transport := &senseNovaMockTransport{handler: senseNovaLoginHandler(steps)}
	token, err := loginSenseNovaWithClient(&http.Client{Transport: transport}, "user-1", "pass-1")
	require.NoError(t, err)
	assert.Equal(t, "access-123", token.AccessToken)
	assert.Equal(t, "refresh-456", token.RefreshToken)
	assert.InDelta(t, 7200, token.ExpiresAt.Sub(time.Now()).Seconds(), 5)

	// A、B、C（3 跳）、D
	require.Equal(t, 6, transport.len())

	// 步骤 A：URL 参数完整（response_type/client_id/PKCE/redirect_uri/scope/state）
	a := transport.request(0)
	assert.Equal(t, http.MethodGet, a.Method)
	aq := a.URL.Query()
	assert.Equal(t, "code", aq.Get("response_type"))
	assert.Equal(t, "nova", aq.Get("client_id"))
	assert.Equal(t, "S256", aq.Get("code_challenge_method"))
	assert.Equal(t, "https://platform.sensenova.cn", aq.Get("redirect_uri"))
	assert.Equal(t, "openid offline offline_access", aq.Get("scope"))
	stateRaw, err := hex.DecodeString(aq.Get("state"))
	require.NoError(t, err)
	assert.Len(t, stateRaw, 16)
	assert.Contains(t, a.URL.RawQuery, "redirect_uri=https%3A%2F%2Fplatform.sensenova.cn", "redirect_uri 必须按规范 URL 编码")
	assert.Contains(t, a.URL.RawQuery, "scope=openid+offline+offline_access")
	challenge := aq.Get("code_challenge")
	require.NotEmpty(t, challenge)

	// 步骤 B：Content-Type/Cookie/Referer/Origin 与请求体
	b := transport.request(1)
	assert.Equal(t, http.MethodPost, b.Method)
	assert.Equal(t, "application/json", b.Header.Get("Content-Type"))
	assert.Equal(t, "oauth2_authentication_csrf="+senseNovaMockCSRF, b.Header.Get("Cookie"), "CSRF cookie 必须从步骤 A 的 jar 中提取并携带")
	assert.Equal(t, "https://platform.sensenova.cn/login", b.Header.Get("Referer"))
	assert.Equal(t, "https://platform.sensenova.cn", b.Header.Get("Origin"))
	assert.Equal(t, "zh-CN", b.Header.Get("Accept-Language"))
	var loginBody map[string]string
	require.NoError(t, common.Unmarshal([]byte(transport.body(1)), &loginBody))
	assert.Equal(t, senseNovaMockChallenge, loginBody["challenge"])
	assert.Equal(t, "", loginBody["tenant_code"])
	assert.Equal(t, "user-1", loginBody["username"])
	assert.Equal(t, "pass-1", loginBody["password"])
	assert.Equal(t, "username", loginBody["login_type"])
	assert.Equal(t, "", loginBody["code_key"])

	// 步骤 C：授权回调重定向链（3 跳：redirect → consent 页 → 授权页 → code）
	c1 := transport.request(2)
	assert.Equal(t, http.MethodGet, c1.Method)
	assert.Equal(t, senseNovaMockRedirect, c1.URL.String(), "第一跳必须访问步骤 B 返回的 redirect 地址")

	c2 := transport.request(3)
	assert.Equal(t, http.MethodGet, c2.Method)
	assert.Equal(t, steps.cLocation, c2.URL.String(), "第二跳必须落在 consent 同意页（iam 域）")
	assert.Equal(t, "", c2.Header.Get("Cookie"), "iam 域首次访问不应携带 platform 域 cookie")

	c3 := transport.request(4)
	assert.Equal(t, http.MethodGet, c3.Method)
	assert.Equal(t, steps.cConsentLocation, c3.URL.String(), "第三跳必须携带 consent_verifier 回到授权页")
	assert.Contains(t, c3.Header.Get("Cookie"), "platform_session=plat-1", "cookie jar 必须跨跳保留会话 cookie")

	// 步骤 D：授权码 + code_verifier 换令牌，verifier 与步骤 A 的 challenge 匹配
	d := transport.request(5)
	assert.Equal(t, http.MethodPost, d.Method)
	assert.Equal(t, "application/x-www-form-urlencoded", d.Header.Get("Content-Type"))
	form, err := url.ParseQuery(transport.body(5))
	require.NoError(t, err)
	assert.Equal(t, "authorization_code", form.Get("grant_type"))
	assert.Equal(t, senseNovaMockCode, form.Get("code"))
	assert.Equal(t, "https://platform.sensenova.cn", form.Get("redirect_uri"))
	assert.Equal(t, "nova", form.Get("client_id"))
	verifier := form.Get("code_verifier")
	require.NotEmpty(t, verifier)
	sum := sha256.Sum256([]byte(verifier))
	assert.Equal(t, challenge, base64.RawURLEncoding.EncodeToString(sum[:]), "code_verifier 必须与步骤 A 的 code_challenge 对应")
}

// TestLoginSenseNovaNoConsentSingleHopCompat 验证无 consent 步骤的旧式单跳
// 链路仍然兼容：redirect → 直接 302 到携带 code 的回调地址。
func TestLoginSenseNovaNoConsentSingleHopCompat(t *testing.T) {
	steps := senseNovaSuccessSteps()
	// 去掉 consent 两跳：第一跳 Location 直接携带 code
	steps.cLocation = "https://platform.sensenova.cn/?code=" + senseNovaMockCode + "&scope=openid+offline+offline_access&state=state-123"
	steps.cConsentLocation = ""
	steps.cFinalLocation = ""

	transport := &senseNovaMockTransport{handler: senseNovaLoginHandler(steps)}
	token, err := loginSenseNovaWithClient(&http.Client{Transport: transport}, "user-1", "pass-1")
	require.NoError(t, err)
	assert.Equal(t, "access-123", token.AccessToken)
	assert.Equal(t, "refresh-456", token.RefreshToken)
	require.Equal(t, 4, transport.len(), "无 consent 时步骤 C 应只有 1 跳（A、B、C、D）")

	c := transport.request(2)
	assert.Equal(t, http.MethodGet, c.Method)
	assert.Equal(t, senseNovaMockRedirect, c.URL.String())
	d := transport.request(3)
	form, err := url.ParseQuery(transport.body(3))
	require.NoError(t, err)
	assert.Equal(t, "authorization_code", form.Get("grant_type"))
	assert.Equal(t, senseNovaMockCode, form.Get("code"), "单跳链路必须同样提取到 code")
	_ = d
}

func TestLoginSenseNovaFlowErrors(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*senseNovaLoginSteps)
		want string
	}{
		{name: "auth step not 302", mut: func(s *senseNovaLoginSteps) { s.aStatus = http.StatusOK }, want: "授权接口返回 200"},
		{name: "auth location missing login_challenge", mut: func(s *senseNovaLoginSteps) { s.aLocation = "https://platform.sensenova.cn/login" }, want: "缺少 login_challenge"},
		{name: "login rejected 400", mut: func(s *senseNovaLoginSteps) { s.bStatus = http.StatusBadRequest }, want: "用户名或密码错误"},
		{name: "login rejected 403", mut: func(s *senseNovaLoginSteps) { s.bStatus = http.StatusForbidden }, want: "用户名或密码错误"},
		{name: "login empty redirect", mut: func(s *senseNovaLoginSteps) { s.bBody = `{"redirect":""}` }, want: "未返回跳转地址"},
		{name: "login redirect to foreign host", mut: func(s *senseNovaLoginSteps) { s.bBody = `{"redirect":"https://evil.example/x"}` }, want: "跳转地址非法"},
		{name: "login response not json", mut: func(s *senseNovaLoginSteps) { s.bBody = `not-json` }, want: "解析登录响应失败"},
		{name: "callback not 302", mut: func(s *senseNovaLoginSteps) { s.cStatus = http.StatusOK }, want: "授权回调返回 200"},
		{name: "callback location empty", mut: func(s *senseNovaLoginSteps) { s.cLocation = "" }, want: "授权回调缺少跳转地址"},
		{name: "consent step not redirect", mut: func(s *senseNovaLoginSteps) { s.cConsentStatus = http.StatusOK }, want: "授权回调返回 200"},
		{name: "callback exceeds max redirects", mut: func(s *senseNovaLoginSteps) { s.cFinalLocation = s.cConsentLocation }, want: "跳转次数超过"},
		{name: "callback redirect to foreign host", mut: func(s *senseNovaLoginSteps) { s.cConsentLocation = "https://evil.example/steal" }, want: "跳转地址非法"},
		{name: "token step 400", mut: func(s *senseNovaLoginSteps) { s.dStatus = http.StatusBadRequest }, want: "令牌接口返回 400"},
		{name: "token missing access_token", mut: func(s *senseNovaLoginSteps) { s.dBody = `{"refresh_token":"x"}` }, want: "缺少 access_token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := senseNovaSuccessSteps()
			tt.mut(steps)
			transport := &senseNovaMockTransport{handler: senseNovaLoginHandler(steps)}
			_, err := loginSenseNovaWithClient(&http.Client{Transport: transport}, "u", "p")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestParseSenseNovaPoolUsageRealFixture(t *testing.T) {
	info, err := parseSenseNovaPoolUsage([]byte(senseNovaPoolUsageFixture))
	require.NoError(t, err)
	assert.Equal(t, "free", info.Plan.ID)
	assert.Equal(t, "Free Plan", info.Plan.Name)
	require.Len(t, info.Pools, 2)

	pool := info.Pools[0]
	assert.Equal(t, "default", pool.PoolType)
	assert.Equal(t, "通用积分池", pool.Name)
	assert.Contains(t, pool.ModelIDs, "deepseek-v4-flash")
	assert.Contains(t, pool.ModelIDs, "sensenova-u1.5-lite")

	// window_5h：字符串数值 → float64
	assert.InDelta(t, 60000, pool.Window5h.Limit, 1e-9)
	assert.InDelta(t, 0.464, pool.Window5h.Used, 1e-9)
	assert.InDelta(t, 59999.536, pool.Window5h.Remaining, 1e-9)
	// 秒级 epoch → RFC3339
	assert.Equal(t, time.Unix(1788248127, 0).UTC().Format(time.RFC3339), pool.Window5h.ResetAt)
	assert.InDelta(t, 11166.4256, pool.Window7d.Used, 1e-9)
	assert.Equal(t, time.Unix(1788474927, 0).UTC().Format(time.RFC3339), pool.Window7d.ResetAt)
	assert.InDelta(t, 114679.3388, pool.GrantBalance, 1e-9)
	assert.Equal(t, time.Unix(1790499600, 0).UTC().Format(time.RFC3339), pool.NearestGrantExpiry)
	assert.InDelta(t, 4.636, pool.NearestGrantExpiringBalance, 1e-9)

	dedicated := info.Pools[1]
	assert.Equal(t, "dedicated", dedicated.PoolType)
	assert.Equal(t, "Flash-Lite积分池", dedicated.Name)
	assert.Equal(t, "", dedicated.NearestGrantExpiry, "0 epoch 必须转空串，不能渲染成 1970")
	assert.InDelta(t, 0, dedicated.GrantBalance, 1e-9)
	assert.InDelta(t, 0, dedicated.NearestGrantExpiringBalance, 1e-9)
}

func TestParseSenseNovaPoolUsageTolerance(t *testing.T) {
	body := `{"plan":{"id":"free"},
	  "pools":[{"name":"p","pool_type":"default",
	    "window_5h":{"limit":"","used":"abc","remaining":"0","reset_at":""},
	    "window_7d":{"limit":"100","used":"1.5","remaining":"98.5","reset_at":"0"},
	    "grant_balance":"","nearest_grant_expiry":"","nearest_grant_expiring_balance":"not-a-number"}]}`
	info, err := parseSenseNovaPoolUsage([]byte(body))
	require.NoError(t, err)
	require.Len(t, info.Pools, 1)
	p := info.Pools[0]
	assert.InDelta(t, 0, p.Window5h.Limit, 1e-9)
	assert.InDelta(t, 0, p.Window5h.Used, 1e-9)
	assert.Equal(t, "", p.Window5h.ResetAt)
	assert.InDelta(t, 100, p.Window7d.Limit, 1e-9)
	assert.InDelta(t, 1.5, p.Window7d.Used, 1e-9)
	assert.Equal(t, "", p.Window7d.ResetAt)
	assert.Equal(t, "", p.NearestGrantExpiry)
	assert.InDelta(t, 0, p.NearestGrantExpiringBalance, 1e-9)

	_, err = parseSenseNovaPoolUsage([]byte(`not-json`))
	require.Error(t, err)
}

func TestSenseNovaEpochToRFC3339(t *testing.T) {
	want := time.Unix(1788248127, 0).UTC().Format(time.RFC3339)
	tests := []struct{ in, want string }{
		{"", ""},
		{"0", ""},
		{"-1", ""},
		{"abc", ""},
		{"1788248127", want},
		{"1788248127000", want}, // 毫秒容错
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, senseNovaEpochToRFC3339(tt.in), "input %q", tt.in)
	}
}

func TestSenseNovaParseFloat(t *testing.T) {
	assert.InDelta(t, 0, senseNovaParseFloat(""), 1e-9)
	assert.InDelta(t, 0, senseNovaParseFloat("abc"), 1e-9)
	assert.InDelta(t, 0.464, senseNovaParseFloat(" 0.464 "), 1e-9)
	assert.InDelta(t, -3.5, senseNovaParseFloat("-3.5"), 1e-9)
}

func TestFetchSenseNovaPoolUsageAuthFailure(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		transport := &senseNovaMockTransport{handler: func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error) {
			return senseNovaMockResponse(status, nil, `{}`), nil
		}}
		_, err := fetchSenseNovaPoolUsageWithClient(&http.Client{Transport: transport}, "tok")
		require.Error(t, err)
		assert.ErrorIs(t, err, errSenseNovaTokenInvalid)
		assert.Equal(t, "Bearer tok", transport.request(0).Header.Get("Authorization"))
	}
}

func TestFetchSenseNovaPoolUsageServerError(t *testing.T) {
	transport := &senseNovaMockTransport{handler: func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error) {
		return senseNovaMockResponse(http.StatusInternalServerError, nil, `{}`), nil
	}}
	_, err := fetchSenseNovaPoolUsageWithClient(&http.Client{Transport: transport}, "tok")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "获取积分池用量失败")
}

func TestFetchSenseNovaPoolUsageSuccess(t *testing.T) {
	transport := &senseNovaMockTransport{handler: func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error) {
		return senseNovaMockResponse(http.StatusOK, nil, senseNovaPoolUsageFixture), nil
	}}
	info, err := fetchSenseNovaPoolUsageWithClient(&http.Client{Transport: transport}, "tok")
	require.NoError(t, err)
	require.Len(t, info.Pools, 2)
	req := transport.request(0)
	assert.Equal(t, "Bearer tok", req.Header.Get("Authorization"))
	assert.Equal(t, senseNovaPoolUsageURL, req.URL.String())
}

func TestGetSenseNovaTokenCachedValid(t *testing.T) {
	const channelID = 9001
	t.Cleanup(func() { invalidateSenseNovaToken(channelID, "u", "p") })
	entry := senseNovaTokenEntryFor(channelID, "u", "p")
	entry.mu.Lock()
	entry.token = &SenseNovaToken{AccessToken: "cached", RefreshToken: "rt", ExpiresAt: time.Now().Add(2 * time.Hour)}
	entry.mu.Unlock()

	transport := &senseNovaMockTransport{handler: func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("缓存有效时不应发起任何上游请求")
	}}
	token, err := getSenseNovaTokenWithClient(&http.Client{Transport: transport}, channelID, "u", "p")
	require.NoError(t, err)
	assert.Equal(t, "cached", token.AccessToken)
	assert.Equal(t, 0, transport.len())
}

// TestGetSenseNovaTokenCredentialChangeTriggersFullLogin 回归测试：缓存键必须包含
// 凭证摘要。管理员更换账号密码后，旧凭证的缓存 token 不得被新凭证复用（否则
// refresh 续期会一直成功，用量接口无限期返回旧账号数据）；新凭证必须触发完整登录，
// 且登录请求携带新凭证。
func TestGetSenseNovaTokenCredentialChangeTriggersFullLogin(t *testing.T) {
	const channelID = 9006
	t.Cleanup(func() {
		invalidateSenseNovaToken(channelID, "old-user", "old-pass")
		invalidateSenseNovaToken(channelID, "new-user", "new-pass")
	})

	// 预置旧凭证（管理员更换前）的有效缓存 token。
	entry := senseNovaTokenEntryFor(channelID, "old-user", "old-pass")
	entry.mu.Lock()
	entry.token = &SenseNovaToken{AccessToken: "old-cached", RefreshToken: "rt-old", ExpiresAt: time.Now().Add(2 * time.Hour)}
	entry.mu.Unlock()

	// 新凭证：缓存 miss → 必须完整登录（4 步），不得复用或续期旧令牌。
	steps := senseNovaSuccessSteps()
	transport := &senseNovaMockTransport{handler: senseNovaLoginHandler(steps)}
	token, err := getSenseNovaTokenWithClient(&http.Client{Transport: transport}, channelID, "new-user", "new-pass")
	require.NoError(t, err)
	assert.Equal(t, "access-123", token.AccessToken)
	require.Equal(t, 6, transport.len(), "凭证变更后必须触发完整登录（A、B、C×3、D），不得复用旧缓存")
	assert.Contains(t, transport.body(1), `"username":"new-user"`, "登录请求必须携带新凭证")
	assert.Contains(t, transport.body(1), `"password":"new-pass"`)

	// 旧凭证缓存不受影响：旧账号仍命中自己的缓存，不发起任何请求。
	oldTransport := &senseNovaMockTransport{handler: func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("旧凭证缓存有效时不应发起任何上游请求")
	}}
	oldToken, err := getSenseNovaTokenWithClient(&http.Client{Transport: oldTransport}, channelID, "old-user", "old-pass")
	require.NoError(t, err)
	assert.Equal(t, "old-cached", oldToken.AccessToken)
	assert.Equal(t, 0, oldTransport.len())
}

func TestGetSenseNovaTokenRefreshNearExpiry(t *testing.T) {
	const channelID = 9002
	t.Cleanup(func() { invalidateSenseNovaToken(channelID, "u", "p") })
	entry := senseNovaTokenEntryFor(channelID, "u", "p")
	entry.mu.Lock()
	entry.token = &SenseNovaToken{AccessToken: "old", RefreshToken: "rt-1", ExpiresAt: time.Now().Add(30 * time.Second)}
	entry.mu.Unlock()

	steps := senseNovaSuccessSteps()
	transport := &senseNovaMockTransport{handler: senseNovaLoginHandler(steps)}
	token, err := getSenseNovaTokenWithClient(&http.Client{Transport: transport}, channelID, "u", "p")
	require.NoError(t, err)
	assert.Equal(t, "refreshed-789", token.AccessToken)
	assert.Equal(t, "refresh-999", token.RefreshToken)
	require.Equal(t, 1, transport.len(), "接近过期只应续期，不应完整登录")
	form, err := url.ParseQuery(transport.body(0))
	require.NoError(t, err)
	assert.Equal(t, "refresh_token", form.Get("grant_type"))
	assert.Equal(t, "rt-1", form.Get("refresh_token"))
	assert.Equal(t, "nova", form.Get("client_id"))
}

func TestGetSenseNovaTokenRefreshFailureFallsBackToLogin(t *testing.T) {
	const channelID = 9003
	t.Cleanup(func() { invalidateSenseNovaToken(channelID, "u", "p") })
	entry := senseNovaTokenEntryFor(channelID, "u", "p")
	entry.mu.Lock()
	entry.token = &SenseNovaToken{AccessToken: "old", RefreshToken: "rt-dead", ExpiresAt: time.Now().Add(30 * time.Second)}
	entry.mu.Unlock()

	steps := senseNovaSuccessSteps()
	steps.refreshStatus = http.StatusBadRequest
	steps.refreshBody = `{"error":"invalid_grant"}`
	transport := &senseNovaMockTransport{handler: senseNovaLoginHandler(steps)}
	token, err := getSenseNovaTokenWithClient(&http.Client{Transport: transport}, channelID, "u", "p")
	require.NoError(t, err)
	assert.Equal(t, "access-123", token.AccessToken)
	assert.Equal(t, 7, transport.len(), "续期失败后应回退完整登录（1 次续期 + 6 步登录）")
}

func TestGetSenseNovaTokenNoRefreshTokenFullLogin(t *testing.T) {
	const channelID = 9005
	t.Cleanup(func() { invalidateSenseNovaToken(channelID, "u", "p") })
	entry := senseNovaTokenEntryFor(channelID, "u", "p")
	entry.mu.Lock()
	entry.token = &SenseNovaToken{AccessToken: "old", ExpiresAt: time.Now().Add(-time.Hour)} // 过期且无 refresh_token
	entry.mu.Unlock()

	steps := senseNovaSuccessSteps()
	transport := &senseNovaMockTransport{handler: senseNovaLoginHandler(steps)}
	token, err := getSenseNovaTokenWithClient(&http.Client{Transport: transport}, channelID, "u", "p")
	require.NoError(t, err)
	assert.Equal(t, "access-123", token.AccessToken)
	assert.Equal(t, 6, transport.len())
}

func TestRefreshSenseNovaTokenKeepsFallbackRefreshToken(t *testing.T) {
	transport := &senseNovaMockTransport{handler: func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error) {
		return senseNovaMockResponse(http.StatusOK, nil, `{"access_token":"new-1","expires_in":7200}`), nil
	}}
	token, err := refreshSenseNovaToken(&http.Client{Transport: transport}, "rt-old")
	require.NoError(t, err)
	assert.Equal(t, "new-1", token.AccessToken)
	assert.Equal(t, "rt-old", token.RefreshToken, "响应未携带 refresh_token 时应沿用旧值")
	form, err := url.ParseQuery(transport.body(0))
	require.NoError(t, err)
	assert.Equal(t, "refresh_token", form.Get("grant_type"))
	assert.Equal(t, "rt-old", form.Get("refresh_token"))
	assert.Equal(t, "nova", form.Get("client_id"))
}

func TestRefreshSenseNovaTokenErrors(t *testing.T) {
	transport := &senseNovaMockTransport{handler: func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error) {
		return senseNovaMockResponse(http.StatusBadRequest, nil, `{}`), nil
	}}
	_, err := refreshSenseNovaToken(&http.Client{Transport: transport}, "rt")
	require.Error(t, err)

	transport = &senseNovaMockTransport{handler: func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error) {
		return senseNovaMockResponse(http.StatusOK, nil, `{"expires_in":7200}`), nil
	}}
	_, err = refreshSenseNovaToken(&http.Client{Transport: transport}, "rt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access_token")
}

func TestParseSenseNovaTokenResponseDefaultTTL(t *testing.T) {
	token, err := parseSenseNovaTokenResponse([]byte(`{"access_token":"x","expires_in":0}`), "rt-fallback")
	require.NoError(t, err)
	assert.Equal(t, "x", token.AccessToken)
	assert.Equal(t, "rt-fallback", token.RefreshToken)
	assert.InDelta(t, senseNovaDefaultTokenTTL.Seconds(), token.ExpiresAt.Sub(time.Now()).Seconds(), 5)
}

// TestFetchSenseNovaUsageRetriesOnTokenInvalid 验证 pool-usage 401 时作废缓存、
// 完整重登并以新令牌重试一次。
func TestFetchSenseNovaUsageRetriesOnTokenInvalid(t *testing.T) {
	const channelID = 9004
	t.Cleanup(func() { invalidateSenseNovaToken(channelID, "u", "p") })
	entry := senseNovaTokenEntryFor(channelID, "u", "p")
	entry.mu.Lock()
	entry.token = &SenseNovaToken{AccessToken: "stale", RefreshToken: "rt", ExpiresAt: time.Now().Add(2 * time.Hour)}
	entry.mu.Unlock()

	steps := senseNovaSuccessSteps()
	var poolSuccessCalls atomic.Int32
	transport := &senseNovaMockTransport{handler: func(m *senseNovaMockTransport, req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/lite/console/v1/tokenplan/pool-usage") {
			if req.Header.Get("Authorization") == "Bearer stale" {
				return senseNovaMockResponse(http.StatusUnauthorized, nil, `{}`), nil
			}
			poolSuccessCalls.Add(1)
			return senseNovaMockResponse(http.StatusOK, nil, senseNovaPoolUsageFixture), nil
		}
		return senseNovaLoginHandler(steps)(m, req)
	}}
	client := &http.Client{Transport: transport}

	info, err := fetchSenseNovaUsageWithClient(client, channelID, "u", "p")
	require.NoError(t, err)
	require.Len(t, info.Pools, 2)
	assert.Equal(t, int32(1), poolSuccessCalls.Load(), "重登后应重新查询 pool-usage 一次")
	assert.Equal(t, 8, transport.len(), "请求序列：pool-usage(401) + 6 步登录 + pool-usage(200)")

	// 重登后的令牌已回写缓存
	entry.mu.Lock()
	defer entry.mu.Unlock()
	require.NotNil(t, entry.token)
	assert.Equal(t, "access-123", entry.token.AccessToken)
}
