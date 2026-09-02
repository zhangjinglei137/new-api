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

const zhipuCodingPlanUsageTimeout = 15 * time.Second

// zhipu 用量查询的错误哨兵，供 controller 做错误分类（credentials /
// no_subscription / usage_schema_unknown / fetch_failed），响应中不泄露任何凭证信息。
var (
	// ErrZhipuCodingPlanUnauthorized 表示 API Key 无效或已过期（HTTP 401）。
	ErrZhipuCodingPlanUnauthorized = errors.New("zhipu api key 无效或已过期")
	// ErrZhipuCodingPlanNoPlan 表示该 API Key 无 GLM Coding Plan 订阅（HTTP 403）。
	ErrZhipuCodingPlanNoPlan = errors.New("该 API Key 无 GLM Coding Plan 订阅")
	// ErrZhipuCodingPlanSchema 表示上游响应结构无法解析。
	ErrZhipuCodingPlanSchema = errors.New("zhipu coding plan usage 响应结构无法解析")
)

// ZhipuCodingPlanWindow 是智谱 GLM Coding Plan 单个时间窗口的用量数据。
type ZhipuCodingPlanWindow struct {
	Period           string  // "session"(5h 滚动窗) | "weekly"(周配额) | "monthly"(MCP 月度工具额度)
	UsedPercent      float64 // 0-100
	RemainingPercent float64 // 0-100
	ResetAt          string  // RFC3339 UTC
	ResetInSec       int64
}

// ZhipuCodingPlanUsageInfo 是智谱 GLM Coding Plan 用量解析结果。
type ZhipuCodingPlanUsageInfo struct {
	Status  string
	Level   string
	Windows []ZhipuCodingPlanWindow
}

// zhipuCodingPlanUsageEndpoint 按 ResolveSpecialPlan 命中的魔法键返回余量查询
// 端点与鉴权方式。国内 bigmodel 端点不使用 Bearer 前缀，国际 api.z.ai 需要
// Bearer 前缀（与 Kimi 等统一 Bearer 的渠道不同，注意区分）。
func zhipuCodingPlanUsageEndpoint(magicKey string) (usageURL string, useBearer bool, ok bool) {
	switch magicKey {
	case "glm-coding-plan":
		return "https://open.bigmodel.cn/api/monitor/usage/quota/limit", false, true
	case "glm-coding-plan-international":
		return "https://api.z.ai/api/monitor/usage/quota/limit", true, true
	}
	return "", false, false
}

// FetchZhipuCodingPlanUsage 通过智谱官方 /api/monitor/usage/quota/limit 端点
// 查询 GLM Coding Plan 余量/用量。区域（国内/国际）由 ResolveSpecialPlan 解析
// 出的魔法键决定，并据此区分鉴权方式（国内无 Bearer、国际带 Bearer）。
// 使用渠道第一个 key，仅发送 Authorization 与 Accept 两个请求头。
func FetchZhipuCodingPlanUsage(channel *model.Channel) (*ZhipuCodingPlanUsageInfo, error) {
	if channel == nil {
		return nil, fmt.Errorf("nil channel")
	}
	keys := channel.GetKeys()
	if len(keys) == 0 || strings.TrimSpace(keys[0]) == "" {
		return nil, fmt.Errorf("zhipu api key 未配置")
	}
	key := strings.TrimSpace(keys[0])

	baseURL := channel.GetBaseURL()
	specialPlan, magicKey, ok := constant.ResolveSpecialPlan(constant.ChannelTypeZhipu_v4, baseURL, channel.GetOtherSettings().EndpointProfile)
	if !ok || specialPlan.OpenAIBaseURL == "" {
		return nil, fmt.Errorf("zhipu coding plan 未启用")
	}
	usageURL, useBearer, ok := zhipuCodingPlanUsageEndpoint(magicKey)
	if !ok {
		return nil, fmt.Errorf("zhipu coding plan 未启用")
	}

	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), zhipuCodingPlanUsageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, err
	}
	if useBearer {
		req.Header.Set("Authorization", "Bearer "+key)
	} else {
		req.Header.Set("Authorization", key)
	}
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
		return nil, ErrZhipuCodingPlanUnauthorized
	case resp.StatusCode == http.StatusForbidden:
		return nil, ErrZhipuCodingPlanNoPlan
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("zhipu usage api 请求失败, status code: %d", resp.StatusCode)
	}
	info, err := parseZhipuCodingPlanUsage(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrZhipuCodingPlanSchema, err)
	}
	return info, nil
}

// parseZhipuCodingPlanUsage 宽松解析 /api/monitor/usage/quota/limit 响应：数字字段
// 可能是字符串或数字。窗口识别只按 type+unit+number 的组合判定（见
// zhipuWindowPeriod），其余 limits 行忽略，避免仅凭 unit 误分类（TIME_LIMIT 的
// unit 也可能是 5）。缺失的窗口不出现，不补 0% 假窗口。
func parseZhipuCodingPlanUsage(body []byte) (*ZhipuCodingPlanUsageInfo, error) {
	var raw map[string]any
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid json body: %w", err)
	}

	info := &ZhipuCodingPlanUsageInfo{}
	data, ok := zhipuJSONMap(raw, "data")
	if !ok {
		return nil, fmt.Errorf("no usable usage window found")
	}
	info.Status = zhipuJSONString(data, "status")
	info.Level = zhipuJSONString(data, "level")

	byPeriod := make(map[string]ZhipuCodingPlanWindow, 3)
	for _, item := range zhipuJSONMapSlice(data, "limits") {
		period, matched := zhipuWindowPeriod(item)
		if !matched {
			continue
		}
		window, ok := zhipuWindowFromFields(period, item)
		if !ok {
			continue
		}
		byPeriod[period] = window
	}

	if len(byPeriod) == 0 {
		return nil, fmt.Errorf("no usable usage window found")
	}
	for _, period := range []string{"session", "weekly", "monthly"} {
		if window, ok := byPeriod[period]; ok {
			info.Windows = append(info.Windows, window)
		}
	}
	return info, nil
}

