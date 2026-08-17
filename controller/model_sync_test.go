package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonEndpointsEqual(t *testing.T) {
	cases := []struct {
		name     string
		local    string
		upstream string
		want     bool
	}{
		{
			name:     "same object different formatting and key order",
			local:    "{\n  \"openai\": {\n    \"method\": \"POST\",\n    \"path\": \"/v1/chat/completions\"\n  }\n}",
			upstream: `{"openai":{"path":"/v1/chat/completions","method":"POST"}}`,
			want:     true,
		},
		{
			name:     "different path",
			local:    `{"openai":{"path":"/v1/chat/completions","method":"POST"}}`,
			upstream: `{"openai":{"path":"/v1/responses","method":"POST"}}`,
			want:     false,
		},
		{
			name:     "different endpoint key",
			local:    `{"openai":{"path":"/v1/chat/completions","method":"POST"}}`,
			upstream: `{"anthropic":{"path":"/v1/messages","method":"POST"}}`,
			want:     false,
		},
		{
			name:     "empty upstream is equal",
			local:    `{"openai":{"path":"/v1/chat/completions","method":"POST"}}`,
			upstream: ``,
			want:     true,
		},
		{
			name:     "empty local with non-empty upstream is not equal",
			local:    ``,
			upstream: `{"openai":{"path":"/v1/chat/completions","method":"POST"}}`,
			want:     false,
		},
		{
			name:     "unparseable local falls back to string compare",
			local:    `not-json`,
			upstream: `not-json`,
			want:     true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, jsonEndpointsEqual(tc.local, []byte(tc.upstream)))
		})
	}
}

func TestOpenCodeGoStatusToModelStatus(t *testing.T) {
	for _, status := range []string{"", "beta", "preview", "stable"} {
		assert.Equal(t, 1, openCodeGoStatusToModelStatus(status), "status %q", status)
	}
}

// setupSyncUpstreamServer 将 SYNC_UPSTREAM_BASE 指向本地 httptest server，
// 按路径返回模型/供应商 envelope JSON，避免测试访问真实上游。
func setupSyncUpstreamServer(t *testing.T, modelsJSON, vendorsJSON string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/newapi/models.json":
			_, _ = w.Write([]byte(modelsJSON))
		case "/api/newapi/vendors.json":
			_, _ = w.Write([]byte(vendorsJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	originalBase, hadBase := os.LookupEnv("SYNC_UPSTREAM_BASE")
	require.NoError(t, os.Setenv("SYNC_UPSTREAM_BASE", server.URL))
	t.Cleanup(func() {
		if hadBase {
			require.NoError(t, os.Setenv("SYNC_UPSTREAM_BASE", originalBase))
		} else {
			require.NoError(t, os.Unsetenv("SYNC_UPSTREAM_BASE"))
		}
	})
}

type syncUpstreamResponse struct {
	Success bool `json:"success"`
	Data    struct {
		CreatedModels int `json:"created_models"`
		UpdatedModels int `json:"updated_models"`
	} `json:"data"`
}

func runSyncUpstreamModels(t *testing.T, body string) syncUpstreamResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/model/sync", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	SyncUpstreamModels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var resp syncUpstreamResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	return resp
}

func TestSyncUpstreamModelsCreatesModelWithSyncOfficial(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	// abilities 引用一个本地元数据表中不存在的模型 → 进入缺失创建路径
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "zz-sync-create-model",
		ChannelId: 1,
		Enabled:   true,
	}).Error)

	modelsJSON := `{"success":true,"message":"","data":[{"model_name":"zz-sync-create-model","description":"upstream desc","status":1,"vendor_name":"OpenAI","endpoints":{"openai":{"path":"/v1/chat/completions","method":"POST"}}}]}`
	vendorsJSON := `{"success":true,"message":"","data":[{"name":"OpenAI","status":1}]}`
	setupSyncUpstreamServer(t, modelsJSON, vendorsJSON)

	resp := runSyncUpstreamModels(t, "")

	assert.Equal(t, 1, resp.Data.CreatedModels)

	var created model.Model
	require.NoError(t, db.Where("model_name = ?", "zz-sync-create-model").First(&created).Error)
	assert.Equal(t, 1, created.SyncOfficial)
	assert.Equal(t, "upstream desc", created.Description)
}

