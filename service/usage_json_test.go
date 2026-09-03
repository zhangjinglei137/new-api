package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONInt64Clamped(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		keys []string
		want int64
	}{
		{name: "normal int", m: map[string]any{"a": float64(42)}, keys: []string{"a"}, want: 42},
		{name: "negative int", m: map[string]any{"a": float64(-7)}, keys: []string{"a"}, want: -7},
		{name: "zero", m: map[string]any{"a": float64(0)}, keys: []string{"a"}, want: 0},
		{name: "numeric string", m: map[string]any{"a": "1786700000000"}, keys: []string{"a"}, want: 1786700000000},
		{name: "float string", m: map[string]any{"a": "42.9"}, keys: []string{"a"}, want: 42},
		{name: "fraction truncates", m: map[string]any{"a": float64(42.9)}, keys: []string{"a"}, want: 42},
		{name: "huge positive clamps to max", m: map[string]any{"a": float64(1e300)}, keys: []string{"a"}, want: math.MaxInt64},
		{name: "huge negative clamps to min", m: map[string]any{"a": float64(-1e300)}, keys: []string{"a"}, want: math.MinInt64},
		{name: "min int64 boundary", m: map[string]any{"a": float64(math.MinInt64)}, keys: []string{"a"}, want: math.MinInt64},
		{name: "missing key returns zero", m: map[string]any{}, keys: []string{"a"}, want: 0},
		{name: "wrong type returns zero", m: map[string]any{"a": []any{1}}, keys: []string{"a"}, want: 0},
		{name: "nan treated as missing", m: map[string]any{"a": math.NaN()}, keys: []string{"a"}, want: 0},
		{name: "alias fallback", m: map[string]any{"b": float64(5)}, keys: []string{"a", "b"}, want: 5},
		{name: "unparseable string falls through keys", m: map[string]any{"a": "not-a-number", "b": float64(8)}, keys: []string{"a", "b"}, want: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, jsonInt64Clamped(tt.m, tt.keys...))
		})
	}
}

func TestJSONInt64ClampedRoundedInt64Overflow(t *testing.T) {
	// float64(math.MaxInt64) 精确表示为 2^63，int64 无法容纳，
	// 必须 clamp 而不是溢出成负数。
	require.Equal(t, int64(math.MaxInt64), jsonInt64Clamped(map[string]any{"a": float64(math.MaxInt64)}, "a"))
	require.Equal(t, int64(math.MinInt64), jsonInt64Clamped(map[string]any{"a": float64(math.MinInt64)}, "a"))
}

func TestJSONFloatOKFiltersNaN(t *testing.T) {
	// NaN（float64 或 "NaN" 字符串）视为缺失，不产生 NaN 展示值。
	f, ok := jsonFloatOK(map[string]any{"a": math.NaN()}, "a")
	assert.False(t, ok)
	assert.Equal(t, float64(0), f)

	f, ok = jsonFloatOK(map[string]any{"a": "NaN"}, "a")
	assert.False(t, ok)

	f, ok = jsonFloatOK(map[string]any{"a": float64(1.5)}, "a")
	assert.True(t, ok)
	assert.Equal(t, 1.5, f)

	f, ok = jsonFloatOK(map[string]any{"a": "2.5"}, "a")
	assert.True(t, ok)
	assert.Equal(t, 2.5, f)

	assert.Equal(t, float64(3.25), jsonFloat(map[string]any{"a": "3.25"}, "a"))
	assert.Equal(t, float64(0), jsonFloat(map[string]any{"a": "NaN"}, "a"))
}
