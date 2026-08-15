package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{modelID: "nemotron-3-ultra-free", vendor: "Nvidia"},
		{modelID: "nemotron-3.5-lightning-free", vendor: "Nvidia"},
		{modelID: "laguna-s-2.1-free", vendor: "Poolside"},
		{modelID: "unknown-model", vendor: "OpenCode Go"},
		{modelID: "", vendor: "OpenCode Go"},
	}
	for _, tc := range cases {
		t.Run(tc.modelID, func(t *testing.T) {
			assert.Equal(t, tc.vendor, openCodeGoVendorForModel(tc.modelID))
		})
	}
}

func TestOpenCodeGoEndpointForProvider(t *testing.T) {
	cases := []struct {
		providerNpm string
		endpoint    string
	}{
		{providerNpm: "@ai-sdk/openai", endpoint: "openai"},
		{providerNpm: "@ai-sdk/anthropic", endpoint: "anthropic"},
		{providerNpm: "@ai-sdk/google", endpoint: "gemini"},
		{providerNpm: "", endpoint: "openai"},
		{providerNpm: "unknown-pkg", endpoint: "openai"},
	}
	for _, tc := range cases {
		t.Run(tc.providerNpm, func(t *testing.T) {
			assert.Equal(t, tc.endpoint, openCodeGoEndpointForProvider(tc.providerNpm))
		})
	}
}

func TestEndpointsJSON(t *testing.T) {
	cases := []struct {
		providerNpm string
		want        string
	}{
		{providerNpm: "@ai-sdk/openai", want: `["openai"]`},
		{providerNpm: "@ai-sdk/anthropic", want: `["anthropic"]`},
		{providerNpm: "@ai-sdk/google", want: `["gemini"]`},
		{providerNpm: "", want: `["openai"]`},
	}
	for _, tc := range cases {
		t.Run(tc.providerNpm, func(t *testing.T) {
			got := endpointsJSON(tc.providerNpm)
			require.NotNil(t, got)
			assert.Equal(t, tc.want, string(got))
		})
	}
}