// zhipuWindowPeriod 依据 spec 识别 limits 行的窗口类型：
//   - type ∈ {TOKENS_LIMIT, CREDIT_LIMIT} 且 unit==3 且 number==5 → session（5h 滚动窗）
//   - type ∈ {TOKENS_LIMIT, CREDIT_LIMIT} 且 unit==6 且 number==1 → weekly（周配额）
//   - type == TIME_LIMIT → monthly（MCP 月度工具额度）
//
// 只按上述组合命中，其余行忽略（TIME_LIMIT 的 unit 也可能是 5，不按 unit 误分类）。
func zhipuWindowPeriod(item map[string]any) (string, bool) {
	typ := strings.ToUpper(zhipuJSONString(item, "type"))
	switch typ {
	case "TOKENS_LIMIT", "CREDIT_LIMIT":
		unit := zhipuJSONInt64(item, "unit")
		number := zhipuJSONInt64(item, "number")
		switch {
		case unit == 3 && number == 5:
			return "session", true
		case unit == 6 && number == 1:
			return "weekly", true
		}
	case "TIME_LIMIT":
		return "monthly", true
	}
	return "", false
}

// zhipuWindowFromFields 从单个 limits 行构造用量数据。percentage（0-100）直接
// 给出时优先使用；缺失时用 currentValue/usage*100 兜底（usage 为分母，无法计算
// 时跳过该窗口，不产出 0% 假窗口）。
func zhipuWindowFromFields(period string, m map[string]any) (ZhipuCodingPlanWindow, bool) {
	var usedPercent float64
	if percentage, ok := zhipuJSONFloatOK(m, "percentage"); ok {
		usedPercent = clampZhipuPercent(percentage)
	} else {
		usage, usageOK := zhipuJSONFloatOK(m, "usage")
		if !usageOK || usage <= 0 {
			return ZhipuCodingPlanWindow{}, false
		}
		current, _ := zhipuJSONFloatOK(m, "currentValue", "current_value", "used")
		if current < 0 {
			current = 0
		}
		usedPercent = clampZhipuPercent(current / usage * 100)
	}
	window := ZhipuCodingPlanWindow{
		Period:           period,
		UsedPercent:      usedPercent,
		RemainingPercent: clampZhipuPercent(100 - usedPercent),
	}
	if resetAt, resetInSec, ok := zhipuResetTime(m); ok {
		window.ResetAt = resetAt
		window.ResetInSec = resetInSec
	}
	return window, true
}

// zhipuResetTime 解析窗口 nextResetTime（毫秒时间戳，可能是字符串或数字）。
// 解析失败返回 false，ResetAt/ResetInSec 保持零值。ResetInSec 为距 reset 的
// 秒数（向上取整），已过期置 0。
func zhipuResetTime(m map[string]any) (resetAt string, resetInSec int64, ok bool) {
	for _, key := range []string{"nextResetTime", "resetTime", "resetAt", "reset_at", "reset_time"} {
		value, present := m[key]
		if !present {
			continue
		}
		var ms int64
		switch n := value.(type) {
		case float64:
			ms = int64(n)
		case string:
			if i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64); err == nil {
				ms = i
			}
		}
		if ms <= 0 {
			continue
		}
		t := time.UnixMilli(ms)
		resetAt = t.UTC().Format(time.RFC3339)
		resetInSec = int64(math.Ceil(time.Until(t).Seconds()))
		if resetInSec < 0 {
			resetInSec = 0
		}
		return resetAt, resetInSec, true
	}
	return "", 0, false
}

// clampZhipuPercent 将百分比夹到 0-100 并保留两位小数。
func clampZhipuPercent(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return math.Round(v*100) / 100
}

func zhipuJSONMap(m map[string]any, keys ...string) (map[string]any, bool) {
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

func zhipuJSONMapSlice(m map[string]any, keys ...string) []map[string]any {
	for _, key := range keys {
		raw, ok := m[key]
		if !ok {
			continue
		}
		items, ok := raw.([]any)
		if !ok {
			continue
		}
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if nested, ok := item.(map[string]any); ok {
				result = append(result, nested)
			}
		}
		return result
	}
	return nil
}

func zhipuJSONString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if s, ok := value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func zhipuJSONFloatOK(m map[string]any, keys ...string) (float64, bool) {
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

func zhipuJSONInt64(m map[string]any, keys ...string) int64 {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		switch n := value.(type) {
		case float64:
			return int64(n)
		case string:
			if i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64); err == nil {
				return i
			}
		}
	}
	return 0
}
