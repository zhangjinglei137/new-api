package controller

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVolcCodingPlanUsageRejectsNonCodingProfile(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	channel := &model.Channel{Type: constant.ChannelTypeVolcEngine, Name: "volc-plain", Key: "k"}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1/volc/codingplan/usage", nil)
	GetVolcCodingPlanUsage(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "Coding Plan")
}

func TestGetVolcCodingPlanUsageRejectsNonVolcChannelType(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "openai", Key: "k"}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1/volc/codingplan/usage", nil)
	GetVolcCodingPlanUsage(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "VolcEngine")
}

func TestGetVolcCodingPlanUsageCredentialsNotConfigured(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	channel := &model.Channel{
		Type:          constant.ChannelTypeVolcEngine,
		Name:          "volc-coding",
		Key:           "k",
		OtherSettings: `{"endpoint_profile":"coding"}`,
	}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1/volc/codingplan/usage", nil)
	GetVolcCodingPlanUsage(ctx)

	var response struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		ErrorCode string `json:"error_code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "credentials_not_configured", response.ErrorCode)
	// 响应不得包含任何凭证内容
	assert.NotContains(t, recorder.Body.String(), "volc_coding_plan_cookie")
}

func TestGetVolcCodingPlanUsageUnsupportedRegion(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	channel := &model.Channel{
		Type:          constant.ChannelTypeVolcEngine,
		Name:          "volc-unknown-region",
		Key:           "k",
		BaseURL:       common.GetPointer("https://ark.cn-hangzhou.volces.com/api/coding"),
		OtherSettings: `{"endpoint_profile":"coding","volc_coding_plan_access_key_id":"enc-ak","volc_coding_plan_secret_access_key":"enc-sk"}`,
	}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1/volc/codingplan/usage", nil)
	GetVolcCodingPlanUsage(ctx)

	var response struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		ErrorCode string `json:"error_code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "unsupported_region", response.ErrorCode)
}

func TestGetVolcCodingPlanUsageAkSkPath(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "controller-volc-aksk-test-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })

	encAK, err := common.EncryptSecret("AKLTtesttesttesttesttesttesttest")
	require.NoError(t, err)
	encSK, err := common.EncryptSecret("sk-test-secret-key-0000000000000000")
	require.NoError(t, err)

	// channel 通过 proxy 后端的 CONNECT 隧道访问 https 上游，尽头是本地 TLS server。
	proxyURL := newVolcCodingPlanConnectProxy(t, func(w http.ResponseWriter, r *http.Request) {
		// AK/SK 路径：携带 V4 签名，但不携带浏览器会话头。
		assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "HMAC-SHA256 Credential="))
		assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", r.Header.Get("X-Content-Sha256"))
		assert.Empty(t, r.Header.Get("x-csrf-token"))
		assert.Empty(t, r.Header.Get("Cookie"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Result":{"Status":"Running","QuotaUsage":[{"Level":"session","Percent":50},{"Level":"monthly","Percent":20}]}}`))
	})

	channel := &model.Channel{
		Type:          constant.ChannelTypeVolcEngine,
		Name:          "volc-aksk",
		Key:           "k",
		BaseURL:       common.GetPointer("https://ark.cn-beijing.volces.com/api/coding"),
		Setting:       common.GetPointer(fmt.Sprintf(`{"proxy":%q}`, proxyURL)),
		OtherSettings: fmt.Sprintf(`{"endpoint_profile":"coding","volc_coding_plan_access_key_id":%q,"volc_coding_plan_secret_access_key":%q}`, encAK, encSK),
	}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1/volc/codingplan/usage", nil)
	GetVolcCodingPlanUsage(ctx)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Status  string `json:"status"`
			Windows []struct {
				Period      string  `json:"period"`
				UsedPercent float64 `json:"used_percent"`
			} `json:"windows"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, "Running", response.Data.Status)
	require.Len(t, response.Data.Windows, 2)
	assert.Equal(t, "session", response.Data.Windows[0].Period)
	assert.Equal(t, "monthly", response.Data.Windows[1].Period)
}

func TestGetVolcCodingPlanUsageAkSkInvalidCredentialsExpired(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "controller-volc-aksk-expired-test-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })

	encAK, err := common.EncryptSecret("AKLTtesttesttesttesttesttesttest")
	require.NoError(t, err)
	encSK, err := common.EncryptSecret("sk-test-secret-key-0000000000000000")
	require.NoError(t, err)

	proxyURL := newVolcCodingPlanConnectProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"SignatureDoesNotMatch"}}`))
	})

	channel := &model.Channel{
		Type:          constant.ChannelTypeVolcEngine,
		Name:          "volc-aksk-invalid",
		Key:           "k",
		BaseURL:       common.GetPointer("https://ark.cn-beijing.volces.com/api/coding"),
		Setting:       common.GetPointer(fmt.Sprintf(`{"proxy":%q}`, proxyURL)),
		OtherSettings: fmt.Sprintf(`{"endpoint_profile":"coding","volc_coding_plan_access_key_id":%q,"volc_coding_plan_secret_access_key":%q}`, encAK, encSK),
	}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1/volc/codingplan/usage", nil)
	GetVolcCodingPlanUsage(ctx)

	var response struct {
		Success        bool   `json:"success"`
		Message        string `json:"message"`
		ErrorCode      string `json:"error_code"`
		UpstreamStatus int    `json:"upstream_status"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "credentials_expired", response.ErrorCode)
	assert.Equal(t, http.StatusUnauthorized, response.UpstreamStatus)
	assert.Contains(t, response.Message, "Access Key")
}

// newVolcCodingPlanConnectProxy 启动一个本地 HTTP 代理（CONNECT 隧道）并把隧道
// 末端接到一个本地 TLS server，用于在 controller 测试中拦截 https 上行
// （fetch 目标固定为 open.volcengineapi.com）。测试期间开启
// common.TLSInsecureSkipVerify 使代理客户端信任自签证书，结束后恢复并清空
// 代理客户端缓存。返回代理 URL。
func newVolcCodingPlanConnectProxy(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	upstream := httptest.NewTLSServer(handler)
	// 一次请求后即关闭隧道连接，否则代理端双向透传会一直阻塞。
	upstream.Config.SetKeepAlivesEnabled(false)
	t.Cleanup(upstream.Close)

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT only", http.StatusMethodNotAllowed)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack unsupported", http.StatusInternalServerError)
			return
		}
		clientConn, buf, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer clientConn.Close()
		upstreamConn, err := net.Dial("tcp", upstream.Listener.Addr().String())
		if err != nil {
			return
		}
		defer upstreamConn.Close()
		if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			return
		}
		// 双向透传：客户端 TLS 字节 <-> 本地 TLS server。buf 里可能已缓存
		// 部分 TLS ClientHello 字节，先从 buf.Reader 读取。
		done := make(chan struct{}, 1)
		go func() {
			defer close(done)
			if buf.Reader.Buffered() > 0 {
				_, _ = io.Copy(upstreamConn, buf.Reader)
			}
			_, _ = io.Copy(upstreamConn, clientConn)
		}()
		_, _ = io.Copy(clientConn, upstreamConn)
		<-done
	}))
	t.Cleanup(proxy.Close)

	previous := common.TLSInsecureSkipVerify
	common.TLSInsecureSkipVerify = true
	service.ResetProxyClientCache()
	t.Cleanup(func() {
		common.TLSInsecureSkipVerify = previous
		service.ResetProxyClientCache()
	})

	return proxy.URL
}
