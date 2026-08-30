package moonshot

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	channelconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := claude.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not supported")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	adaptor := openai.Adaptor{}
	return adaptor.ConvertImageRequest(c, info, request)
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := info.ChannelBaseUrl
	specialPlan, _, hasSpecialPlan := channelconstant.ResolveSpecialPlan(info.ChannelType, baseURL, info.ChannelOtherSettings.EndpointProfile)
	if hasSpecialPlan {
		switch info.RelayFormat {
		case types.RelayFormatClaude:
			return fmt.Sprintf("%s/v1/messages", specialPlan.ClaudeBaseURL), nil
		case types.RelayFormatOpenAI:
			if info.RelayMode == constant.RelayModeChatCompletions {
				return fmt.Sprintf("%s/chat/completions", specialPlan.OpenAIBaseURL), nil
			}
		}
		// 失败关闭：Coding Plan 套餐不支持该请求模式，绝不回退按量端点
		return "", types.NewErrorWithStatusCode(
			errors.New("moonshot coding plan 套餐不支持该请求模式"),
			types.ErrorCodeBadRequestBody,
			http.StatusBadRequest,
		)
	}

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return fmt.Sprintf("%s/anthropic/v1/messages", baseURL), nil
	default:
		if info.RelayMode == constant.RelayModeRerank {
			return fmt.Sprintf("%s/v1/rerank", baseURL), nil
		} else if info.RelayMode == constant.RelayModeEmbeddings {
			return fmt.Sprintf("%s/v1/embeddings", baseURL), nil
		} else if info.RelayMode == constant.RelayModeChatCompletions {
			return fmt.Sprintf("%s/v1/chat/completions", baseURL), nil
		} else if info.RelayMode == constant.RelayModeCompletions {
			return fmt.Sprintf("%s/v1/completions", baseURL), nil
		}
		return fmt.Sprintf("%s/v1/chat/completions", baseURL), nil
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request.Temperature != nil && isTemperatureOneOnlyModel(getUpstreamModelName(info, request.Model)) && *request.Temperature != 1.0 {
		request.Temperature = common.GetPointer[float64](1.0)
	}
	return request, nil
}

func getUpstreamModelName(info *relaycommon.RelayInfo, fallback string) string {
	if info != nil && info.ChannelMeta != nil && info.UpstreamModelName != "" {
		return info.UpstreamModelName
	}
	return fallback
}

func isTemperatureOneOnlyModel(model string) bool {
	return strings.EqualFold(model, "kimi-k2.6")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		adaptor := claude.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
