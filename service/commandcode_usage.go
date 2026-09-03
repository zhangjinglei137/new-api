package service

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// commandCodeCreditsURL / commandCodeSubscriptionsURL 为上游内部计费接口地址。
// 用 var 而非 const，便于测试覆盖为本地端点验证代理链路。
var (
	commandCodeCreditsURL       = "https://api.commandcode.ai/internal/billing/credits"
	commandCodeSubscriptionsURL = "https://api.commandcode.ai/internal/billing/subscriptions"
)

const (
	commandCodeCreditsTimeout = 15 * time.Second
	commandCodeSubsTimeout    = 6 * time.Second
	commandCodeUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// commandCodeCookieWhitelist 是允许从整段 Cookie 提取并转发的会话 cookie 名，
// 其余 cookie 一律丢弃。
var commandCodeCookieWhitelist = []string{
	"__Secure-commandcode_prod_.session_token",
	"__Secure-commandcode_prod_.session_data",
	"__Host-commandcode_prod_.session_token",
	"__Host-commandcode_prod_.session_data",
	"commandcode_prod_.session_token",
	"commandcode_prod_.session_data",
}

// commandCodePlanSpec 描述已知订阅套餐的月度额度与 5h/weekly 窗口 cap（USD）。
type commandCodePlanSpec struct {
	Monthly  float64
	FiveHour float64
	Weekly   float64
}

// commandCodePlanCatalog 是 planId → 套餐额度的静态目录，用于月度百分比反推。
// individual-provider 为按量付费（无 caps），不收录。
var commandCodePlanCatalog = map[string]commandCodePlanSpec{
	"individual-go":     {Monthly: 10, FiveHour: 3, Weekly: 6},
	"individual-goat":   {Monthly: 70, FiveHour: 14, Weekly: 35},
	"individual-pro":    {Monthly: 30, FiveHour: 3, Weekly: 6},
	"individual-pro-v1": {Monthly: 80, FiveHour: 16, Weekly: 40},
	"individual-max":    {Monthly: 150, FiveHour: 45, Weekly: 90},
	"individual-ultra":  {Monthly: 300, FiveHour: 90, Weekly: 180},
	"teams-pro":         {Monthly: 40, FiveHour: 12, Weekly: 24},
}

// CommandCodeWindow 是 CommandCode 单个时间窗口的用量数据。
// 无效百分比约定与 opencode 一致：UsedPercent/RemainingPercent 为 -1；
// Metered=false 表示 monthly/topup 为金额模式（仅展示剩余金额，不展示百分比，
// 此时 Used 字段承载剩余金额）。
type CommandCodeWindow struct {
	Period           string  // "session" | "weekly" | "monthly" | "topup"
	Status           string  // 窗口状态（"ok"/"exceeded"）
	UsedPercent      float64 // 已用%（0-100）；无效时为 -1
	RemainingPercent float64 // 剩余%（0-100）；无效时为 -1
	Used             float64 // 已用金额（USD）；金额模式下为剩余金额
	Limit            float64 // 额度（USD），未知为 0
	ResetAt          string  // RFC3339 UTC；无重置为空串
	Metered          bool    // 是否为可计量额度窗口；false 表示仅显示剩余金额
}

// CommandCodeUsageInfo 是 CommandCode 用量解析结果。
type CommandCodeUsageInfo struct {
	Windows []CommandCodeWindow
}

// commandCodeCredits 是 /internal/billing/credits 的解析结果。
type commandCodeCredits struct {
	MonthlyCredits   float64 // 剩余月度 grant（USD）
	PurchasedCredits float64 // top-up 余额（USD，独立池、永不过期）
	WindowLimits     *commandCodeWindowLimits
}

// commandCodeWindowLimits 是 5h/weekly 限额窗口。
type commandCodeWindowLimits struct {
	FiveHour *commandCodeLimitWindow
	Weekly   *commandCodeLimitWindow
}

// commandCodeLimitWindow 是单个限额窗口的原始字段。
type commandCodeLimitWindow struct {
	Used     float64
	Cap      float64
	Exceeded bool
	ResetAt  int64 // 毫秒或秒 epoch；0 表示未触碰（无重置）
}

// commandCodeSubscription 是 /internal/billing/subscriptions 的解析结果。
type commandCodeSubscription struct {
	PlanID           string
	CurrentPeriodEnd string // RFC3339，月度重置时间
	HasData          bool   // data 为 null（免费用户）时为 false
}

// FetchCommandCodeUsage 查询 CommandCode 渠道的用量（会话窗口 + 订阅信息）。
// 依赖渠道配置的登录会话 Cookie（commandcode_cookie，先解密、再白名单归一化）。
// 先调 credits（必需），再调 subscriptions（可选 enrichment，失败不阻塞）。
func FetchCommandCodeUsage(channel *model.Channel) (*CommandCodeUsageInfo, error) {
	if channel == nil {
		return nil, fmt.Errorf("nil channel")
	}
	rawCookie := strings.TrimSpace(channel.GetOtherSettings().CommandCodeCookie)
	if rawCookie == "" {
		return nil, fmt.Errorf("commandcode cookie 未配置")
	}
	plainCookie, err := common.DecryptSecret(rawCookie)
	if err != nil {
		return nil, err
	}
	cookie := normalizeCommandCodeCookie(plainCookie)
	if cookie == "" {
		return nil, fmt.Errorf("commandcode cookie 中未找到有效的会话凭证")
	}

	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	return fetchCommandCodeUsageWithClient(client, cookie)
}

// fetchCommandCodeUsageWithClient 在给定 client 上执行 credits + subscriptions
// 查询并组装结果。cookie 已归一化（仅含白名单会话 cookie）。
func fetchCommandCodeUsageWithClient(client *http.Client, cookie string) (*CommandCodeUsageInfo, error) {
	creditsStatus, creditsBody, err := doCommandCodeGet(client, commandCodeCreditsURL, commandCodeCreditsTimeout, cookie)
	if err != nil {
		return nil, err
	}
	switch {
	case creditsStatus == http.StatusUnauthorized || creditsStatus == http.StatusForbidden:
		return nil, fmt.Errorf("commandcode cookie 无效或已过期")
	case creditsStatus < 200 || creditsStatus >= 300:
		return nil, fmt.Errorf("commandcode 用量接口请求失败, status code: %d", creditsStatus)
	}
	credits, err := parseCommandCodeCredits(creditsBody)
	if err != nil {
		return nil, err
	}

	// subscriptions 为可选 enrichment：失败/非 2xx 忽略，不影响 credits 结果。
	sub := &commandCodeSubscription{}
	if subsStatus, subsBody, err := doCommandCodeGet(client, commandCodeSubscriptionsURL, commandCodeSubsTimeout, cookie); err == nil &&
		subsStatus >= 200 && subsStatus < 300 {
		if parsed, perr := parseCommandCodeSubscriptions(subsBody); perr == nil {
			sub = parsed
		}
	}

	return buildCommandCodeUsageInfo(credits, sub), nil
}

// doCommandCodeGet 发起带固定请求头的 GET 请求（登录会话 Cookie、UA、
// Origin/Referer，无 CSRF 头），返回状态码与响应体。
func doCommandCodeGet(client *http.Client, url string, timeout time.Duration, cookie string) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", commandCodeUserAgent)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Origin", "https://commandcode.ai")
	req.Header.Set("Referer", "https://commandcode.ai/")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// normalizeCommandCodeCookie 从整段 Cookie（分号分隔）中仅提取白名单会话
