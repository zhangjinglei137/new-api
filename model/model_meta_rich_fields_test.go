package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModelMetaRichFieldsRoundTrip 覆盖富模型元数据字段的 Create→Read→Update
// 往返，防止 Model.Update() 的 Select 白名单漏列导致新字段无法持久化。
func TestModelMetaRichFieldsRoundTrip(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Model{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM models")
	})

	boolPtr := func(b bool) *bool { return &b }
	capabilities := `{"modalities":{"input":["text","image"],"output":["text"]},"limits":{"context":1000000,"output":131072},"reasoning_options":[{"type":"effort","values":["low","high"]}]}`

	m := &Model{
		ModelName:           "glm-5.3-flash",
		DisplayName:         "GLM-5.3-Flash",
		Family:              "glm",
		ProviderNpm:         "@ai-sdk/anthropic",
		ReleaseDate:         "2026-08-26",
		LastUpdated:         "2026-08-26",
		OpenWeights:         boolPtr(false),
		CapAttachment:       boolPtr(true),
		CapReasoning:        boolPtr(true),
		CapToolCall:         boolPtr(true),
		CapStructuredOutput: boolPtr(false),
		CapTemperature:      boolPtr(false),
		Capabilities:        capabilities,
	}
	require.NoError(t, m.Insert())

	// Read back after insert
	var read Model
	require.NoError(t, DB.First(&read, m.Id).Error)
	assert.Equal(t, "GLM-5.3-Flash", read.DisplayName)
	assert.Equal(t, "glm", read.Family)
	assert.Equal(t, "@ai-sdk/anthropic", read.ProviderNpm)
	assert.Equal(t, "2026-08-26", read.ReleaseDate)
	assert.Equal(t, "2026-08-26", read.LastUpdated)
	require.NotNil(t, read.OpenWeights)
	assert.False(t, *read.OpenWeights)
	require.NotNil(t, read.CapReasoning)
	assert.True(t, *read.CapReasoning)
	assert.Equal(t, capabilities, read.Capabilities)

	// Update and verify all rich fields persist (guards the Update() Select whitelist)
	updated := &Model{
		Id:                  m.Id,
		ModelName:           "glm-5.3-flash",
		DisplayName:         "GLM-5.3-Flash-v2",
		Family:              "glm",
		ProviderNpm:         "@ai-sdk/openai-compatible",
		ReleaseDate:         "2026-08-27",
		LastUpdated:         "2026-08-27",
		OpenWeights:         boolPtr(true),
		CapAttachment:       boolPtr(false),
		CapReasoning:        boolPtr(true),
		CapToolCall:         boolPtr(false),
		CapStructuredOutput: boolPtr(true),
		CapTemperature:      boolPtr(true),
		Capabilities:        `{"modalities":{"input":["text"],"output":["text"]}}`,
	}
	require.NoError(t, updated.Update())

	var reRead Model
	require.NoError(t, DB.First(&reRead, m.Id).Error)
	assert.Equal(t, "GLM-5.3-Flash-v2", reRead.DisplayName)
	assert.Equal(t, "2026-08-27", reRead.ReleaseDate)
	require.NotNil(t, reRead.OpenWeights)
	assert.True(t, *reRead.OpenWeights)
	require.NotNil(t, reRead.CapAttachment)
	assert.False(t, *reRead.CapAttachment)
	require.NotNil(t, reRead.CapStructuredOutput)
	assert.True(t, *reRead.CapStructuredOutput)
	assert.Equal(t, `{"modalities":{"input":["text"],"output":["text"]}}`, reRead.Capabilities)
}

// TestModelMetaBoolTriState 覆盖 *bool 三态语义：未设置时（nil）必须保持 NULL，
// 不被数据库默认值污染成 false/true。
func TestModelMetaBoolTriState(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Model{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM models")
	})

	m := &Model{ModelName: "tri-state-model"}
	require.NoError(t, m.Insert())

	var read Model
	require.NoError(t, DB.First(&read, m.Id).Error)
	assert.Nil(t, read.OpenWeights)
	assert.Nil(t, read.CapReasoning)
	assert.Nil(t, read.CapAttachment)
	assert.Nil(t, read.CapToolCall)
	assert.Nil(t, read.CapStructuredOutput)
	assert.Nil(t, read.CapTemperature)
}
