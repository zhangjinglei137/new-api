package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelOtherSettingsResolveProxy(t *testing.T) {
	tests := []struct {
		name         string
		rules        []ModelProxyRule
		model        string
		defaultProxy string
		want         string
	}{
		{
			name:         "no rules falls back to channel proxy",
			model:        "gpt-5.6-luna",
			defaultProxy: "socks5://127.0.0.1:1080",
			want:         "socks5://127.0.0.1:1080",
		},
		{
			name:         "exact model match wins",
			rules:        []ModelProxyRule{{Models: []string{"gpt-5.6-luna"}, Proxy: "socks5://127.0.0.1:1081"}},
			model:        "gpt-5.6-luna",
			defaultProxy: "socks5://127.0.0.1:1080",
			want:         "socks5://127.0.0.1:1081",
		},
		{
			name:         "regex match hits",
			rules:        []ModelProxyRule{{Models: []string{"regex:^gpt-"}, Proxy: "http://127.0.0.1:8080"}},
			model:        "gpt-5.6-luna",
			defaultProxy: "socks5://127.0.0.1:1080",
			want:         "http://127.0.0.1:8080",
		},
		{
			name:         "exact model priority over regex even when regex comes first",
			rules: []ModelProxyRule{
				{Models: []string{"regex:^gpt-"}, Proxy: "http://127.0.0.1:8080"},
				{Models: []string{"gpt-5.6-luna"}, Proxy: "socks5://127.0.0.1:1081"},
			},
			model:        "gpt-5.6-luna",
			defaultProxy: "socks5://127.0.0.1:1080",
			want:         "socks5://127.0.0.1:1081",
		},
		{
			name: "first matching rule in array order wins within same kind",
			rules: []ModelProxyRule{
				{Models: []string{"gpt-5.6-luna"}, Proxy: "socks5://127.0.0.1:1081"},
				{Models: []string{"gpt-5.6-luna"}, Proxy: "socks5://127.0.0.1:1082"},
			},
			model:        "gpt-5.6-luna",
			defaultProxy: "socks5://127.0.0.1:1080",
			want:         "socks5://127.0.0.1:1081",
		},
		{
			name:         "no rule matches falls back to channel proxy",
			rules:        []ModelProxyRule{{Models: []string{"regex:^claude-"}, Proxy: "http://127.0.0.1:8080"}},
			model:        "gpt-5.6-luna",
			defaultProxy: "socks5://127.0.0.1:1080",
			want:         "socks5://127.0.0.1:1080",
		},
		{
			name:         "matched rule with empty proxy means explicit direct connection",
			rules:        []ModelProxyRule{{Models: []string{"gpt-5.6-luna"}, Proxy: ""}},
			model:        "gpt-5.6-luna",
			defaultProxy: "socks5://127.0.0.1:1080",
			want:         "",
		},
		{
			name:         "empty settings object falls back",
			rules:        nil,
			model:        "gpt-5.6-luna",
			defaultProxy: "socks5://127.0.0.1:1080",
			want:         "socks5://127.0.0.1:1080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &ChannelOtherSettings{ModelProxyRules: tt.rules}
			got := settings.ResolveProxy(tt.model, tt.defaultProxy)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestChannelOtherSettingsResolveProxyNilReceiver(t *testing.T) {
	var settings *ChannelOtherSettings
	assert.Equal(t, "socks5://127.0.0.1:1080", settings.ResolveProxy("gpt-5.6-luna", "socks5://127.0.0.1:1080"))
}
