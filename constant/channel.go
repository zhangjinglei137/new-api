package constant

const (
	ChannelTypeUnknown        = 0
	ChannelTypeOpenAI         = 1
	ChannelTypeMidjourney     = 2
	ChannelTypeAzure          = 3
	ChannelTypeOllama         = 4
	ChannelTypeMidjourneyPlus = 5
	ChannelTypeOpenAIMax      = 6
	ChannelTypeOhMyGPT        = 7
	ChannelTypeCustom         = 8
	ChannelTypeAILS           = 9
	ChannelTypeAIProxy        = 10
	ChannelTypePaLM           = 11
	ChannelTypeAPI2GPT        = 12
	ChannelTypeAIGC2D         = 13
	ChannelTypeAnthropic      = 14
	ChannelTypeBaidu          = 15
	ChannelTypeZhipu          = 16
	ChannelTypeAli            = 17
	ChannelTypeXunfei         = 18
	ChannelType360            = 19
	ChannelTypeOpenRouter     = 20
	ChannelTypeAIProxyLibrary = 21
	ChannelTypeFastGPT        = 22
	ChannelTypeTencent        = 23
	ChannelTypeGemini         = 24
	ChannelTypeMoonshot       = 25
	ChannelTypeZhipu_v4       = 26
	ChannelTypePerplexity     = 27
	ChannelTypeLingYiWanWu    = 31
	ChannelTypeAws            = 33
	ChannelTypeCohere         = 34
	ChannelTypeMiniMax        = 35
	ChannelTypeSunoAPI        = 36
	ChannelTypeDify           = 37
	ChannelTypeJina           = 38
	ChannelCloudflare         = 39
	ChannelTypeSiliconFlow    = 40
	ChannelTypeVertexAi       = 41
	ChannelTypeMistral        = 42
	ChannelTypeDeepSeek       = 43
	ChannelTypeMokaAI         = 44
	ChannelTypeVolcEngine     = 45
	ChannelTypeBaiduV2        = 46
	ChannelTypeXinference     = 47
	ChannelTypeXai            = 48
	ChannelTypeCoze           = 49
	ChannelTypeKling          = 50
	ChannelTypeJimeng         = 51
	ChannelTypeVidu           = 52
	ChannelTypeSubmodel       = 53
	ChannelTypeDoubaoVideo    = 54
	ChannelTypeSora           = 55
	ChannelTypeReplicate      = 56
	ChannelTypeCodex          = 57
	ChannelTypeAdvancedCustom = 58
	ChannelTypeSub2API        = 59
	ChannelTypeNewAPI         = 60
	ChannelTypeTaskPlugin     = 61
	ChannelTypeRadeonCloud    = 95
	ChannelTypeSenseNova      = 97
	ChannelTypeCommandCode    = 98
	ChannelTypeOpenCodeGo     = 99
	ChannelTypeDummy          = 100 // this one is only for count, do not add any channel after this

)

