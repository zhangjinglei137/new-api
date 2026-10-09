package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const clinePlanUsageTimeout = 15 * time.Second

var (
	ErrClinePlanUnauthorized = errors.New("cline 凭证无效或已过期")
	ErrClinePlanUnavailable  = errors.New("cline 计划用量不可用")
	ErrClinePlanSchema       = errors.New("cline 计划用量响应结构无法解析")
)

type ClinePlanLimit struct {
	Type        string  `json:"type"`
	PercentUsed float64 `json:"percentUsed"`
	ResetsAt    string  `json:"resetsAt"`
}

type ClinePlanUsageInfo struct {
	Limits []ClinePlanLimit `json:"limits"`
}

// FetchClinePlanUsage 查询 ClinePass 的 Coding Plan 用量。
func FetchClinePlanUsage(channel *model.Channel) (*ClinePlanUsageInfo, error) {
	if channel == nil {
		return nil, fmt.Errorf("nil channel")
	}
	if channel.GetOtherSettings().EndpointProfile != "clinepass" {
		return nil, fmt.Errorf("cline 计划用量仅适用于 ClinePass 渠道")
	}
	keys := channel.GetKeys()
	if len(keys) == 0 || strings.TrimSpace(keys[0]) == "" {
		return nil, fmt.Errorf("cline api key 未配置")
	}
	key := strings.TrimSpace(keys[0])
	baseURL := strings.TrimRight(channel.GetBaseURL(), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("cline base url 未配置")
	}
	usageURL := fmt.Sprintf("%s/v1/users/me/plan/usage-limits", baseURL)

	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), clinePlanUsageTimeout)
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
		return nil, ErrClinePlanUnauthorized
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("%w: status code %d", ErrClinePlanUnavailable, resp.StatusCode)
	}
	info, err := parseClinePlanUsage(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrClinePlanSchema, err)
	}
	return info, nil
}

func parseClinePlanUsage(body []byte) (*ClinePlanUsageInfo, error) {
	var raw struct {
		Data struct {
			Limits []ClinePlanLimit `json:"limits"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid json body: %w", err)
	}
	info := &ClinePlanUsageInfo{Limits: raw.Data.Limits}
	for i := range info.Limits {
		if info.Limits[i].PercentUsed < 0 {
			info.Limits[i].PercentUsed = 0
		}
		if info.Limits[i].PercentUsed > 100 {
			info.Limits[i].PercentUsed = 100
		}
	}
	return info, nil
}
