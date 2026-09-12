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
// normalizeLocale 会把空 locale 归一到 "zh"，故响应 /api/i18n/zh/newapi/ 路径。
func setupSyncUpstreamServer(t *testing.T, modelsJSON, vendorsJSON string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/i18n/zh/newapi/models.json":
			_, _ = w.Write([]byte(modelsJSON))
		case "/api/i18n/zh/newapi/vendors.json":
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

type syncPreviewResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Candidates []metadataSyncCandidate `json:"candidates"`
		Source     metadataSyncSource      `json:"source"`
	} `json:"data"`
}

func runSyncUpstreamPreview(t *testing.T, query string) syncPreviewResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/models/sync_upstream/preview?"+query, nil)
	SyncUpstreamPreview(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var resp syncPreviewResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	return resp
}

type syncApplyResponse struct {
	Success bool `json:"success"`
	Data    struct {
		CreatedModels  []string                          `json:"created_models"`
		UpdatedModels  []model.MetadataSyncSelection     `json:"updated_models"`
		CreatedVendors []string                          `json:"created_vendors"`
	} `json:"data"`
}

func runSyncUpstreamApply(t *testing.T, body string) (*httptest.ResponseRecorder, syncApplyResponse) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/models/sync_upstream", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	SyncUpstreamModels(ctx)
	var resp syncApplyResponse
	if recorder.Code == http.StatusOK {
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	}
	return recorder, resp
}

// findCandidate 返回指定模型的预览候选。
func findCandidate(t *testing.T, resp syncPreviewResponse, modelName string) metadataSyncCandidate {
	t.Helper()
	for _, candidate := range resp.Data.Candidates {
		if candidate.ModelName == modelName {
			return candidate
		}
	}
	require.FailNowf(t, "candidate not found", "model %s not in preview", modelName)
	return metadataSyncCandidate{}
}

func TestSyncPreviewExposesCandidatesAndVersion(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	// 渠道引用本地元数据不存在的模型 → GetMissingModels 返回它（site 范围）
	require.NoError(t, db.Create(&model.Channel{Name: "zz-sync-ch", Type: 1, Status: common.ChannelStatusEnabled}).Error)
	var ch model.Channel
	require.NoError(t, db.Where("name = ?", "zz-sync-ch").First(&ch).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "zz-sync-create-model",
		ChannelId: ch.Id,
		Enabled:   true,
	}).Error)
	// 已有模型（上游有差异 → update）
	require.NoError(t, db.Create(&model.Model{
		ModelName:    "zz-sync-update-model",
		Description:  "old desc",
		Status:       1,
		SyncOfficial: 1,
	}).Error)
	// 本地存在但上游没有 → missing_upstream
	require.NoError(t, db.Create(&model.Model{
		ModelName:    "zz-sync-local-only",
		Description:  "local only",
		Status:       1,
		SyncOfficial: 1,
	}).Error)

	modelsJSON := `{"success":true,"message":"","data":[
		{"model_name":"zz-sync-create-model","description":"upstream desc","status":1,"vendor_name":"OpenAI"},
		{"model_name":"zz-sync-update-model","description":"new desc","status":1,"vendor_name":"OpenAI"}
	]}`
	vendorsJSON := `{"success":true,"message":"","data":[{"name":"OpenAI","status":1}]}`
	setupSyncUpstreamServer(t, modelsJSON, vendorsJSON)

	resp := runSyncUpstreamPreview(t, "locale=zh&source=official")

	require.NotEmpty(t, resp.Data.Source.Version, "preview must return a version fingerprint")
	require.NotEmpty(t, resp.Data.Source.ModelsURL)
	create := findCandidate(t, resp, "zz-sync-create-model")
	assert.Equal(t, "create", create.Kind)
	assert.Equal(t, "site", create.Scope)
	assert.NotEmpty(t, create.RecordVersion)
	update := findCandidate(t, resp, "zz-sync-update-model")
	assert.Equal(t, "update", update.Kind)
	missingUp := findCandidate(t, resp, "zz-sync-local-only")
	assert.Equal(t, "missing_upstream", missingUp.Kind)
}

func TestSyncUpstreamApplyCreatesMissingModelWithRichFields(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Name: "zz-sync-ch", Type: 1, Status: common.ChannelStatusEnabled}).Error)
	var ch model.Channel
	require.NoError(t, db.Where("name = ?", "zz-sync-ch").First(&ch).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "zz-sync-create-model",
		ChannelId: ch.Id,
		Enabled:   true,
	}).Error)

	modelsJSON := `{"success":true,"message":"","data":[{"model_name":"zz-sync-create-model","description":"upstream desc","status":1,"vendor_name":"OpenAI","endpoints":{"openai":{"path":"/v1/chat/completions","method":"POST"}}}]}`
	vendorsJSON := `{"success":true,"message":"","data":[{"name":"OpenAI","status":1}]}`
	setupSyncUpstreamServer(t, modelsJSON, vendorsJSON)

	preview := runSyncUpstreamPreview(t, "locale=zh&source=official")
	create := findCandidate(t, preview, "zz-sync-create-model")
	body, err := common.Marshal(map[string]any{
		"locale":        "zh",
		"source":        "official",
		"source_version": preview.Data.Source.Version,
		"selections": []model.MetadataSyncSelection{{
			ModelName:     create.ModelName,
			RecordVersion: create.RecordVersion,
			Create:        true,
			Fields:        []string{},
		}},
	})
	require.NoError(t, err)
	_, resp := runSyncUpstreamApply(t, string(body))

	assert.Equal(t, []string{"zz-sync-create-model"}, resp.Data.CreatedModels)
	var created model.Model
	require.NoError(t, db.Where("model_name = ?", "zz-sync-create-model").First(&created).Error)
	assert.Equal(t, 1, created.SyncOfficial)
	assert.Equal(t, "upstream desc", created.Description)
}

