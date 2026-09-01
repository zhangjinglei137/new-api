package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// SenseNova（日日新）渠道用量查询：登录（PKCE + OAuth 授权码 4 步流程）→
// 积分池用量。所有上游请求均通过渠道代理 client（GetHttpClientWithProxy）发出。

const (
	senseNovaAuthURL        = "https://platform.sensenova.cn/oauth2/auth"
	senseNovaLoginAPIURL    = "https://iam.sensecoreapi.cn/iam/authn/v1/auth/nova/login"
	senseNovaTokenURL       = "https://signin.sensecore.cn/oauth2/token"
	senseNovaPoolUsageURL   = "https://platform.sensenova.cn/lite/console/v1/tokenplan/pool-usage"
	senseNovaClientID       = "nova"
	senseNovaRedirectURI    = "https://platform.sensenova.cn"
	senseNovaPlatformHost   = "platform.sensenova.cn"
	senseNovaCSRFCookieName = "oauth2_authentication_csrf"
	senseNovaUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	senseNovaLoginTimeout     = 30 * time.Second
	senseNovaPoolUsageTimeout = 15 * time.Second
	// senseNovaDefaultTokenTTL 是上游未返回 expires_in 时的兜底有效期。
	senseNovaDefaultTokenTTL = 2 * time.Hour
	// senseNovaRefreshMargin 提前续期余量：令牌剩余有效期不足该值时先尝试续期。
	senseNovaRefreshMargin = 60 * time.Second
	// senseNovaMaxAuthRedirects 是步骤 C 授权回调链允许的最大跳数
	// （正常为 3 跳：redirect → consent → code；留余量兜底异常循环）。
	senseNovaMaxAuthRedirects = 5
)

// senseNovaRedirectHosts 是步骤 C 授权回调链允许的跳转目标 host 白名单：
// platform（授权/回调页）、iam（consent 同意页）、signin（令牌端点）。
var senseNovaRedirectHosts = map[string]struct{}{
	senseNovaPlatformHost: {},
	"iam.sensecoreapi.cn": {},
	"signin.sensecore.cn": {},
}

// errSenseNovaTokenInvalid 表示访问令牌已失效（pool-usage 401/403），
// 调用方应作废缓存并重新登录。
var errSenseNovaTokenInvalid = errors.New("sensenova token 已失效")

// SenseNovaToken 是 SenseNova 登录/续期得到的令牌。
type SenseNovaToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// SenseNovaWindow 是 SenseNova 单个时间窗口的用量数据。
type SenseNovaWindow struct {
	Limit     float64
	Used      float64
	Remaining float64
	ResetAt   string // RFC3339 UTC；0/无 → 空串
}

// SenseNovaPoolUsage 是 SenseNova 单个积分池的用量数据。
type SenseNovaPoolUsage struct {
	PoolType                    string
	Name                        string
	ModelIDs                    []string
	Window5h                    SenseNovaWindow
	Window7d                    SenseNovaWindow
	GrantBalance                float64
	NearestGrantExpiry          string // RFC3339 UTC；0/无 → 空串
	NearestGrantExpiringBalance float64
}

// SenseNovaPlan 是 SenseNova 积分套餐信息。
type SenseNovaPlan struct {
	ID   string
	Name string
}

// SenseNovaUsageInfo 是 SenseNova 积分池用量解析结果。
type SenseNovaUsageInfo struct {
	Plan  SenseNovaPlan
	Pools []SenseNovaPoolUsage
}

// senseNovaPKCE 是一次登录流程使用的 PKCE 材料。
type senseNovaPKCE struct {
	verifier  string // 43 随机字节 → base64url
	challenge string // base64url(sha256(verifier))
	state     string // 16 随机字节 hex
}

// newSenseNovaPKCE 生成 PKCE code_verifier / code_challenge / state。
func newSenseNovaPKCE() (*senseNovaPKCE, error) {
	verifierRaw := make([]byte, 43)
	if _, err := rand.Read(verifierRaw); err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierRaw)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	stateRaw := make([]byte, 16)
	if _, err := rand.Read(stateRaw); err != nil {
		return nil, err
	}
	return &senseNovaPKCE{verifier: verifier, challenge: challenge, state: hex.EncodeToString(stateRaw)}, nil
}

// senseNovaTokenEntry 是单个渠道+凭证组合的进程内令牌缓存项（含过期时间与续期锁）。
type senseNovaTokenEntry struct {
	mu    sync.Mutex
	token *SenseNovaToken
}

