package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/channel/advancedcustom"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/ollama"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

const (
	channelUpstreamModelUpdateTaskDefaultIntervalMinutes  = 30
	channelUpstreamModelUpdateTaskBatchSize               = 100
	channelUpstreamModelUpdateMinCheckIntervalSeconds     = 300
	channelUpstreamModelUpdateNotifySuppressWindowSeconds = 86400
	channelUpstreamModelUpdateNotifyMaxChannelDetails     = 8
	channelUpstreamModelUpdateNotifyMaxModelDetails       = 12
	channelUpstreamModelUpdateNotifyMaxFailedChannelIDs   = 10
)

// channelUpstreamModelUpdateSelectColumns 返回上游模型巡检所需 channel 列。
// GORM Select([]string{...}) 不会给列名加引号：group 是 MySQL/PostgreSQL 的
// 保留字（裸写会生成非法 SQL），且本功能无消费方，故直接不列出；
// key 同为 MySQL 保留字但拉取上游模型时必须读取，故按数据库类型显式引用
// （规则同 model.initCol 的 commonGroupCol/commonKeyCol：PostgreSQL 用
// "key"，其余用 `key`）。
func channelUpstreamModelUpdateSelectColumns() []string {
	keyCol := "`key`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		keyCol = `"key"`
	}
	return []string{
		"id",
		"name",
		"type",
		keyCol,
		"status",
		"base_url",
		"models",
		"model_mapping",
		"settings",
		"setting",
		"other",
		"priority",
		"weight",
		"tag",
		"channel_info",
		"header_override",
	}
}

var channelUpstreamModelUpdateNotifyState = struct {
	sync.Mutex
	lastNotifiedAt      int64
	lastChangedChannels int
	lastFailedChannels  int
}{}

type applyChannelUpstreamModelUpdatesRequest struct {
	ID           int      `json:"id"`
	AddModels    []string `json:"add_models"`
	RemoveModels []string `json:"remove_models"`
	IgnoreModels []string `json:"ignore_models"`
}

type applyAllChannelUpstreamModelUpdatesResult struct {
	ChannelID             int      `json:"channel_id"`
	ChannelName           string   `json:"channel_name"`
	AddedModels           []string `json:"added_models"`
	RemovedModels         []string `json:"removed_models"`
	RemainingModels       []string `json:"remaining_models"`
	RemainingRemoveModels []string `json:"remaining_remove_models"`
}

type detectChannelUpstreamModelUpdatesResult struct {
	ChannelID       int      `json:"channel_id"`
	ChannelName     string   `json:"channel_name"`
	AddModels       []string `json:"add_models"`
	RemoveModels    []string `json:"remove_models"`
	LastCheckTime   int64    `json:"last_check_time"`
	AutoAddedModels int      `json:"auto_added_models"`
}

type upstreamModelUpdateChannelSummary struct {
	ChannelName string
	AddCount    int
	RemoveCount int
}

func normalizeModelNames(models []string) []string {
	return lo.Uniq(lo.FilterMap(models, func(model string, _ int) (string, bool) {
		trimmed := strings.TrimSpace(model)
		return trimmed, trimmed != ""
	}))
}

func mergeModelNames(base []string, appended []string) []string {
	merged := normalizeModelNames(base)
	seen := make(map[string]struct{}, len(merged))
	for _, model := range merged {
		seen[model] = struct{}{}
	}
	for _, model := range normalizeModelNames(appended) {
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		merged = append(merged, model)
	}
	return merged
}

func subtractModelNames(base []string, removed []string) []string {
	removeSet := make(map[string]struct{}, len(removed))
	for _, model := range normalizeModelNames(removed) {
		removeSet[model] = struct{}{}
	}
	return lo.Filter(normalizeModelNames(base), func(model string, _ int) bool {
		_, ok := removeSet[model]
		return !ok
	})
}

func intersectModelNames(base []string, allowed []string) []string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, model := range normalizeModelNames(allowed) {
		allowedSet[model] = struct{}{}
	}
	return lo.Filter(normalizeModelNames(base), func(model string, _ int) bool {
		_, ok := allowedSet[model]
		return ok
	})
}

func applySelectedModelChanges(originModels []string, addModels []string, removeModels []string) []string {
	// Add wins when the same model appears in both selected lists.
	normalizedAdd := normalizeModelNames(addModels)
	normalizedRemove := subtractModelNames(normalizeModelNames(removeModels), normalizedAdd)
	return subtractModelNames(mergeModelNames(originModels, normalizedAdd), normalizedRemove)
}

