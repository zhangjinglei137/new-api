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

// TestOpenCodeGoVendorForProvider 验证 provider.npm → 供应商映射。
func TestOpenCodeGoVendorForProvider(t *testing.T) {
	cases := []struct {
		npm  string
		want string
	}{
		{"@ai-sdk/openai", "OpenAI"},
		{"@ai-sdk/anthropic", "Anthropic"},
		{"@ai-sdk/google", "Google"},
		{"@ai-sdk/google-vertex", "Google"},
		{"@ai-sdk/xai", "xAI"},
		{"@ai-sdk/deepseek", "DeepSeek"},
		{"@ai-sdk/qwen", "Qwen"},
		{"@ai-sdk/minimax", "MiniMax"},
		{"@ai-sdk/tencent-hunyuan", "Hunyuan"},
		{"@ai-sdk/zhipu", "Z.AI"},
		{"@ai-sdk/openrouter", "OpenRouter"},
		{"", ""},
		// 泛化包不映射，交由正则/兜底，避免误判
		{"@ai-sdk/openai-compatible", ""},
	}
	for _, tc := range cases {
		t.Run(tc.npm, func(t *testing.T) {
			assert.Equal(t, tc.want, openCodeGoVendorForProvider(tc.npm))
		})
	}
}

// TestOpenCodeGoVendorForModel 验证模型 ID 正则归属（含新增主流厂商前缀）。
func TestOpenCodeGoVendorForModel(t *testing.T) {
	cases := []struct {
		modelID string
		want    string
	}{
		{"gpt-4o", "OpenAI"},
		{"grok-3", "xAI"},
		{"glm-4", "Z.AI"},
		{"deepseek-chat", "DeepSeek"},
		{"qwen2.5-coder", "Qwen"},
		// 新增前缀
		{"claude-3-5-sonnet", "Anthropic"},
		{"llama-3.1-8b", "Meta"},
		{"mistral-large", "Mistral"},
		{"gemma-2-27b", "Google"},
		{"gemini-2.5-flash", "Google"},
		{"command-r", "Cohere"},
		{"o1-mini", "OpenAI"},
		{"o3-mini", "OpenAI"},
		// opencode-go 分组前缀补全
		{"longcat-2.0", "LongCat"},
		{"ling-3.0-flash-fin-free", "AntGroup"},
		{"ring-2.6-1t-free", "AntGroup"},
		{"north-mini-code-free", "Cohere"},
		{"trinity-large-preview-free", "Arcee"},
		{"muse-spark-1.2-contributor-free", "Meta"},
		{"ox-alpha-free", "OpenCode Go"},
		{"big-pickle", "OpenCode Go"},
		{"x-preview-f-free", "OpenCode Go"},
		// 已有规则确认覆盖
		{"minimax-m3", "MiniMax"},
		{"minimax-m2.7", "MiniMax"},
		{"qwen3.8-flash", "Qwen"},
		{"grok-4.6", "xAI"},
		{"laguna-s-2.1-free", "Poolside"},
		{"hy3-free", "Hunyuan"},
		{"hy4-preview", "Hunyuan"},
		// 兜底
		{"zz-some-unknown-model", "OpenCode Go"},
	}
	for _, tc := range cases {
		t.Run(tc.modelID, func(t *testing.T) {
			assert.Equal(t, tc.want, openCodeGoVendorForModel(tc.modelID))
		})
	}
}

