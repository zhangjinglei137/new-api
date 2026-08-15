package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const googleTranslateURL = "https://translate.googleapis.com/translate_a/single"

// TranslateText 将英文文本翻译为 targetLang 指定的语言；任何失败都返回原文，
// 同步流程不因翻译失败中断。targetLang 为 zh/zh-CN/zh-TW 时翻译为简体中文，
// 为 ja 时翻译为日语，其余语言（含 en、空）直接返回原文。
func TranslateText(text, targetLang string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	tl := ""
	switch strings.ToLower(strings.TrimSpace(targetLang)) {
	case "zh", "zh-cn", "zh-tw":
		tl = "zh-CN"
	case "ja":
		tl = "ja"
	default:
		return text
	}

	requestURL := fmt.Sprintf("%s?client=gtx&sl=en&tl=%s&dt=t&q=%s",
		googleTranslateURL, tl, url.QueryEscape(text))
	client, err := GetHttpClientWithProxy("")
	if err != nil {
		return text
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return text
	}
	resp, err := client.Do(req)
	if err != nil {
		return text
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return text
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return text
	}
	translated, err := parseGoogleTranslateResponse(body)
	if err != nil {
		return text
	}
	return translated
}

// parseGoogleTranslateResponse 解析 Google Translate 响应（嵌套数组
// [[["译文","原文",...],...],...]），拼接第一段译文。
func parseGoogleTranslateResponse(body []byte) (string, error) {
	var segments [][]any
	if err := common.Unmarshal(body, &segments); err != nil {
		return "", fmt.Errorf("invalid google translate response: %w", err)
	}
	var builder strings.Builder
	for _, segment := range segments {
		if len(segment) == 0 {
			continue
		}
		text, ok := segment[0].(string)
		if !ok {
			continue
		}
		builder.WriteString(text)
	}
	if builder.Len() == 0 {
		return "", fmt.Errorf("google translate response contains no translated segments")
	}
	return builder.String(), nil
}
