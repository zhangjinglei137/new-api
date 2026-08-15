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

	"github.com/QuantumNous/new-api/model"
)

const (
	openCodeGoBalanceURLPattern     = "https://opencode.ai/workspace/%s/go"
	openCodeGoDefaultRewardCents    = 500
	openCodeGoMonthlyCapUSD         = 60
	openCodeGoBalanceRequestTimeout = 30 * time.Second
)

var (
	openCodeGoMonthlyUsageRe = regexp.MustCompile(`monthlyUsage:\$R\[\d+\]=\{status:"[^"]*",resetInSec:(\d+),usagePercent:(\d+)\}`)
	openCodeGoRewardAmountRe = regexp.MustCompile(`rewardAmount[=:]\s*(\d+)`)
	openCodeGoRewardsRe      = regexp.MustCompile(`(?s)rewards:\$R\[\d+\]=\[(.*?)\]\)`)
	openCodeGoRewardEntryRe  = regexp.MustCompile(`\{id:"([^"]+)",source:"([^"]+)",status:"([^"]+)",email:"([^"]+)",amount:(\d+)`)
)

// parseOpenCodeGoBalancePage 从 opencode 工作区页面 HTML 中解析剩余额度（美元）。
// 余额 = 60 × (1 − usagePercent/100) + 未领取奖励数 × rewardAmount/100。
func parseOpenCodeGoBalancePage(html string) (float64, error) {
	monthlyMatch := openCodeGoMonthlyUsageRe.FindStringSubmatch(html)
	if monthlyMatch == nil {
		return 0, fmt.Errorf("无法从 opencode 页面解析用量数据，请检查 Workspace ID 与 Cookie 是否有效")
	}
	usagePercent, err := strconv.Atoi(monthlyMatch[2])
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

	return float64(openCodeGoMonthlyCapUSD)*(1-float64(usagePercent)/100) + float64(unused)*float64(rewardAmount)/100, nil
}

// UpdateOpenCodeGoBalance 通过 opencode 工作区页面查询剩余额度（美元）。
func UpdateOpenCodeGoBalance(channel *model.Channel) (float64, error) {
	settings := channel.GetOtherSettings()
	workspaceID := strings.TrimSpace(settings.OpenCodeWorkspaceId)
	cookie := strings.TrimSpace(settings.OpenCodeAuthCookie)
	if workspaceID == "" {
		return 0, fmt.Errorf("opencode workspace id 未配置")
	}
	if cookie == "" {
		return 0, fmt.Errorf("opencode auth cookie 未配置")
	}
	if !strings.Contains(cookie, "auth=") {
		cookie = "auth=" + cookie
	}

	requestURL := fmt.Sprintf(openCodeGoBalanceURLPattern, url.PathEscape(workspaceID))
	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), openCodeGoBalanceRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Cookie", cookie)

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status code: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	return parseOpenCodeGoBalancePage(string(body))
}
