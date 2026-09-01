package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// volcCodingPlanUsageURLFmt 火山方舟 Coding Plan 用量查询接口。
// region 取值如 cn-beijing / ap-southeast。
const volcCodingPlanUsageURLFmt = "https://console.volcengine.com/api/top/ark/%s/2024-01-01/GetCodingPlanUsage"

// RegionFromVolcBaseURL 将火山方舟渠道 base_url 映射为控制台 region。
// 未知 base_url 返回 false，不做猜测。
func RegionFromVolcBaseURL(baseURL string) (string, bool) {
	bu := strings.ToLower(strings.TrimSpace(baseURL))
	switch {
	case strings.Contains(bu, "ark.cn-beijing.volces.com"):
		return "cn-beijing", true
	case strings.Contains(bu, "ark.ap-southeast.bytepluses.com"):
		return "ap-southeast", true
	default:
		return "", false
	}
}

// VolcCodingPlanUsageInfo 是火山方舟 Coding Plan 用量解析结果。
type VolcCodingPlanUsageInfo struct {
	Status           string
	Period           string
	UsedPercent      float64
	RemainingPercent float64
	ResetAt          string
	ResetInSec       int64
}

type volcCodingPlanWindow struct {
	level          string
	percent        float64
	cap            float64
	resetTimestamp int64
}

// ParseVolcCodingPlanUsage 解析 GetCodingPlanUsage 响应。优先取 Level==monthly
// 窗口，其次 weekly、session；remaining = clamp((Cap-Percent)/Cap*100,0,100)，
// Cap 缺省 100。ResetTimestamp 秒级(<1e12)或毫秒(>=1e12)自动判断。
// 解析不出任何窗口时返回错误。
func ParseVolcCodingPlanUsage(body []byte) (*VolcCodingPlanUsageInfo, error) {
	var raw map[string]any
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid json body: %w", err)
	}
	result, ok := raw["Result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing Result in response")
	}

	windows := volcJSONMapSlice(result, "QuotaUsage", "Usages", "Details")
	if len(windows) == 0 {
		return nil, fmt.Errorf("no usage windows found")
	}
	best := pickVolcCodingPlanWindow(windows)
	if best == nil {
		return nil, fmt.Errorf("no usable usage window found")
	}

	remaining := (best.cap - best.percent) / best.cap * 100
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}

	info := &VolcCodingPlanUsageInfo{
		Status:           volcJSONString(result, "Status"),
		Period:           best.level,
		UsedPercent:      best.percent,
		RemainingPercent: remaining,
	}
	if best.resetTimestamp > 0 {
		info.ResetAt = time.Unix(best.resetTimestamp, 0).UTC().Format(time.RFC3339)
		resetIn := best.resetTimestamp - time.Now().Unix()
		if resetIn < 0 {
			resetIn = 0
		}
		info.ResetInSec = resetIn
	}
	return info, nil
}

func pickVolcCodingPlanWindow(windows []map[string]any) *volcCodingPlanWindow {
	priority := map[string]int{"monthly": 3, "weekly": 2, "session": 1}
	var best *volcCodingPlanWindow
	for _, window := range windows {
		candidate := &volcCodingPlanWindow{
			level:          volcJSONString(window, "Level", "Type", "Period", "Label", "Window"),
			percent:        volcJSONFloat(window, "Percent", "UsedPercent", "UsagePercent"),
			cap:            volcJSONFloat(window, "Cap"),
			resetTimestamp: volcJSONInt64(window, "ResetTimestamp", "ResetTime"),
		}
		if candidate.cap == 0 {
			candidate.cap = 100
		}
		if candidate.resetTimestamp >= 1e12 {
			candidate.resetTimestamp /= 1000
		}
		if best == nil || priority[strings.ToLower(candidate.level)] > priority[strings.ToLower(best.level)] {
			best = candidate
		}
	}
	return best
}

func volcJSONMapSlice(m map[string]any, keys ...string) []map[string]any {
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
			if window, ok := item.(map[string]any); ok {
				result = append(result, window)
			}
		}
		return result
	}
	return nil
}

func volcJSONString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if s, ok := value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func volcJSONFloat(m map[string]any, keys ...string) float64 {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		switch n := value.(type) {
		case float64:
			return n
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
				return f
			}
		}
	}
	return 0
}

func volcJSONInt64(m map[string]any, keys ...string) int64 {
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

// FetchVolcCodingPlanUsage 向火山方舟控制台发起 Coding Plan 用量查询。
// 入参非空校验；响应 body 原样返回。
func FetchVolcCodingPlanUsage(ctx context.Context, client *http.Client, region, csrfToken, cookie string) (statusCode int, body []byte, err error) {
	if client == nil {
		return 0, nil, fmt.Errorf("nil http client")
	}
	region = strings.TrimSpace(region)
	csrfToken = strings.TrimSpace(csrfToken)
	cookie = strings.TrimSpace(cookie)
	if region == "" {
		return 0, nil, fmt.Errorf("empty region")
	}
	if csrfToken == "" {
		return 0, nil, fmt.Errorf("empty x-csrf-token")
	}
	if cookie == "" {
		return 0, nil, fmt.Errorf("empty cookie")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(volcCodingPlanUsageURLFmt, region), nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-csrf-token", csrfToken)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Origin", "https://console.volcengine.com")
	req.Header.Set("Referer", fmt.Sprintf("https://console.volcengine.com/ark/region:%s/subscription/coding-plan", region))

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}
