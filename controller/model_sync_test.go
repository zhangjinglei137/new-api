package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenCodeGoVendorForModel(t *testing.T) {
	cases := []struct {
		modelID string
		vendor  string
	}{
		{modelID: "grok-4.5", vendor: "xAI"},
		{modelID: "gpt-5.6-luna", vendor: "OpenAI"},
		{modelID: "glm-5.3", vendor: "Z.AI"},
		{modelID: "kimi-k3", vendor: "Moonshot AI"},
		{modelID: "mimo-v2.5-pro", vendor: "Xiaomi"},
		{modelID: "minimax-m3", vendor: "MiniMax (minimax.io)"},
		{modelID: "qwen3.8-max", vendor: "Alibaba"},
		{modelID: "deepseek-v4-pro", vendor: "DeepSeek"},
		{modelID: "deepseek-v4-flash-free", vendor: "DeepSeek"},
		{modelID: "hy3", vendor: "Tencent"},
		{modelID: "hy3-free", vendor: "Tencent"},
		{modelID: "unknown-model", vendor: "OpenCode Go"},
		{modelID: "", vendor: "OpenCode Go"},
	}
	for _, tc := range cases {
		t.Run(tc.modelID, func(t *testing.T) {
			assert.Equal(t, tc.vendor, openCodeGoVendorForModel(tc.modelID))
		})
	}
}