var ChannelBaseURLs = []string{
	"",                                    // 0
	"https://api.openai.com",              // 1
	"https://oa.api2d.net",                // 2
	"",                                    // 3
	"http://localhost:11434",              // 4
	"https://api.openai-sb.com",           // 5
	"https://api.openaimax.com",           // 6
	"https://api.ohmygpt.com",             // 7
	"",                                    // 8
	"https://api.caipacity.com",           // 9
	"https://api.aiproxy.io",              // 10
	"",                                    // 11
	"https://api.api2gpt.com",             // 12
	"https://api.aigc2d.com",              // 13
	"https://api.anthropic.com",           // 14
	"https://aip.baidubce.com",            // 15
	"https://open.bigmodel.cn",            // 16
	"https://dashscope.aliyuncs.com",      // 17
	"",                                    // 18
	"https://api.360.cn",                  // 19
	"https://openrouter.ai/api",           // 20
	"https://api.aiproxy.io",              // 21
	"https://fastgpt.run/api/openapi",     // 22
	"https://hunyuan.tencentcloudapi.com", //23
	"https://generativelanguage.googleapis.com", //24
	"https://api.moonshot.cn",                   //25
	"https://open.bigmodel.cn",                  //26
	"https://api.perplexity.ai",                 //27
	"",                                          //28
	"",                                          //29
	"",                                          //30
	"https://api.lingyiwanwu.com",               //31
	"",                                          //32
	"",                                          //33
	"https://api.cohere.ai",                     //34
	"https://api.minimax.chat",                  //35
	"",                                          //36
	"https://api.dify.ai",                       //37
	"https://api.jina.ai",                       //38
	"https://api.cloudflare.com",                //39
	"https://api.siliconflow.cn",                //40
	"",                                          //41
	"https://api.mistral.ai",                    //42
	"https://api.deepseek.com",                  //43
	"https://api.moka.ai",                       //44
	"https://ark.cn-beijing.volces.com",         //45
	"https://qianfan.baidubce.com",              //46
	"",                                          //47
	"https://api.x.ai",                          //48
	"https://api.coze.cn",                       //49
	"https://api.klingai.com",                   //50
	"https://visual.volcengineapi.com",          //51
	"https://api.vidu.cn",                       //52
	"https://llm.submodel.ai",                   //53
	"https://ark.cn-beijing.volces.com",         //54
	"https://api.openai.com",                    //55
	"https://api.replicate.com",                 //56
	"https://chatgpt.com",                       //57
	"",                                          //58
	"",                                          //59
	"",                                          //60
	"",                                          //61
	"",                                          //62
	"",                                          //63
	"",                                          //64
	"",                                          //65
	"",                                          //66
	"",                                          //67
	"",                                          //68
	"",                                          //69
	"",                                          //70
	"",                                          //71
	"",                                          //72
	"",                                          //73
	"",                                          //74
	"",                                          //75
	"",                                          //76
	"",                                          //77
	"",                                          //78
	"",                                          //79
	"",                                          //80
	"",                                          //81
	"",                                          //82
	"",                                          //83
	"",                                          //84
	"",                                          //85
	"",                                          //86
	"",                                          //87
	"",                                          //88
	"",                                          //89
	"",                                          //90
	"",                                          //91
	"",                                          //92
	"",                                          //93
	"",                                          //94
	"https://developer.amd.com.cn/radeon",       //95
	"",                                          //96
	"https://token.sensenova.cn",                //97
	"https://api.commandcode.ai/provider",       //98
	"https://opencode.ai/zen/go",                //99
}

func GetChannelBaseURL(channelType int) string {
	if channelType < 0 || channelType >= len(ChannelBaseURLs) {
		return ""
	}
	return ChannelBaseURLs[channelType]
}

var ChannelTypeNames = map[int]string{
	ChannelTypeUnknown:        "Unknown",
	ChannelTypeOpenAI:         "OpenAI",
	ChannelTypeMidjourney:     "Midjourney",
	ChannelTypeAzure:          "Azure",
	ChannelTypeOllama:         "Ollama",
	ChannelTypeMidjourneyPlus: "MidjourneyPlus",
	ChannelTypeOpenAIMax:      "OpenAIMax",
	ChannelTypeOhMyGPT:        "OhMyGPT",
	ChannelTypeCustom:         "Custom",
	ChannelTypeAILS:           "AILS",
	ChannelTypeAIProxy:        "AIProxy",
	ChannelTypePaLM:           "PaLM",
	ChannelTypeAPI2GPT:        "API2GPT",
	ChannelTypeAIGC2D:         "AIGC2D",
	ChannelTypeAnthropic:      "Anthropic",
	ChannelTypeBaidu:          "Baidu",
	ChannelTypeZhipu:          "Zhipu",
	ChannelTypeAli:            "Ali",
	ChannelTypeXunfei:         "Xunfei",
	ChannelType360:            "360",
	ChannelTypeOpenRouter:     "OpenRouter",
	ChannelTypeAIProxyLibrary: "AIProxyLibrary",
	ChannelTypeFastGPT:        "FastGPT",
	ChannelTypeTencent:        "Tencent",
	ChannelTypeGemini:         "Gemini",
	ChannelTypeMoonshot:       "Moonshot",
	ChannelTypeZhipu_v4:       "ZhipuV4",
	ChannelTypePerplexity:     "Perplexity",
	ChannelTypeLingYiWanWu:    "LingYiWanWu",
	ChannelTypeAws:            "AWS",
	ChannelTypeCohere:         "Cohere",
	ChannelTypeMiniMax:        "MiniMax",
	ChannelTypeSunoAPI:        "SunoAPI",
	ChannelTypeDify:           "Dify",
	ChannelTypeJina:           "Jina",
	ChannelCloudflare:         "Cloudflare",
	ChannelTypeSiliconFlow:    "SiliconFlow",
	ChannelTypeVertexAi:       "VertexAI",
	ChannelTypeMistral:        "Mistral",
	ChannelTypeDeepSeek:       "DeepSeek",
	ChannelTypeMokaAI:         "MokaAI",
	ChannelTypeVolcEngine:     "VolcEngine",
	ChannelTypeBaiduV2:        "BaiduV2",
	ChannelTypeXinference:     "Xinference",
	ChannelTypeXai:            "xAI",
	ChannelTypeCoze:           "Coze",
	ChannelTypeKling:          "Kling",
	ChannelTypeJimeng:         "Jimeng",
	ChannelTypeVidu:           "Vidu",
	ChannelTypeSubmodel:       "Submodel",
	ChannelTypeDoubaoVideo:    "DoubaoVideo",
	ChannelTypeSora:           "Sora",
	ChannelTypeReplicate:      "Replicate",
	ChannelTypeCodex:          "ChatGPT Subscription (Codex)",
	ChannelTypeAdvancedCustom: "Advanced Custom",
	ChannelTypeSub2API:        "Sub2API",
	ChannelTypeNewAPI:         "New API",
	ChannelTypeTaskPlugin:     "Task Plugin",
	ChannelTypeRadeonCloud:    "AMD Radeon Cloud",
	ChannelTypeSenseNova:      "SenseNova",
	ChannelTypeCommandCode:    "CommandCode",
	ChannelTypeOpenCodeGo:     "OpenCode Go",
}

