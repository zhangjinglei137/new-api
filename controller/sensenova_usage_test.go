package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSenseNovaUsageControllerTest(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = database
	t.Cleanup(func() { model.DB = originalDB })
}

func callGetSenseNovaUsage(t *testing.T, id int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}}
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/"+strconv.Itoa(id)+"/sensenova/usage", nil)
	GetSenseNovaUsage(context)
	return recorder
}

func TestGetSenseNovaUsageRejectsNonSenseNovaChannel(t *testing.T) {
	setupSenseNovaUsageControllerTest(t)
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "openai", Key: "k", Models: "gpt-4o", Group: "default"}
	require.NoError(t, channel.Insert())

	recorder := callGetSenseNovaUsage(t, channel.Id)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), `"channel type is not SenseNova"`)
}

func TestGetSenseNovaUsageRejectsMissingCredentials(t *testing.T) {
	setupSenseNovaUsageControllerTest(t)
	channel := model.Channel{Type: constant.ChannelTypeSenseNova, Status: common.ChannelStatusEnabled, Name: "sensenova", Key: "k", Models: "deepseek-v4-flash", Group: "default"}
	require.NoError(t, channel.Insert())

	recorder := callGetSenseNovaUsage(t, channel.Id)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), `"sensenova 账号未配置"`)
}

func callGetSenseNovaAPIKeys(t *testing.T, id int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}}
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/"+strconv.Itoa(id)+"/sensenova/api-keys", nil)
	GetSenseNovaAPIKeys(context)
	return recorder
}

func callDeleteSenseNovaAPIKey(t *testing.T, id int, keyID string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}, {Key: "keyId", Value: keyID}}
	context.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/"+strconv.Itoa(id)+"/sensenova/api-keys/"+keyID, nil)
	DeleteSenseNovaAPIKey(context)
	return recorder
}

// senseNovaControllerUpstreamHandler 模拟 SenseNova 登录 4 步流程与 api-keys
// 列表/删除接口，供 controller 层成功路径测试经本地 CONNECT 代理访问。
func senseNovaControllerUpstreamHandler(t *testing.T, keysJSON string) http.HandlerFunc {
	t.Helper()
	const redirect = "https://platform.sensenova.cn/oauth2/auth?client_id=nova&login_verifier=verifier-abc"
	const consent = "https://iam.sensecoreapi.cn/iam/authn/v1/auth/consent?consent_challenge=chal-1"
	const consentBack = "https://platform.sensenova.cn/oauth2/auth?client_id=nova&consent_verifier=verifier-xyz&redirect_uri=https%3A%2F%2Fplatform.sensenova.cn"
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.Host == "iam.sensecoreapi.cn":
			// 步骤 B：账号密码登录
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"redirect":"` + redirect + `"}`))
		case r.Method == http.MethodGet && r.Host == "platform.sensenova.cn" &&
			strings.HasPrefix(r.URL.Path, "/oauth2/auth") && r.URL.Query().Get("consent_verifier") != "":
			// 步骤 C 第三跳：consent 后回到授权页 → 303 携带 code
			w.Header().Set("Location", "https://platform.sensenova.cn/?code=auth-code-123")
			w.WriteHeader(http.StatusSeeOther)
		case r.Method == http.MethodGet && r.Host == "platform.sensenova.cn" &&
			strings.HasPrefix(r.URL.Path, "/oauth2/auth") && r.URL.Query().Get("login_verifier") != "":
			// 步骤 C 第一跳：授权重定向 → consent 页
			w.Header().Set("Location", consent)
			w.WriteHeader(http.StatusFound)
		case r.Method == http.MethodGet && r.Host == "iam.sensecoreapi.cn":
			// 步骤 C 第二跳：consent 同意页 → 回授权页
			w.Header().Set("Location", consentBack)
			w.WriteHeader(http.StatusFound)
		case r.Method == http.MethodGet && r.Host == "platform.sensenova.cn" &&
			strings.HasPrefix(r.URL.Path, "/oauth2/auth"):
			// 步骤 A：授权页，302 + CSRF cookie
			w.Header().Set("Set-Cookie", "oauth2_authentication_csrf=csrf-1")
			w.Header().Set("Location", "https://platform.sensenova.cn/login?login_challenge=chal-1")
			w.WriteHeader(http.StatusFound)
		case r.Method == http.MethodPost && r.Host == "signin.sensecore.cn":
			// 步骤 D：授权码换令牌
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"access_token":"access-123","refresh_token":"refresh-456","expires_in":7200}`))
		case r.Method == http.MethodGet && r.Host == "platform.sensenova.cn" &&
			strings.HasPrefix(r.URL.Path, "/lite/console/v1/metered/api-keys"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(keysJSON))
		case r.Method == http.MethodDelete && r.Host == "platform.sensenova.cn" &&
			strings.HasPrefix(r.URL.Path, "/lite/console/v1/metered/api-keys/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected upstream request: %s %s%s", r.Method, r.Host, r.URL.Path)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}
}

// newSenseNovaTestChannel 构造一个配置了加密凭证与代理的 SenseNova 渠道。
func newSenseNovaTestChannel(t *testing.T, key, proxyURL, username, password string) *model.Channel {
	t.Helper()
	encUser, err := common.EncryptSecret(username)
	require.NoError(t, err)
	encPass, err := common.EncryptSecret(password)
	require.NoError(t, err)
	return &model.Channel{
		Type:          constant.ChannelTypeSenseNova,
		Status:        common.ChannelStatusEnabled,
		Name:          "sensenova",
		Key:           key,
		Models:        "deepseek-v4-flash",
		Group:         "default",
		Setting:       common.GetPointer(fmt.Sprintf(`{"proxy":%q}`, proxyURL)),
		OtherSettings: fmt.Sprintf(`{"sensenova_username":%q,"sensenova_password":%q}`, encUser, encPass),
	}
}

