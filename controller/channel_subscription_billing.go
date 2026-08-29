package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// subscriptionBillingConfigResponse 渠道订阅计费配置的展示结构
// （同时返回 USD 展示值与 quota 值 / bps）。
type subscriptionBillingConfigResponse struct {
	BillingMode          int                               `json:"billing_mode"`
	MonthlyTotalUSD      float64                           `json:"monthly_total_usd"`
	MonthlyTotalQuota    int64                             `json:"monthly_total_quota"`
	FiveHourRatioPercent float64                           `json:"five_hour_ratio_percent"`
	FiveHourRatioBps     int                               `json:"five_hour_ratio_bps"`
	WeeklyRatioPercent   float64                           `json:"weekly_ratio_percent"`
	WeeklyRatioBps       int                               `json:"weekly_ratio_bps"`
	ModelTiers           []subscriptionBillingTierResponse `json:"model_tiers"`
	UpdatedAt            int64                             `json:"updated_at"`
}

type subscriptionBillingTierResponse struct {
	Model        string  `json:"model"`
	MonthlyQuota int64   `json:"monthly_quota"`
	MonthlyUSD   float64 `json:"monthly_usd"`
}

func buildSubscriptionBillingConfigResponse(cfg *dto.SubscriptionBillingConfig) subscriptionBillingConfigResponse {
	resp := subscriptionBillingConfigResponse{}
	if cfg == nil {
		return resp
	}
	resp.BillingMode = cfg.BillingMode
	resp.MonthlyTotalQuota = cfg.MonthlyTotalQuota
	resp.MonthlyTotalUSD = service.SubscriptionQuotaToUSD(cfg.MonthlyTotalQuota)
	resp.FiveHourRatioBps = cfg.FiveHourRatioBps
	resp.FiveHourRatioPercent = float64(cfg.FiveHourRatioBps) / 100.0
	resp.WeeklyRatioBps = cfg.WeeklyRatioBps
	resp.WeeklyRatioPercent = float64(cfg.WeeklyRatioBps) / 100.0
	resp.UpdatedAt = cfg.UpdatedAt
	resp.ModelTiers = make([]subscriptionBillingTierResponse, 0, len(cfg.ModelTiers))
	for _, tier := range cfg.ModelTiers {
		resp.ModelTiers = append(resp.ModelTiers, subscriptionBillingTierResponse{
			Model:        tier.Model,
			MonthlyQuota: tier.MonthlyQuota,
			MonthlyUSD:   service.SubscriptionQuotaToUSD(tier.MonthlyQuota),
		})
	}
	return resp
}

// defaultSubscriptionBillingConfig 返回默认订阅配置（月额度 60 USD、5h=20%、周=50%）。
func defaultSubscriptionBillingConfig() (*dto.SubscriptionBillingConfig, error) {
	cfg := &dto.SubscriptionBillingConfig{
		BillingMode:      dto.SubscriptionBillingModePayAsYouGo,
		FiveHourRatioBps: service.SubscriptionDefaultFiveHourRatioBps,
		WeeklyRatioBps:   service.SubscriptionDefaultWeeklyRatioBps,
	}
	quota, err := service.SubscriptionUSDToQuota(service.SubscriptionDefaultMonthlyUSD)
	if err != nil {
		return nil, err
	}
	cfg.MonthlyTotalQuota = quota
	return cfg, nil
}

