package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelSubscriptionUsage 渠道订阅计费的增量统计状态（主库）。
// 数据只做统计展示，不参与扣费链路。
type ChannelSubscriptionUsage struct {
	ChannelId        int64  `gorm:"primaryKey" json:"channel_id"`
	LastCheckpointAt int64  `json:"last_checkpoint_at"` // 增量已处理到的 created_at 边界(半开区间左端)
	LastRefreshAt    int64  `json:"last_refresh_at"`
	LastError        string `json:"last_error"`
	Partial          bool   `json:"partial"` // 日志保留期<31天，月窗口不完整

	// 5h 窗口（对齐 opencode Go 官方滑窗语义：rollingUsage + timeRollingUpdated）。
	// UsedQuota5h 是自 time_rolling_updated 起累计的增量；滑窗过期（timeRollingUpdated < now-5h）即归 0。
	UsedQuota5h        int64 `json:"used_quota_5h"`
	TimeRollingUpdated int64 `gorm:"default:0" json:"time_rolling_updated"`
	// 周窗口（对齐官方 UTC 自然周语义：weeklyUsage + timeWeeklyUpdated）。
	UsedQuota7d       int64 `json:"used_quota_7d"`
	TimeWeeklyUpdated int64 `gorm:"default:0" json:"time_weekly_updated"`
	// 月窗口（保留固定 31d 桶语义）。
	BucketStart31d int64 `json:"bucket_start_31d"`
	UsedQuota31d   int64 `json:"used_quota_31d"`

	// 三窗口各自独立的手动基线（bps，2000=20%；允许 >10000 表示超限）。
	// 基线是用户声明的"已用百分比"底数，与窗口累计增量叠加展示。
	BaselineBps5h    int   `gorm:"default:0" json:"baseline_bps_5h"`
	BaselineBps7d    int   `gorm:"default:0" json:"baseline_bps_7d"`
	BaselineBps31d   int   `gorm:"default:0" json:"baseline_bps_31d"`
	BaselineSetAt5h  int64 `gorm:"default:0" json:"baseline_set_at_5h"`
	BaselineSetAt7d  int64 `gorm:"default:0" json:"baseline_set_at_7d"`
	BaselineSetAt31d int64 `gorm:"default:0" json:"baseline_set_at_31d"`

	ManualInitialized bool  `json:"manual_initialized"` // 是否已手动设置过基线
	UpdatedAt         int64 `json:"updated_at"`
}

func (ChannelSubscriptionUsage) TableName() string {
	return "channel_subscription_usages"
}

// UpsertChannelSubscriptionUsage 按 channel_id 冲突时整体覆盖，三库兼容。
func UpsertChannelSubscriptionUsage(usage *ChannelSubscriptionUsage) error {
	if usage == nil {
		return errors.New("usage is required")
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"last_checkpoint_at",
			"last_refresh_at",
			"last_error",
			"partial",
			"used_quota5h",
			"time_rolling_updated",
			"used_quota7d",
			"time_weekly_updated",
			"bucket_start31d",
			"used_quota31d",
			"baseline_bps5h",
			"baseline_bps7d",
			"baseline_bps31d",
			"baseline_set_at5h",
			"baseline_set_at7d",
			"baseline_set_at31d",
			"manual_initialized",
			"updated_at",
		}),
	}).Create(usage).Error
}

// GetChannelSubscriptionUsage 读取单个渠道的订阅统计状态。
// 不存在时返回 gorm.ErrRecordNotFound。
func GetChannelSubscriptionUsage(channelId int64) (*ChannelSubscriptionUsage, error) {
	usage := &ChannelSubscriptionUsage{ChannelId: channelId}
	err := DB.First(usage, "channel_id = ?", channelId).Error
	if err != nil {
		return nil, err
	}
	return usage, nil
}

// GetChannelSubscriptionUsagesByIds 批量读取订阅统计状态（避免列表页 N+1）。
// 返回 map[channelId]*ChannelSubscriptionUsage，仅包含存在的记录。
func GetChannelSubscriptionUsagesByIds(channelIds []int64) (map[int64]*ChannelSubscriptionUsage, error) {
	result := make(map[int64]*ChannelSubscriptionUsage, len(channelIds))
	if len(channelIds) == 0 {
		return result, nil
	}
	var usages []*ChannelSubscriptionUsage
	err := DB.Where("channel_id IN ?", channelIds).Find(&usages).Error
	if err != nil {
		return nil, err
	}
	for _, usage := range usages {
		if usage != nil {
			result[usage.ChannelId] = usage
		}
	}
	return result, nil
}

// DeleteChannelSubscriptionUsage 渠道删除时清理统计状态。
func DeleteChannelSubscriptionUsage(channelId int64) error {
	return DB.Where("channel_id = ?", channelId).Delete(&ChannelSubscriptionUsage{}).Error
}

// DeleteChannelSubscriptionUsages 批量清理统计状态（按状态/条件批量删渠道时使用）。
func DeleteChannelSubscriptionUsages(channelIds []int64) error {
	if len(channelIds) == 0 {
		return nil
	}
	return DB.Where("channel_id IN ?", channelIds).Delete(&ChannelSubscriptionUsage{}).Error
}

// MigrateChannelSubscriptionUsageBaselines 一次性迁移旧版单值月基线到三窗口独立基线。
// 旧结构只有 baseline_bps31d + baseline_set_at；升级后需回填到 5h/7d/31d 三份，
// 幂等（仅处理 manual_initialized=true 且尚未回填 baseline_set_at31d 的行）。
// 新库无旧列时直接跳过。迁移在 AutoMigrate 完成后调用。
func MigrateChannelSubscriptionUsageBaselines() error {
	if !DB.Migrator().HasColumn(&ChannelSubscriptionUsage{}, "baseline_set_at") {
		return nil // 全新库：无旧列可迁移
	}
	return DB.Model(&ChannelSubscriptionUsage{}).
		Where("manual_initialized = ? AND baseline_set_at > 0 AND baseline_set_at31d = 0", true).
		Updates(map[string]interface{}{
			"baseline_bps5h":     gorm.Expr("baseline_bps31d"),
			"baseline_bps7d":     gorm.Expr("baseline_bps31d"),
			"baseline_set_at5h":  gorm.Expr("baseline_set_at"),
			"baseline_set_at7d":  gorm.Expr("baseline_set_at"),
			"baseline_set_at31d": gorm.Expr("baseline_set_at"),
		}).Error
}
