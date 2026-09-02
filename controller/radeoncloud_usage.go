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

// GetRadeonCloudUsage 查询 AMD Radeon Cloud 渠道的每日免费额度用量
// （官方 /api/v1/usage 端点，Bearer 认证）。任何响应都不得包含凭证。
func GetRadeonCloudUsage(c *gin.Context) {
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
	if ch.Type != constant.ChannelTypeRadeonCloud {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not RadeonCloud"})
		return
	}
	if ch.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "multi-key channel is not supported"})
		return
	}

	info, err := service.FetchRadeonCloudUsage(ch)
	if err != nil {
		common.SysError("failed to fetch radeon cloud usage: " + err.Error())
		switch {
		case errors.Is(err, service.ErrRadeonCloudUnauthorized):
			c.JSON(http.StatusOK, gin.H{
				"success":    false,
				"message":    "API Key 无效或已过期，请在渠道设置中更新凭证",
				"error_code": "credentials_expired",
			})
		case errors.Is(err, service.ErrRadeonCloudSchema):
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

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "",
		"upstream_status": 200,
		"error_code":      "",
		"data": gin.H{
			"rpm_limit":                info.RpmLimit,
			"daily_limit_points":       info.DailyLimitPoints,
			"daily_used_points":        info.DailyUsedPoints,
			"daily_remaining_points":   info.DailyRemainingPoints,
			"daily_used_percent":       info.DailyUsedPercent,
			"daily_reset_at":           info.DailyResetAt,
			"daily_reset_in_sec":       info.DailyResetInSec,
			"today_requests":           info.TodayRequests,
			"today_tokens":             info.TodayTokens,
			"last_24h_requests":        info.Last24hRequests,
			"last_24h_tokens":          info.Last24hTokens,
			"last_24h_last_request_at": info.Last24hLastRequestAt,
			"period_started_at":        info.PeriodStartedAt,
		},
	})
}
