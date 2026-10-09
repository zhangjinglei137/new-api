package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestClineChannelType2APIType(t *testing.T) {
	apiType, ok := ChannelType2APIType(constant.ChannelTypeCline)
	require.True(t, ok)
	require.Equal(t, constant.APITypeCline, apiType)
}
