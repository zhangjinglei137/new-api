package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetChannelBalanceZerosBalanceAndUsedQuota(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	channel := &model.Channel{
		Type:               constant.ChannelTypeOpenAI,
		Name:               "reset-me",
		Key:                "k",
		Balance:            12.5,
		UsedQuota:          1000,
		BalanceUpdatedTime: 123456,
	}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/reset_balance/"+fmt.Sprintf("%d", channel.Id), nil)

	ResetChannelBalance(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)

	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, float64(0), stored.Balance)
	assert.Equal(t, int64(0), stored.UsedQuota)
	assert.Equal(t, int64(123456), stored.BalanceUpdatedTime)

	var auditLog model.Log
	require.NoError(t, db.Order("id desc").First(&auditLog).Error)
	var auditData struct {
		Operation struct {
			Action string         `json:"action"`
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	require.NoError(t, common.UnmarshalJsonStr(auditLog.Other, &auditData))
	assert.Equal(t, "channel.reset_balance", auditData.Operation.Action)
	assert.Equal(t, float64(channel.Id), auditData.Operation.Params["id"])
	assert.Equal(t, "reset-me", auditData.Operation.Params["name"])
}

func TestResetChannelBalanceRejectsInvalidID(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "not-a-number"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/reset_balance/not-a-number", nil)

	ResetChannelBalance(ctx)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
}

func TestResetChannelBalanceChannelNotFound(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "999999"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/reset_balance/999999", nil)

	ResetChannelBalance(ctx)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
}

func TestResetChannelBalanceRejectsCodexChannel(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	channel := &model.Channel{
		Type: constant.ChannelTypeCodex,
		Name: "codex-channel",
		Key:  "k",
	}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/reset_balance/"+fmt.Sprintf("%d", channel.Id), nil)

	ResetChannelBalance(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "Codex")
}