// GetChannelSubscriptionBilling 返回渠道订阅计费配置。
func GetChannelSubscriptionBilling(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cfg := channel.GetOtherSettings().SubscriptionBilling
	if cfg == nil {
		cfg, err = defaultSubscriptionBillingConfig()
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	common.ApiSuccess(c, buildSubscriptionBillingConfigResponse(cfg))
}

type subscriptionBillingTierRequest struct {
	Model      string   `json:"model"`
	MonthlyUSD *float64 `json:"monthly_usd"`
}

type subscriptionBillingPutRequest struct {
	BillingMode          *int                              `json:"billing_mode"`
	MonthlyTotalUSD      *float64                          `json:"monthly_total_usd"`
	FiveHourRatioPercent *float64                          `json:"five_hour_ratio_percent"`
	WeeklyRatioPercent   *float64                          `json:"weekly_ratio_percent"`
	ModelTiers           *[]subscriptionBillingTierRequest `json:"model_tiers"`
}

// UpdateChannelSubscriptionBilling 保存渠道订阅计费配置。
// 只接收 subscription_billing 子字段；服务端将 USD 换算为 quota、
// 百分比换算为 bps 后存储并校验；配置变更时重置统计。
func UpdateChannelSubscriptionBilling(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req subscriptionBillingPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	otherSettings := channel.GetOtherSettings()
	existing := otherSettings.SubscriptionBilling
	cfg := &dto.SubscriptionBillingConfig{}
	if existing != nil {
		*cfg = *existing
	}

	if req.BillingMode != nil {
		cfg.BillingMode = *req.BillingMode
	}
	if req.MonthlyTotalUSD != nil {
		quota, err := service.SubscriptionUSDToQuota(*req.MonthlyTotalUSD)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		cfg.MonthlyTotalQuota = quota
	} else if existing == nil || existing.MonthlyTotalQuota == 0 {
		defaultCfg, err := defaultSubscriptionBillingConfig()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		cfg.MonthlyTotalQuota = defaultCfg.MonthlyTotalQuota
	}
	if req.FiveHourRatioPercent != nil {
		bps, err := service.SubscriptionRatioBpsFromPercent(*req.FiveHourRatioPercent)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		cfg.FiveHourRatioBps = bps
	} else if existing == nil || existing.FiveHourRatioBps == 0 {
		cfg.FiveHourRatioBps = service.SubscriptionDefaultFiveHourRatioBps
	}
	if req.WeeklyRatioPercent != nil {
		bps, err := service.SubscriptionRatioBpsFromPercent(*req.WeeklyRatioPercent)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		cfg.WeeklyRatioBps = bps
	} else if existing == nil || existing.WeeklyRatioBps == 0 {
		cfg.WeeklyRatioBps = service.SubscriptionDefaultWeeklyRatioBps
	}
	if req.ModelTiers != nil {
		tiers := make([]dto.SubscriptionModelTier, 0, len(*req.ModelTiers))
		for i, tier := range *req.ModelTiers {
			modelName := strings.TrimSpace(tier.Model)
			if modelName == "" {
				common.ApiError(c, fmt.Errorf("model_tiers[%d].model is required", i))
				return
			}
			if tier.MonthlyUSD == nil {
				common.ApiError(c, fmt.Errorf("model_tiers[%d].monthly_usd is required", i))
				return
			}
			quota, err := service.SubscriptionUSDToQuota(*tier.MonthlyUSD)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			tiers = append(tiers, dto.SubscriptionModelTier{Model: modelName, MonthlyQuota: quota})
		}
		cfg.ModelTiers = tiers
	}
	cfg.UpdatedAt = common.GetTimestamp()

	if err := service.ValidateSubscriptionBillingConfig(cfg); err != nil {
		common.ApiError(c, err)
		return
	}

	otherSettings.SubscriptionBilling = cfg
	channel.SetOtherSettings(otherSettings)
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", channelId).Update("settings", channel.OtherSettings).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()

	// 注意：保存配置不再重置订阅统计。用量统计与配置解耦——
	// 若需重置/设置初始用量，请通过 baseline 接口显式操作。

	recordManageAudit(c, "channel.subscription_billing_update", map[string]interface{}{
		"id":           channelId,
		"name":         channel.Name,
		"billing_mode": cfg.BillingMode,
	})
	common.ApiSuccess(c, buildSubscriptionBillingConfigResponse(cfg))
}

// GetChannelSubscriptionUsage 返回渠道订阅用量（先执行幂等增量刷新）。
func GetChannelSubscriptionUsage(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cfg := channel.GetOtherSettings().SubscriptionBilling
	if cfg == nil || cfg.BillingMode != dto.SubscriptionBillingModeSubscribe {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "渠道未启用订阅计费",
		})
		return
	}

	if err := service.RefreshChannelSubscriptionUsage(channelId); err != nil {
		common.SysError(fmt.Sprintf("failed to refresh subscription usage: channel_id=%d, error=%v", channelId, err))
		common.ApiError(c, errors.New("订阅用量刷新失败，请稍后重试"))
		return
	}

	data, err := service.BuildSubscriptionUsageData(channelId, cfg, common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

type subscriptionBaselineResponse struct {
	UsedPercent5h     *float64 `json:"used_percent_5h,omitempty"`
	UsedPercent7d     *float64 `json:"used_percent_7d,omitempty"`
	UsedPercent31d    *float64 `json:"used_percent_31d,omitempty"`
	BaselineSetAt5h   int64    `json:"baseline_set_at_5h"`
	BaselineSetAt7d   int64    `json:"baseline_set_at_7d"`
	BaselineSetAt31d  int64    `json:"baseline_set_at_31d"`
	ManualInitialized bool     `json:"manual_initialized"`
}

// GetChannelSubscriptionBaseline 返回渠道订阅用量基线（三窗口各自的已用百分比 + 起始时间）。
func GetChannelSubscriptionBaseline(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	usage, err := model.GetChannelSubscriptionUsage(int64(channelId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 从未初始化：返回空基线（nil 表示该窗口未设置）。
			common.ApiSuccess(c, subscriptionBaselineResponse{})
			return
		}
		common.ApiError(c, err)
		return
	}
	toPercent := func(bps int) *float64 {
		if bps <= 0 {
			return nil
		}
		p := float64(bps) / 100
		return &p
	}
	common.ApiSuccess(c, subscriptionBaselineResponse{
		UsedPercent5h:     toPercent(usage.BaselineBps5h),
		UsedPercent7d:     toPercent(usage.BaselineBps7d),
		UsedPercent31d:    toPercent(usage.BaselineBps31d),
		BaselineSetAt5h:   usage.BaselineSetAt5h,
		BaselineSetAt7d:   usage.BaselineSetAt7d,
		BaselineSetAt31d:  usage.BaselineSetAt31d,
		ManualInitialized: usage.ManualInitialized,
	})
}

// SetChannelSubscriptionBaseline 手动设置渠道订阅用量基线（三窗口各自独立）：
// 每个窗口记录各自起始时间为统计起点，之后增量只从该时刻累计，不再重读更早历史日志。
func SetChannelSubscriptionBaseline(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cfg := channel.GetOtherSettings().SubscriptionBilling
	if cfg == nil || cfg.BillingMode != dto.SubscriptionBillingModeSubscribe {
		common.ApiError(c, errors.New("渠道未启用订阅计费"))
		return
	}

	var req service.SubscriptionBaselineInput
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	if err := service.SetChannelSubscriptionBaseline(channelId, req); err != nil {
		common.ApiError(c, err)
		return
	}

	recordManageAudit(c, "channel.subscription_baseline_set", map[string]interface{}{
		"id":               channelId,
		"name":             channel.Name,
		"baseline_5h_bps":  derefBps(req.UsedPercent5h),
		"baseline_7d_bps":  derefBps(req.UsedPercent7d),
		"baseline_31d_bps": derefBps(req.UsedPercent31d),
		"baseline_5h_at":   derefAt(req.BaselineAt5h),
		"baseline_7d_at":   derefAt(req.BaselineAt7d),
		"baseline_31d_at":  derefAt(req.BaselineAt31d),
	})
	common.ApiSuccess(c, gin.H{"success": true})
}

func derefBps(p *float64) int {
	if p == nil {
		return 0
	}
	bps, err := service.SubscriptionBaselineBpsFromPercent(*p)
	if err != nil {
		return 0
	}
	return bps
}

func derefAt(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
