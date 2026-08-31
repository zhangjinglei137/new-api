package controller

import (
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// endpointSettingsRequest 端点定义配置请求体（与响应同构）。
type endpointSettingsRequest struct {
	Endpoints []common.EndpointDefinition `json:"endpoints"`
}

// GetEndpointSettings 返回合并配置覆盖与内置默认的完整端点定义列表（10 条），
// 以及所有出现过的 NPM 包名（models 表 provider_npm 去重值 ∪ 定义里的 npm 值，
// 去空、去重、排序），供管理端 NPM 字段的可输入下拉使用。
func GetEndpointSettings(c *gin.Context) {
	defs := common.GetEndpointDefinitions()
	npmSet := make(map[string]struct{}, len(defs))
	for _, d := range defs {
		if n := strings.TrimSpace(d.NPM); n != "" {
			npmSet[n] = struct{}{}
		}
	}
	// 聚合查询失败时降级：仅返回定义里的 npm 值，不报错
	if dbNpms, err := model.GetAllProviderNPMs(); err == nil {
		for _, n := range dbNpms {
			if n = strings.TrimSpace(n); n != "" {
				npmSet[n] = struct{}{}
			}
		}
	}
	npms := make([]string, 0, len(npmSet))
	for n := range npmSet {
		npms = append(npms, n)
	}
	sort.Strings(npms)
	common.ApiSuccess(c, gin.H{"endpoints": defs, "npm_options": npms})
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
