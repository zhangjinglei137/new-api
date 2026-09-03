package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

const moonshotCodingPlanUsageTimeout = 15 * time.Second

// moonshot 用量查询的错误哨兵，供 controller 做错误分类（credentials /
// fetch_failed / schema），响应中不泄露任何凭证信息。
var (
	// ErrMoonshotCodingPlanUnauthorized 表示 API Key 无效或已过期（HTTP 401）。
	ErrMoonshotCodingPlanUnauthorized = errors.New("moonshot api key 无效或已过期")
	// ErrMoonshotCodingPlanNoPlan 表示该 API Key 无 Kimi Coding Plan 订阅（HTTP 403）。
	ErrMoonshotCodingPlanNoPlan = errors.New("该 API Key 无 Kimi Coding Plan 订阅")
	// ErrMoonshotCodingPlanSchema 表示上游响应结构无法解析。
	ErrMoonshotCodingPlanSchema = errors.New("moonshot coding plan usage 响应结构无法解析")
)

// MoonshotCodingPlanWindow 是 Kimi Coding Plan 单个时间窗口的用量数据。
type MoonshotCodingPlanWindow struct {
	Period           string  // "session"(5h 滚动窗) | "weekly"(周配额)
	UsedPercent      float64 // 0-100
	RemainingPercent float64 // 0-100
	ResetAt          string  // RFC3339 UTC
	ResetInSec       int64
}

// MoonshotCodingPlanUsageInfo 是 Kimi Coding Plan 用量解析结果。
type MoonshotCodingPlanUsageInfo struct {
	Status  string
	Windows []MoonshotCodingPlanWindow
}

// FetchMoonshotCodingPlanUsage 通过 Kimi 官方 /usages 端点查询 Coding Plan 余量/用量。
// 使用渠道第一个 key（Bearer 认证），仅发送 Authorization 与 Accept 两个请求头。
// 目标 URL 由 ResolveSpecialPlan 解析（https://api.kimi.com/coding/v1/usages），
// 未命中 Coding Plan 套餐时返回错误。
func FetchMoonshotCodingPlanUsage(channel *model.Channel) (*MoonshotCodingPlanUsageInfo, error) {
	if channel == nil {
		return nil, fmt.Errorf("nil channel")
	}
	keys := channel.GetKeys()
	if len(keys) == 0 || strings.TrimSpace(keys[0]) == "" {
		return nil, fmt.Errorf("moonshot api key 未配置")
	}
	key := strings.TrimSpace(keys[0])

	baseURL := channel.GetBaseURL()
	specialPlan, _, ok := constant.ResolveSpecialPlan(constant.ChannelTypeMoonshot, baseURL, channel.GetOtherSettings().EndpointProfile)
	if !ok || specialPlan.OpenAIBaseURL == "" {
		return nil, fmt.Errorf("moonshot coding plan 未启用")
	}
	usageURL := fmt.Sprintf("%s/usages", strings.TrimRight(specialPlan.OpenAIBaseURL, "/"))

	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), moonshotCodingPlanUsageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
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
		return nil, ErrMoonshotCodingPlanUnauthorized
	case resp.StatusCode == http.StatusForbidden:
		return nil, ErrMoonshotCodingPlanNoPlan
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("moonshot usage api 请求失败, status code: %d", resp.StatusCode)
	}
	info, err := parseMoonshotCodingPlanUsage(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMoonshotCodingPlanSchema, err)
	}
	return info, nil
}

// parseMoonshotCodingPlanUsage 宽松解析 /usages 响应：数字字段可能是字符串或数字，
// 字段别名（resetTime/resetAt/reset_at/reset_time）需兼容回退。
// 窗口顺序固定为 session -> weekly，缺失的窗口不出现；无法计算百分比的窗口被跳过。
func parseMoonshotCodingPlanUsage(body []byte) (*MoonshotCodingPlanUsageInfo, error) {
	var raw map[string]any
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid json body: %w", err)
	}

	info := &MoonshotCodingPlanUsageInfo{Status: jsonString(raw, "status", "planStatus")}
	byPeriod := make(map[string]MoonshotCodingPlanWindow, 2)

	// 顶层 usage：周配额（weekly）。
	if usage, ok := jsonMapGet(raw, "usage", "quota"); ok {
		if window, ok := moonshotWindowFromFields("weekly", usage); ok {
			byPeriod["weekly"] = window
		}
	}

	// limits 数组：duration=300（分钟）的 5h 滚动窗 → session，其余窗口跳过。
	for _, item := range jsonMapSlice(raw, "limits") {
		if !moonshotIsSessionWindow(item) {
			continue
		}
		detail, ok := jsonMapGet(item, "detail", "usage")
		if !ok {
			continue
		}
		if window, ok := moonshotWindowFromFields("session", detail); ok {
			byPeriod["session"] = window
		}
	}

	if len(byPeriod) == 0 {
		return nil, fmt.Errorf("no usable usage window found")
	}
	for _, period := range []string{"session", "weekly"} {
		if window, ok := byPeriod[period]; ok {
			info.Windows = append(info.Windows, window)
		}
	}
	return info, nil
}

