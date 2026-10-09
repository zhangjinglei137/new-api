package cline

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLByRelayFormat(t *testing.T) {
	cases := []struct {
		name      string
		format    types.RelayFormat
		relayMode int
		baseURL   string
		want      string
	}{
		{"chat completions", types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, "https://api.cline.bot/api/", "https://api.cline.bot/api/v1/chat/completions"},
		{"responses", types.RelayFormatOpenAI, relayconstant.RelayModeResponses, "https://api.cline.bot/api/", "https://api.cline.bot/api/v1/responses"},
		{"messages", types.RelayFormatClaude, relayconstant.RelayModeChatCompletions, "https://api.cline.bot/api/", "https://api.cline.bot/api/v1/messages"},
		{"messages falls back to default base", types.RelayFormatClaude, relayconstant.RelayModeChatCompletions, "", "https://api.cline.bot/api/v1/messages"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Adaptor{}
			info := &relaycommon.RelayInfo{
				RelayFormat: tc.format,
				RelayMode:   tc.relayMode,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl: tc.baseURL,
				},
			}
			got, err := a.GetRequestURL(info)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSetupRequestHeaderSetsBearer(t *testing.T) {
	a := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "sk-test"},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	header := http.Header{}
	require.NoError(t, a.SetupRequestHeader(c, &header, info))
	require.Equal(t, "Bearer sk-test", header.Get("Authorization"))
}

func TestGetChannelName(t *testing.T) {
	a := &Adaptor{}
	require.Equal(t, "Cline", a.GetChannelName())
}
