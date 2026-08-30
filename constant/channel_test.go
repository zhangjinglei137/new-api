package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveSpecialPlan(t *testing.T) {
	tests := []struct {
		name            string
		channelType     int
		baseURL         string
		endpointProfile string
		wantHit         bool
		wantMagicKey    string
		wantClaude      string
		wantOpenAI      string
	}{
		{
			name:            "volcengine coding profile",
			channelType:     ChannelTypeVolcEngine,
			endpointProfile: "coding",
			wantHit:         true,
			wantMagicKey:    "doubao-coding-plan",
			wantClaude:      "https://ark.cn-beijing.volces.com/api/coding",
			wantOpenAI:      "https://ark.cn-beijing.volces.com/api/coding/v3",
		},
		{
			name:            "zhipu_v4 coding profile",
			channelType:     ChannelTypeZhipu_v4,
			endpointProfile: "coding",
			wantHit:         true,
			wantMagicKey:    "glm-coding-plan",
			wantClaude:      "https://open.bigmodel.cn/api/anthropic",
			wantOpenAI:      "https://open.bigmodel.cn/api/coding/paas/v4",
		},
		{
			name:            "zhipu_v4 coding-intl profile",
			channelType:     ChannelTypeZhipu_v4,
			endpointProfile: "coding-intl",
			wantHit:         true,
			wantMagicKey:    "glm-coding-plan-international",
			wantClaude:      "https://api.z.ai/api/anthropic",
			wantOpenAI:      "https://api.z.ai/api/coding/paas/v4",
		},
		{
			name:            "moonshot coding profile",
			channelType:     ChannelTypeMoonshot,
			endpointProfile: "coding",
			wantHit:         true,
			wantMagicKey:    "kimi-coding-plan",
			wantClaude:      "https://api.kimi.com/coding",
			wantOpenAI:      "https://api.kimi.com/coding/v1",
		},
		{
			name:         "legacy base_url magic key without profile",
			channelType:  ChannelTypeVolcEngine,
			baseURL:      "doubao-coding-plan",
			wantHit:      true,
			wantMagicKey: "doubao-coding-plan",
			wantClaude:   "https://ark.cn-beijing.volces.com/api/coding",
			wantOpenAI:   "https://ark.cn-beijing.volces.com/api/coding/v3",
		},
		{
			name:            "profile wins over base_url magic key",
			channelType:     ChannelTypeVolcEngine,
			baseURL:         "kimi-coding-plan",
			endpointProfile: "coding",
			wantHit:         true,
			wantMagicKey:    "doubao-coding-plan",
			wantClaude:      "https://ark.cn-beijing.volces.com/api/coding",
			wantOpenAI:      "https://ark.cn-beijing.volces.com/api/coding/v3",
		},
		{
			name:            "unknown profile falls back to base_url magic key",
			channelType:     ChannelTypeVolcEngine,
			baseURL:         "doubao-coding-plan",
			endpointProfile: "coding-unknown",
			wantHit:         true,
			wantMagicKey:    "doubao-coding-plan",
			wantClaude:      "https://ark.cn-beijing.volces.com/api/coding",
			wantOpenAI:      "https://ark.cn-beijing.volces.com/api/coding/v3",
		},
		{
			name:            "unknown profile with real base_url does not hit",
			channelType:     ChannelTypeVolcEngine,
			baseURL:         "https://ark.cn-beijing.volces.com",
			endpointProfile: "coding-unknown",
			wantHit:         false,
		},
		{
			name:            "profile unsupported for channel type does not hit",
			channelType:     ChannelTypeOpenAI,
			baseURL:         "https://api.openai.com",
			endpointProfile: "coding",
			wantHit:         false,
		},
		{
			name:        "real base_url without profile does not hit",
			channelType: ChannelTypeVolcEngine,
			baseURL:     "https://ark.cn-beijing.volces.com",
			wantHit:     false,
		},
		{
			name:        "empty everything does not hit",
			channelType: ChannelTypeVolcEngine,
			wantHit:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, magicKey, hit := ResolveSpecialPlan(tt.channelType, tt.baseURL, tt.endpointProfile)
			require.Equal(t, tt.wantHit, hit)
			if !tt.wantHit {
				return
			}
			assert.Equal(t, tt.wantMagicKey, magicKey)
			assert.Equal(t, tt.wantClaude, plan.ClaudeBaseURL)
			assert.Equal(t, tt.wantOpenAI, plan.OpenAIBaseURL)
		})
	}
}