func normalizeChannelModelMapping(channel *model.Channel) map[string]string {
	if channel == nil || channel.ModelMapping == nil {
		return nil
	}
	rawMapping := strings.TrimSpace(*channel.ModelMapping)
	if rawMapping == "" || rawMapping == "{}" {
		return nil
	}
	parsed := make(map[string]string)
	if err := common.UnmarshalJsonStr(rawMapping, &parsed); err != nil {
		return nil
	}
	normalized := make(map[string]string, len(parsed))
	for source, target := range parsed {
		normalizedSource := strings.TrimSpace(source)
		normalizedTarget := strings.TrimSpace(target)
		if normalizedSource == "" || normalizedTarget == "" {
			continue
		}
		normalized[normalizedSource] = normalizedTarget
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func collectPendingUpstreamModelChangesFromModels(
	localModels []string,
	upstreamModels []string,
	ignoredModels []string,
	modelMapping map[string]string,
) (pendingAddModels []string, pendingRemoveModels []string) {
	localSet := make(map[string]struct{})
	localModels = normalizeModelNames(localModels)
	upstreamModels = normalizeModelNames(upstreamModels)
	for _, modelName := range localModels {
		localSet[modelName] = struct{}{}
	}
	upstreamSet := make(map[string]struct{}, len(upstreamModels))
	for _, modelName := range upstreamModels {
		upstreamSet[modelName] = struct{}{}
	}

	normalizedIgnoredModels := normalizeModelNames(ignoredModels)

	redirectSourceSet := make(map[string]struct{}, len(modelMapping))
	redirectTargetSet := make(map[string]struct{}, len(modelMapping))
	for source, target := range modelMapping {
		redirectSourceSet[source] = struct{}{}
		redirectTargetSet[target] = struct{}{}
	}

	coveredUpstreamSet := make(map[string]struct{}, len(localSet)+len(redirectTargetSet))
	for modelName := range localSet {
		coveredUpstreamSet[modelName] = struct{}{}
	}
	for modelName := range redirectTargetSet {
		coveredUpstreamSet[modelName] = struct{}{}
	}

	pendingAdd := lo.Filter(upstreamModels, func(modelName string, _ int) bool {
		if _, ok := coveredUpstreamSet[modelName]; ok {
			return false
		}
		if lo.ContainsBy(normalizedIgnoredModels, func(ignoredModel string) bool {
			if regexBody, ok := strings.CutPrefix(ignoredModel, "regex:"); ok {
				matched, err := regexp.MatchString(strings.TrimSpace(regexBody), modelName)
				return err == nil && matched
			}
			return ignoredModel == modelName
		}) {
			return false
		}
		return true
	})
	pendingRemove := lo.Filter(localModels, func(modelName string, _ int) bool {
		// Redirect source models are virtual aliases and should not be removed
		// only because they are absent from upstream model list.
		if _, ok := redirectSourceSet[modelName]; ok {
			return false
		}
		_, ok := upstreamSet[modelName]
		return !ok
	})
	return normalizeModelNames(pendingAdd), normalizeModelNames(pendingRemove)
}

func collectPendingUpstreamModelChanges(channel *model.Channel, settings dto.ChannelOtherSettings) (pendingAddModels []string, pendingRemoveModels []string, err error) {
	upstreamModels, err := fetchChannelUpstreamModelIDs(channel)
	if err != nil {
		return nil, nil, err
	}
	pendingAddModels, pendingRemoveModels = collectPendingUpstreamModelChangesFromModels(
		channel.GetModels(),
		upstreamModels,
		settings.UpstreamModelUpdateIgnoredModels,
		normalizeChannelModelMapping(channel),
	)
	return pendingAddModels, pendingRemoveModels, nil
}

func getUpstreamModelUpdateMinCheckIntervalSeconds() int64 {
	interval := int64(common.GetEnvOrDefault(
		"CHANNEL_UPSTREAM_MODEL_UPDATE_MIN_CHECK_INTERVAL_SECONDS",
		channelUpstreamModelUpdateMinCheckIntervalSeconds,
	))
	if interval < 0 {
		return channelUpstreamModelUpdateMinCheckIntervalSeconds
	}
	return interval
}

func parseOpenAIModelIDs(body []byte) ([]string, error) {
	var result struct {
		Data *[]OpenAIModel `json:"data"`
	}
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("invalid OpenAI Models response: %w", err)
	}
	if result.Data == nil {
		return nil, fmt.Errorf("invalid OpenAI Models response: data is required")
	}
	ids := normalizeModelNames(lo.Map(*result.Data, func(item OpenAIModel, _ int) string {
		return item.ID
	}))
	if len(ids) == 0 {
		return nil, fmt.Errorf("OpenAI Models response contains no valid model IDs")
	}
	return ids, nil
}

func sanitizeFetchModelsError(err error, key string) error {
	if err == nil {
		return nil
	}

	// net/http includes the complete request URL in url.Error. Discovery routes
	// may put the API key in a custom query name or value, so never return that
	// wrapper to an API client.
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		err = urlErr.Err
	}

	message := err.Error()
	key = strings.TrimSpace(key)
	if key != "" {
		message = strings.ReplaceAll(message, key, "[REDACTED]")
		message = strings.ReplaceAll(message, url.QueryEscape(key), "[REDACTED]")
		message = strings.ReplaceAll(message, url.PathEscape(key), "[REDACTED]")
	}
	return errors.New(message)
}

func sanitizeAdvancedCustomRequestError(err error, key string, requestURL string) error {
	err = sanitizeFetchModelsError(err, key)
	if err == nil {
		return nil
	}
	parsedURL, parseErr := url.Parse(requestURL)
	if parseErr != nil {
		return err
	}
	message := err.Error()
	for _, value := range parsedURL.Query() {
		for _, secret := range value {
			if secret == "" {
				continue
			}
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
			message = strings.ReplaceAll(message, url.QueryEscape(secret), "[REDACTED]")
			message = strings.ReplaceAll(message, url.PathEscape(secret), "[REDACTED]")
		}
	}
	if key != "" {
		message = strings.ReplaceAll(message, key, "[REDACTED]")
		message = strings.ReplaceAll(message, url.QueryEscape(key), "[REDACTED]")
		message = strings.ReplaceAll(message, url.PathEscape(key), "[REDACTED]")
	}
	return errors.New(message)
}

