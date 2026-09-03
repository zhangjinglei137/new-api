package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
)

// doCodexWhamRequest 执行一次 WHAM 后端 API 请求：统一做入参校验、
// 路径拼接与公共请求头（Authorization / chatgpt-account-id / Accept /
// originator）。body 非空时设置 JSON Content-Type。返回状态码与响应体。
func doCodexWhamRequest(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	method string,
	path string,
	accessToken string,
	accountID string,
	body []byte,
) (statusCode int, respBody []byte, err error) {
	if client == nil {
		return 0, nil, fmt.Errorf("nil http client")
	}
	bu := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if bu == "" {
		return 0, nil, fmt.Errorf("empty baseURL")
	}
	at := strings.TrimSpace(accessToken)
	aid := strings.TrimSpace(accountID)
	if at == "" {
		return 0, nil, fmt.Errorf("empty accessToken")
	}
	if aid == "" {
		return 0, nil, fmt.Errorf("empty accountID")
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, bu+path, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	setCodexWhamRequestHeaders(req, at, aid)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func FetchCodexWhamUsage(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	return doCodexWhamRequest(ctx, client, baseURL, http.MethodGet, "/backend-api/wham/usage", accessToken, accountID, nil)
}

func FetchCodexWhamRateLimitResetCredits(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	return doCodexWhamRequest(ctx, client, baseURL, http.MethodGet, "/backend-api/wham/rate-limit-reset-credits", accessToken, accountID, nil)
}

func ConsumeCodexWhamRateLimitResetCredit(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	requestBody, err := common.Marshal(map[string]string{
		"redeem_request_id": uuid.NewString(),
	})
	if err != nil {
		return 0, nil, err
	}
	return doCodexWhamRequest(ctx, client, baseURL, http.MethodPost, "/backend-api/wham/rate-limit-reset-credits/consume", accessToken, accountID, requestBody)
}

func setCodexWhamRequestHeaders(req *http.Request, accessToken string, accountID string) {
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("chatgpt-account-id", accountID)
	req.Header.Set("Accept", "application/json")
	if req.Header.Get("originator") == "" {
		req.Header.Set("originator", "codex_cli_rs")
	}
}