// senseNovaTokenKey 是令牌缓存键：渠道 ID + 凭证摘要。管理员更换账号密码后
// 摘要变化，自然 miss 缓存触发完整登录；多副本实例各自按读取到的凭证命中，
// 也都正确。
type senseNovaTokenKey struct {
	channelID int
	cred      string
}

var (
	senseNovaTokenEntriesMu sync.Mutex
	senseNovaTokenEntries   = make(map[senseNovaTokenKey]*senseNovaTokenEntry)
)

// senseNovaCredDigest 返回凭证摘要 sha256(username+password) 的 hex。
func senseNovaCredDigest(username, password string) string {
	sum := sha256.Sum256([]byte(username + password))
	return hex.EncodeToString(sum[:])
}

func senseNovaTokenKeyFor(channelID int, username, password string) senseNovaTokenKey {
	return senseNovaTokenKey{channelID: channelID, cred: senseNovaCredDigest(username, password)}
}

func senseNovaTokenEntryFor(channelID int, username, password string) *senseNovaTokenEntry {
	key := senseNovaTokenKeyFor(channelID, username, password)
	senseNovaTokenEntriesMu.Lock()
	defer senseNovaTokenEntriesMu.Unlock()
	entry, ok := senseNovaTokenEntries[key]
	if !ok {
		entry = &senseNovaTokenEntry{}
		senseNovaTokenEntries[key] = entry
	}
	return entry
}

// invalidateSenseNovaToken 作废渠道指定凭证的缓存令牌（401/403 或凭证变更后调用）。
func invalidateSenseNovaToken(channelID int, username, password string) {
	entry := senseNovaTokenEntryFor(channelID, username, password)
	entry.mu.Lock()
	entry.token = nil
	entry.mu.Unlock()
}

// FetchSenseNovaUsage 获取渠道积分池用量：带缓存登录 → pool-usage；
// token 失效（401/403）时作废缓存重登并重试一次。
func FetchSenseNovaUsage(channelID int, username, password, proxy string) (*SenseNovaUsageInfo, error) {
	client, err := GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	return fetchSenseNovaUsageWithClient(client, channelID, username, password)
}

// fetchSenseNovaUsageWithClient 在给定 client（transport 已按渠道代理配置）上
// 执行带缓存登录 → pool-usage；token 失效时重登重试一次。
func fetchSenseNovaUsageWithClient(client *http.Client, channelID int, username, password string) (*SenseNovaUsageInfo, error) {
	token, err := getSenseNovaTokenWithClient(client, channelID, username, password)
	if err != nil {
		return nil, err
	}
	info, err := fetchSenseNovaPoolUsageWithClient(client, token.AccessToken)
	if err == nil {
		return info, nil
	}
	if !errors.Is(err, errSenseNovaTokenInvalid) {
		return nil, err
	}
	// 访问令牌失效：作废缓存并完整重登后重试一次。
	invalidateSenseNovaToken(channelID, username, password)
	token, err = getSenseNovaTokenWithClient(client, channelID, username, password)
	if err != nil {
		return nil, err
	}
	return fetchSenseNovaPoolUsageWithClient(client, token.AccessToken)
}

// getSenseNovaTokenWithClient 返回渠道缓存令牌；缓存缺失/过期时登录，
// 接近过期时优先 refresh_token 续期，续期失败回退完整登录。
func getSenseNovaTokenWithClient(client *http.Client, channelID int, username, password string) (*SenseNovaToken, error) {
	entry := senseNovaTokenEntryFor(channelID, username, password)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	now := time.Now()
	if entry.token != nil && !senseNovaTokenExpired(entry.token, now) {
		return entry.token, nil
	}
	if entry.token != nil && entry.token.RefreshToken != "" {
		if refreshed, err := refreshSenseNovaToken(client, entry.token.RefreshToken); err == nil {
			entry.token = refreshed
			return refreshed, nil
		}
		// 续期失败：清空缓存，回退完整登录。
		entry.token = nil
	}
	token, err := loginSenseNovaWithClient(client, username, password)
	if err != nil {
		return nil, err
	}
	entry.token = token
	return token, nil
}

// senseNovaTokenExpired 判断令牌是否需要刷新（已过期或剩余有效期不足余量）。
func senseNovaTokenExpired(t *SenseNovaToken, now time.Time) bool {
	return !now.Before(t.ExpiresAt.Add(-senseNovaRefreshMargin))
}

