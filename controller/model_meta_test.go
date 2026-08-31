package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateModelCapabilities 验证 capabilities 结构校验（双格式放行，无词表校验）。
func TestValidateModelCapabilities(t *testing.T) {
	// 空串合法
	assert.NoError(t, validateModelCapabilities(""))

	// 旧格式合法
	assert.NoError(t, validateModelCapabilities(`{"limits":{"context":1000000},"modalities":{"input":["text"],"output":["text"]}}`))
	// 新格式合法（单组/多组）
	assert.NoError(t, validateModelCapabilities(`{"groups":[{"name":"chat","limits":{"context":1000000}}]}`))
	assert.NoError(t, validateModelCapabilities(`{"groups":[{"name":"chat","limits":{"context":1000000}},{"name":"reasoning","modalities":{"input":["text"],"output":["text"]}}]}`))
	// 仅 name 组合法
	assert.NoError(t, validateModelCapabilities(`{"groups":[{"name":"chat"}]}`))

	// 坏 JSON
	require.Error(t, validateModelCapabilities(`{not-json`))
	// groups 非数组
	require.Error(t, validateModelCapabilities(`{"groups":{"name":"chat"}}`))
	// group 缺 name
	require.Error(t, validateModelCapabilities(`{"groups":[{"limits":{"context":1}}]}`))
	// group name 为空
	require.Error(t, validateModelCapabilities(`{"groups":[{"name":"","limits":{"context":1}}]}`))
	// limits 负数
	require.Error(t, validateModelCapabilities(`{"limits":{"context":-1}}`))
	// limits 非整数
	require.Error(t, validateModelCapabilities(`{"limits":{"context":1.5}}`))
	// limits 非数字
	require.Error(t, validateModelCapabilities(`{"limits":{"context":"big"}}`))
	// modalities 非字符串数组
	require.Error(t, validateModelCapabilities(`{"modalities":{"input":[1,2]}}`))
	// reasoning_options 非数组
	require.Error(t, validateModelCapabilities(`{"reasoning_options":{"type":"effort"}}`))

	// 未配置的键不校验（无词表校验）
	assert.NoError(t, validateModelCapabilities(`{"groups":[{"name":"chat","whatever":[1,2,3]}]}`))
}
