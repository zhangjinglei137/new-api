package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// volcCodingPlanOpenAPIURLFmt 火山方舟 OpenAPI 通用端点（AK/SK V4 签名认证路径）。
// Action/Region 通过 %s 填入，Version 固定为 2024-01-01。
const volcCodingPlanOpenAPIURLFmt = "https://open.volcengineapi.com/?Action=%s&Region=%s&Version=2024-01-01"

// 火山方舟 OpenAPI V4 签名固定参数（与 AWS SigV4 有致命差异：SignedHeaders
// 不按字母序、SK 不加 "AWS4" 前缀，参考 cc-switch 生产实现）。
const (
	volcOpenAPIHost         = "open.volcengineapi.com"
	volcOpenAPIContentType  = "application/json; charset=utf-8"
	volcOpenAPISignedHeader = "host;x-date;x-content-sha256;content-type"
)

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

// VolcCodingPlanWindow 是火山方舟 Coding Plan 单个时间窗口的用量数据。
type VolcCodingPlanWindow struct {
	Period           string  // "session" | "weekly" | "monthly"
	UsedPercent      float64 // 已用%（0-100，两位小数）
	RemainingPercent float64 // 100-UsedPercent（clamp 0-100，两位小数）
	ResetAt          string  // RFC3339 UTC
	ResetInSec       int64
}

// VolcCodingPlanUsageInfo 是火山方舟 Coding Plan 用量解析结果。
type VolcCodingPlanUsageInfo struct {
	Status  string
	Windows []VolcCodingPlanWindow
}

