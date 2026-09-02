package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

const radeonCloudUsageTimeout = 15 * time.Second

// AMD Radeon Cloud 用量查询的错误哨兵，供 controller 做错误分类
// （credentials_expired / fetch_failed / usage_schema_unknown），
// 响应中不泄露任何凭证信息。
var (
	// ErrRadeonCloudUnauthorized 表示 API Key 无效或已过期（HTTP 401）。
	ErrRadeonCloudUnauthorized = errors.New("radeon cloud api key 无效或已过期")
	// ErrRadeonCloudFetchFailed 表示网络/HTTP 层请求失败（含非 2xx 状态码）。
	ErrRadeonCloudFetchFailed = errors.New("radeon cloud usage api 请求失败")
	// ErrRadeonCloudSchema 表示上游响应结构无法解析。
	ErrRadeonCloudSchema = errors.New("radeon cloud usage 响应结构无法解析")
)

// RadeonCloudUsageInfo 是 AMD Radeon Cloud 每日免费额度用量摘要。
// 金额字段（USD）统一乘以 1e6 转为 points 展示：1 USD = 1000000 pts。
type RadeonCloudUsageInfo struct {
	RpmLimit             int     // 每分钟请求上限
	DailyLimitPoints     float64 // 每日额度（points）
	DailyUsedPoints      float64 // 今日已用（points）
	DailyRemainingPoints float64 // 今日剩余（points）
	DailyUsedPercent     float64 // 已用占每日额度的比例（0-1，保留 4 位小数）
	DailyResetAt         string  // 每日额度重置时刻（RFC3339 UTC）
	DailyResetInSec      int64   // 距每日重置的秒数（已过期置 0）
	TodayRequests        int     // 今日请求数
	TodayTokens          int64   // 今日 tokens
	Last24hRequests      int     // 最近 24h 请求数
	Last24hTokens        int64   // 最近 24h tokens
	Last24hLastRequestAt string  // 最近 24h 最后一次请求时刻
	PeriodStartedAt      string  // 计费周期起始时刻
}

// FetchRadeonCloudUsage 通过官方 /api/v1/usage 端点查询 AMD Radeon Cloud
// 渠道的每日免费额度用量（Bearer 认证，仅 Authorization 与 Accept 两个请求头）。
// base 取 channel.GetBaseURL()，为空时回退到渠道默认 BaseURL。
func FetchRadeonCloudUsage(channel *model.Channel) (*RadeonCloudUsageInfo, error) {
	if channel == nil {
		return nil, fmt.Errorf("nil channel")
	}
	keys := channel.GetKeys()
	if len(keys) == 0 || strings.TrimSpace(keys[0]) == "" {
		return nil, fmt.Errorf("radeon cloud api key 未配置")
	}
	key := strings.TrimSpace(keys[0])

	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.GetChannelBaseURL(constant.ChannelTypeRadeonCloud)
	}
	// 兼容渠道配置的 Base URL 带或不带 "/api" 前缀两种写法，
	// 避免拼接出 "/api/api/v1/usage" 的错误路径。
	base := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/api")
	usageURL := fmt.Sprintf("%s/api/v1/usage", base)

	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRadeonCloudFetchFailed, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), radeonCloudUsageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRadeonCloudFetchFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRadeonCloudFetchFailed, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRadeonCloudFetchFailed, err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrRadeonCloudUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status code %d", ErrRadeonCloudFetchFailed, resp.StatusCode)
	}
	info, err := parseRadeonCloudUsage(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRadeonCloudSchema, err)
	}
	return info, nil
}

