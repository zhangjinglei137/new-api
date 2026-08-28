package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestUpdateChannelSubscriptionBillingDoesNotResetUsage 回归：保存订阅配置不得清零已累计的用量统计。
// 历史 bug：UpdateChannelSubscriptionBilling 在 BillingMode=订阅时调用 ResetChannelSubscriptionUsage，
// 每次保存配置都会把三窗口用量清零。本测试锁定该契约。
func TestUpdateChannelSubscriptionBillingDoesNotResetUsage(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelSubscriptionUsage{}, &model.Log{}))

	channel := &model.Channel{Name: "sub-channel", Type: 1, Key: "sk-sub-test", Group: "default"}
	channel.SetOtherSettings(dto.ChannelOtherSettings{SubscriptionBilling: &dto.SubscriptionBillingConfig{
		BillingMode:       dto.SubscriptionBillingModeSubscribe,
		MonthlyTotalQuota: 30000000,
		FiveHourRatioBps:  2000,
		WeeklyRatioBps:    5000,
	}})
	require.NoError(t, db.Create(channel).Error)

	now := common.GetTimestamp()
	require.NoError(t, model.UpsertChannelSubscriptionUsage(&model.ChannelSubscriptionUsage{
		ChannelId:        int64(channel.Id),
		LastCheckpointAt: now - 100,
		LastRefreshAt:    now - 100,
		BucketStart5h:    now - 100, UsedQuota5h: 111,
		BucketStart7d: now - 100, UsedQuota7d: 222,
		BucketStart31d: now - 100, UsedQuota31d: 333,
		UpdatedAt: now - 100,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/"+strconv.Itoa(channel.Id)+"/subscription-billing",
		strings.NewReader(`{"billing_mode":1}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateChannelSubscriptionBilling(ctx)

	var resp struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success, "body=%s", recorder.Body.String())

	// 配置保存后用量统计保持原值
	usage, err := model.GetChannelSubscriptionUsage(int64(channel.Id))
	require.NoError(t, err)
	require.Equal(t, int64(111), usage.UsedQuota5h)
	require.Equal(t, int64(222), usage.UsedQuota7d)
	require.Equal(t, int64(333), usage.UsedQuota31d)
}
