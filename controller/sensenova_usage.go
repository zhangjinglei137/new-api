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

// GetSenseNovaUsage 返回 SenseNova（日日新）渠道的积分池用量：
// 套餐信息 + 各积分池的 5h/7d 窗口用量、grant 余额与到期时间。
// 依赖渠道配置的账号密码（sensenova_username / sensenova_password，AES-GCM
// 密文存储，读取时解密兼容历史明文），登录与用量请求均走渠道代理；
// 响应绝不含凭证。
func GetSenseNovaUsage(c *gin.Context) {
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
	if ch.Type != constant.ChannelTypeSenseNova {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not SenseNova"})
		return
	}
	settings := ch.GetOtherSettings()
	if strings.TrimSpace(settings.SenseNovaUsername) == "" || strings.TrimSpace(settings.SenseNovaPassword) == "" {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    "sensenova 账号未配置",
			"error_code": service.SenseNovaErrorCodeCredentialsNotConfigured,
		})
		return
	}
	username, err := common.DecryptSecret(strings.TrimSpace(settings.SenseNovaUsername))
	if err != nil {
		common.SysError("failed to decrypt sensenova username: " + err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    "sensenova 账号未配置",
			"error_code": service.SenseNovaErrorCodeCredentialsNotConfigured,
		})
		return
	}
	password, err := common.DecryptSecret(strings.TrimSpace(settings.SenseNovaPassword))
	if err != nil {
		common.SysError("failed to decrypt sensenova password: " + err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    "sensenova 账号未配置",
			"error_code": service.SenseNovaErrorCodeCredentialsNotConfigured,
		})
		return
	}
	if username == "" || password == "" {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    "sensenova 账号未配置",
			"error_code": service.SenseNovaErrorCodeCredentialsNotConfigured,
		})
		return
	}

	info, err := service.FetchSenseNovaUsage(ch.Id, username, password, ch.GetSetting().Proxy)
	if err != nil {
		common.SysError("failed to fetch sensenova usage: " + err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    err.Error(),
			"error_code": service.ClassifySenseNovaUsageError(err),
		})
		return
	}
	pools := make([]gin.H, 0, len(info.Pools))
	for _, p := range info.Pools {
		pools = append(pools, gin.H{
			"pool_type": p.PoolType,
			"name":      p.Name,
			"model_ids": p.ModelIDs,
			"window_5h": gin.H{
				"limit":     p.Window5h.Limit,
				"used":      p.Window5h.Used,
				"remaining": p.Window5h.Remaining,
				"reset_at":  p.Window5h.ResetAt,
			},
			"window_7d": gin.H{
				"limit":     p.Window7d.Limit,
				"used":      p.Window7d.Used,
				"remaining": p.Window7d.Remaining,
				"reset_at":  p.Window7d.ResetAt,
			},
			"grant_balance":                  p.GrantBalance,
			"nearest_grant_expiry":           p.NearestGrantExpiry,
			"nearest_grant_expiring_balance": p.NearestGrantExpiringBalance,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "",
		"error_code": service.SenseNovaErrorCodeNone,
		"data": gin.H{
			"plan":  gin.H{"id": info.Plan.ID, "name": info.Plan.Name},
			"pools": pools,
		},
	})
}
