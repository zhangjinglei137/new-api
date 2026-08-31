package operation_setting

import "strings"

var DemoSiteEnabled = false
var SelfUseModeEnabled = false

// ModelMetadataExtendedEnabled 控制 /v1/models（OpenAI 分支）是否返回富模型
// 元数据（name/family/modalities/limit/cost/reasoning 等扩展字段）。
// 默认关闭以保持 OpenAI 兼容输出的最小形态。
var ModelMetadataExtendedEnabled = false

var AutomaticDisableKeywords = []string{
	"Your credit balance is too low",
	"This organization has been disabled.",
	"You exceeded your current quota",
	"Permission denied",
	"The security token included in the request is invalid",
	"Operation not allowed",
	"Your account is not authorized",
}

func AutomaticDisableKeywordsToString() string {
	return strings.Join(AutomaticDisableKeywords, "\n")
}

func AutomaticDisableKeywordsFromString(s string) {
	AutomaticDisableKeywords = []string{}
	ak := strings.Split(s, "\n")
	for _, k := range ak {
		k = strings.TrimSpace(k)
		k = strings.ToLower(k)
		if k != "" {
			AutomaticDisableKeywords = append(AutomaticDisableKeywords, k)
		}
	}
}
