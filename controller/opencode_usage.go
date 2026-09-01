package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetOpenCodeGoUsage 返回 opencode 渠道的用量信息（按时间窗口：
// 5 小时 / 每周 / 每月，各含已用%/剩余%/重置倒计时）。
// 响应绝不含 cookie 等敏感凭证。
func GetOpenCodeGoUsage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	ch, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if ch == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
		return
	}
	if ch.Type != constant.ChannelTypeOpenCodeGo {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not OpenCodeGo"})
		return
	}
	settings := ch.GetOtherSettings()
	if strings.TrimSpace(settings.OpenCodeWorkspaceId) == "" || strings.TrimSpace(settings.OpenCodeAuthCookie) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "opencode workspace id / cookie 未配置"})
		return
	}
	info, err := service.FetchOpenCodeGoUsage(ch)
	if err != nil {
		common.SysError("failed to fetch opencode go usage: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	windows := make([]gin.H, 0, len(info.Windows))
	for _, w := range info.Windows {
		windows = append(windows, gin.H{
			"period":            w.Period,
			"used_percent":      w.UsedPercent,
			"remaining_percent": w.RemainingPercent,
			"reset_in_sec":      w.ResetInSec,
		})
	}
	common.ApiSuccess(c, gin.H{"windows": windows})
}
