package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	updateCheckRepoURL   = "https://api.github.com/repos/zhangjinglei137/new-api/releases/latest"
	updateCheckMirrorURL = "https://gh-proxy.com/https://api.github.com/repos/zhangjinglei137/new-api/releases/latest"
	// UpdateCheckProxyKey 是检查更新使用的出站代理配置项（options 表）。
	UpdateCheckProxyKey = "UpdateCheckProxy"

	// updateCheckRequestTimeout 是单个候选站点的请求超时。直连 GitHub 在部分
	// 网络环境会 TLS 挂起或匿名限流(403)，因此每个 URL 用独立超时，避免一个
	// 站点拖垮整个检查；两者串行最坏耗时不超过前端 fetch 的超时窗口。
	updateCheckRequestTimeout = 5 * time.Second
)

type updateCheckRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Prerelease  bool   `json:"prerelease"`
}

// CheckUpdate 检查最新 release：优先直连 GitHub（可配置代理），失败后回退国内镜像。
func CheckUpdate(c *gin.Context) {
	proxy := common.Interface2String(common.OptionMap[UpdateCheckProxyKey])
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var lastErr error
	for _, url := range []string{updateCheckRepoURL, updateCheckMirrorURL} {
		// 每个候选站独立超时：直连 GitHub 在部分网络环境会 TLS 挂起或 403 限流，
		// 若共享一个总 ctx，直连会耗尽预算导致镜像请求直接 context deadline exceeded。
		reqCtx, reqCancel := context.WithTimeout(c.Request.Context(), updateCheckRequestTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
		if err != nil {
			reqCancel()
			lastErr = err
			continue
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "new-api-dashboard")
		resp, err := client.Do(req)
		if err != nil {
			reqCancel()
			lastErr = err
			common.SysLog(fmt.Sprintf("update check failed for %s: %v", url, err))
			continue
		}
		// 先读完并关闭 body，再取消请求 ctx：ctx 一旦被 cancel，transport 会
		// 关闭响应 body，提前 cancel 会导致 io.ReadAll 返回 context canceled。
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		reqCancel()
		if readErr != nil {
			lastErr = readErr
			common.SysLog(fmt.Sprintf("update check read body failed for %s: %v", url, readErr))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status code: %d", resp.StatusCode)
			common.SysLog(fmt.Sprintf("update check got %s: %d", url, resp.StatusCode))
			continue
		}
		var release updateCheckRelease
		if err := common.Unmarshal(body, &release); err != nil {
			lastErr = err
			continue
		}
		if release.TagName == "" {
			lastErr = fmt.Errorf("unexpected release payload")
			continue
		}
		common.ApiSuccess(c, release)
		return
	}
	common.ApiError(c, lastErr)
}
