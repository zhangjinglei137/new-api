package model

// GetMissingModels returns model names that are referenced in the system
func GetMissingModels() ([]string, error) {
	// 1. 获取所有已启用模型（去重），只统计绑定到仍存在渠道的能力：
	//    渠道删除后若其 abilities 未清理（历史残留/漏删路径），孤儿能力
	//    会把已软删或未配置的模型误报为"缺失模型"，故在源头过滤孤儿
	//    （channel_id 子查询：渠道被删后 id 不在 channels 表，其能力即
	//    视为孤儿；软删渠道同样被排除）。
	var models []string
	if err := DB.Table("abilities").
		Where("enabled = ?", true).
		Where("channel_id IN (?)", DB.Model(&Channel{}).Select("id")).
		Distinct("model").
		Pluck("model", &models).Error; err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return []string{}, nil
	}

	// 2. 查询已有的元数据模型名
	var existing []string
	if err := DB.Model(&Model{}).Where("model_name IN ?", models).Pluck("model_name", &existing).Error; err != nil {
		return nil, err
	}

	existingSet := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		existingSet[e] = struct{}{}
	}

	// 3. 收集缺失模型
	var missing []string
	for _, name := range models {
		if _, ok := existingSet[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing, nil
}