// loginSenseNovaWithClient 在给定 client（transport 已按渠道代理配置）上执行
// 完整 4 步登录流程：授权页 → 账号密码登录 → 授权回调 → 换发令牌。
func loginSenseNovaWithClient(baseClient *http.Client, username, password string) (*SenseNovaToken, error) {
	pkce, err := newSenseNovaPKCE()
	if err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: 生成 PKCE 参数失败: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: 初始化 cookie 存储失败: %w", err)
	}
	// 登录流程需要读取每一步的 302 Location，不自动跟随重定向。
	client := &http.Client{
		Transport: baseClient.Transport,
		Jar:       jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), senseNovaLoginTimeout)
	defer cancel()

	// 步骤 A：授权端点 → 302 登录页（携带 login_challenge），并下发 CSRF cookie。
	authParams := url.Values{}
	authParams.Set("response_type", "code")
	authParams.Set("client_id", senseNovaClientID)
	authParams.Set("code_challenge_method", "S256")
	authParams.Set("code_challenge", pkce.challenge)
	authParams.Set("redirect_uri", senseNovaRedirectURI)
	authParams.Set("scope", "openid offline offline_access")
	authParams.Set("state", pkce.state)
	authURL := senseNovaAuthURL + "?" + authParams.Encode()
	authURLParsed, _ := url.Parse(authURL)
	resp, err := senseNovaGet(client, ctx, authURL)
	if err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: 授权请求失败: %w", err)
	}
	if resp.StatusCode != http.StatusFound {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("sensenova 登录失败: 授权接口返回 %d", resp.StatusCode)
	}
	loginChallenge, err := parseSenseNovaLoginChallenge(resp.Header.Get("Location"))
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	csrf := senseNovaCSRFToken(jar, authURLParsed)
	if csrf == "" {
		return nil, fmt.Errorf("sensenova 登录失败: 授权响应缺少 %s cookie", senseNovaCSRFCookieName)
	}

	// 步骤 B：账号密码登录 → 返回授权重定向地址（字段名 redirect）。
	loginPayload, err := common.Marshal(map[string]string{
		"challenge":   loginChallenge,
		"tenant_code": "",
		"username":    username,
		"password":    password,
		"login_type":  "username",
		"code_key":    "",
	})
	if err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: 构造登录请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, senseNovaLoginAPIURL, strings.NewReader(string(loginPayload)))
	if err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: 构造登录请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", senseNovaCSRFCookieName+"="+csrf)
	req.Header.Set("Referer", "https://platform.sensenova.cn/login")
	req.Header.Set("Origin", "https://platform.sensenova.cn")
	req.Header.Set("Accept-Language", "zh-CN")
	req.Header.Set("User-Agent", senseNovaUserAgent)
	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: 登录请求失败: %w", err)
	}
	loginBody, err := readSenseNovaBody(resp)
	if err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: 读取登录响应失败: %w", err)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("sensenova 登录失败: 用户名或密码错误")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sensenova 登录失败: 登录接口返回 %d", resp.StatusCode)
	}
	var loginResp struct {
		Redirect string `json:"redirect"`
	}
	if err := common.Unmarshal(loginBody, &loginResp); err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: 解析登录响应失败: %w", err)
	}
	redirectURL := strings.TrimSpace(loginResp.Redirect)
	if redirectURL == "" {
		return nil, fmt.Errorf("sensenova 登录失败: 登录未返回跳转地址")
	}
	// 安全守卫：登录跳转只允许回到平台授权域名，防止上游异常时被引向任意地址。
	parsedRedirect, err := url.Parse(redirectURL)
	if err != nil || parsedRedirect.Scheme != "https" || parsedRedirect.Host != senseNovaPlatformHost {
		return nil, fmt.Errorf("sensenova 登录失败: 登录跳转地址非法")
	}

	// 步骤 C：跟随授权回调重定向链（上游可能经过 consent 页），直到某跳
	// Location 携带 code 为止；每跳均校验跳转目标 host 白名单。
	code, err := senseNovaFollowAuthRedirects(client, ctx, redirectURL)
	if err != nil {
		return nil, err
	}

	// 步骤 D：用授权码换发令牌。
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", senseNovaRedirectURI)
	form.Set("client_id", senseNovaClientID)
	form.Set("code_verifier", pkce.verifier)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, senseNovaTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: 构造令牌请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", senseNovaUserAgent)
	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: 令牌请求失败: %w", err)
	}
	tokenBody, err := readSenseNovaBody(resp)
	if err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: 读取令牌响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sensenova 登录失败: 令牌接口返回 %d", resp.StatusCode)
	}
	token, err := parseSenseNovaTokenResponse(tokenBody, "")
	if err != nil {
		return nil, fmt.Errorf("sensenova 登录失败: %w", err)
	}
	return token, nil
}