func getFetchModelsResponseBody(method string, requestURL string, channel *model.Channel, headers http.Header) ([]byte, error) {
	request, err := http.NewRequest(method, requestURL, nil)
	if err != nil {
		return nil, err
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
		if strings.EqualFold(name, "Host") {
			request.Host = headers.Get(name)
		}
	}
	client, err := service.NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", response.StatusCode)
	}
	return io.ReadAll(response.Body)
}

func fetchChannelUpstreamModelIDs(channel *model.Channel) ([]string, error) {
	if channel.Type == constant.ChannelTypeTaskPlugin {
		plugin, ok := jsplugin.DefaultRegistry.Get(channel.GetSetting().TaskPluginKey)
		if !ok {
			return nil, fmt.Errorf("task plugin %q is not registered", channel.GetSetting().TaskPluginKey)
		}
		return normalizeModelNames(plugin.Meta.Models), nil
	}
	baseURL := constant.GetChannelBaseURL(channel.Type)
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	if channel.Type == constant.ChannelTypeOllama {
		key := strings.TrimSpace(strings.Split(channel.Key, "\n")[0])
		models, err := ollama.FetchOllamaModels(baseURL, key)
		if err != nil {
			return nil, err
		}
		return normalizeModelNames(lo.Map(models, func(item ollama.OllamaModel, _ int) string {
			return item.Name
		})), nil
	}

	if channel.Type == constant.ChannelTypeGemini {
		key, _, apiErr := channel.GetNextEnabledKey()
		if apiErr != nil {
			return nil, fmt.Errorf("获取渠道密钥失败: %w", apiErr)
		}
		key = strings.TrimSpace(key)
		models, err := gemini.FetchGeminiModels(baseURL, key, channel.GetSetting().Proxy)
		if err != nil {
			return nil, err
		}
		return normalizeModelNames(models), nil
	}

	if channel.Type == constant.ChannelTypeAdvancedCustom {
		return fetchAdvancedCustomUpstreamModelIDs(channel, baseURL)
	}

	if channel.Type == constant.ChannelTypeOpenCodeGo {
		return service.FetchOpenCodeGoModels()
	}

	if channel.Type == constant.ChannelTypeCodex {
		return service.FetchCodexChannelModels(channel)
	}

	var url string
	switch channel.Type {
	case constant.ChannelTypeAli:
		url = fmt.Sprintf("%s/compatible-mode/v1/models", baseURL)
	case constant.ChannelTypeZhipu_v4:
		if plan, _, ok := constant.ResolveSpecialPlan(channel.Type, baseURL, channel.GetOtherSettings().EndpointProfile); ok && plan.OpenAIBaseURL != "" {
			url = fmt.Sprintf("%s/models", plan.OpenAIBaseURL)
		} else {
			url = fmt.Sprintf("%s/api/paas/v4/models", baseURL)
		}
	case constant.ChannelTypeVolcEngine:
		if plan, _, ok := constant.ResolveSpecialPlan(channel.Type, baseURL, channel.GetOtherSettings().EndpointProfile); ok && plan.ClaudeBaseURL != "" {
			// doubao-coding-plan 的 OpenAIBaseURL 是 /api/coding/v3，直接拼 /v1/models
			// 会得到错误的 /api/coding/v3/v1/models；ClaudeBaseURL 是 /api/coding，
			// 正确端点为 /api/coding/v1/models。
			url = fmt.Sprintf("%s/v1/models", plan.ClaudeBaseURL)
		} else {
			url = fmt.Sprintf("%s/v1/models", baseURL)
		}
	case constant.ChannelTypeMoonshot:
		if plan, _, ok := constant.ResolveSpecialPlan(channel.Type, baseURL, channel.GetOtherSettings().EndpointProfile); ok && plan.OpenAIBaseURL != "" {
			url = fmt.Sprintf("%s/models", plan.OpenAIBaseURL)
		} else {
			url = fmt.Sprintf("%s/v1/models", baseURL)
		}
	case constant.ChannelTypeRadeonCloud:
		// 兼容渠道配置的 Base URL 带或不带 "/api" 前缀两种写法
		normalized := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/api")
		url = fmt.Sprintf("%s/api/v1/models", normalized)
	default:
		url = fmt.Sprintf("%s/v1/models", baseURL)
	}

	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return nil, fmt.Errorf("获取渠道密钥失败: %w", apiErr)
	}
	key = strings.TrimSpace(key)

	headers, err := buildFetchModelsHeaders(channel, key)
	if err != nil {
		return nil, sanitizeFetchModelsError(err, key)
	}

	body, err := getFetchModelsResponseBody(http.MethodGet, url, channel, headers)
	if err != nil {
		return nil, sanitizeAdvancedCustomRequestError(err, key, url)
	}

	var result OpenAIModelsResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	ids := lo.Map(result.Data, func(item OpenAIModel, _ int) string {
		if channel.Type == constant.ChannelTypeGemini {
			return strings.TrimPrefix(item.ID, "models/")
		}
		return item.ID
	})
	return normalizeModelNames(ids), nil
}

