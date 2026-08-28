package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	channelSubscriptionUsageTickInterval          = 5 * time.Minute
	channelSubscriptionUsageFullCalibrateInterval = 24 * time.Hour
	channelSubscriptionUsageBatchSize             = 200
)

var (
	channelSubscriptionUsageOnce          sync.Once
	channelSubscriptionUsageRunning       atomic.Bool
	channelSubscriptionUsageLastCalibrate atomic.Int64
)

// StartChannelSubscriptionUsageTask 每 5 分钟对启用订阅计费的渠道做增量刷新，
// 每 24 小时执行一次全量校正。复用现有 gopool + ticker 任务框架，不新建调度器。
func StartChannelSubscriptionUsageTask() {
	channelSubscriptionUsageOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("channel subscription usage task started: tick=%s full_calibrate=%s",
				channelSubscriptionUsageTickInterval, channelSubscriptionUsageFullCalibrateInterval))
			ticker := time.NewTicker(channelSubscriptionUsageTickInterval)
			defer ticker.Stop()

			runChannelSubscriptionUsageOnce()
			for range ticker.C {
				runChannelSubscriptionUsageOnce()
			}
		})
	})
}

// listSubscriptionBillingChannelIds 扫描启用状态且开启订阅计费的渠道。
func listSubscriptionBillingChannelIds() ([]int, error) {
	var channelIds []int
	offset := 0
	for {
		var channels []*model.Channel
		err := model.DB.
			Select("id", "status", "settings").
			Where("status = ?", common.ChannelStatusEnabled).
			Order("id asc").
			Limit(channelSubscriptionUsageBatchSize).
			Offset(offset).
			Find(&channels).Error
		if err != nil {
			return nil, err
		}
		if len(channels) == 0 {
			break
		}
		offset += channelSubscriptionUsageBatchSize
		for _, ch := range channels {
			if ch == nil {
				continue
			}
			cfg := ch.GetOtherSettings().SubscriptionBilling
			if cfg == nil || cfg.BillingMode != dto.SubscriptionBillingModeSubscribe {
				continue
			}
			channelIds = append(channelIds, ch.Id)
		}
	}
	return channelIds, nil
}

func runChannelSubscriptionUsageOnce() {
	if !channelSubscriptionUsageRunning.CompareAndSwap(false, true) {
		return
	}
	defer channelSubscriptionUsageRunning.Store(false)

	ctx := context.Background()
	now := time.Now()
	fullCalibrate := now.Sub(time.Unix(channelSubscriptionUsageLastCalibrate.Load(), 0)) >= channelSubscriptionUsageFullCalibrateInterval

	channelIds, err := listSubscriptionBillingChannelIds()
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("channel subscription usage: query channels failed: %v", err))
		return
	}

	var refreshed int
	for _, channelId := range channelIds {
		var refreshErr error
		if fullCalibrate {
			refreshErr = FullRecalibrateChannelSubscriptionUsage(channelId)
		} else {
			refreshErr = RefreshChannelSubscriptionUsage(channelId)
		}
		if refreshErr != nil {
			logger.LogWarn(ctx, fmt.Sprintf("channel subscription usage: channel_id=%d refresh failed: %v", channelId, refreshErr))
			continue
		}
		refreshed++
	}

	if fullCalibrate {
		channelSubscriptionUsageLastCalibrate.Store(time.Now().Unix())
	}
	if common.DebugEnabled {
		logger.LogDebug(ctx, "channel subscription usage: channels=%d refreshed=%d full_calibrate=%v", len(channelIds), refreshed, fullCalibrate)
	}
}