// refreshSenseNovaToken 用 refresh_token 换取新访问令牌。
func refreshSenseNovaToken(client *http.Client, refreshToken string) (*SenseNovaToken, error) {
	ctx, cancel := context.WithTimeout(context.Background(), senseNovaLoginTimeout)
	defer cancel()
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", senseNovaClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, senseNovaTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("sensenova 续期失败: 构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", senseNovaUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sensenova 续期失败: %w", err)
	}
	body, err := readSenseNovaBody(resp)
	if err != nil {
		return nil, fmt.Errorf("sensenova 续期失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sensenova 续期失败: 令牌接口返回 %d", resp.StatusCode)
	}
	return parseSenseNovaTokenResponse(body, refreshToken)
}

// parseSenseNovaTokenResponse 解析令牌接口响应；响应未携带 refresh_token 时
// 沿用 fallbackRefreshToken。
func parseSenseNovaTokenResponse(body []byte, fallbackRefreshToken string) (*SenseNovaToken, error) {
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("解析令牌响应失败: %w", err)
	}
	accessToken := strings.TrimSpace(raw.AccessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("令牌响应缺少 access_token")
	}
	refreshToken := strings.TrimSpace(raw.RefreshToken)
	if refreshToken == "" {
		refreshToken = fallbackRefreshToken
	}
	ttl := time.Duration(raw.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = senseNovaDefaultTokenTTL
	}
	return &SenseNovaToken{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(ttl),
	}, nil
}

// fetchSenseNovaPoolUsageWithClient 查询积分池用量；401/403 视为令牌失效。
func fetchSenseNovaPoolUsageWithClient(client *http.Client, accessToken string) (*SenseNovaUsageInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), senseNovaPoolUsageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, senseNovaPoolUsageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("获取积分池用量失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", senseNovaUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取积分池用量失败: %w", err)
	}
	body, err := readSenseNovaBody(resp)
	if err != nil {
		return nil, fmt.Errorf("获取积分池用量失败: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w", errSenseNovaTokenInvalid)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("获取积分池用量失败: 接口返回 %d", resp.StatusCode)
	}
	info, err := parseSenseNovaPoolUsage(body)
	if err != nil {
		return nil, fmt.Errorf("获取积分池用量失败: %w", err)
	}
	return info, nil
}

