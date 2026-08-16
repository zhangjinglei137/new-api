package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ensureNonEmptyVersion 保护版本兜底契约：构建时注入空版本字符串
// （例如 VERSION 文件为空时 $(cat VERSION) 输出为空）不得让
// /api/status 等接口返回空版本。
func TestEnsureNonEmptyVersion(t *testing.T) {
	require.Equal(t, "v0.0.0", ensureNonEmptyVersion(""))
	require.Equal(t, "v0.0.0", ensureNonEmptyVersion("v0.0.0"))
	require.Equal(t, "v1.2.3", ensureNonEmptyVersion("v1.2.3"))
	require.Equal(t, "main-20260817-a1b2c3d", ensureNonEmptyVersion("main-20260817-a1b2c3d"))
}
