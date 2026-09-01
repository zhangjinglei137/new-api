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

func TestGetVolcCodingPlanUsageRejectsNonCodingProfile(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	channel := &model.Channel{Type: constant.ChannelTypeVolcEngine, Name: "volc-plain", Key: "k"}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1/volc/codingplan/usage", nil)
	GetVolcCodingPlanUsage(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "Coding Plan")
}

func TestGetVolcCodingPlanUsageRejectsNonVolcChannelType(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "openai", Key: "k"}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1/volc/codingplan/usage", nil)
	GetVolcCodingPlanUsage(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "VolcEngine")
}

func TestGetVolcCodingPlanUsageCredentialsNotConfigured(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	channel := &model.Channel{
		Type:          constant.ChannelTypeVolcEngine,
		Name:          "volc-coding",
		Key:           "k",
		OtherSettings: `{"endpoint_profile":"coding"}`,
	}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1/volc/codingplan/usage", nil)
	GetVolcCodingPlanUsage(ctx)

	var response struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		ErrorCode string `json:"error_code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "credentials_not_configured", response.ErrorCode)
	// 响应不得包含任何凭证内容
	assert.NotContains(t, recorder.Body.String(), "volc_coding_plan_cookie")
}

func TestGetVolcCodingPlanUsageUnsupportedRegion(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	channel := &model.Channel{
		Type:          constant.ChannelTypeVolcEngine,
		Name:          "volc-unknown-region",
		Key:           "k",
		BaseURL:       common.GetPointer("https://ark.cn-hangzhou.volces.com/api/coding"),
		OtherSettings: `{"endpoint_profile":"coding","volc_coding_plan_csrf_token":"e30","volc_coding_plan_cookie":"e30"}`,
	}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1/volc/codingplan/usage", nil)
	GetVolcCodingPlanUsage(ctx)

	var response struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		ErrorCode string `json:"error_code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "unsupported_region", response.ErrorCode)
}

func TestUpdateVolcCodingPlanCredentialsRejectsNonCodingProfile(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	channel := &model.Channel{Type: constant.ChannelTypeVolcEngine, Name: "volc-plain", Key: "k"}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/api/channel/1/volc/codingplan/credentials", nil)
	UpdateVolcCodingPlanCredentials(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "Coding Plan")
}
