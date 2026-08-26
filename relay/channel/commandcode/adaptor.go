package commandcode

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const ChannelName = "CommandCode"

type Adaptor struct {
}

// isClaudeModel 判断上游模型是否为 Anthropic/Claude 模型。
// Command Code 的 /v1/messages 端点只支持 Anthropic 模型，
// OpenAI/OSS 模型（如 MiniMax M3）必须走 /v1/chat/completions。
func isClaudeModel(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "claude")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// Claude 模型走 Anthropic Messages 端点；OpenAI/OSS 模型（如 MiniMax M3）
	// 只能走 OpenAI Chat Completions 端点。
	path := "/v1/chat/completions"
	if info.RelayFormat == types.RelayFormatClaude && isClaudeModel(info.UpstreamModelName) {
		path = "/v1/messages"
	}
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, path, info.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	// Command Code exposes an OpenAI Chat Completions compatible endpoint,
	// pass the request through as-is.
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if isClaudeModel(info.UpstreamModelName) {
		// Command Code exposes an Anthropic Messages compatible endpoint,
		// pass the request through as-is.
		return request, nil
	}
	// OpenAI/OSS 模型只能走 /v1/chat/completions，将 Claude 请求转换为
	// OpenAI Chat Completions 请求。
	result, err := service.ConvertRequestByID(c, info, relayconvert.ConverterClaudeMessagesToOpenAIChat, request)
	if err != nil {
		return nil, err
	}
	chatRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
	}
	return chatRequest, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// Command Code does not support the OpenAI Responses API, convert the
	// responses request to an OpenAI Chat Completions request instead.
	result, err := service.ConvertRequestByID(c, info, relayconvert.ConverterOpenAIResponsesToOpenAIChat, request)
	if err != nil {
		return nil, err
	}
	chatRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
	}
	return chatRequest, nil
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("commandcode: endpoint not supported")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("commandcode: endpoint not supported")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("commandcode: endpoint not supported")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("commandcode: endpoint not supported")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("commandcode: endpoint not supported")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == relayconstant.RelayModeResponses {
		// The original request was an OpenAI Responses request converted to
		// Chat Completions, convert the upstream chat response back.
		if info.IsStream {
			return openai.OaiChatToResponsesStreamHandler(c, info, resp)
		}
		return openai.OaiChatToResponsesHandler(c, info, resp)
	}
	if info.RelayFormat == types.RelayFormatClaude {
		if isClaudeModel(info.UpstreamModelName) {
			// 上游 /v1/messages 直接返回 Anthropic Messages 格式，透传。
			adaptor := claude.Adaptor{}
			return adaptor.DoResponse(c, resp, info)
		}
		// OpenAI/OSS 模型走了 /v1/chat/completions，上游返回 OpenAI Chat
		// Completions 格式，由 openai adaptor 按 info.RelayFormat 转换回 Claude。
		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	}
	adaptor := openai.Adaptor{}
	return adaptor.DoResponse(c, resp, info)
}

func (a *Adaptor) GetModelList() []string {
	// Model list is populated by upstream model sync (GET {baseURL}/v1/models),
	// do not hardcode models here.
	return []string{}
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
