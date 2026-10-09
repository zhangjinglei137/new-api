package cline

// ChannelName 是 Cline 渠道的展示名。
const ChannelName = "Cline"

// PlanProfile 是 ClinePass（计划方式）对应的 EndpointProfile 取值；
// 空字符串表示 Cline（按量方式）。
const PlanProfile = "clinepass"

// ModelList 为内置模型列表。Cline 模型通过上游 /v1/models 或
// recommended-models 动态获取，这里保持为空。
var ModelList = []string{}
