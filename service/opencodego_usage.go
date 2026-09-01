package service

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	openCodeGoUsageURL            = "https://opencode.ai/zen/go/v1/usage"
	openCodeGoUsageRequestTimeout = 15 * time.Second
)

// OpenCodeGoWindow 是 opencode 单个时间窗口的用量数据。
type OpenCodeGoWindow struct {
	Period           string  // "session" | "weekly" | "monthly"
	Status           string  // 上游 status（"ok" 等），原样透传
	UsedPercent      float64 // 已用%（0-100）；上游 percent 缺失/无效时为 -1
	RemainingPercent float64 // 100-UsedPercent, clamp 0-100；无效时为 -1
	ResetInSec       int64   // 重置倒计时秒（由 resetsAt 计算，保留以兼容现有前端展示）
	ResetAt          string  // 官方 resetsAt（RFC3339）
}

// OpenCodeGoUsageInfo 是 opencode 官方 usage API 解析结果。
type OpenCodeGoUsageInfo struct {
	Windows []OpenCodeGoWindow
}

// openCodeGoUsageResponse 对应官方 usage API 的 JSON 结构。
type openCodeGoUsageResponse struct {
	Usage openCodeGoUsageBody `json:"usage"`
}

// openCodeGoUsageBody 持有三个时间窗口。自定义 UnmarshalJSON 处理
// rolling / rolling_usage / session 的别名回退（官方字段为 rolling）。
type openCodeGoUsageBody struct {
	Rolling *openCodeGoWindowRaw
	Weekly  *openCodeGoWindowRaw
	Monthly *openCodeGoWindowRaw
}

func (u *openCodeGoUsageBody) UnmarshalJSON(data []byte) error {
	var raw struct {
		Rolling      *openCodeGoWindowRaw `json:"rolling"`
		RollingUsage *openCodeGoWindowRaw `json:"rolling_usage"`
		Session      *openCodeGoWindowRaw `json:"session"`
		Weekly       *openCodeGoWindowRaw `json:"weekly"`
		Monthly      *openCodeGoWindowRaw `json:"monthly"`
	}
	if err := common.Unmarshal(data, &raw); err != nil {
		return err
	}
	u.Weekly = raw.Weekly
	u.Monthly = raw.Monthly
	switch {
	case raw.Rolling != nil:
		u.Rolling = raw.Rolling
	case raw.RollingUsage != nil:
		u.Rolling = raw.RollingUsage
	default:
		u.Rolling = raw.Session
	}
	return nil
}

// openCodeGoWindowRaw 是官方 usage API 单个窗口的原始字段。
// percent 用 any 以容忍上游返回非数字值；percent/usage_percent、
// resetsAt/reset_at 均支持别名回退。
type openCodeGoWindowRaw struct {
	Status       string `json:"status"`
	Percent      any    `json:"percent"`
	UsagePercent any    `json:"usage_percent"`
	ResetsAt     string `json:"resetsAt"`
	ResetAt      string `json:"reset_at"`
}

// FetchOpenCodeGoUsage 通过官方 usage API 获取 opencode 账号级用量信息。
// 使用渠道已有的 API Key（Bearer 认证），不再依赖工作区页面与会话 Cookie。
func FetchOpenCodeGoUsage(channel *model.Channel) (*OpenCodeGoUsageInfo, error) {
	keys := channel.GetKeys()
	if len(keys) == 0 || strings.TrimSpace(keys[0]) == "" {
		return nil, fmt.Errorf("opencode api key 未配置")
	}
	key := strings.TrimSpace(keys[0])

	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), openCodeGoUsageRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openCodeGoUsageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("opencode api key 无效或已过期")
	case resp.StatusCode == http.StatusForbidden || strings.Contains(string(body), "EntitlementError"):
		return nil, fmt.Errorf("该 API Key 无 OpenCode Go 订阅")
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("opencode usage api 请求失败, status code: %d", resp.StatusCode)
	}
	return parseOpenCodeGoUsage(body)
}

// parseOpenCodeGoUsage 解析官方 usage API 响应。
// 窗口固定按 session -> weekly -> monthly 顺序返回；某窗口缺失则不出现；
// 某个窗口存在但 status 非 "ok" 或 percent 无效时仍返回（used/remaining 为 -1，
// status 原样透传，供前端显示 "-"）。没有任何窗口时返回错误。
func parseOpenCodeGoUsage(body []byte) (*OpenCodeGoUsageInfo, error) {
	var resp openCodeGoUsageResponse
	if err := common.UnmarshalJsonStr(string(body), &resp); err != nil {
		return nil, fmt.Errorf("无法解析 opencode usage api 响应: %w", err)
	}
	windows := make([]OpenCodeGoWindow, 0, 3)
	if resp.Usage.Rolling != nil {
		windows = append(windows, toOpenCodeGoWindow("session", resp.Usage.Rolling))
	}
	if resp.Usage.Weekly != nil {
		windows = append(windows, toOpenCodeGoWindow("weekly", resp.Usage.Weekly))
	}
	if resp.Usage.Monthly != nil {
		windows = append(windows, toOpenCodeGoWindow("monthly", resp.Usage.Monthly))
	}
	if len(windows) == 0 {
		return nil, fmt.Errorf("opencode usage api 未返回任何用量窗口")
	}
	return &OpenCodeGoUsageInfo{Windows: windows}, nil
}

// toOpenCodeGoWindow 将上游原始窗口转换为语义字段。percent 缺失/无效时
// UsedPercent 与 RemainingPercent 置 -1（约定：前端显示 "-"），status 原样透传。
func toOpenCodeGoWindow(period string, raw *openCodeGoWindowRaw) OpenCodeGoWindow {
	w := OpenCodeGoWindow{
		Period:           period,
		Status:           raw.Status,
		ResetAt:          raw.ResetsAt,
		UsedPercent:      -1,
		RemainingPercent: -1,
	}
	if w.ResetAt == "" {
		w.ResetAt = raw.ResetAt
	}
	if percent, ok := openCodeGoWindowPercent(raw); ok {
		used := max(0.0, min(100.0, percent))
		w.UsedPercent = used
		w.RemainingPercent = max(0.0, min(100.0, 100-used))
	}
	w.ResetInSec = openCodeGoResetInSec(w.ResetAt)
	return w
}

// openCodeGoWindowPercent 提取窗口 percent（支持 percent/usage_percent 别名），
// 数字与数字字符串均接受；缺失或非数字返回 ok=false。
func openCodeGoWindowPercent(raw *openCodeGoWindowRaw) (float64, bool) {
	value := raw.Percent
	if value == nil {
		value = raw.UsagePercent
	}
	if value == nil {
		return 0, false
	}
	var f float64
	switch n := value.(type) {
	case float64:
		f = n
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, false
		}
		f = parsed
	default:
		return 0, false
	}
	if math.IsNaN(f) {
		return 0, false
	}
	return f, true
}

// openCodeGoResetInSec 计算距 resetAt（RFC3339）的剩余秒数；解析失败或已过期时返回 0。
func openCodeGoResetInSec(resetAt string) int64 {
	t, err := time.Parse(time.RFC3339, resetAt)
	if err != nil {
		return 0
	}
	remain := int64(time.Until(t).Seconds())
	if remain < 0 {
		return 0
	}
	return remain
}
