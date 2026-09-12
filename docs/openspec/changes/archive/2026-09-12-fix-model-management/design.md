# Design: 修复模型管理三个问题

## 修复方案

### 1. 删除模型（单个 + 批量）

**后端 `model` 层**：
- `model/ability.go` 新增 `DeleteAbilitiesByModel(modelName string) (channelCount int64, err error)`：`WHERE model = ?` 删除 abilities，按去重渠道统计受影响渠道数。
- `model/model_pricing_config.go` 新增 `DeleteModelPricingConfig(modelName string) error`：在 `mutateModelPricingOptions` 事务内把该模型从所有 pricing option maps 中删除，恢复内置定价。

**后端 `controller/model_meta.go`**：
- 重写 `DeleteModelMeta`：解析查询参数 `remove_from_channels`（bool）、`remove_pricing`（bool）；删除模型记录后，按参数执行渠道 abilities 清理与价格配置删除；返回 `gin.H{"deleted_count": 1, "updated_channels": n}`。
- 新增 `BatchDeleteModels`：接收 `{ model_ids: []int, remove_from_channels: bool, remove_pricing: bool }`；逐模型删除并累计统计；返回 `gin.H{"deleted_count": n, "updated_channels": m}`。
- 删除后统一 `model.RefreshPricing()`。

**后端 `router/api-router.go`**：在 modelsRoute 下注册 `POST /delete` → `BatchDeleteModels`。

**前端 `model-delete-dialog.tsx`**：`onSuccess` 中对 `result` 做防御性兜底（`result?.deleted_count ?? props.models.length`），避免后端异常结构导致崩溃。

### 2. 模型同步契约升级（controller/model_sync.go）

参照上游 new-api 实现，将 preview / apply 升级为 candidates / selections 契约，并融合本地 opencode-go 来源扩展：

- **`fetchMetadataCatalog`**（上游移植）：拉取 llm-metadata 目录（models + vendors），组装 `model.MetadataValues`，用 `sha256(locale, models, vendors)` 计算 `source.Version`；同时校验 `normalizeLocale` 与 `ValidateMetadataValues`。
- **opencode-go 来源融合**：`fetchMetadataCatalog` 增加 source 参数；当 `source == "opencode-go"` 时改用 `service.FetchOpenCodeGoModelEntries()` 拉取 models.opencode.ai，模型/供应商转换逻辑复用本地既有 `openCodeGoVendorForModelAndProvider`、`endpointsJSON`、`openCodeGoStatusToModelStatus` 等，富元数据字段映射为 `MetadataValues`（仅 description/icon/tags/vendor/endpoints/name_rule/status 七字段契约；富字段由 opencode-go 模型创建逻辑保留——见下）。
- **`SyncUpstreamPreview` 重写**：返回 `{ source, candidates }`。candidates 按上游逻辑生成：`site`（本地存在或缺失模型）/`catalog`（全部上游名）scope；kind ∈ create/update/unchanged/blocked/missing_upstream/missing_vendor；`record_version = model.MetadataRecordVersion(local, localVendor, FindMetadataVendor(...))`；fields 为七字段的 local/upstream 差异。
- **`SyncUpstreamModels` 重写**：接收 `{ locale, source_version, selections }`；重新拉取目录并校验 `source.Version == source_version`（不一致返回 409）；组装 `[]model.MetadataSyncUpdate` 调 `model.ApplyMetadataSync(updates, vendors)`；返回 `MetadataSyncResult`。
- **opencode-go 富字段保留**：本地 opencode-go 模型创建时含 DisplayName/Family/ProviderNpm/ReleaseDate/LastUpdated/能力字段等富元数据。新契约 `ApplyMetadataSync` 只写七字段 + 新建 vendor。为保留本地能力，在 opencode-go 来源的 candidates `create` 分支中，将富字段并入创建（方案：升级 `model.ApplyMetadataSync` 增加可选 `RichFields map[string]any` 透传，或 controller 在 selections 中携带富字段——倾向在 model 层 `MetadataSyncSelection` 增加可选 `rich_fields` 字段，`ApplyMetadataSync` 创建时一并写入，保持上游七字段契约不变）。
- **旧函数清理**：`SyncUpstreamModels`/`SyncUpstreamPreview` 的旧 missing/conflicts/overwrite 逻辑、`buildSyncSourceInfo`、`missingRichFieldsMap`、`overwriteField` 等随重写移除或保留给 `GetMissingModels`（`/api/models/missing` 独立接口不受影响）。

### 3. 价格预设文案 i18n

- 在 `web/src/i18n/locales/{zh,zh-TW,fr,ru,ja,vi}.json` 补充 `OpenCode Go pricing preset` 翻译键（en 为 base 键即原文）。
  - zh: `OpenCode Go 价格预设`；zh-TW: `OpenCode Go 價格預設`；其余语言给出对应翻译。
- 运行 `bun run i18n:sync` 同步。
- 检查 `upstream-ratio-sync-helpers.test.ts` 断言是否需要适配。

## 验证方式

- 后端：`go build ./...`、相关 controller 单测（model_sync_test.go 契约测试更新、删除模型测试）。
- 前端：`bun run typecheck`、`bun run lint`、相关测试（model-deletion、metadata-sync、upstream-ratio-sync-helpers）、`bun run build`。
- 手工：删除模型弹窗、同步向导预览/应用、价格预设文案。