func TestSyncUpstreamModelsOverwriteSkipsSyncDisabledAndKeepsOfficial(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Model{
		ModelName:    "zz-sync-overwrite-model",
		Description:  "old desc",
		Status:       1,
		SyncOfficial: 1,
	}).Error)
	// 注意: gorm:"default:1" 会把 Create 的零值 sync_official 替换为 1，
	// 因此 sync_official = 0 的模型必须先 Create 再显式 Update。
	skipModel := &model.Model{
		ModelName:   "zz-sync-skip-model",
		Description: "keep desc",
		Status:      1,
	}
	require.NoError(t, db.Create(skipModel).Error)
	require.NoError(t, db.Model(&model.Model{}).Where("id = ?", skipModel.Id).Update("sync_official", 0).Error)

	modelsJSON := `{"success":true,"message":"","data":[{"model_name":"zz-sync-overwrite-model","description":"new desc","status":1,"vendor_name":"OpenAI"},{"model_name":"zz-sync-skip-model","description":"should not apply","status":1,"vendor_name":"OpenAI"}]}`
	vendorsJSON := `{"success":true,"message":"","data":[{"name":"OpenAI","status":1}]}`
	setupSyncUpstreamServer(t, modelsJSON, vendorsJSON)

	body := `{"overwrite":[{"model_name":"zz-sync-overwrite-model","fields":["description"]},{"model_name":"zz-sync-skip-model","fields":["description"]}]}`
	resp := runSyncUpstreamModels(t, body)

	// sync_official = 0 的模型被跳过，仅 1 个模型被更新
	assert.Equal(t, 1, resp.Data.UpdatedModels)

	var overwritten model.Model
	require.NoError(t, db.Where("model_name = ?", "zz-sync-overwrite-model").First(&overwritten).Error)
	assert.Equal(t, "new desc", overwritten.Description)
	assert.Equal(t, 1, overwritten.SyncOfficial)

	var skipped model.Model
	require.NoError(t, db.Where("model_name = ?", "zz-sync-skip-model").First(&skipped).Error)
	assert.Equal(t, "keep desc", skipped.Description)
	assert.Equal(t, 0, skipped.SyncOfficial)
}

func TestSyncUpstreamModelsOverwriteSkipsIdenticalEndpoints(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	// 本地端点为前端 UI 保存的带缩进、键序不同的 JSON，与上游紧凑 JSON 语义相同
	require.NoError(t, db.Create(&model.Model{
		ModelName: "zz-sync-endpoint-model",
		Endpoints: "{\n  \"openai\": {\n    \"method\": \"POST\",\n    \"path\": \"/v1/chat/completions\"\n  }\n}",
		Status:    1,
		SyncOfficial: 1,
	}).Error)

	modelsJSON := `{"success":true,"message":"","data":[{"model_name":"zz-sync-endpoint-model","status":1,"endpoints":{"openai":{"path":"/v1/chat/completions","method":"POST"}}}]}`
	vendorsJSON := `{"success":true,"message":"","data":[]}`
	setupSyncUpstreamServer(t, modelsJSON, vendorsJSON)

	body := `{"overwrite":[{"model_name":"zz-sync-endpoint-model","fields":["endpoints"]}]}`
	resp := runSyncUpstreamModels(t, body)

	// 语义相等时不写库、不计入更新
	assert.Equal(t, 0, resp.Data.UpdatedModels)

	var stored model.Model
	require.NoError(t, db.Where("model_name = ?", "zz-sync-endpoint-model").First(&stored).Error)
	assert.Equal(t, "{\n  \"openai\": {\n    \"method\": \"POST\",\n    \"path\": \"/v1/chat/completions\"\n  }\n}", stored.Endpoints)
}
