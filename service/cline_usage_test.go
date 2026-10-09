package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseClinePlanUsage(t *testing.T) {
	body := []byte(`{"data":{"limits":[{"type":"five_hour","percentUsed":0,"resetsAt":"2026-10-09T08:44:01Z"},{"type":"weekly","percentUsed":42.5,"resetsAt":"2026-10-16T03:44:01Z"},{"type":"monthly","percentUsed":100,"resetsAt":"2026-11-08T03:44:01Z"}]},"success":true}`)
	info, err := parseClinePlanUsage(body)
	require.NoError(t, err)
	require.Len(t, info.Limits, 3)
	require.Equal(t, "five_hour", info.Limits[0].Type)
	require.Equal(t, 42.5, info.Limits[1].PercentUsed)
	require.Equal(t, "2026-10-16T03:44:01Z", info.Limits[1].ResetsAt)
}

func TestParseClinePlanUsageEmptyLimits(t *testing.T) {
	info, err := parseClinePlanUsage([]byte(`{"data":{"limits":[]}}`))
	require.NoError(t, err)
	require.Empty(t, info.Limits)
}

func TestParseClinePlanUsageInvalidJSON(t *testing.T) {
	_, err := parseClinePlanUsage([]byte(`not-json`))
	require.Error(t, err)
}
