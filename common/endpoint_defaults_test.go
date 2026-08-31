package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetDefaultEndpointInfoDefault 验证无配置覆盖时的默认行为：
// 9 种类型返回 ok=true，openai-video 返回 ok=false（保持既有语义）。
func TestGetDefaultEndpointInfoDefault(t *testing.T) {
	t.Cleanup(ResetEndpointDefinitions)

	info, ok := GetDefaultEndpointInfo(constant.EndpointTypeOpenAI)
	require.True(t, ok)
	assert.Equal(t, "/v1/chat/completions", info.Path)
	assert.Equal(t, "POST", info.Method)

	info, ok = GetDefaultEndpointInfo(constant.EndpointTypeAnthropic)
	require.True(t, ok)
	assert.Equal(t, "/v1/messages", info.Path)

	info, ok = GetDefaultEndpointInfo(constant.EndpointTypeGemini)
	require.True(t, ok)
	assert.Equal(t, "/v1beta/models/{model}:generateContent", info.Path)

	_, ok = GetDefaultEndpointInfo(constant.EndpointTypeOpenAIVideo)
	assert.False(t, ok, "openai-video has no default path, must stay ok=false")
}

// TestGetDefaultEndpointInfoOverride 验证配置覆盖优先于内置：
// path 为空的类型仍返回 ok=false；覆盖缺失的类型回退内置。
func TestGetDefaultEndpointInfoOverride(t *testing.T) {
	t.Cleanup(ResetEndpointDefinitions)

	override := `[
		{"type":"openai","display_name":"Custom Chat","path":"/custom/chat","method":"POST"},
		{"type":"openai-video","display_name":"Custom Video","path":"","method":"POST"}
	]`
	require.NoError(t, SetEndpointDefinitions(override))

	info, ok := GetDefaultEndpointInfo(constant.EndpointTypeOpenAI)
	require.True(t, ok)
	assert.Equal(t, "/custom/chat", info.Path)

	_, ok = GetDefaultEndpointInfo(constant.EndpointTypeOpenAIVideo)
	assert.False(t, ok, "override path empty must stay ok=false")

	// 覆盖缺失的类型（anthropic）回退内置
	info, ok = GetDefaultEndpointInfo(constant.EndpointTypeAnthropic)
	require.True(t, ok)
	assert.Equal(t, "/v1/messages", info.Path)
}

// TestGetEndpointDefinitions 验证返回 10 条完整定义，顺序与内置一致，且为副本。
func TestGetEndpointDefinitions(t *testing.T) {
	t.Cleanup(ResetEndpointDefinitions)

	defs := GetEndpointDefinitions()
	require.Len(t, defs, 10)
	assert.Equal(t, string(constant.EndpointTypeOpenAI), defs[0].Type)
	assert.Equal(t, "OpenAI Chat Completions", defs[0].DisplayName)
	assert.Equal(t, string(constant.EndpointTypeOpenAIVideo), defs[9].Type)
	assert.Equal(t, "", defs[9].Path)

	// npm 默认值：仅两个有据可查的 SDK 包
	assert.Equal(t, "@ai-sdk/anthropic", GetEndpointNPM(string(constant.EndpointTypeAnthropic)))
	assert.Equal(t, "@ai-sdk/openai", GetEndpointNPM(string(constant.EndpointTypeOpenAIResponse)))
	assert.Equal(t, "", GetEndpointNPM(string(constant.EndpointTypeOpenAI)))
	assert.Equal(t, "", GetEndpointNPM(string(constant.EndpointTypeGemini)))

	// 修改返回值不影响缓存
	defs[0].Path = "/mutated"
	info, _ := GetDefaultEndpointInfo(constant.EndpointTypeOpenAI)
	assert.Equal(t, "/v1/chat/completions", info.Path)
}

// TestValidateEndpointDefinitions 验证校验规则。
func TestValidateEndpointDefinitions(t *testing.T) {
	valid := EndpointDefinitionsDefaultJSON()
	require.NoError(t, ValidateEndpointDefinitions(valid))

	assert.Error(t, ValidateEndpointDefinitions(""), "empty must be rejected")
	assert.Error(t, ValidateEndpointDefinitions("{not-json"), "bad json must be rejected")

	// 未知类型
	bad := `[{"type":"unknown","display_name":"X","path":"/x","method":"POST"}]`
	assert.Error(t, ValidateEndpointDefinitions(bad))

	// 重复类型
	dup := `[{"type":"openai","display_name":"A","path":"/a","method":"POST"},{"type":"openai","display_name":"B","path":"/b","method":"POST"}]`
	assert.Error(t, ValidateEndpointDefinitions(dup))

	// display_name 为空
	noName := `[{"type":"openai","display_name":"","path":"/a","method":"POST"}]`
	assert.Error(t, ValidateEndpointDefinitions(noName))

	// 非 openai-video 的 path 为空
	noPath := `[{"type":"openai","display_name":"A","path":"","method":"POST"}]`
	assert.Error(t, ValidateEndpointDefinitions(noPath))

	// openai-video 的 path 允许为空
	videoEmptyPath := `[{"type":"openai-video","display_name":"Video","path":"","method":"POST"}]`
	require.NoError(t, ValidateEndpointDefinitions(videoEmptyPath))

	// 非法 method
	badMethod := `[{"type":"openai","display_name":"A","path":"/a","method":"PATCHX"}]`
	assert.Error(t, ValidateEndpointDefinitions(badMethod))

	// 合法 method 集合
	for _, m := range []string{"GET", "POST", "PUT", "DELETE", "PATCH"} {
		raw := `[{"type":"openai","display_name":"A","path":"/a","method":"` + m + `"}]`
		assert.NoError(t, ValidateEndpointDefinitions(raw), "method %s", m)
	}

	// npm 非字符串 → 解析失败拒绝
	badNpm := `[{"type":"openai","display_name":"A","path":"/a","method":"POST","npm":123}]`
	assert.Error(t, ValidateEndpointDefinitions(badNpm))
}

// TestSetEndpointDefinitionsFallback 验证损坏值回退内置默认并以错误返回。
func TestSetEndpointDefinitionsFallback(t *testing.T) {
	t.Cleanup(ResetEndpointDefinitions)

	require.NoError(t, SetEndpointDefinitions(`[{"type":"openai","display_name":"A","path":"/custom","method":"POST"}]`))
	info, ok := GetDefaultEndpointInfo(constant.EndpointTypeOpenAI)
	require.True(t, ok)
	assert.Equal(t, "/custom", info.Path)

	// 坏 JSON → 清空覆盖回退内置
	require.Error(t, SetEndpointDefinitions("{broken"))
	info, ok = GetDefaultEndpointInfo(constant.EndpointTypeOpenAI)
	require.True(t, ok)
	assert.Equal(t, "/v1/chat/completions", info.Path)

	// 校验失败（未知类型）→ 同样回退内置
	require.Error(t, SetEndpointDefinitions(`[{"type":"unknown","display_name":"A","path":"/x","method":"POST"}]`))
	info, ok = GetDefaultEndpointInfo(constant.EndpointTypeOpenAI)
	require.True(t, ok)
	assert.Equal(t, "/v1/chat/completions", info.Path)
}

// TestEndpointDefinitionsDefaultJSON 验证默认 JSON 可解析且含 10 条。
func TestEndpointDefinitionsDefaultJSON(t *testing.T) {
	raw := EndpointDefinitionsDefaultJSON()
	var defs []EndpointDefinition
	require.NoError(t, Unmarshal([]byte(raw), &defs))
	require.Len(t, defs, 10)
}