// senseNovaPoolUsageWire 是 pool-usage 响应的原始结构（数值字段全部为字符串）。
type senseNovaPoolUsageWire struct {
	Plan struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"plan"`
	Pools []struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		ModelIDs []string `json:"model_ids"`
		PoolType string   `json:"pool_type"`
		Window5h struct {
			Limit     string `json:"limit"`
			Used      string `json:"used"`
			Remaining string `json:"remaining"`
			ResetAt   string `json:"reset_at"`
		} `json:"window_5h"`
		Window7d struct {
			Limit     string `json:"limit"`
			Used      string `json:"used"`
			Remaining string `json:"remaining"`
			ResetAt   string `json:"reset_at"`
		} `json:"window_7d"`
		GrantBalance                string `json:"grant_balance"`
		NearestGrantExpiry          string `json:"nearest_grant_expiry"`
		NearestGrantExpiringBalance string `json:"nearest_grant_expiring_balance"`
	} `json:"pools"`
}

// parseSenseNovaPoolUsage 解析 pool-usage 响应：字符串数值容错转换，
// 秒级 epoch 转 RFC3339（0/空 → 空串）。
func parseSenseNovaPoolUsage(body []byte) (*SenseNovaUsageInfo, error) {
	var wire senseNovaPoolUsageWire
	if err := common.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("invalid pool-usage json: %w", err)
	}
	info := &SenseNovaUsageInfo{
		Plan: SenseNovaPlan{
			ID:   strings.TrimSpace(wire.Plan.ID),
			Name: strings.TrimSpace(wire.Plan.Name),
		},
	}
	for _, p := range wire.Pools {
		info.Pools = append(info.Pools, SenseNovaPoolUsage{
			PoolType: strings.TrimSpace(p.PoolType),
			Name:     strings.TrimSpace(p.Name),
			ModelIDs: p.ModelIDs,
			Window5h: SenseNovaWindow{
				Limit:     senseNovaParseFloat(p.Window5h.Limit),
				Used:      senseNovaParseFloat(p.Window5h.Used),
				Remaining: senseNovaParseFloat(p.Window5h.Remaining),
				ResetAt:   senseNovaEpochToRFC3339(p.Window5h.ResetAt),
			},
			Window7d: SenseNovaWindow{
				Limit:     senseNovaParseFloat(p.Window7d.Limit),
				Used:      senseNovaParseFloat(p.Window7d.Used),
				Remaining: senseNovaParseFloat(p.Window7d.Remaining),
				ResetAt:   senseNovaEpochToRFC3339(p.Window7d.ResetAt),
			},
			GrantBalance:                senseNovaParseFloat(p.GrantBalance),
			NearestGrantExpiry:          senseNovaEpochToRFC3339(p.NearestGrantExpiry),
			NearestGrantExpiringBalance: senseNovaParseFloat(p.NearestGrantExpiringBalance),
		})
	}
	return info, nil
}

// senseNovaParseFloat 将上游字符串数值安全转为 float64；空串/非数字 → 0。
func senseNovaParseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// senseNovaEpochToRFC3339 将秒级 epoch（容错毫秒）转为 RFC3339 UTC；
// 0/空/非法 → 空串（不渲染成 1970）。
func senseNovaEpochToRFC3339(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	epoch, err := strconv.ParseInt(s, 10, 64)
	if err != nil || epoch <= 0 {
		return ""
	}
	if epoch >= 1e12 {
		epoch /= 1000
	}
	return time.Unix(epoch, 0).UTC().Format(time.RFC3339)
}

// senseNovaFollowAuthRedirects 从步骤 B 返回的授权重定向地址开始，用同一
// client（带 cookie jar，跨域跳转时按域自动分发会话 cookie）循环跟随 302/303
// 跳转，直到某跳 Location 携带 code 参数为止；返回该 code。
// 每跳均校验目标 https + host 白名单；非重定向响应、空 Location 或超过最大
// 跳数均报错。Location 为相对路径时基于当前请求 URL 解析。
func senseNovaFollowAuthRedirects(client *http.Client, ctx context.Context, startURL string) (string, error) {
	current := startURL
	for hop := 0; hop < senseNovaMaxAuthRedirects; hop++ {
		resp, err := senseNovaGet(client, ctx, current)
		if err != nil {
			return "", fmt.Errorf("sensenova 登录失败: 授权回调请求失败: %w", err)
		}
		location := resp.Header.Get("Location")
		if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
			_ = resp.Body.Close()
			return "", fmt.Errorf("sensenova 登录失败: 授权回调返回 %d", resp.StatusCode)
		}
		if location == "" {
			_ = resp.Body.Close()
			return "", fmt.Errorf("sensenova 登录失败: 授权回调缺少跳转地址")
		}
		parsed, err := url.Parse(location)
		if err != nil {
			_ = resp.Body.Close()
			return "", fmt.Errorf("sensenova 登录失败: 无效的回调跳转地址: %w", err)
		}
		if !parsed.IsAbs() {
			base, berr := url.Parse(current)
			if berr != nil {
				_ = resp.Body.Close()
				return "", fmt.Errorf("sensenova 登录失败: 无效的授权回调地址: %w", berr)
			}
			parsed = base.ResolveReference(parsed)
		}
		if parsed.Scheme != "https" || !senseNovaIsAllowedRedirectHost(parsed.Host) {
			_ = resp.Body.Close()
			return "", fmt.Errorf("sensenova 登录失败: 授权回调跳转地址非法")
		}
		_ = resp.Body.Close()
		if code := parsed.Query().Get("code"); code != "" {
			return code, nil
		}
		current = parsed.String()
	}
	return "", fmt.Errorf("sensenova 登录失败: 授权回调跳转次数超过 %d 次", senseNovaMaxAuthRedirects)
}

// senseNovaIsAllowedRedirectHost 判断跳转目标 host 是否在白名单内（忽略端口）。
func senseNovaIsAllowedRedirectHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	_, ok := senseNovaRedirectHosts[host]
	return ok
}

// senseNovaGet 发起带 UA 的 GET 请求（调用方负责关闭响应体）。
func senseNovaGet(client *http.Client, ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", senseNovaUserAgent)
	return client.Do(req)
}

// readSenseNovaBody 读取并关闭响应体。
func readSenseNovaBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// senseNovaCSRFToken 从 cookie jar 中提取 oauth2_authentication_csrf。
func senseNovaCSRFToken(jar *cookiejar.Jar, u *url.URL) string {
	if jar == nil || u == nil {
		return ""
	}
	for _, c := range jar.Cookies(u) {
		if c.Name == senseNovaCSRFCookieName {
			return c.Value
		}
	}
	return ""
}

// parseSenseNovaLoginChallenge 从授权页跳转地址提取 login_challenge。
func parseSenseNovaLoginChallenge(location string) (string, error) {
	u, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("sensenova 登录失败: 无效的授权跳转地址: %w", err)
	}
	challenge := u.Query().Get("login_challenge")
	if challenge == "" {
		return "", fmt.Errorf("sensenova 登录失败: 授权跳转缺少 login_challenge")
	}
	return challenge, nil
}