// TestOpenCodeGoVendorForModelAndProvider 验证判定顺序：模型 ID 正则 >
// Provider(npm) 映射 > 兜底 "OpenCode Go"。上游 provider 已被证实会错标
// （minimax-m3 标 @ai-sdk/anthropic、grok-4.6 标 @ai-sdk/openai），
// 正则优先可修正这些错标。
func TestOpenCodeGoVendorForModelAndProvider(t *testing.T) {
	// 正则优先于 provider：即使 provider 错标，正则命中的真实供应商胜出
	assert.Equal(t, "MiniMax", openCodeGoVendorForModelAndProvider("minimax-m3", "@ai-sdk/anthropic"))
	assert.Equal(t, "MiniMax", openCodeGoVendorForModelAndProvider("minimax-m2.7", "@ai-sdk/anthropic"))
	assert.Equal(t, "Qwen", openCodeGoVendorForModelAndProvider("qwen3.8-flash", "@ai-sdk/anthropic"))
	assert.Equal(t, "xAI", openCodeGoVendorForModelAndProvider("grok-4.6", "@ai-sdk/openai"))
	assert.Equal(t, "Anthropic", openCodeGoVendorForModelAndProvider("claude-3-5-sonnet", "@ai-sdk/openai-compatible"))
	// 无 provider 时正则命中
	assert.Equal(t, "OpenAI", openCodeGoVendorForModelAndProvider("gpt-4o", ""))
	assert.Equal(t, "LongCat", openCodeGoVendorForModelAndProvider("longcat-2.0", ""))
	assert.Equal(t, "AntGroup", openCodeGoVendorForModelAndProvider("ling-3.0-flash-fin-free", ""))
	// 正则未命中 → provider 映射兜底（muse- 有正则则优先正则；无正则的
	// 任意 id + 有映射的 npm → provider 供应商，至少比 OpenCode Go 有意义）
	assert.Equal(t, "OpenAI", openCodeGoVendorForModelAndProvider("zz-unknown-served", "@ai-sdk/openai"))
	assert.Equal(t, "Anthropic", openCodeGoVendorForModelAndProvider("zz-anthropic-served", "@ai-sdk/anthropic"))
	// 均不命中 → 兜底
	assert.Equal(t, "OpenCode Go", openCodeGoVendorForModelAndProvider("zz-unknown-xyz", ""))
	assert.Equal(t, "OpenCode Go", openCodeGoVendorForModelAndProvider("zz-unknown-xyz", "@ai-sdk/openai-compatible"))
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
		FilledModels  int `json:"filled_models"`
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
	// 先建渠道再建能力：指向不存在渠道的能力是孤儿，会被 GetMissingModels
	// 排除（缺失模型误报修复），不会进入缺失创建路径
	require.NoError(t, db.Create(&model.Channel{Name: "zz-sync-ch", Type: 1, Status: common.ChannelStatusEnabled}).Error)
	var ch model.Channel
	require.NoError(t, db.Where("name = ?", "zz-sync-ch").First(&ch).Error)
	// abilities 引用一个本地元数据表中不存在的模型 → 进入缺失创建路径
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "zz-sync-create-model",
		ChannelId: ch.Id,
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

// TestSyncUpstreamModelsRemapsFallbackVendor 验证存量误映射修复：模型当前归属
// 兜底供应商 "OpenCode Go"，上游按新判定顺序推导出真实供应商 → vendor_id 被
// 重映射到真实供应商（复用 DB 已存在的同名供应商）。
func TestSyncUpstreamModelsRemapsFallbackVendor(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	fallback := &model.Vendor{Name: "OpenCode Go"}
	require.NoError(t, db.Create(fallback).Error)
	anthropic := &model.Vendor{Name: "Anthropic"}
	require.NoError(t, db.Create(anthropic).Error)

	require.NoError(t, db.Create(&model.Model{
		ModelName: "zz-remap-model",
		VendorID:  fallback.Id,
		Status:    1,
	}).Error)

	modelsJSON := `{"success":true,"message":"","data":[{"model_name":"zz-remap-model","description":"","status":1,"vendor_name":"Anthropic","provider_npm":"@ai-sdk/anthropic"}]}`
	vendorsJSON := `{"success":true,"message":"","data":[{"name":"Anthropic","status":1}]}`
	setupSyncUpstreamServer(t, modelsJSON, vendorsJSON)

	// 触发同步（overwrite 非空 → 拉取上游并走 step 4 补齐/重映射路径）
	body := `{"overwrite":[{"model_name":"zz-remap-model","fields":["description"]}]}`
	resp := runSyncUpstreamModels(t, body)

	assert.Equal(t, 1, resp.Data.FilledModels, "vendor remap should be counted as filled")

	var stored model.Model
	require.NoError(t, db.Where("model_name = ?", "zz-remap-model").First(&stored).Error)
	assert.Equal(t, anthropic.Id, stored.VendorID, "vendor_id should be remapped from fallback to real vendor")
}

// TestSyncUpstreamModelsKeepsFallbackWhenNoRealVendorDerived 验证无法推导真实
// 供应商时保持兜底，不做重映射。
func TestSyncUpstreamModelsKeepsFallbackWhenNoRealVendorDerived(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	fallback := &model.Vendor{Name: "OpenCode Go"}
	require.NoError(t, db.Create(fallback).Error)

	require.NoError(t, db.Create(&model.Model{
		ModelName: "zz-noderive-model",
		VendorID:  fallback.Id,
		Status:    1,
	}).Error)

	// provider_npm 为空且模型 ID 无匹配正则 → 上游 vendor_name 仍为兜底，不重映射
	modelsJSON := `{"success":true,"message":"","data":[{"model_name":"zz-noderive-model","description":"","status":1,"vendor_name":"OpenCode Go"}]}`
	vendorsJSON := `{"success":true,"message":"","data":[]}`
	setupSyncUpstreamServer(t, modelsJSON, vendorsJSON)

	body := `{"overwrite":[{"model_name":"zz-noderive-model","fields":["description"]}]}`
	runSyncUpstreamModels(t, body)

	var stored model.Model
	require.NoError(t, db.Where("model_name = ?", "zz-noderive-model").First(&stored).Error)
	assert.Equal(t, fallback.Id, stored.VendorID, "vendor_id should stay on fallback when no real vendor is derivable")
}

// TestSyncUpstreamModelsDoesNotRemapUserSetVendor 验证非兜底供应商的模型不受
// 重映射影响（用户手动指定过的供应商不动）。
func TestSyncUpstreamModelsDoesNotRemapUserSetVendor(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	fallback := &model.Vendor{Name: "OpenCode Go"}
	require.NoError(t, db.Create(fallback).Error)
	openAI := &model.Vendor{Name: "OpenAI"}
	require.NoError(t, db.Create(openAI).Error)

	// 模型已归属 OpenAI（非兜底），即便上游可推导出 Anthropic 也不改
	require.NoError(t, db.Create(&model.Model{
		ModelName: "zz-user-set-vendor",
		VendorID:  openAI.Id,
		Status:    1,
	}).Error)

	modelsJSON := `{"success":true,"message":"","data":[{"model_name":"zz-user-set-vendor","description":"","status":1,"vendor_name":"Anthropic","provider_npm":"@ai-sdk/anthropic"}]}`
	vendorsJSON := `{"success":true,"message":"","data":[]}`
	setupSyncUpstreamServer(t, modelsJSON, vendorsJSON)

	body := `{"overwrite":[{"model_name":"zz-user-set-vendor","fields":["description"]}]}`
	runSyncUpstreamModels(t, body)

	var stored model.Model
	require.NoError(t, db.Where("model_name = ?", "zz-user-set-vendor").First(&stored).Error)
	assert.Equal(t, openAI.Id, stored.VendorID, "non-fallback vendor must not be overwritten")
}
