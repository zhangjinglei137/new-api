package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelValidateSettingsRejectsInvalidHTTPTransport(t *testing.T) {
	tests := []struct {
		name    string
		setting dto.ChannelSettings
		wantErr string
	}{
		{
			name:    "auto with shards is valid",
			setting: dto.ChannelSettings{HTTPProtocol: "auto", HTTP2ConnectionShards: 4},
		},
		{
			name:    "http1 with shards greater than one rejected",
			setting: dto.ChannelSettings{HTTPProtocol: "http1", HTTP2ConnectionShards: 2},
			wantErr: "http2_connection_shards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{}
			channel.SetSetting(tt.setting)
			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestChannelValidateSettingsRejectsInvalidModelProxyRules(t *testing.T) {
	tests := []struct {
		name    string
		rules   []dto.ModelProxyRule
		wantErr string
	}{
		{
			name:  "no rules is valid",
			rules: nil,
		},
		{
			name:  "exact model with empty proxy (explicit direct) is valid",
			rules: []dto.ModelProxyRule{{Models: []string{"gpt-5.6-luna"}, Proxy: ""}},
		},
		{
			name:  "valid regex rule is accepted",
			rules: []dto.ModelProxyRule{{Models: []string{"regex:^gpt-"}, Proxy: "socks5://127.0.0.1:1080"}},
		},
		{
			name:    "empty models rejected",
			rules:   []dto.ModelProxyRule{{Proxy: "socks5://127.0.0.1:1080"}},
			wantErr: "models must not be empty",
		},
		{
			name:    "empty model entry rejected",
			rules:   []dto.ModelProxyRule{{Models: []string{"  "}, Proxy: "socks5://127.0.0.1:1080"}},
			wantErr: "empty entries",
		},
		{
			name:    "empty regex pattern rejected",
			rules:   []dto.ModelProxyRule{{Models: []string{"regex:"}, Proxy: ""}},
			wantErr: "regex must not be empty",
		},
		{
			name:    "invalid regex rejected",
			rules:   []dto.ModelProxyRule{{Models: []string{"regex:("}, Proxy: ""}},
			wantErr: "invalid regex",
		},
		{
			name:    "invalid proxy rejected",
			rules:   []dto.ModelProxyRule{{Models: []string{"gpt-5"}, Proxy: "ftp://host"}},
			wantErr: "invalid model_proxy_rules[0].proxy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{}
			channel.SetOtherSettings(dto.ChannelOtherSettings{ModelProxyRules: tt.rules})
			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAdvancedCustomChannelRequiresModelListRouteOnlyWhenUpdateChecksEnabled(t *testing.T) {
	inferenceRoute := dto.AdvancedCustomRoute{
		IncomingPath: "/v1/chat/completions",
		UpstreamPath: "/v1/chat/completions",
		Converter:    "none",
	}

	tests := []struct {
		name          string
		checksEnabled bool
		routes        []dto.AdvancedCustomRoute
		wantErr       string
	}{
		{
			name:   "legacy channel without discovery route remains valid",
			routes: []dto.AdvancedCustomRoute{inferenceRoute},
		},
		{
			name:          "enabled checks require discovery route",
			checksEnabled: true,
			routes:        []dto.AdvancedCustomRoute{inferenceRoute},
			wantErr:       dto.AdvancedCustomModelListPath,
		},
		{
			name:          "enabled checks accept discovery route",
			checksEnabled: true,
			routes: []dto.AdvancedCustomRoute{
				inferenceRoute,
				{
					IncomingPath: dto.AdvancedCustomModelListPath,
					UpstreamPath: dto.AdvancedCustomModelListPath,
					Converter:    "none",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Type: constant.ChannelTypeAdvancedCustom}
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				UpstreamModelUpdateCheckEnabled: tt.checksEnabled,
				AdvancedCustom: &dto.AdvancedCustomConfig{
					Routes: tt.routes,
				},
			})

			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
