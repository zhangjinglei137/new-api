package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	openCodeGoBalanceURLPattern     = "https://opencode.ai/workspace/%s/go"
	openCodeGoDefaultRewardCents    = 500
	openCodeGoMonthlyCapUSD         = 60
	openCodeGoBalanceRequestTimeout = 30 * time.Second
)

var (
	// 兼容新旧工作区页面：新页面 usagePercent 为浮点（如 65.8）且 status 为 "ok"，
	// 旧页面为整数（如 40）且 status 为 "active"。
	openCodeGoMonthlyUsageRe = regexp.MustCompile(`monthlyUsage:\$R\[\d+\]=\{status:"[^"]*",resetInSec:\d+,usagePercent:(-?[\d.]+)`)
	openCodeGoRewardAmountRe = regexp.MustCompile(`rewardAmount[=:]\s*(\d+)`)
	// 新页面 rewards 段以 ]}) 结尾（rewards 位于 referral 对象内），旧页面以 ]) 结尾。
	openCodeGoRewardsRe     = regexp.MustCompile(`(?s)rewards:\$R\[\d+\]=\[(.*?)\](?:\)|\}\))`)
	openCodeGoRewardEntryRe = regexp.MustCompile(`\{id:"([^"]+)",source:"([^"]+)",status:"([^"]+)",email:"([^"]+)",amount:(\d+)`)
)

// parseOpenCodeGoBalancePage 从 opencode 工作区页面 HTML 中解析剩余额度（美元）。
// 余额 = 60 × (1 − usagePercent/100) + 未领取奖励数 × rewardAmount/100。
func parseOpenCodeGoBalancePage(html string) (float64, error) {
	monthlyMatch := openCodeGoMonthlyUsageRe.FindStringSubmatch(html)
	if monthlyMatch == nil {
		return 0, fmt.Errorf("无法从 opencode 页面解析用量数据，请检查 Workspace ID 与 Cookie 是否有效")
	}
	usagePercent, err := strconv.ParseFloat(monthlyMatch[1], 64)
	if err != nil {
		return 0, fmt.Errorf("无法从 opencode 页面解析用量数据: %w", err)
	}
	if usagePercent < 0 {
		usagePercent = 0
	}
	if usagePercent > 100 {
		usagePercent = 100
	}

	rewardAmount := openCodeGoDefaultRewardCents
	if match := openCodeGoRewardAmountRe.FindStringSubmatch(html); match != nil {
		if amount, err := strconv.Atoi(match[1]); err == nil && amount > 0 {
			rewardAmount = amount
		}
	}

	unused := 0
	if rewardsMatch := openCodeGoRewardsRe.FindStringSubmatch(html); rewardsMatch != nil {
		entries := openCodeGoRewardEntryRe.FindAllStringSubmatch(rewardsMatch[1], -1)
		applied := 0
		for _, entry := range entries {
			if entry[3] == "applied" {
				applied++
			}
		}
		unused = len(entries) - applied
	}

	return float64(openCodeGoMonthlyCapUSD)*(1-usagePercent/100) + float64(unused)*float64(rewardAmount)/100, nil
}

// fetchOpenCodeGoPage 抓取 opencode 工作区页面 HTML，供余额/用量解析共用：
// workspace id + 解密后的 cookie（历史明文自动透传）+ 代理 + 30s 超时 + User-Agent。
func fetchOpenCodeGoPage(channel *model.Channel) (string, error) {
	settings := channel.GetOtherSettings()
	workspaceID := strings.TrimSpace(settings.OpenCodeWorkspaceId)
	cookie := strings.TrimSpace(settings.OpenCodeAuthCookie)
	if workspaceID == "" {
		return "", fmt.Errorf("opencode workspace id 未配置")
	}
	if cookie == "" {
		return "", fmt.Errorf("opencode auth cookie 未配置")
	}
	plainCookie, err := common.DecryptSecret(cookie)
	if err != nil {
		return "", err
	}
	cookie = plainCookie
	if !strings.Contains(cookie, "auth=") {
		cookie = "auth=" + cookie
	}

	requestURL := fmt.Sprintf(openCodeGoBalanceURLPattern, url.PathEscape(workspaceID))
	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), openCodeGoBalanceRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Cookie", cookie)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status code: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// UpdateOpenCodeGoBalance 通过 opencode 工作区页面查询剩余额度（美元）。
func UpdateOpenCodeGoBalance(channel *model.Channel) (float64, error) {
	body, err := fetchOpenCodeGoPage(channel)
	if err != nil {
		return 0, err
	}
	return parseOpenCodeGoBalancePage(body)
}
