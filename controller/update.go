package controller

import (
	"context"
	"encoding/json"
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
)

type updateCheckRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// CheckUpdate 检查最新 release：优先直连 GitHub（可配置代理），失败后回退国内镜像。
func CheckUpdate(c *gin.Context) {
	proxy := common.Interface2String(common.OptionMap[UpdateCheckProxyKey])
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	var lastErr error
	for _, url := range []string{updateCheckRepoURL, updateCheckMirrorURL} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "new-api-dashboard")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status code: %d", resp.StatusCode)
			continue
		}
		var release updateCheckRelease
		if err := json.Unmarshal(body, &release); err != nil {
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