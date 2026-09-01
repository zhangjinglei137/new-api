package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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
