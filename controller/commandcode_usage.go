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

// GetCommandCodeUsage 返回 CommandCode 渠道的用量信息（按时间窗口：
// session / 每周 / 每月 / topup，各含已用%/剩余%/已用金额/额度/重置时刻）。
// 依赖渠道配置的登录会话 Cookie 调用上游内部接口，响应绝不含 cookie 等敏感凭证。
func GetCommandCodeUsage(c *gin.Context) {
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
	if ch.Type != constant.ChannelTypeCommandCode {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not CommandCode"})
		return
	}
	if strings.TrimSpace(ch.GetOtherSettings().CommandCodeCookie) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "commandcode cookie 未配置"})
		return
	}
	info, err := service.FetchCommandCodeUsage(ch)
	if err != nil {
		common.SysError("failed to fetch commandcode usage: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	windows := make([]gin.H, 0, len(info.Windows))
	for _, w := range info.Windows {
		windows = append(windows, gin.H{
			"period":            w.Period,
			"status":            w.Status,
			"used_percent":      w.UsedPercent,
			"remaining_percent": w.RemainingPercent,
			"used":              w.Used,
			"limit":             w.Limit,
			"reset_at":          w.ResetAt,
			"metered":           w.Metered,
		})
	}
	common.ApiSuccess(c, gin.H{"windows": windows})
}
