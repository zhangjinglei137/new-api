package controller

import (
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deleteVendorViaController(t *testing.T, id int) (bool, string) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}}
	DeleteVendorMeta(c)
	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	return body.Success, body.Message
}

// TestDeleteVendorMetaRejectsWhenReferencedByModels 验证仍有模型引用时删除被拒绝，
// 错误消息包含引用数。
func TestDeleteVendorMetaRejectsWhenReferencedByModels(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	v := &model.Vendor{Name: "zz-vendor-referenced"}
	require.NoError(t, db.Create(v).Error)
	require.NoError(t, db.Create(&model.Model{
		ModelName: "zz-ref-model",
		VendorID:  v.Id,
	}).Error)

	success, msg := deleteVendorViaController(t, v.Id)
	assert.False(t, success)
	assert.Contains(t, msg, "1")

	// 供应商未被删除
	var count int64
	require.NoError(t, db.Model(&model.Vendor{}).Where("id = ?", v.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// TestDeleteVendorMetaIgnoresSoftDeletedModels 验证软删模型不计入引用。
func TestDeleteVendorMetaIgnoresSoftDeletedModels(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	v := &model.Vendor{Name: "zz-vendor-softdel"}
	require.NoError(t, db.Create(v).Error)
	m := &model.Model{ModelName: "zz-softdel-model", VendorID: v.Id}
	require.NoError(t, db.Create(m).Error)
	require.NoError(t, db.Delete(m).Error)

	success, _ := deleteVendorViaController(t, v.Id)
	assert.True(t, success)

	var count int64
	require.NoError(t, db.Model(&model.Vendor{}).Where("id = ?", v.Id).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

// TestDeleteVendorMetaSucceedsWhenUnreferenced 验证无引用时正常删除。
func TestDeleteVendorMetaSucceedsWhenUnreferenced(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	v := &model.Vendor{Name: "zz-vendor-free"}
	require.NoError(t, db.Create(v).Error)

	success, _ := deleteVendorViaController(t, v.Id)
	assert.True(t, success)

	var count int64
	require.NoError(t, db.Model(&model.Vendor{}).Where("id = ?", v.Id).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}
