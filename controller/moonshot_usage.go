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

// GetMoonshotCodingPlanUsage 查询 Kimi（Moonshot）Coding Plan 渠道的余量/用量
// （官方 /usages 端点，Bearer 认证）。任何响应都不得包含凭证。
func GetMoonshotCodingPlanUsage(c *gin.Context) {
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
	if ch.Type != constant.ChannelTypeMoonshot {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not Moonshot"})
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

	info, err := service.FetchMoonshotCodingPlanUsage(ch)
	if err != nil {
		common.SysError("failed to fetch moonshot coding plan usage: " + err.Error())
		switch {
		case errors.Is(err, service.ErrMoonshotCodingPlanUnauthorized):
			c.JSON(http.StatusOK, gin.H{
				"success":    false,
				"message":    "API Key 无效或已过期，请在渠道设置中更新凭证",
				"error_code": "credentials_expired",
			})
		case errors.Is(err, service.ErrMoonshotCodingPlanNoPlan):
			c.JSON(http.StatusOK, gin.H{
				"success":    false,
				"message":    "该 API Key 无 Kimi Coding Plan 订阅",
				"error_code": "no_subscription",
			})
		case errors.Is(err, service.ErrMoonshotCodingPlanSchema):
			c.JSON(http.StatusOK, gin.H{
				"success":    false,
				"message":    "无法解析上游用量数据",
				"error_code": "usage_schema_unknown",
			})
		default:
			c.JSON(http.StatusOK, gin.H{
				"success":    false,
				"message":    "获取用量失败，请稍后重试",
				"error_code": "fetch_failed",
			})
		}
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