// cookie，重组为新的 Cookie header；无任何白名单 cookie 时返回空串。
func normalizeCommandCodeCookie(cookie string) string {
	whitelist := make(map[string]struct{}, len(commandCodeCookieWhitelist))
	for _, name := range commandCodeCookieWhitelist {
		whitelist[name] = struct{}{}
	}
	parts := make([]string, 0, len(commandCodeCookieWhitelist))
	for _, item := range strings.Split(cookie, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, value, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		if _, ok := whitelist[name]; !ok {
			continue
		}
		parts = append(parts, name+"="+strings.TrimSpace(value))
	}
	return strings.Join(parts, "; ")
}

// parseCommandCodeCredits 解析 /internal/billing/credits 响应。
// 兼容 camelCase / snake_case 字段名，以及 windowLimits 内嵌在 credits
// （旧版）或置于顶层（新版）两种结构。解析不出 credits 时报错。
func parseCommandCodeCredits(body []byte) (*commandCodeCredits, error) {
	var raw map[string]any
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid credits json: %w", err)
	}
	creditsMap, ok := jsonMapGet(raw, "credits")
	if !ok {
		return nil, fmt.Errorf("missing credits in response")
	}
	credits := &commandCodeCredits{
		MonthlyCredits:   jsonFloat(creditsMap, "monthlyCredits", "monthly_credits"),
		PurchasedCredits: jsonFloat(creditsMap, "purchasedCredits", "purchased_credits"),
	}
	limitsMap, ok := jsonMapGet(raw, "windowLimits", "window_limits")
	if !ok {
		limitsMap, ok = jsonMapGet(creditsMap, "windowLimits", "window_limits")
	}
	if ok {
		credits.WindowLimits = parseCommandCodeWindowLimits(limitsMap)
	}
	return credits, nil
}