func GetChannelTypeName(channelType int) string {
	if name, ok := ChannelTypeNames[channelType]; ok {
		return name
	}
	return "Unknown"
}

type ChannelSpecialBase struct {
	ClaudeBaseURL string
	OpenAIBaseURL string
}

var ChannelSpecialBases = map[string]ChannelSpecialBase{
	"glm-coding-plan": {
		ClaudeBaseURL: "https://open.bigmodel.cn/api/anthropic",
		OpenAIBaseURL: "https://open.bigmodel.cn/api/coding/paas/v4",
	},
	"glm-coding-plan-international": {
		ClaudeBaseURL: "https://api.z.ai/api/anthropic",
		OpenAIBaseURL: "https://api.z.ai/api/coding/paas/v4",
	},
	"kimi-coding-plan": {
		ClaudeBaseURL: "https://api.kimi.com/coding",
		OpenAIBaseURL: "https://api.kimi.com/coding/v1",
	},
	"doubao-coding-plan": {
		ClaudeBaseURL: "https://ark.cn-beijing.volces.com/api/coding",
		OpenAIBaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3",
	},
}

// ChannelSpecialPlanProfiles 定义各渠道类型支持的套餐端点 profile 到魔法键的映射。
var ChannelSpecialPlanProfiles = map[int]map[string]string{
	ChannelTypeVolcEngine: {
		"coding": "doubao-coding-plan",
	},
	ChannelTypeZhipu_v4: {
		"coding":      "glm-coding-plan",
		"coding-intl": "glm-coding-plan-international",
	},
	ChannelTypeMoonshot: {
		"coding": "kimi-coding-plan",
	},
}

// ResolveSpecialPlan 按 显式 profile > 存量 base_url 魔法键 顺序解析套餐端点。
// 返回命中的端点信息、命中用的魔法键以及是否命中套餐。未命中套餐时返回空值与 false。
func ResolveSpecialPlan(channelType int, baseURL, endpointProfile string) (ChannelSpecialBase, string, bool) {
	if endpointProfile != "" {
		if profiles, ok := ChannelSpecialPlanProfiles[channelType]; ok {
			if magicKey, ok := profiles[endpointProfile]; ok {
				if plan, ok := ChannelSpecialBases[magicKey]; ok {
					return plan, magicKey, true
				}
			}
		}
	}
	// 存量兼容：base_url 直接填魔法键（无显式 profile）时同样命中。
	// 限定 channelType 支持特殊套餐，避免其它渠道类型误命中魔法键表。
	if _, typeOk := ChannelSpecialPlanProfiles[channelType]; typeOk {
		if plan, ok := ChannelSpecialBases[baseURL]; ok {
			return plan, baseURL, true
		}
	}
	return ChannelSpecialBase{}, "", false
}

func IsAdvancedCustomChannelType(t int) bool {
	return t == ChannelTypeAdvancedCustom || t == ChannelTypeOpenCodeGo
}
