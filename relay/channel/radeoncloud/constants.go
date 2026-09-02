package radeoncloud

import "strings"

var ModelList = []string{
	"DeepSeek-V4-Flash",
}

var ChannelName = "radeoncloud"

// NormalizeBaseURL 兼容用户在渠道里配置的 Base URL 带或不带 "/api" 前缀
// 的两种写法，统一返回不含尾部 "/api" 的根地址。
// 例如 "https://developer.amd.com.cn/radeon/api" 与
// "https://developer.amd.com.cn/radeon" 都会归一化为后者，
// 后续拼接 "/api/v1/..." 时不会产生 "/api/api/..." 的错误路径。
func NormalizeBaseURL(baseURL string) string {
	return strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/api")
}