func fetchAdvancedCustomUpstreamModelIDs(channel *model.Channel, baseURL string) ([]string, error) {
	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return nil, fmt.Errorf("获取渠道密钥失败: %w", apiErr)
	}
	key = strings.TrimSpace(key)

	info := &relaycommon.RelayInfo{
		RelayFormat:    types.RelayFormatOpenAI,
		RelayMode:      relayconstant.RelayModeUnknown,
		RequestURLPath: dto.AdvancedCustomModelListPath,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:          constant.ChannelTypeAdvancedCustom,
			ChannelBaseUrl:       baseURL,
			ApiKey:               key,
			ChannelOtherSettings: channel.GetOtherSettings(),
		},
	}

	adaptor := &advancedcustom.Adaptor{}
	url, headers, err := adaptor.BuildModelListRequest(info)
	if err != nil {
		return nil, sanitizeFetchModelsError(err, key)
	}
	if err := applyFetchModelsHeaderOverrides(channel, key, headers); err != nil {
		return nil, sanitizeFetchModelsError(err, key)
	}

	body, err := getFetchModelsResponseBody(http.MethodGet, url, channel, headers)
	if err != nil {
		return nil, sanitizeFetchModelsError(err, key)
	}
	return parseOpenAIModelIDs(body)
}

// persistChannelUpstreamModelSettings 在事务 + 行锁内对 channel 行执行
// read-modify-write：先 SELECT ... FOR UPDATE（SQLite 自动跳过）锁读最新行，
// 再写入由锁内计算产出的 settings/models。对比直接 model.DB.Updates 的旧写法，
// 这保证并发巡检/手动应用等写者基于最新行计算，不会互相覆盖丢更新。
// settings 在事务内基于锁读行的最新 OtherSettings 派生，调用方可通过
// persistChannelUpstreamModelSettings 的输出拿到该派生值。
//
// NOTICE：行锁代码一律走 model.LockForUpdate（导出自 model.lockForUpdate），
// 不要在任何调用点重复 clause.Locking（GORM v1 的 Set("gorm:query_option")
// 在 v2 下静默失效）。
func checkAndPersistChannelUpstreamModelUpdates(
	channel *model.Channel,
	settings *dto.ChannelOtherSettings,
	force bool,
	allowAutoApply bool,
) (modelsChanged bool, autoAdded int, err error) {
	now := common.GetTimestamp()
	if !force {
		minInterval := getUpstreamModelUpdateMinCheckIntervalSeconds()
		if settings.UpstreamModelUpdateLastCheckTime > 0 &&
			now-settings.UpstreamModelUpdateLastCheckTime < minInterval {
			return false, 0, nil
		}
	}

	// 上游模型列表拉取（HTTP 往返）不需要行锁；行锁只保护对 channel 行的读-改-写。
	pendingAddModels, pendingRemoveModels, fetchErr := collectPendingUpstreamModelChanges(channel, *settings)

	// 巡检失败同样推进 last_check_time。事务内锁读最新行，基于最新 settings
	// 改写，避免覆盖并发写入的其它设置段。
	if fetchErr != nil {
		settings.UpstreamModelUpdateLastCheckTime = now
		if persistErr := model.DB.Transaction(func(tx *gorm.DB) error {
			var latest model.Channel
			if lockErr := model.LockForUpdate(tx).Where("id = ?", channel.Id).First(&latest).Error; lockErr != nil {
				return lockErr
			}
			s := latest.GetOtherSettings()
			s.UpstreamModelUpdateLastCheckTime = now
			latest.SetOtherSettings(s)
			*settings = s
			return tx.Model(&model.Channel{}).Where("id = ?", latest.Id).Update("settings", latest.OtherSettings).Error
		}); persistErr != nil {
			return false, 0, persistErr
		}
		return false, 0, fetchErr
	}

	// 锁内完成 读最新行 → 计算 next 状态 → 写 的整段 read-modify-write：
	// MySQL/PostgreSQL 下 SELECT ... FOR UPDATE 让同一行的并发写者串行化，
	// 后到者基于最新行重新计算，不会覆盖先到者的更新；SQLite 自动跳过
	//（单写者模型，冲突事务直接失败而非双双提交）。
	if writeErr := model.DB.Transaction(func(tx *gorm.DB) error {
		var latest model.Channel
		if lockErr := model.LockForUpdate(tx).Where("id = ?", channel.Id).First(&latest).Error; lockErr != nil {
			return lockErr
		}
		s := latest.GetOtherSettings()
		s.UpstreamModelUpdateLastCheckTime = now
		s.UpstreamModelUpdateLastRemovedModels = pendingRemoveModels
		if allowAutoApply && s.UpstreamModelUpdateAutoSyncEnabled && len(pendingAddModels) > 0 {
			originModels := normalizeModelNames(latest.GetModels())
			mergedModels := mergeModelNames(originModels, pendingAddModels)
			if len(mergedModels) > len(originModels) {
				latest.Models = strings.Join(mergedModels, ",")
				autoAdded = len(mergedModels) - len(originModels)
				modelsChanged = true
			}
			s.UpstreamModelUpdateLastDetectedModels = []string{}
		} else {
			s.UpstreamModelUpdateLastDetectedModels = pendingAddModels
		}
		latest.SetOtherSettings(s)
		updates := map[string]interface{}{
			"settings": latest.OtherSettings,
		}
		if modelsChanged {
			updates["models"] = latest.Models
		}
		if updateErr := tx.Model(&model.Channel{}).Where("id = ?", latest.Id).Updates(updates).Error; updateErr != nil {
			return updateErr
		}
		// 回填：调用方（后台巡检 / Detect 接口）依赖 settings 展示本次检测结果
		*settings = s
		return nil
	}); writeErr != nil {
		return false, autoAdded, writeErr
	}

	if modelsChanged {
		if err = channel.UpdateAbilities(nil); err != nil {
			return true, autoAdded, err
		}
	}
	return modelsChanged, autoAdded, nil
}