func TestGetSenseNovaAPIKeysRejectsNonSenseNovaChannel(t *testing.T) {
	setupSenseNovaUsageControllerTest(t)
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "openai", Key: "k", Models: "gpt-4o", Group: "default"}
	require.NoError(t, channel.Insert())

	recorder := callGetSenseNovaAPIKeys(t, channel.Id)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), `"channel type is not SenseNova"`)
}

func TestGetSenseNovaAPIKeysRejectsMissingCredentials(t *testing.T) {
	setupSenseNovaUsageControllerTest(t)
	channel := model.Channel{Type: constant.ChannelTypeSenseNova, Status: common.ChannelStatusEnabled, Name: "sensenova", Key: "k", Models: "deepseek-v4-flash", Group: "default"}
	require.NoError(t, channel.Insert())

	recorder := callGetSenseNovaAPIKeys(t, channel.Id)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), `"sensenova 账号未配置"`)
}

// TestGetSenseNovaAPIKeysRejectsMultiKeyChannel 验证多 key 渠道被拒绝：多 key 时
// ch.Key 是拼接整串，无法与上游单个 key 比较，in_use 与删除兜底都会失效，故
// 直接拒绝（与 moonshot/radeoncloud/codex 等用量接口一致）。
func TestGetSenseNovaAPIKeysRejectsMultiKeyChannel(t *testing.T) {
	setupSenseNovaUsageControllerTest(t)
	channel := model.Channel{Type: constant.ChannelTypeSenseNova, Status: common.ChannelStatusEnabled, Name: "sensenova", Key: "k", Models: "deepseek-v4-flash", Group: "default"}
	channel.ChannelInfo.IsMultiKey = true
	require.NoError(t, channel.Insert())

	recorder := callGetSenseNovaAPIKeys(t, channel.Id)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), "multi-key channel is not supported")
}

// TestGetSenseNovaAPIKeysMarksInUseAndMasksKey 验证列表标记 in_use（等于渠道
// 当前 key 的项）且对外只返回掩码 api_key，绝不泄露完整明文。
func TestGetSenseNovaAPIKeysMarksInUseAndMasksKey(t *testing.T) {
	setupSenseNovaUsageControllerTest(t)
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "controller-sensenova-apikeys-test-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })

	const inUseKey = "sk-hQAsW1234567890fY1"
	const otherKey = "sk-ZZZZZZZZZZZZZZZZ9999"
	keysJSON := `{"api_keys":[` +
		`{"id":"key-1","displayname":"默认","key_type":"API_KEY_TYPE_TOKEN_PLAN","api_key":"` + inUseKey + `","create_time":"2026-07-16T06:56:14.816395Z","status":"enabled","is_default":true},` +
		`{"id":"key-2","displayname":"备用","key_type":"API_KEY_TYPE_TOKEN_PLAN","api_key":"` + otherKey + `","create_time":"2026-07-17T06:56:14.816395Z","status":"enabled","is_default":false}` +
		`],"next_page_token":"","total_count":2}`

	proxyURL := newVolcCodingPlanConnectProxy(t, senseNovaControllerUpstreamHandler(t, keysJSON))
	channel := newSenseNovaTestChannel(t, inUseKey, proxyURL, "sn-user-1", "sn-pass-1")
	require.NoError(t, channel.Insert())

	recorder := callGetSenseNovaAPIKeys(t, channel.Id)
	body := recorder.Body.String()
	assert.Contains(t, body, `"success":true`)
	assert.Contains(t, body, `"id":"key-1"`)
	assert.Contains(t, body, `"in_use":true`)
	assert.Contains(t, body, `"in_use":false`)
	assert.Contains(t, body, `"api_key":"sk-hQAsW…0fY1"`, "必须返回掩码 key（前 8 + … + 后 4）")
	assert.NotContains(t, body, inUseKey, "响应绝不能包含完整 key")
	assert.NotContains(t, body, otherKey, "响应绝不能包含完整 key")
}

// TestDeleteSenseNovaAPIKeyRejectsInUseKey 验证服务端兜底拒绝删除渠道当前
// 正在使用的 key。
func TestDeleteSenseNovaAPIKeyRejectsInUseKey(t *testing.T) {
	setupSenseNovaUsageControllerTest(t)
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "controller-sensenova-del-inuse-test-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })

	const inUseKey = "sk-hQAsW1234567890fY1"
	keysJSON := `{"api_keys":[{"id":"key-1","displayname":"默认","key_type":"API_KEY_TYPE_TOKEN_PLAN","api_key":"` + inUseKey + `","create_time":"2026-07-16T06:56:14.816395Z","status":"enabled","is_default":true}],"next_page_token":"","total_count":1}`

	proxyURL := newVolcCodingPlanConnectProxy(t, senseNovaControllerUpstreamHandler(t, keysJSON))
	channel := newSenseNovaTestChannel(t, inUseKey, proxyURL, "sn-user-2", "sn-pass-2")
	require.NoError(t, channel.Insert())

	recorder := callDeleteSenseNovaAPIKey(t, channel.Id, "key-1")
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), "不能删除正在使用的 apikey")
}
