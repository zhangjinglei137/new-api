package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// endpointSettingsRequest 端点定义配置请求体（与响应同构）。
type endpointSettingsRequest struct {
	Endpoints []common.EndpointDefinition `json:"endpoints"`
}

// GetEndpointSettings 返回合并配置覆盖与内置默认的完整端点定义列表（10 条）。
func GetEndpointSettings(c *gin.Context) {
	common.ApiSuccess(c, gin.H{"endpoints": common.GetEndpointDefinitions()})
}

// UpdateEndpointSettings 校验并持久化端点定义到 option 键
// （model_setting.endpoint_definitions）。校验失败返回 400 中文错误。
func UpdateEndpointSettings(c *gin.Context) {
	var req endpointSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求体格式错误: " + err.Error()})
		return
	}
	if len(req.Endpoints) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "端点定义不能为空"})
		return
	}
	data, err := common.Marshal(req.Endpoints)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "端点定义序列化失败"})
		return
	}
	if err := common.ValidateEndpointDefinitions(string(data)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateOption(common.EndpointDefinitionsOptionKey, string(data)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "保存端点定义失败: " + err.Error()})
		return
	}
	common.ApiSuccess(c, nil)
}