func refreshChannelRuntimeCache() {
	if common.MemoryCacheEnabled {
		func() {
			defer func() {
				if r := recover(); r != nil {
					common.SysLog(fmt.Sprintf("InitChannelCache panic: %v", r))
				}
			}()
			model.InitChannelCache()
		}()
	}
}

func shouldSendUpstreamModelUpdateNotification(now int64, changedChannels int, failedChannels int) bool {
	if changedChannels <= 0 && failedChannels <= 0 {
		return true
	}

	channelUpstreamModelUpdateNotifyState.Lock()
	defer channelUpstreamModelUpdateNotifyState.Unlock()

	if channelUpstreamModelUpdateNotifyState.lastNotifiedAt > 0 &&
		now-channelUpstreamModelUpdateNotifyState.lastNotifiedAt < channelUpstreamModelUpdateNotifySuppressWindowSeconds &&
		channelUpstreamModelUpdateNotifyState.lastChangedChannels == changedChannels &&
		channelUpstreamModelUpdateNotifyState.lastFailedChannels == failedChannels {
		return false
	}

	channelUpstreamModelUpdateNotifyState.lastNotifiedAt = now
	channelUpstreamModelUpdateNotifyState.lastChangedChannels = changedChannels
	channelUpstreamModelUpdateNotifyState.lastFailedChannels = failedChannels
	return true
}

func buildUpstreamModelUpdateTaskNotificationContent(
	checkedChannels int,
	changedChannels int,
	detectedAddModels int,
	detectedRemoveModels int,
	autoAddedModels int,
	failedChannelIDs []int,
	channelSummaries []upstreamModelUpdateChannelSummary,
	addModelSamples []string,
	removeModelSamples []string,
) string {
	var builder strings.Builder
	failedChannels := len(failedChannelIDs)
	builder.WriteString(fmt.Sprintf(
		"上游模型巡检摘要：检测渠道 %d 个，发现变更 %d 个，新增 %d 个，删除 %d 个，自动同步新增 %d 个，失败 %d 个。",
		checkedChannels,
		changedChannels,
		detectedAddModels,
		detectedRemoveModels,
		autoAddedModels,
		failedChannels,
	))

	if len(channelSummaries) > 0 {
		displayCount := min(len(channelSummaries), channelUpstreamModelUpdateNotifyMaxChannelDetails)
		builder.WriteString(fmt.Sprintf("\n\n变更渠道明细（展示 %d/%d）：", displayCount, len(channelSummaries)))
		for _, summary := range channelSummaries[:displayCount] {
			builder.WriteString(fmt.Sprintf("\n- %s (+%d / -%d)", summary.ChannelName, summary.AddCount, summary.RemoveCount))
		}
		if len(channelSummaries) > displayCount {
			builder.WriteString(fmt.Sprintf("\n- 其余 %d 个渠道已省略", len(channelSummaries)-displayCount))
		}
	}

	normalizedAddModelSamples := normalizeModelNames(addModelSamples)
	if len(normalizedAddModelSamples) > 0 {
		displayCount := min(len(normalizedAddModelSamples), channelUpstreamModelUpdateNotifyMaxModelDetails)
		builder.WriteString(fmt.Sprintf("\n\n新增模型示例（展示 %d/%d）：%s",
			displayCount,
			len(normalizedAddModelSamples),
			strings.Join(normalizedAddModelSamples[:displayCount], ", "),
		))
		if len(normalizedAddModelSamples) > displayCount {
			builder.WriteString(fmt.Sprintf("（其余 %d 个已省略）", len(normalizedAddModelSamples)-displayCount))
		}
	}

	normalizedRemoveModelSamples := normalizeModelNames(removeModelSamples)
	if len(normalizedRemoveModelSamples) > 0 {
		displayCount := min(len(normalizedRemoveModelSamples), channelUpstreamModelUpdateNotifyMaxModelDetails)
		builder.WriteString(fmt.Sprintf("\n\n删除模型示例（展示 %d/%d）：%s",
			displayCount,
			len(normalizedRemoveModelSamples),
			strings.Join(normalizedRemoveModelSamples[:displayCount], ", "),
		))
		if len(normalizedRemoveModelSamples) > displayCount {
			builder.WriteString(fmt.Sprintf("（其余 %d 个已省略）", len(normalizedRemoveModelSamples)-displayCount))
		}
	}

	if failedChannels > 0 {
		displayCount := min(failedChannels, channelUpstreamModelUpdateNotifyMaxFailedChannelIDs)
		displayIDs := lo.Map(failedChannelIDs[:displayCount], func(channelID int, _ int) string {
			return fmt.Sprintf("%d", channelID)
		})
		builder.WriteString(fmt.Sprintf(
			"\n\n失败渠道 ID（展示 %d/%d）：%s",
			displayCount,
			failedChannels,
			strings.Join(displayIDs, ", "),
		))
		if failedChannels > displayCount {
			builder.WriteString(fmt.Sprintf("（其余 %d 个已省略）", failedChannels-displayCount))
		}
	}
	return builder.String()
}

