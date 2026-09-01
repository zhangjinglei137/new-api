package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetVolcCodingPlanUsage 查询火山方舟 Coding Plan 渠道的用量（AK/SK OpenAPI 签名认证）。
// 任何响应都不得包含凭证。
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

	// 仅支持 AK/SK：OpenAPI V4 签名认证。AccessKeyId 与 SecretAccessKey 任一缺失
	// 时视为未配置凭证。
	if settings.VolcCodingPlanAccessKeyId == "" || settings.VolcCodingPlanSecretAccessKey == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "OpenAPI Access Key / Secret Access Key 未配置，请在渠道设置中更新凭证", "error_code": "credentials_not_configured"})
		return
	}

	region, ok := service.RegionFromVolcBaseURL(ch.GetBaseURL())
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "不支持的区域", "error_code": "unsupported_region"})
		return
	}

	var accessKeyID, secretAccessKey string
	accessKeyID, err = common.DecryptSecret(settings.VolcCodingPlanAccessKeyId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "OpenAPI 凭证不可用，请重新配置", "error_code": "credentials_unavailable"})
		return
	}
	secretAccessKey, err = common.DecryptSecret(settings.VolcCodingPlanSecretAccessKey)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "OpenAPI 凭证不可用，请重新配置", "error_code": "credentials_unavailable"})
		return
	}

	client, err := service.GetHttpClientWithProxy(ch.GetSetting().Proxy)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	statusCode, body, err := service.FetchVolcCodingPlanUsageByAkSk(ctx, client, region, accessKeyID, secretAccessKey)
	if err != nil {
		common.SysError("failed to fetch volc coding plan usage: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取用量失败，请稍后重试", "error_code": "fetch_failed"})
		return
	}

	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		c.JSON(http.StatusOK, gin.H{
			"success":         false,
			"message":         "火山 OpenAPI Access Key / Secret Access Key 无效或已被禁用，请在渠道设置中更新凭证",
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

	windows := make([]gin.H, 0, len(info.Windows))
	for _, w := range info.Windows {
		windows = append(windows, gin.H{
			"period":            w.Period,
			"used_percent":      w.UsedPercent,
			"remaining_percent": w.RemainingPercent,
			"reset_at":          w.ResetAt,
			"reset_in_sec":      w.ResetInSec,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "",
		"upstream_status": 200,
		"error_code":      "",
		"data": gin.H{
			"status":  info.Status,
			"windows": windows,
		},
	})
}
