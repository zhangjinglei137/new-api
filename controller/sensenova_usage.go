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

// respondSenseNovaCredentialsNotConfigured 返回「渠道未配置账号密码」的标准
// 错误响应；解密失败与字段为空都归到该分类，避免泄露凭证细节。
func respondSenseNovaCredentialsNotConfigured(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success":    false,
		"message":    "sensenova 账号未配置",
		"error_code": service.SenseNovaErrorCodeCredentialsNotConfigured,
	})
}

// loadSenseNovaChannelCredentials 按渠道 ID 取 SenseNova 渠道并解密其登录凭证；
// 任一校验失败时已写入错误响应并返回 ok=false，调用方直接 return。
// 账号密码以 AES-GCM 密文存储（sensenova_username / sensenova_password），
// 解密兼容历史明文；返回的凭证绝不出现在响应中。
func loadSenseNovaChannelCredentials(c *gin.Context, id int) (*model.Channel, string, string, bool) {
	ch, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return nil, "", "", false
	}
	if ch == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
		return nil, "", "", false
	}
	if ch.Type != constant.ChannelTypeSenseNova {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not SenseNova"})
		return nil, "", "", false
	}
	settings := ch.GetOtherSettings()
	if strings.TrimSpace(settings.SenseNovaUsername) == "" || strings.TrimSpace(settings.SenseNovaPassword) == "" {
		respondSenseNovaCredentialsNotConfigured(c)
		return nil, "", "", false
	}
	username, err := common.DecryptSecret(strings.TrimSpace(settings.SenseNovaUsername))
	if err != nil {
		common.SysError("failed to decrypt sensenova username: " + err.Error())
		respondSenseNovaCredentialsNotConfigured(c)
		return nil, "", "", false
	}
	password, err := common.DecryptSecret(strings.TrimSpace(settings.SenseNovaPassword))
	if err != nil {
		common.SysError("failed to decrypt sensenova password: " + err.Error())
		respondSenseNovaCredentialsNotConfigured(c)
		return nil, "", "", false
	}
	if username == "" || password == "" {
		respondSenseNovaCredentialsNotConfigured(c)
		return nil, "", "", false
	}
	return ch, username, password, true
}

// maskSenseNovaAPIKey 对 API key 做掩码：保留前缀 8 位与末尾 4 位，中间以 …
// 代替；过短时整体掩码。对外响应绝不返回完整明文。
func maskSenseNovaAPIKey(key string) string {
	const prefixLen, suffixLen = 8, 4
	if len(key) <= prefixLen+suffixLen {
		return "…"
	}
	return key[:prefixLen] + "…" + key[len(key)-suffixLen:]
}

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
	ch, username, password, ok := loadSenseNovaChannelCredentials(c, id)
	if !ok {
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

// GetSenseNovaAPIKeys 返回 SenseNova 渠道账号下的 API key 列表。凭证与登录
// 流程与 GetSenseNovaUsage 共用；对外只返回掩码后的 api_key，并用 in_use
// 标记该 key 是否为渠道当前使用的 key。
func GetSenseNovaAPIKeys(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	ch, username, password, ok := loadSenseNovaChannelCredentials(c, id)
	if !ok {
		return
	}
	keys, err := service.ListSenseNovaAPIKeys(ch.Id, username, password, ch.GetSetting().Proxy)
	if err != nil {
		common.SysError("failed to list sensenova api keys: " + err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    err.Error(),
			"error_code": service.ClassifySenseNovaUsageError(err),
		})
		return
	}
	out := make([]gin.H, 0, len(keys))
	for _, k := range keys {
		out = append(out, gin.H{
			"id":           k.ID,
			"display_name": k.DisplayName,
			"api_key":      maskSenseNovaAPIKey(k.APIKey),
			"key_type":     k.KeyType,
			"status":       k.Status,
			"is_default":   k.IsDefault,
			"create_time":  k.CreateTime,
			"in_use":       k.APIKey == ch.Key,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "",
		"error_code": service.SenseNovaErrorCodeNone,
		"data":       gin.H{"keys": out},
	})
}

// DeleteSenseNovaAPIKey 删除 SenseNova 渠道账号下指定 API key。删除前先拉取
// 列表确认该 key 存在，并拒绝删除渠道当前正在使用的 key（服务端兜底，不能
// 只依赖前端）。
func DeleteSenseNovaAPIKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	keyID := c.Param("keyId")
	ch, username, password, ok := loadSenseNovaChannelCredentials(c, id)
	if !ok {
		return
	}
	proxy := ch.GetSetting().Proxy
	keys, err := service.ListSenseNovaAPIKeys(ch.Id, username, password, proxy)
	if err != nil {
		common.SysError("failed to list sensenova api keys: " + err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    err.Error(),
			"error_code": service.ClassifySenseNovaUsageError(err),
		})
		return
	}
	var target *service.SenseNovaAPIKey
	for i := range keys {
		if keys[i].ID == keyID {
			target = &keys[i]
			break
		}
	}
	if target == nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    "apikey 不存在",
			"error_code": service.SenseNovaErrorCodeNone,
		})
		return
	}
	if target.APIKey == ch.Key {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    "不能删除正在使用的 apikey",
			"error_code": service.SenseNovaErrorCodeNone,
		})
		return
	}
	if err := service.DeleteSenseNovaAPIKey(ch.Id, username, password, proxy, keyID); err != nil {
		common.SysError("failed to delete sensenova api key: " + err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    err.Error(),
			"error_code": service.ClassifySenseNovaUsageError(err),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}