// parseRadeonCloudUsage 宽松解析 /api/v1/usage 响应：字段可能缺失或类型
// 变化（数字可能是 float64 或整数或数字字符串），缺失一律兜底为 0。
func parseRadeonCloudUsage(body []byte) (*RadeonCloudUsageInfo, error) {
	var raw map[string]any
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid json body: %w", err)
	}

	info := &RadeonCloudUsageInfo{
		RpmLimit:        radeonJSONInt(raw, "rpm_limit"),
		DailyResetAt:    radeonJSONString(raw, "daily_reset_at"),
		PeriodStartedAt: radeonJSONString(raw, "period_started_at"),
	}
	limitUSD := radeonJSONFloat(raw, "daily_cost_limit_usd")
	usedUSD := radeonJSONFloat(raw, "daily_cost_used_usd")
	remainingUSD := radeonJSONFloat(raw, "daily_cost_remaining_usd")
	info.DailyLimitPoints = radeonPointsFromUSD(limitUSD)
	info.DailyUsedPoints = radeonPointsFromUSD(usedUSD)
	info.DailyRemainingPoints = radeonPointsFromUSD(remainingUSD)
	if limitUSD > 0 && !math.IsNaN(usedUSD) {
		info.DailyUsedPercent = radeonRound(usedUSD/limitUSD, 4)
	}
	info.DailyResetInSec = radeonResetInSec(raw)

	if today, ok := radeonJSONMap(raw, "today"); ok {
		info.TodayRequests = radeonJSONInt(today, "requests")
		info.TodayTokens = radeonJSONInt64(today, "total_tokens")
	}
	if last24h, ok := radeonJSONMap(raw, "last_24_hours"); ok {
		info.Last24hRequests = radeonJSONInt(last24h, "requests")
		info.Last24hTokens = radeonJSONInt64(last24h, "total_tokens")
		info.Last24hLastRequestAt = radeonJSONString(last24h, "last_request_at")
	}
	return info, nil
}

// radeonPointsFromUSD 将 USD 金额乘以 1e6 转为 points，保留两位小数。
// 非法（NaN/负数）输入兜底为 0。
func radeonPointsFromUSD(usd float64) float64 {
	if math.IsNaN(usd) || usd < 0 {
		return 0
	}
	return radeonRound(usd*1e6, 2)
}

// radeonRound 将 v 保留 digits 位小数（四舍五入）；NaN 兜底为 0。
func radeonRound(v float64, digits int) float64 {
	if math.IsNaN(v) {
		return 0
	}
	pow := math.Pow(10, float64(digits))
	return math.Round(v*pow) / pow
}

// radeonResetInSec 计算距每日额度重置的剩余秒数。优先使用
// daily_reset_epoch（秒时间戳），缺失时回退解析 daily_reset_at（RFC3339）。
// 无法解析或已过期时返回 0。
func radeonResetInSec(raw map[string]any) int64 {
	epoch := radeonJSONInt64(raw, "daily_reset_epoch")
	var t time.Time
	switch {
	case epoch > 0:
		t = time.Unix(epoch, 0)
	case radeonJSONString(raw, "daily_reset_at") != "":
		t, _ = time.Parse(time.RFC3339, radeonJSONString(raw, "daily_reset_at"))
	}
	if t.IsZero() {
		return 0
	}
	remain := int64(time.Until(t).Seconds())
	if remain < 0 {
		return 0
	}
	return remain
}

func radeonJSONMap(m map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			return nested, true
		}
	}
	return nil, false
}

func radeonJSONString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if s, ok := value.(string); ok {
				return s
			}
		}
	}
	return ""
}

// radeonJSONFloatOK 提取数字字段：接受 JSON number（float64）、整数与
// 数字字符串；NaN 视为缺失。返回 ok=false 表示字段缺失或类型不兼容。
func radeonJSONFloatOK(m map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		switch n := value.(type) {
		case float64:
			if math.IsNaN(n) {
				continue
			}
			return n, true
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil && !math.IsNaN(f) {
				return f, true
			}
		}
	}
	return 0, false
}

func radeonJSONFloat(m map[string]any, keys ...string) float64 {
	f, _ := radeonJSONFloatOK(m, keys...)
	return f
}

func radeonJSONInt(m map[string]any, keys ...string) int {
	f, ok := radeonJSONFloatOK(m, keys...)
	if !ok {
		return 0
	}
	return int(f)
}

func radeonJSONInt64(m map[string]any, keys ...string) int64 {
	f, ok := radeonJSONFloatOK(m, keys...)
	if !ok {
		return 0
	}
	return int64(f)
}
