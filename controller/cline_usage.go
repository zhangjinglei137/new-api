package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetClinePlanUsage 查询 ClinePass 渠道的 Coding Plan 用量（Bearer 认证）。
// 任何响应都不得包含凭证。
func GetClinePlanUsage(c *gin.Context) {
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
	if ch.Type != constant.ChannelTypeCline {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not Cline"})
		return
	}
	if ch.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "multi-key channel is not supported"})
		return
	}
	if ch.GetOtherSettings().EndpointProfile != "clinepass" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "该渠道未启用 ClinePass 计划方式"})
		return
	}

	info, err := service.FetchClinePlanUsage(ch)
	if err != nil {
		common.SysError("failed to fetch cline plan usage: " + err.Error())
		switch {
		case errors.Is(err, service.ErrClinePlanUnauthorized):
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "API Key 无效或已过期，请在渠道设置中更新凭证", "error_code": "credentials_expired"})
		case errors.Is(err, service.ErrClinePlanSchema):
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "无法解析上游用量数据", "error_code": "usage_schema_unknown"})
		default:
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取用量失败，请稍后重试", "error_code": "fetch_failed"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "",
		"upstream_status": 200,
		"error_code":      "",
		"data": gin.H{
			"limits": info.Limits,
		},
	})
}
