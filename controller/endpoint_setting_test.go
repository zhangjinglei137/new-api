package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetEndpointSettingsReturnsTen 验证 GET 返回 10 条完整端点定义，
// 且 npm_options 含定义内置的 npm 值（去空、排序）。
func TestGetEndpointSettingsReturnsTen(t *testing.T) {
	t.Cleanup(common.ResetEndpointDefinitions)
	setupModelListControllerTestDB(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	GetEndpointSettings(c)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Endpoints  []common.EndpointDefinition `json:"endpoints"`
			NPMOptions []string                    `json:"npm_options"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data.Endpoints, 10)
	assert.Equal(t, string(constant.EndpointTypeOpenAI), body.Data.Endpoints[0].Type)
	assert.Equal(t, string(constant.EndpointTypeOpenAIVideo), body.Data.Endpoints[9].Type)
	// 内置定义的 npm 值（@ai-sdk/anthropic、@ai-sdk/openai）
	assert.Equal(t, []string{"@ai-sdk/anthropic", "@ai-sdk/openai"}, body.Data.NPMOptions)
}

// TestGetEndpointSettingsNPMOptionsMergesDBAndDefinitions 验证 npm_options
// 合并 models 表 provider_npm 去重值 ∪ 定义里的 npm 值：去空、去重、排序；
// 空白/空串行被排除，软删行被排除。
func TestGetEndpointSettingsNPMOptionsMergesDBAndDefinitions(t *testing.T) {
	t.Cleanup(common.ResetEndpointDefinitions)
	db := setupModelListControllerTestDB(t)

	// DB-only 值
	require.NoError(t, db.Create(&model.Model{ModelName: "zz-npm-db-only", ProviderNpm: "@ai-sdk/gemini"}).Error)
	// 与定义重复的值（应去重）
	require.NoError(t, db.Create(&model.Model{ModelName: "zz-npm-dup", ProviderNpm: "@ai-sdk/anthropic"}).Error)
	// 空白/空串值（应排除）
	require.NoError(t, db.Create(&model.Model{ModelName: "zz-npm-blank", ProviderNpm: "   "}).Error)
	require.NoError(t, db.Create(&model.Model{ModelName: "zz-npm-empty"}).Error)
	// 软删行（应排除）
	deleted := &model.Model{ModelName: "zz-npm-deleted", ProviderNpm: "@ai-sdk/amazon-bedrock"}
	require.NoError(t, db.Create(deleted).Error)
	require.NoError(t, db.Delete(deleted).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	GetEndpointSettings(c)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Endpoints  []common.EndpointDefinition `json:"endpoints"`
			NPMOptions []string                    `json:"npm_options"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data.Endpoints, 10)
	assert.Equal(t, []string{"@ai-sdk/anthropic", "@ai-sdk/gemini", "@ai-sdk/openai"}, body.Data.NPMOptions)
}

// TestUpdateEndpointSettingsPersistsAndRefreshes 验证 PUT 落 option 键并刷新
// common 配置缓存。
func TestUpdateEndpointSettingsPersistsAndRefreshes(t *testing.T) {
	t.Cleanup(common.ResetEndpointDefinitions)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	// updateOptionMap 写入 common.OptionMap，正常由 InitOptionMap 初始化
	originalOptionMap := common.OptionMap
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	// 基于默认 10 条修改一个 path 后整体写回
	defs := common.GetEndpointDefinitions()
	defs[0].Path = "/custom/chat"
	payload, err := common.Marshal(map[string]any{"endpoints": defs})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/model_setting/endpoints", strings.NewReader(string(payload)))
	c.Request.Header.Set("Content-Type", "application/json")
	UpdateEndpointSettings(c)

	var body struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)

	// option 表已持久化
	var opt model.Option
	require.NoError(t, db.Where("key = ?", common.EndpointDefinitionsOptionKey).First(&opt).Error)
	assert.NotEmpty(t, opt.Value)

	// common 缓存已刷新
	info, ok := common.GetDefaultEndpointInfo(constant.EndpointTypeOpenAI)
	require.True(t, ok)
	assert.Equal(t, "/custom/chat", info.Path)
}

// TestUpdateEndpointSettingsRejectsInvalid 验证非法端点定义返回 400 且不落库。
func TestUpdateEndpointSettingsRejectsInvalid(t *testing.T) {
	t.Cleanup(common.ResetEndpointDefinitions)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	originalOptionMap := common.OptionMap
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	payload := `{"endpoints":[{"type":"unknown","display_name":"X","path":"/x","method":"POST"}]}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/model_setting/endpoints", strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	UpdateEndpointSettings(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.NotEmpty(t, body.Message)

	// 未落库
	var count int64
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", common.EndpointDefinitionsOptionKey).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}
