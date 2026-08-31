package model

import (
	"strings"
)

// GetModelMetasByNames 一次性批量加载指定模型名（以及可用于通配匹配的规则
// 模型）的元数据，避免 /v1/models 富元数据场景下的 N+1 查询。
// 匹配优先级与 updatePricing 一致：精确 > 前缀 > 后缀 > 包含。
func GetModelMetasByNames(names []string) map[string]*Model {
	result := make(map[string]*Model, len(names))
	if len(names) == 0 {
		return result
	}
	quotedNames := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		quotedNames = append(quotedNames, n)
	}
	if len(quotedNames) == 0 {
		return result
	}

	var metas []Model
	// 一次性取回精确匹配 + 全部规则模型（规则模型数量通常很小）
	if err := DB.Where("model_name IN ? OR name_rule <> ?", quotedNames, NameRuleExact).
		Find(&metas).Error; err != nil {
		return result
	}

	exact := make(map[string]*Model)
	prefix := make([]*Model, 0)
	suffix := make([]*Model, 0)
	contains := make([]*Model, 0)
	for i := range metas {
		m := &metas[i]
		switch m.NameRule {
		case NameRuleExact:
			exact[m.ModelName] = m
		case NameRulePrefix:
			prefix = append(prefix, m)
		case NameRuleSuffix:
			suffix = append(suffix, m)
		case NameRuleContains:
			contains = append(contains, m)
		}
	}

	for _, name := range quotedNames {
		if m, ok := exact[name]; ok {
			result[name] = m
			continue
		}
		var matched *Model
		for _, m := range prefix {
			if strings.HasPrefix(name, m.ModelName) {
				matched = m
				break
			}
		}
		if matched == nil {
			for _, m := range suffix {
				if strings.HasSuffix(name, m.ModelName) {
					matched = m
					break
				}
			}
		}
		if matched == nil {
			for _, m := range contains {
				if strings.Contains(name, m.ModelName) {
					matched = m
					break
				}
			}
		}
		if matched != nil {
			result[name] = matched
		}
	}
	return result
}