// parseCommandCodeWindowLimits 解析窗口限额（5h/weekly），单个窗口缺失不报错。
func parseCommandCodeWindowLimits(m map[string]any) *commandCodeWindowLimits {
	limits := &commandCodeWindowLimits{}
	if five, ok := jsonMapGet(m, "fiveHour", "five_hour"); ok {
		limits.FiveHour = &commandCodeLimitWindow{
			Used:     jsonFloat(five, "used"),
			Cap:      jsonFloat(five, "cap"),
			Exceeded: jsonBool(five, "exceeded"),
			ResetAt:  jsonInt64Clamped(five, "resetAt", "reset_at"),
		}
	}
	if weekly, ok := jsonMapGet(m, "weekly"); ok {
		limits.Weekly = &commandCodeLimitWindow{
			Used:     jsonFloat(weekly, "used"),
			Cap:      jsonFloat(weekly, "cap"),
			Exceeded: jsonBool(weekly, "exceeded"),
			ResetAt:  jsonInt64Clamped(weekly, "resetAt", "reset_at"),
		}
	}
	return limits
}

// parseCommandCodeSubscriptions 解析 /internal/billing/subscriptions 响应。
// data 为 null（免费用户）或缺失时返回空订阅（HasData=false），不视为错误。
func parseCommandCodeSubscriptions(body []byte) (*commandCodeSubscription, error) {
	var raw map[string]any
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid subscriptions json: %w", err)
	}
	sub := &commandCodeSubscription{}
	dataMap, ok := jsonMapGet(raw, "data")
	if !ok {
		return sub, nil
	}
	sub.HasData = true
	sub.PlanID = jsonString(dataMap, "planId", "plan_id")
	sub.CurrentPeriodEnd = jsonString(dataMap, "currentPeriodEnd", "current_period_end")
	return sub, nil
}

// buildCommandCodeUsageInfo 将 credits + 订阅解析结果组装为固定顺序的窗口：
// session -> weekly -> monthly -> topup。
func buildCommandCodeUsageInfo(credits *commandCodeCredits, sub *commandCodeSubscription) *CommandCodeUsageInfo {
	windows := make([]CommandCodeWindow, 0, 4)
	if limits := credits.WindowLimits; limits != nil {
		if limits.FiveHour != nil {
			windows = append(windows, commandCodeLimitWindowToWindow("session", limits.FiveHour))
		}
		if limits.Weekly != nil {
			windows = append(windows, commandCodeLimitWindowToWindow("weekly", limits.Weekly))
		}
	}
	windows = append(windows, commandCodeMonthlyWindow(credits, sub))
	if credits.PurchasedCredits > 0 {
		windows = append(windows, CommandCodeWindow{
			Period:           "topup",
			Status:           "ok",
			UsedPercent:      -1,
			RemainingPercent: -1,
			Used:             credits.PurchasedCredits, // 剩余 top-up 金额（永不过期）
			Limit:            0,
			Metered:          false,
		})
	}
	return &CommandCodeUsageInfo{Windows: windows}
}

