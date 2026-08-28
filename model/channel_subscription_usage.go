package model

import (
	"errors"

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
	// 手动基线：用户在"已使用额度"tab 设置的当前月已用百分比（bps，2000=20%）。
	// 允许 >10000 表示超限；仅存月维度，5h/周由配置比例实时派生。
	BaselineBps31d    int   `json:"baseline_bps_31d"`
	BaselineSetAt     int64 `json:"baseline_set_at"`    // 基线设置时刻（unix 秒），即统计起点
	ManualInitialized bool  `json:"manual_initialized"` // 是否已手动设置基线
	BucketStart5h     int64 `json:"bucket_start_5h"`
	UsedQuota5h       int64 `json:"used_quota_5h"`
	BucketStart7d     int64 `json:"bucket_start_7d"`
	UsedQuota7d       int64 `json:"used_quota_7d"`
	BucketStart31d    int64 `json:"bucket_start_31d"`
	UsedQuota31d      int64 `json:"used_quota_31d"`
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
			"baseline_bps31d",
			"baseline_set_at",
			"manual_initialized",
			"bucket_start5h",
			"used_quota5h",
			"bucket_start7d",
			"used_quota7d",
			"bucket_start31d",
			"used_quota31d",
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
