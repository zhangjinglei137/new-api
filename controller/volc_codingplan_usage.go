package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetVolcCodingPlanUsage 查询火山方舟 Coding Plan 渠道的用量。任何响应都不得包含凭证。
func GetVolcCodingPlanUsage(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}

	ch, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if ch == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
		return
	}
	if ch.Type != constant.ChannelTypeVolcEngine {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not VolcEngine"})
		return
	}
	if ch.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "multi-key channel is not supported"})
		return
	}

	settings := ch.GetOtherSettings()
	if settings.EndpointProfile != "coding" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "该渠道未启用 Coding Plan 端点"})
		return
	}
	if settings.VolcCodingPlanCsrfToken == "" || settings.VolcCodingPlanCookie == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "浏览器会话凭证未配置", "error_code": "credentials_not_configured"})
		return
	}

	region, ok := service.RegionFromVolcBaseURL(ch.GetBaseURL())
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "不支持的区域", "error_code": "unsupported_region"})
		return
	}

	csrfToken, err := common.DecryptSecret(settings.VolcCodingPlanCsrfToken)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "浏览器会话凭证不可用，请重新配置", "error_code": "credentials_unavailable"})
		return
	}
	cookie, err := common.DecryptSecret(settings.VolcCodingPlanCookie)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "浏览器会话凭证不可用，请重新配置", "error_code": "credentials_unavailable"})
		return
	}

	client, err := service.GetHttpClientWithProxy(ch.GetSetting().Proxy)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	statusCode, body, err := service.FetchVolcCodingPlanUsage(ctx, client, region, csrfToken, cookie)
	if err != nil {
		common.SysError("failed to fetch volc coding plan usage: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取用量失败，请稍后重试", "error_code": "fetch_failed"})
		return
	}

	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		c.JSON(http.StatusOK, gin.H{
			"success":         false,
			"message":         "火山控制台会话已过期，请在渠道设置中更新 Cookie 与 x-csrf-token",
			"upstream_status": statusCode,
			"error_code":      "credentials_expired",
		})
		return
	}

	info, err := service.ParseVolcCodingPlanUsage(body)
	if err != nil {
		common.SysError("failed to parse volc coding plan usage: " + err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success":         false,
			"message":         "无法解析上游用量数据",
			"upstream_status": statusCode,
			"error_code":      "usage_schema_unknown",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "",
		"upstream_status": 200,
		"error_code":      "",
		"data": gin.H{
			"status":             info.Status,
			"period":             info.Period,
			"remaining_percent":  info.RemainingPercent,
			"used_percent":       info.UsedPercent,
			"reset_at":           info.ResetAt,
			"reset_in_sec":       info.ResetInSec,
		},
	})
}

type updateVolcCodingPlanCredentialsRequest struct {
	CsrfToken   *string `json:"csrf_token"`
	Cookie      *string `json:"cookie"`
	ClearCsrf   bool    `json:"clear_csrf"`
	ClearCookie bool    `json:"clear_cookie"`
}

// UpdateVolcCodingPlanCredentials 通过独立 PATCH 接口更新火山方舟 Coding Plan
// 控制台会话凭证（AES-GCM 加密后入库）。审计日志只记录 channel_id / channel_name / changed。
func UpdateVolcCodingPlanCredentials(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}

	ch, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if ch == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
		return
	}
	if ch.Type != constant.ChannelTypeVolcEngine {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not VolcEngine"})
		return
	}
	if ch.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "multi-key channel is not supported"})
		return
	}

	settings := ch.GetOtherSettings()
	if settings.EndpointProfile != "coding" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "该渠道未启用 Coding Plan 端点"})
		return
	}

	rawBody, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req updateVolcCodingPlanCredentialsRequest
	if err := common.Unmarshal(rawBody, &req); err != nil {
		common.ApiError(c, err)
		return
	}

	if (req.ClearCsrf && req.CsrfToken != nil) || (req.ClearCookie && req.Cookie != nil) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "clear_* 与对应新值不能同时出现"})
		return
	}
	if req.CsrfToken != nil && (strings.Contains(*req.CsrfToken, "\r") || strings.Contains(*req.CsrfToken, "\n")) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "csrf_token 包含非法字符"})
		return
	}
	if req.Cookie != nil && (strings.Contains(*req.Cookie, "\r") || strings.Contains(*req.Cookie, "\n")) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "cookie 包含非法字符"})
		return
	}
	if req.CsrfToken != nil && len(*req.CsrfToken) > 1024 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "csrf_token 长度超限"})
		return
	}
	if req.Cookie != nil && len(*req.Cookie) > 32768 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "cookie 长度超限"})
		return
	}

	changed := false
	switch {
	case req.ClearCsrf:
		if settings.VolcCodingPlanCsrfToken != "" {
			settings.VolcCodingPlanCsrfToken = ""
			changed = true
		}
	case req.CsrfToken != nil:
		encrypted, err := common.EncryptSecret(*req.CsrfToken)
		if err != nil {
			common.SysError("failed to encrypt volc coding plan csrf token: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "凭证加密失败，请稍后重试"})
			return
		}
		if settings.VolcCodingPlanCsrfToken != encrypted {
			settings.VolcCodingPlanCsrfToken = encrypted
			changed = true
		}
	}
	switch {
	case req.ClearCookie:
		if settings.VolcCodingPlanCookie != "" {
			settings.VolcCodingPlanCookie = ""
			changed = true
		}
	case req.Cookie != nil:
		encrypted, err := common.EncryptSecret(*req.Cookie)
		if err != nil {
			common.SysError("failed to encrypt volc coding plan cookie: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "凭证加密失败，请稍后重试"})
			return
		}
		if settings.VolcCodingPlanCookie != encrypted {
			settings.VolcCodingPlanCookie = encrypted
			changed = true
		}
	}

	if changed {
		ch.SetOtherSettings(settings)
		err = model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("settings", ch.OtherSettings).Error
		if err != nil {
			common.SysError("failed to update volc coding plan credentials: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "保存失败，请稍后重试"})
			return
		}
		model.InitChannelCache()
	}

	recordManageAudit(c, "channel.volc_coding_plan_credentials", map[string]interface{}{
		"channel_id":   ch.Id,
		"channel_name": ch.Name,
		"changed":      changed,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"csrf_token_configured": settings.VolcCodingPlanCsrfToken != "",
			"cookie_configured":     settings.VolcCodingPlanCookie != "",
		},
	})
}