func TestSyncUpstreamApplyUpdatesSelectedFields(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Model{
		ModelName:    "zz-sync-update-model",
		Description:  "old desc",
		Status:       1,
		SyncOfficial: 1,
	}).Error)

	modelsJSON := `{"success":true,"message":"","data":[{"model_name":"zz-sync-update-model","description":"new desc","status":1,"vendor_name":"OpenAI"}]}`
	vendorsJSON := `{"success":true,"message":"","data":[{"name":"OpenAI","status":1}]}`
	setupSyncUpstreamServer(t, modelsJSON, vendorsJSON)

	preview := runSyncUpstreamPreview(t, "locale=zh&source=official")
	update := findCandidate(t, preview, "zz-sync-update-model")
	require.Equal(t, "update", update.Kind)

	body, err := common.Marshal(map[string]any{
		"locale":         "zh",
		"source":         "official",
		"source_version": preview.Data.Source.Version,
		"selections": []model.MetadataSyncSelection{{
			ModelName:     update.ModelName,
			RecordVersion: update.RecordVersion,
			Create:        false,
			Fields:        []string{"description"},
		}},
	})
	require.NoError(t, err)
	_, resp := runSyncUpstreamApply(t, string(body))

	require.Len(t, resp.Data.UpdatedModels, 1)
	assert.Equal(t, "zz-sync-update-model", resp.Data.UpdatedModels[0].ModelName)
	var stored model.Model
	require.NoError(t, db.Where("model_name = ?", "zz-sync-update-model").First(&stored).Error)
	assert.Equal(t, "new desc", stored.Description)
	assert.Equal(t, 1, stored.SyncOfficial)
}

func TestSyncUpstreamApplyRejectsVersionMismatch(t *testing.T) {
	setupModelListControllerTestDB(t)
	modelsJSON := `{"success":true,"message":"","data":[{"model_name":"zz-sync-mismatch","description":"d","status":1,"vendor_name":"OpenAI"}]}`
	vendorsJSON := `{"success":true,"message":"","data":[{"name":"OpenAI","status":1}]}`
	setupSyncUpstreamServer(t, modelsJSON, vendorsJSON)

	body := `{"locale":"zh","source":"official","source_version":"stale-version","selections":[{"model_name":"zz-sync-mismatch","record_version":"x","create":true,"fields":[]}]}`
	recorder, _ := runSyncUpstreamApply(t, body)
	assert.Equal(t, http.StatusConflict, recorder.Code)
}

func TestSyncUpstreamApplyRejectsSyncDisabledModel(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	skipModel := &model.Model{
		ModelName:   "zz-sync-skip-model",
		Description: "keep desc",
		Status:      1,
	}
	require.NoError(t, db.Create(skipModel).Error)
	require.NoError(t, db.Model(&model.Model{}).Where("id = ?", skipModel.Id).Update("sync_official", 0).Error)

	modelsJSON := `{"success":true,"message":"","data":[{"model_name":"zz-sync-skip-model","description":"should not apply","status":1,"vendor_name":"OpenAI"}]}`
	vendorsJSON := `{"success":true,"message":"","data":[{"name":"OpenAI","status":1}]}`
	setupSyncUpstreamServer(t, modelsJSON, vendorsJSON)

	preview := runSyncUpstreamPreview(t, "locale=zh&source=official")
	blocked := findCandidate(t, preview, "zz-sync-skip-model")
	assert.Equal(t, "blocked", blocked.Kind)

	body, err := common.Marshal(map[string]any{
		"locale":         "zh",
		"source":         "official",
		"source_version": preview.Data.Source.Version,
		"selections": []model.MetadataSyncSelection{{
			ModelName:     blocked.ModelName,
			RecordVersion: blocked.RecordVersion,
			Create:        false,
			Fields:        []string{"description"},
		}},
	})
	require.NoError(t, err)
	recorder, _ := runSyncUpstreamApply(t, string(body))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	var stored model.Model
	require.NoError(t, db.Where("model_name = ?", "zz-sync-skip-model").First(&stored).Error)
	assert.Equal(t, "keep desc", stored.Description)
}

func TestSyncUpstreamApplyRequiresSelectionsAndVersion(t *testing.T) {
	setupModelListControllerTestDB(t)
	recorder, _ := runSyncUpstreamApply(t, `{}`)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}