// ParseVolcCodingPlanUsage 解析 GetCodingPlanUsage 响应，返回全部时间窗口
// （session / weekly / monthly），顺序固定为 session -> weekly -> monthly，
// 缺失的窗口不出现。Level（含别名回退）无法归一化为上述三者之一的条目被跳过。
// 百分比统一四舍五入到两位小数；ResetTimestamp 秒级(<1e12)或毫秒(>=1e12)
// 自动判断。解析不出任何窗口时返回错误。
func ParseVolcCodingPlanUsage(body []byte) (*VolcCodingPlanUsageInfo, error) {
	var raw map[string]any
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid json body: %w", err)
	}
	result, ok := raw["Result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing Result in response")
	}

	entries := jsonMapSlice(result, "QuotaUsage", "Usages", "Details")
	if len(entries) == 0 {
		return nil, fmt.Errorf("no usage windows found")
	}

	info := &VolcCodingPlanUsageInfo{Status: jsonString(result, "Status")}
	byPeriod := make(map[string]VolcCodingPlanWindow, len(entries))
	for _, entry := range entries {
		period := volcCodingPlanPeriod(entry)
		if period == "" {
			continue
		}
		used := volcRoundPercent(jsonFloat(entry, "Percent", "UsedPercent", "UsagePercent"))
		remaining := 100 - used
		if remaining < 0 {
			remaining = 0
		}
		if remaining > 100 {
			remaining = 100
		}
		remaining = volcRoundPercent(remaining)

		window := VolcCodingPlanWindow{
			Period:           period,
			UsedPercent:      used,
			RemainingPercent: remaining,
		}
		if reset := jsonInt64Clamped(entry, "ResetTimestamp", "ResetTime"); reset > 0 {
			if reset >= 1e12 {
				reset /= 1000
			}
			window.ResetAt = time.Unix(reset, 0).UTC().Format(time.RFC3339)
			resetIn := reset - time.Now().Unix()
			if resetIn < 0 {
				resetIn = 0
			}
			window.ResetInSec = resetIn
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

// volcCodingPlanPeriod 将窗口条目中的 Level（含别名回退）归一化为
// session/weekly/monthly；未识别返回空串。
func volcCodingPlanPeriod(entry map[string]any) string {
	level := strings.ToLower(strings.TrimSpace(jsonString(entry, "Level", "Type", "Period", "Label", "Window")))
	switch level {
	case "session", "weekly", "monthly":
		return level
	default:
		return ""
	}
}

// volcRoundPercent 将百分比四舍五入到两位小数；NaN/Inf 兜底为 0。
func volcRoundPercent(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

// signVolcOpenAPIV4 为火山方舟 OpenAPI（open.volcengineapi.com）请求生成 V4
// 签名三件套 Authorization / X-Date / X-Content-Sha256。与 AWS SigV4 不同：
// canonical headers 顺序固定（host/x-date/x-content-sha256/content-type，不按
// 字母序）、SK 派生密钥不加 "AWS4" 前缀、算法串为 HMAC-SHA256。仅用于空 body
// 的 POST 请求（x-content-sha256 为固定空哈希）。now 参数化便于测试。
func signVolcOpenAPIV4(accessKeyID, secretAccessKey, region, action string, now time.Time) (authorization, xDate, xContentSha256 string) {
	xDate = now.UTC().Format("20060102T150405Z")
	shortDate := now.UTC().Format("20060102")
	emptyBodyHash := sha256.Sum256(nil)
	xContentSha256 = hex.EncodeToString(emptyBodyHash[:])

	canonicalQuery := "Action=" + action + "&Region=" + region + "&Version=2024-01-01"
	canonicalHeaders := "host:" + volcOpenAPIHost +
		"\nx-date:" + xDate +
		"\nx-content-sha256:" + xContentSha256 +
		"\ncontent-type:" + volcOpenAPIContentType + "\n"
	canonicalRequest := "POST\n/\n" + canonicalQuery + "\n" + canonicalHeaders + "\n" +
		volcOpenAPISignedHeader + "\n" + xContentSha256
	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	scope := shortDate + "/" + region + "/ark/request"
	stringToSign := "HMAC-SHA256\n" + xDate + "\n" + scope + "\n" + hex.EncodeToString(canonicalHash[:])

	sign := func(key []byte, value string) []byte {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(value))
		return mac.Sum(nil)
	}
	signingKey := sign(sign(sign([]byte(secretAccessKey), shortDate), region), "ark")
	signingKey = sign(signingKey, "request")
	signature := hex.EncodeToString(sign(signingKey, stringToSign))

	authorization = "HMAC-SHA256 Credential=" + accessKeyID + "/" + scope +
		", SignedHeaders=" + volcOpenAPISignedHeader + ", Signature=" + signature
	return authorization, xDate, xContentSha256
}

// FetchVolcCodingPlanUsageByAkSk 通过火山方舟 OpenAPI（AK/SK V4 签名认证）查询
// Coding Plan 用量。入参非空校验；POST 空 body，请求响应 body 原样返回。
func FetchVolcCodingPlanUsageByAkSk(ctx context.Context, client *http.Client, region, accessKeyID, secretAccessKey string) (statusCode int, body []byte, err error) {
	if client == nil {
		return 0, nil, fmt.Errorf("nil http client")
	}
	region = strings.TrimSpace(region)
	accessKeyID = strings.TrimSpace(accessKeyID)
	secretAccessKey = strings.TrimSpace(secretAccessKey)
	if region == "" {
		return 0, nil, fmt.Errorf("empty region")
	}
	if accessKeyID == "" {
		return 0, nil, fmt.Errorf("empty access key id")
	}
	if secretAccessKey == "" {
		return 0, nil, fmt.Errorf("empty secret access key")
	}

	action := "GetCodingPlanUsage"
	// 先签名后构造请求，避免签名时引入额外动态字段。
	authorization, xDate, xContentSha256 := signVolcOpenAPIV4(accessKeyID, secretAccessKey, region, action, time.Now())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(volcCodingPlanOpenAPIURLFmt, action, region), nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("X-Date", xDate)
	req.Header.Set("X-Content-Sha256", xContentSha256)
	req.Header.Set("Content-Type", volcOpenAPIContentType)

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