type upstreamModelUpdateSummary struct {
	CheckedChannels      int `json:"checked_channels"`
	ChangedChannels      int `json:"changed_channels"`
	DetectedAddModels    int `json:"detected_add_models"`
	DetectedRemoveModels int `json:"detected_remove_models"`
	FailedChannels       int `json:"failed_channels"`
	AutoAddedModels      int `json:"auto_added_models"`
}

// runChannelUpstreamModelUpdateTaskOnce runs one synchronous upstream model
// detection cycle and returns a summary for system task history. It honors ctx
// cancellation between batches so a runner that loses its lease stops promptly.
// force bypasses the per-channel minimum check interval and allowAutoApply lets
// channels with auto-sync enabled adopt detected models automatically. The
// scheduled job calls (force=false, allowAutoApply=true); the manual "detect
// all" trigger calls (force=true, allowAutoApply=false) so it always re-checks
// and only stages changes for explicit review.
func runChannelUpstreamModelUpdateTaskOnce(ctx context.Context, force bool, allowAutoApply bool, report func(processed, total int)) upstreamModelUpdateSummary {
	checkedChannels := 0
	failedChannels := 0
	failedChannelIDs := make([]int, 0)
	changedChannels := 0
	detectedAddModels := 0
	detectedRemoveModels := 0
	autoAddedModels := 0
	channelSummaries := make([]upstreamModelUpdateChannelSummary, 0)
	addModelSamples := make([]string, 0)
	removeModelSamples := make([]string, 0)
	refreshNeeded := false

	// Count the enabled channels up front so progress can be reported as a
	// percentage; a count error is non-fatal (progress just won't show a %).
	var totalChannels int64
	if err := model.DB.Model(&model.Channel{}).Where("status = ?", common.ChannelStatusEnabled).Count(&totalChannels).Error; err != nil {
		totalChannels = 0
	}
	processed := 0

	lastID := 0
scanLoop:
	for {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		var channels []*model.Channel
		query := model.DB.
			Select(channelUpstreamModelUpdateSelectColumns()).
			Where("status = ?", common.ChannelStatusEnabled).
			Order("id asc").
			Limit(channelUpstreamModelUpdateTaskBatchSize)
		if lastID > 0 {
			query = query.Where("id > ?", lastID)
		}
		err := query.Find(&channels).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("upstream model update task query failed: %v", err))
			break
		}
		if len(channels) == 0 {
			break
		}
		lastID = channels[len(channels)-1].Id

		for _, channel := range channels {
			if channel == nil {
				continue
			}
			if ctx != nil && ctx.Err() != nil {
				break scanLoop
			}

			processed++
			if report != nil {
				report(processed, int(totalChannels))
			}

			settings := channel.GetOtherSettings()
			if !settings.UpstreamModelUpdateCheckEnabled {
				continue
			}

			checkedChannels++
			modelsChanged, autoAdded, err := checkAndPersistChannelUpstreamModelUpdates(channel, &settings, force, allowAutoApply)
			if err != nil {
				failedChannels++
				failedChannelIDs = append(failedChannelIDs, channel.Id)
				common.SysLog(fmt.Sprintf("upstream model update check failed: channel_id=%d channel_name=%s err=%v", channel.Id, channel.Name, err))
				continue
			}
			currentAddModels := normalizeModelNames(settings.UpstreamModelUpdateLastDetectedModels)
			currentRemoveModels := normalizeModelNames(settings.UpstreamModelUpdateLastRemovedModels)
			currentAddCount := len(currentAddModels) + autoAdded
			currentRemoveCount := len(currentRemoveModels)
			detectedAddModels += currentAddCount
			detectedRemoveModels += currentRemoveCount
			if currentAddCount > 0 || currentRemoveCount > 0 {
				changedChannels++
				channelSummaries = append(channelSummaries, upstreamModelUpdateChannelSummary{
					ChannelName: channel.Name,
					AddCount:    currentAddCount,
					RemoveCount: currentRemoveCount,
				})
			}
			addModelSamples = mergeModelNames(addModelSamples, currentAddModels)
			removeModelSamples = mergeModelNames(removeModelSamples, currentRemoveModels)
			if modelsChanged {
				refreshNeeded = true
			}
			autoAddedModels += autoAdded

			if common.RequestInterval > 0 {
				if ctx == nil {
					time.Sleep(common.RequestInterval)
				} else {
					select {
					case <-ctx.Done():
						break scanLoop
					case <-time.After(common.RequestInterval):
					}
				}
			}
		}

		if len(channels) < channelUpstreamModelUpdateTaskBatchSize {
			break
		}
	}

	if report != nil && (ctx == nil || ctx.Err() == nil) {
		report(int(totalChannels), int(totalChannels)) // mark complete only when the full scan finished
	}

	if refreshNeeded {
		refreshChannelRuntimeCache()
	}

	summary := upstreamModelUpdateSummary{
		CheckedChannels:      checkedChannels,
		ChangedChannels:      changedChannels,
		DetectedAddModels:    detectedAddModels,
		DetectedRemoveModels: detectedRemoveModels,
		FailedChannels:       failedChannels,
		AutoAddedModels:      autoAddedModels,
	}

	if checkedChannels > 0 || common.DebugEnabled {
		common.SysLog(fmt.Sprintf(
			"upstream model update task done: checked_channels=%d changed_channels=%d detected_add_models=%d detected_remove_models=%d failed_channels=%d auto_added_models=%d",
			checkedChannels,
			changedChannels,
			detectedAddModels,
			detectedRemoveModels,
			failedChannels,
			autoAddedModels,
		))
	}
	if changedChannels > 0 || failedChannels > 0 {
		now := common.GetTimestamp()
		if !shouldSendUpstreamModelUpdateNotification(now, changedChannels, failedChannels) {
			common.SysLog(fmt.Sprintf(
				"upstream model update notification skipped in 24h window: changed_channels=%d failed_channels=%d",
				changedChannels,
				failedChannels,
			))
			return summary
		}
		service.NotifyUpstreamModelUpdateWatchers(
			"上游模型巡检通知",
			buildUpstreamModelUpdateTaskNotificationContent(
				checkedChannels,
				changedChannels,
				detectedAddModels,
				detectedRemoveModels,
				autoAddedModels,
				failedChannelIDs,
				channelSummaries,
				addModelSamples,
				removeModelSamples,
			),
		)
	}
	return summary
}