// moonshotIsSessionWindow 判断 limits 条目是否命中 5h 滚动窗
// （duration=300 且 timeUnit=TIME_UNIT_MINUTE；时间单位缺失时仅看 duration）。
func moonshotIsSessionWindow(item map[string]any) bool {
	window, ok := jsonMapGet(item, "window")
	if !ok {
		return false
	}
	timeUnit := strings.ToUpper(jsonString(window, "timeUnit", "time_unit", "unit"))
	if timeUnit != "" && timeUnit != "TIME_UNIT_MINUTE" && timeUnit != "MINUTE" {
		return false
	}
	return jsonInt64Clamped(window, "duration") == 300
}

// moonshotWindowFromFields 从单个窗口字段构造用量数据。limit 无效或
// used/remaining 全部缺失（无法计算百分比）时返回 false，跳过该窗口。
func moonshotWindowFromFields(period string, m map[string]any) (MoonshotCodingPlanWindow, bool) {
	limit := jsonFloat(m, "limit", "quota", "quotaLimit")
	if limit <= 0 {
		return MoonshotCodingPlanWindow{}, false
	}
	used, usedOK := jsonFloatOK(m, "used", "usage")
	if !usedOK {
		// used 缺失时用 limit-remaining 兜底。
		if remaining, remainingOK := jsonFloatOK(m, "remaining", "remain", "quotaRemaining"); remainingOK {
			used = limit - remaining
			usedOK = true
		}
	}
	if !usedOK {
		return MoonshotCodingPlanWindow{}, false
	}
	if used < 0 {
		used = 0
	}
	usedPercent := clampMoonshotPercent(used / limit * 100)
	window := MoonshotCodingPlanWindow{
		Period:           period,
		UsedPercent:      usedPercent,
		RemainingPercent: clampMoonshotPercent(100 - usedPercent),
	}
	if resetAt, resetInSec, ok := moonshotResetTime(m); ok {
		window.ResetAt = resetAt
		window.ResetInSec = resetInSec
	}
	return window, true
}

// moonshotResetTime 解析窗口 reset 时间（支持 resetTime/resetAt/reset_at/reset_time
// 别名；字符串 RFC3339 或秒/毫秒时间戳）。解析失败返回 false，ResetAt/ResetInSec
// 保持零值。ResetInSec 为距 reset 的秒数，已过期置 0。
func moonshotResetTime(m map[string]any) (resetAt string, resetInSec int64, ok bool) {
	for _, key := range []string{"resetTime", "resetAt", "reset_at", "reset_time"} {
		value, present := m[key]
		if !present {
			continue
		}
		var t time.Time
		switch n := value.(type) {
		case string:
			trimmed := strings.TrimSpace(n)
			if trimmed == "" {
				continue
			}
			t, _ = time.Parse(time.RFC3339, trimmed)
			if t.IsZero() {
				t, _ = time.Parse("2006-01-02 15:04:05", trimmed)
			}
		case float64:
			sec := clampToInt64(n)
			if sec >= 1e12 {
				sec /= 1000
			}
			t = time.Unix(sec, 0)
		}
		if t.IsZero() {
			continue
		}
		resetAt = t.UTC().Format(time.RFC3339)
		resetInSec = int64(time.Until(t).Seconds())
		if resetInSec < 0 {
			resetInSec = 0
		}
		return resetAt, resetInSec, true
	}
	return "", 0, false
}

// clampMoonshotPercent 将百分比夹到 0-100 并保留两位小数。
func clampMoonshotPercent(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return math.Round(v*100) / 100
}
