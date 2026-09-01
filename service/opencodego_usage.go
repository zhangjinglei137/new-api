package service

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/QuantumNous/new-api/model"
)

// openCodeGoMonthlyResetRe 捕获月度用量重置倒计时（秒）。锚定在 monthlyUsage 段内，
// 避免误匹配页面中滚动/周用量的 resetInSec。
var openCodeGoMonthlyResetRe = regexp.MustCompile(`monthlyUsage:\$R\[\d+\]=\{status:"[^"]*",resetInSec:(\d+)`)

// OpenCodeGoUsageInfo 是 opencode 月度用量信息。
type OpenCodeGoUsageInfo struct {
	UsagePercent     float64 // 已用% (0-100)
	RemainingPercent float64 // 100-UsagePercent, clamp 0-100
	Balance          float64 // 美元（复用余额计算）
	ResetInSec       int64   // 重置倒计时秒
	MonthlyCapUSD    float64
}

// FetchOpenCodeGoUsage 抓取 opencode 工作区页面并解析月度用量信息。
func FetchOpenCodeGoUsage(channel *model.Channel) (*OpenCodeGoUsageInfo, error) {
	body, err := fetchOpenCodeGoPage(channel)
	if err != nil {
		return nil, err
	}
	return parseOpenCodeGoUsagePage(body)
}

// parseOpenCodeGoUsagePage 从 opencode 工作区页面 HTML 中解析月度用量信息。
func parseOpenCodeGoUsagePage(html string) (*OpenCodeGoUsageInfo, error) {
	monthlyMatch := openCodeGoMonthlyUsageRe.FindStringSubmatch(html)
	if monthlyMatch == nil {
		return nil, fmt.Errorf("无法从 opencode 页面解析用量数据，请检查 Workspace ID 与 Cookie 是否有效")
	}
	usagePercent, err := strconv.ParseFloat(monthlyMatch[1], 64)
	if err != nil {
		return nil, fmt.Errorf("无法从 opencode 页面解析用量数据: %w", err)
	}
	if usagePercent < 0 {
		usagePercent = 0
	}
	if usagePercent > 100 {
		usagePercent = 100
	}

	remaining := 100 - usagePercent
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}

	var resetInSec int64
	if resetMatch := openCodeGoMonthlyResetRe.FindStringSubmatch(html); resetMatch != nil {
		if parsed, err := strconv.ParseInt(resetMatch[1], 10, 64); err == nil {
			resetInSec = parsed
		}
	}

	balance, err := parseOpenCodeGoBalancePage(html)
	if err != nil {
		return nil, err
	}

	return &OpenCodeGoUsageInfo{
		UsagePercent:     usagePercent,
		RemainingPercent: remaining,
		Balance:          balance,
		ResetInSec:       resetInSec,
		MonthlyCapUSD:    openCodeGoMonthlyCapUSD,
	}, nil
}
