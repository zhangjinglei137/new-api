package service

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/QuantumNous/new-api/model"
)

// 三个时间窗口的正则。opencode 工作区页面每个窗口独立成段
// （rollingUsage / weeklyUsage / monthlyUsage），字段顺序为
// resetInSec 在前、usagePercent 在后。命名加 Usage 前缀以区别于
// opencodego_balance.go 中不捕获 resetInSec 的 openCodeGoMonthlyUsageRe。
var (
	openCodeGoUsageRollingRe = regexp.MustCompile(`rollingUsage:\$R\[\d+\]=\{status:"[^"]*",resetInSec:(\d+),usagePercent:(-?[\d.]+)`)
	openCodeGoUsageWeeklyRe  = regexp.MustCompile(`weeklyUsage:\$R\[\d+\]=\{status:"[^"]*",resetInSec:(\d+),usagePercent:(-?[\d.]+)`)
	openCodeGoUsageMonthlyRe = regexp.MustCompile(`monthlyUsage:\$R\[\d+\]=\{status:"[^"]*",resetInSec:(\d+),usagePercent:(-?[\d.]+)`)
)

// OpenCodeGoWindow 是 opencode 单个时间窗口的用量数据。
type OpenCodeGoWindow struct {
	Period           string  // "session" | "weekly" | "monthly"
	UsedPercent      float64 // 已用%（0-100）
	RemainingPercent float64 // 100-UsedPercent, clamp 0-100
	ResetInSec       int64   // 重置倒计时秒
}

// OpenCodeGoUsageInfo 是 opencode 工作区用量解析结果。
type OpenCodeGoUsageInfo struct {
	Windows []OpenCodeGoWindow
}

// FetchOpenCodeGoUsage 抓取 opencode 工作区页面并解析用量信息。
func FetchOpenCodeGoUsage(channel *model.Channel) (*OpenCodeGoUsageInfo, error) {
	body, err := fetchOpenCodeGoPage(channel)
	if err != nil {
		return nil, err
	}
	return parseOpenCodeGoUsagePage(body)
}

// parseOpenCodeGoUsagePage 从 opencode 工作区页面 HTML 中解析用量信息。
// 依次匹配 rollingUsage（5 小时窗口）/ weeklyUsage / monthlyUsage 段，
// 存在的窗口按 session -> weekly -> monthly 顺序返回，缺失的窗口不出现；
// 旧页面只有 monthlyUsage 时仅返回该窗口。没有任何窗口时返回错误。
func parseOpenCodeGoUsagePage(html string) (*OpenCodeGoUsageInfo, error) {
	windows := make([]OpenCodeGoWindow, 0, 3)
	for _, entry := range []struct {
		period  string
		pattern *regexp.Regexp
	}{
		{period: "session", pattern: openCodeGoUsageRollingRe},
		{period: "weekly", pattern: openCodeGoUsageWeeklyRe},
		{period: "monthly", pattern: openCodeGoUsageMonthlyRe},
	} {
		if window, ok := parseOpenCodeGoWindow(html, entry.period, entry.pattern); ok {
			windows = append(windows, window)
		}
	}
	if len(windows) == 0 {
		return nil, fmt.Errorf("无法从 opencode 页面解析用量数据，请检查 Workspace ID 与 Cookie 是否有效")
	}
	return &OpenCodeGoUsageInfo{Windows: windows}, nil
}

// parseOpenCodeGoWindow 解析单个时间窗口段；未匹配或字段解析失败时返回 ok=false。
func parseOpenCodeGoWindow(html, period string, pattern *regexp.Regexp) (OpenCodeGoWindow, bool) {
	match := pattern.FindStringSubmatch(html)
	if match == nil {
		return OpenCodeGoWindow{}, false
	}
	usedPercent, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return OpenCodeGoWindow{}, false
	}
	usedPercent = max(0.0, min(100.0, usedPercent))
	remaining := max(0.0, min(100.0, 100-usedPercent))
	resetInSec, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return OpenCodeGoWindow{}, false
	}
	return OpenCodeGoWindow{
		Period:           period,
		UsedPercent:      usedPercent,
		RemainingPercent: remaining,
		ResetInSec:       resetInSec,
	}, true
}
