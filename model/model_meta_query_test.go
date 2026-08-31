package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetModelMetasByNamesMatchPriority 验证批量加载的匹配优先级：
// 精确 > 前缀 > 后缀 > 包含；同一规则内先匹配先赢。
func TestGetModelMetasByNamesMatchPriority(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Model{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM models")
	})

	require.NoError(t, DB.Create(&[]Model{
		{ModelName: "glm-5.3-flash", NameRule: NameRuleExact, Family: "exact-family"},
		{ModelName: "glm-", NameRule: NameRulePrefix, Family: "prefix-family"},
		{ModelName: "-flash", NameRule: NameRuleSuffix, Family: "suffix-family"},
		{ModelName: "5.3", NameRule: NameRuleContains, Family: "contains-family"},
	}).Error)

	metas := GetModelMetasByNames([]string{"glm-5.3-flash"})
	require.Contains(t, metas, "glm-5.3-flash")
	assert.Equal(t, "exact-family", metas["glm-5.3-flash"].Family, "exact must win over rules")

	// 无精确记录：前缀优先
	metas = GetModelMetasByNames([]string{"glm-anything"})
	require.Contains(t, metas, "glm-anything")
	assert.Equal(t, "prefix-family", metas["glm-anything"].Family)
}

// TestGetModelMetasByNamesEdgeCases 覆盖空输入与无匹配。
func TestGetModelMetasByNamesEdgeCases(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Model{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM models")
	})

	assert.Empty(t, GetModelMetasByNames(nil))
	assert.Empty(t, GetModelMetasByNames([]string{}))
	assert.Empty(t, GetModelMetasByNames([]string{"", "  "}))

	require.NoError(t, DB.Create(&Model{ModelName: "known", NameRule: NameRuleExact}).Error)
	metas := GetModelMetasByNames([]string{"known", "unknown-model"})
	require.Contains(t, metas, "known")
	assert.NotContains(t, metas, "unknown-model", "no match should be omitted")
}