func ApplyChannelUpstreamModelUpdates(c *gin.Context) {
	var req applyChannelUpstreamModelUpdatesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ID <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid channel id",
		})
		return
	}

	channel, err := model.GetChannelById(req.ID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	beforeSettings := channel.GetOtherSettings()
	ignoredModels := intersectModelNames(req.IgnoreModels, beforeSettings.UpstreamModelUpdateLastDetectedModels)

	addedModels, removedModels, remainingModels, remainingRemoveModels, modelsChanged, err := applyChannelUpstreamModelUpdates(
		channel,
		req.AddModels,
		req.IgnoreModels,
		req.RemoveModels,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if modelsChanged {
		refreshChannelRuntimeCache()
	}

	recordManageAudit(c, "channel.upstream_apply", map[string]interface{}{
		"id": channel.Id,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id":                      channel.Id,
			"added_models":            addedModels,
			"removed_models":          removedModels,
			"ignored_models":          ignoredModels,
			"remaining_models":        remainingModels,
			"remaining_remove_models": remainingRemoveModels,
			"models":                  channel.Models,
			"settings":                channel.OtherSettings,
		},
	})
}

func DetectChannelUpstreamModelUpdates(c *gin.Context) {
	var req applyChannelUpstreamModelUpdatesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ID <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid channel id",
		})
		return
	}

	channel, err := model.GetChannelById(req.ID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	settings := channel.GetOtherSettings()
	modelsChanged, autoAdded, err := checkAndPersistChannelUpstreamModelUpdates(channel, &settings, true, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if modelsChanged {
		refreshChannelRuntimeCache()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": detectChannelUpstreamModelUpdatesResult{
			ChannelID:       channel.Id,
			ChannelName:     channel.Name,
			AddModels:       normalizeModelNames(settings.UpstreamModelUpdateLastDetectedModels),
			RemoveModels:    normalizeModelNames(settings.UpstreamModelUpdateLastRemovedModels),
			LastCheckTime:   settings.UpstreamModelUpdateLastCheckTime,
			AutoAddedModels: autoAdded,
		},
	})
}

func applyChannelUpstreamModelUpdates(
	channel *model.Channel,
	addModelsInput []string,
	ignoreModelsInput []string,
	removeModelsInput []string,
) (
	addedModels []string,
	removedModels []string,
	remainingModels []string,
	remainingRemoveModels []string,
	modelsChanged bool,
	err error,
) {
	// 锁内完成 读最新行 → 计算 next 状态 → 写 的整段 read-modify-write，
	// 防止并发巡检/手动应用互覆盖：MySQL/PostgreSQL 下 FOR UPDATE 让并发写者
	// 串行化，后到者基于锁读的最新行重新计算（不会用旧快照覆盖新状态）；
	// SQLite 自动跳过（单写者模型）。行锁统一走 model.LockForUpdate。
	if txErr := model.DB.Transaction(func(tx *gorm.DB) error {
		var latest model.Channel
		if lockErr := model.LockForUpdate(tx).Where("id = ?", channel.Id).First(&latest).Error; lockErr != nil {
			return lockErr
		}

		settings := latest.GetOtherSettings()
		pendingAddModels := normalizeModelNames(settings.UpstreamModelUpdateLastDetectedModels)
		pendingRemoveModels := normalizeModelNames(settings.UpstreamModelUpdateLastRemovedModels)
		addModels := intersectModelNames(addModelsInput, pendingAddModels)
		ignoreModels := intersectModelNames(ignoreModelsInput, pendingAddModels)
		removedModels = intersectModelNames(removeModelsInput, pendingRemoveModels)
		removedModels = subtractModelNames(removedModels, addModels)

		originModels := normalizeModelNames(latest.GetModels())
		nextModels := applySelectedModelChanges(originModels, addModels, removedModels)
		changed := !slices.Equal(originModels, nextModels)

		settings.UpstreamModelUpdateIgnoredModels = mergeModelNames(settings.UpstreamModelUpdateIgnoredModels, ignoreModels)
		if len(addModels) > 0 {
			settings.UpstreamModelUpdateIgnoredModels = subtractModelNames(settings.UpstreamModelUpdateIgnoredModels, addModels)
		}
		remainingModels = subtractModelNames(pendingAddModels, append(addModels, ignoreModels...))
		remainingRemoveModels = subtractModelNames(pendingRemoveModels, removedModels)
		settings.UpstreamModelUpdateLastDetectedModels = remainingModels
		settings.UpstreamModelUpdateLastRemovedModels = remainingRemoveModels
		settings.UpstreamModelUpdateLastCheckTime = common.GetTimestamp()

		latest.SetOtherSettings(settings)
		updates := map[string]interface{}{
			"settings": latest.OtherSettings,
		}
		if changed {
			latest.Models = strings.Join(nextModels, ",")
			updates["models"] = latest.Models
		}
		if updateErr := tx.Model(&model.Channel{}).Where("id = ?", latest.Id).Updates(updates).Error; updateErr != nil {
			return updateErr
		}

		// 回填：调用方（ApplyChannelUpstreamModelUpdates 响应）依赖 channel 最新状态展示
		channel.SetOtherSettings(settings)
		channel.Models = latest.Models
		addedModels = addModels
		modelsChanged = changed
		return nil
	}); txErr != nil {
		return nil, nil, nil, nil, false, txErr
	}

	if modelsChanged {
		if err := channel.UpdateAbilities(nil); err != nil {
			return addedModels, removedModels, remainingModels, remainingRemoveModels, true, err
		}
	}
	return addedModels, removedModels, remainingModels, remainingRemoveModels, modelsChanged, nil
}