// commandCodeLimitWindowToWindow 将 5h/weekly 限额窗口转为语义窗口。
// cap 无效（<=0）时百分比置 -1；used/cap 百分比直接读 wire 并 clamp 0-100。
func commandCodeLimitWindowToWindow(period string, raw *commandCodeLimitWindow) CommandCodeWindow {
	w := CommandCodeWindow{
		Period:           period,
		Status:           "ok",
		UsedPercent:      -1,
		RemainingPercent: -1,
		Used:             raw.Used,
		Limit:            raw.Cap,
		ResetAt:          commandCodeResetAt(raw.ResetAt),
		Metered:          true,
	}
	if raw.Exceeded {
		w.Status = "exceeded"
	}
	if raw.Cap > 0 {
		usedPercent := commandCodeClampPercent(raw.Used / raw.Cap * 100)
		w.UsedPercent = usedPercent
		w.RemainingPercent = commandCodeClampPercent(100 - usedPercent)
	}
	return w
}

// commandCodeMonthlyWindow 组装月度窗口。catalog 守卫命中时反推为额度窗口
// （Metered=true）；否则降级为仅显示剩余金额（Metered=false，Used 承载剩余
// 金额、Limit 取 catalog 月度额度（若有），百分比为 -1）。
// resetAt 取订阅 currentPeriodEnd；无订阅 data 时为空串。
func commandCodeMonthlyWindow(credits *commandCodeCredits, sub *commandCodeSubscription) CommandCodeWindow {
	window := CommandCodeWindow{
		Period:           "monthly",
		Status:           "ok",
		UsedPercent:      -1,
		RemainingPercent: -1,
		Used:             credits.MonthlyCredits, // 剩余月度 grant（USD）
		Limit:            0,
		Metered:          false,
	}
	planID := ""
	if sub != nil && sub.HasData {
		planID = sub.PlanID
		window.ResetAt = sub.CurrentPeriodEnd
	}
	if spec, ok := commandCodeMonthlyMetered(planID, credits); ok {
		used := spec.Monthly - credits.MonthlyCredits
		if used < 0 {
			used = 0
		}
		usedPercent := commandCodeClampPercent(used / spec.Monthly * 100)
		window.Metered = true
		window.Used = used
		window.Limit = spec.Monthly
		window.UsedPercent = usedPercent
		window.RemainingPercent = commandCodeClampPercent(100 - usedPercent)
	} else if spec, ok := commandCodePlanCatalog[planID]; ok {
		window.Limit = spec.Monthly
	}
	return window
}

// commandCodeMonthlyMetered 判断月度窗口是否可反推为额度窗口。守卫全部满足才
// 返回 true：planId 在 catalog 中、wire 的 5h/weekly cap 与目录一致、
// 剩余金额不超过目录月度额度、windowLimits 存在。
func commandCodeMonthlyMetered(planID string, credits *commandCodeCredits) (commandCodePlanSpec, bool) {
	spec, ok := commandCodePlanCatalog[planID]
	if !ok {
		return commandCodePlanSpec{}, false
	}
	limits := credits.WindowLimits
	if limits == nil || limits.FiveHour == nil || limits.Weekly == nil {
		return commandCodePlanSpec{}, false
	}
	if limits.FiveHour.Cap != spec.FiveHour || limits.Weekly.Cap != spec.Weekly {
		return commandCodePlanSpec{}, false
	}
	if credits.MonthlyCredits > spec.Monthly {
		return commandCodePlanSpec{}, false
	}
	return spec, true
}

// commandCodeResetAt 将上游 resetAt（毫秒或秒 epoch，0=未触碰）转为 RFC3339
// UTC；0 或无效时返回空串（不渲染成 1970）。
func commandCodeResetAt(epoch int64) string {
	if epoch <= 0 {
		return ""
	}
	if epoch >= 1e12 {
		epoch /= 1000
	}
	return time.Unix(epoch, 0).UTC().Format(time.RFC3339)
}

// commandCodeClampPercent 将百分比 clamp 到 0-100；NaN/Inf 视为 0。
func commandCodeClampPercent(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Max(0, math.Min(100, v))
}
