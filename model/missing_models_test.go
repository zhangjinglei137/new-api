package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMissingModelsTest(t *testing.T) {
	t.Helper()
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}, &Model{}, &Vendor{}))
	for _, table := range []string{"abilities", "channels", "models", "vendors"} {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []string{"abilities", "channels", "models", "vendors"} {
			require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
		}
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})
}

// TestGetMissingModelsEmpty 无任何启用能力时返回空列表。
func TestGetMissingModelsEmpty(t *testing.T) {
	setupMissingModelsTest(t)
	require.NoError(t, DB.Create(&Channel{Name: "zz-empty-ch", Type: 1, Status: common.ChannelStatusEnabled}).Error)

	missing, err := GetMissingModels()
	require.NoError(t, err)
	assert.Empty(t, missing)
}

// TestGetMissingModelsSoftDeletedModelIsMissing 已软删模型在 models 表
// 查不到 → 仍算缺失（孤儿能力被排除后，只有真正软删+有正常渠道能力的
// 模型才进缺失列表）。
func TestGetMissingModelsSoftDeletedModelIsMissing(t *testing.T) {
	setupMissingModelsTest(t)

	ch := &Channel{Name: "zz-softdel-ch", Type: 1, Status: common.ChannelStatusEnabled}
	require.NoError(t, DB.Create(ch).Error)
	m := &Model{ModelName: "zz-softdel-model"}
	require.NoError(t, DB.Create(m).Error)
	require.NoError(t, DB.Delete(m).Error) // 软删

	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "zz-softdel-model", ChannelId: ch.Id, Enabled: true,
	}).Error)

	missing, err := GetMissingModels()
	require.NoError(t, err)
	assert.Equal(t, []string{"zz-softdel-model"}, missing)
}

// TestDeleteDisabledChannelCleansAbilities 验证删除禁用渠道时同步清理能力，
// 不再产生孤儿能力。
func TestDeleteDisabledChannelCleansAbilities(t *testing.T) {
	setupMissingModelsTest(t)

	ch := &Channel{Name: "zz-disabled-ch", Type: 1, Status: common.ChannelStatusManuallyDisabled}
	require.NoError(t, DB.Create(ch).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "zz-disabled-model", ChannelId: ch.Id, Enabled: false,
	}).Error)

	rows, err := DeleteDisabledChannel()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)

	var count int64
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", ch.Id).Count(&count).Error)
	assert.Equal(t, int64(0), count, "abilities of deleted disabled channel must be cleaned")
}