func collectPendingApplyUpstreamModelChanges(settings dto.ChannelOtherSettings) (pendingAddModels []string, pendingRemoveModels []string) {
	return normalizeModelNames(settings.UpstreamModelUpdateLastDetectedModels), normalizeModelNames(settings.UpstreamModelUpdateLastRemovedModels)
}

func findEnabledChannelsAfterID(lastID int, batchSize int) ([]*model.Channel, error) {
	var channels []*model.Channel
	query := model.DB.
		Select(channelUpstreamModelUpdateSelectColumns()).
		Where("status = ?", common.ChannelStatusEnabled).
		Order("id asc").
		Limit(batchSize)
	if lastID > 0 {
		query = query.Where("id > ?", lastID)
	}
	return channels, query.Find(&channels).Error
}

func ApplyAllChannelUpstreamModelUpdates(c *gin.Context) {
	results := make([]applyAllChannelUpstreamModelUpdatesResult, 0)
	failed := make([]int, 0)
	refreshNeeded := false
	addedModelCount := 0
	removedModelCount := 0

	lastID := 0
	for {
		channels, err := findEnabledChannelsAfterID(lastID, channelUpstreamModelUpdateTaskBatchSize)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if len(channels) == 0 {
			break
		}
		lastID = channels[len(channels)-1].Id

		for _, channel := range channels {
			if channel == nil {
				continue
			}

			settings := channel.GetOtherSettings()
			if !settings.UpstreamModelUpdateCheckEnabled {
				continue
			}

			pendingAddModels, pendingRemoveModels := collectPendingApplyUpstreamModelChanges(settings)
			if len(pendingAddModels) == 0 && len(pendingRemoveModels) == 0 {
				continue
			}

			addedModels, removedModels, remainingModels, remainingRemoveModels, modelsChanged, err := applyChannelUpstreamModelUpdates(
				channel,
				pendingAddModels,
				nil,
				pendingRemoveModels,
			)
			if err != nil {
				failed = append(failed, channel.Id)
				continue
			}
			if modelsChanged {
				refreshNeeded = true
			}
			addedModelCount += len(addedModels)
			removedModelCount += len(removedModels)
			results = append(results, applyAllChannelUpstreamModelUpdatesResult{
				ChannelID:             channel.Id,
				ChannelName:           channel.Name,
				AddedModels:           addedModels,
				RemovedModels:         removedModels,
				RemainingModels:       remainingModels,
				RemainingRemoveModels: remainingRemoveModels,
			})
		}

		if len(channels) < channelUpstreamModelUpdateTaskBatchSize {
			break
		}
	}

	if refreshNeeded {
		refreshChannelRuntimeCache()
	}

	recordManageAudit(c, "channel.upstream_apply_all", map[string]interface{}{
		"count": len(results),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"processed_channels": len(results),
			"added_models":       addedModelCount,
			"removed_models":     removedModelCount,
			"failed_channel_ids": failed,
			"results":            results,
		},
	})
}

// DetectAllChannelUpstreamModelUpdates enqueues a model_update system task
// (manual variant) instead of scanning inline. Routing the manual trigger
// through the framework gives it the same cross-instance lease dedup and run
// history as the scheduled scan. If any model_update task is already active, the
// manual run is rejected so the caller does not mistake a scheduled run for this
// manual one.
func DetectAllChannelUpstreamModelUpdates(c *gin.Context) {
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeModelUpdate, modelUpdateTaskPayload{Manual: true})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "已有模型更新任务正在运行或等待中，不能启动本次手动任务",
			"data": gin.H{
				"task_id": task.TaskID,
				"status":  task.Status,
				"type":    task.Type,
			},
		})
		return
	}

	recordManageAudit(c, "channel.upstream_detect_all", map[string]interface{}{
		"task_id": task.TaskID,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"task_id": task.TaskID,
			"status":  task.Status,
		},
	})
}
