# Tasks: 修复模型管理三个问题

## Bug 1 — 删除模型崩溃与批量删除断链

- [x] 1.1 `model/ability.go`：新增按模型名删除 abilities 并统计去重渠道数的方法
- [x] 1.2 `model/model_pricing_config.go`：新增删除指定模型价格配置的方法（事务内从各 pricing option 删除）
- [x] 1.3 `controller/model_meta.go`：重写 `DeleteModelMeta`（解析 remove_from_channels/remove_pricing，返回 deleted_count/updated_channels）
- [x] 1.4 `controller/model_meta.go`：新增 `BatchDeleteModels`（接收 model_ids + 两个布尔参数，累计统计返回）
- [x] 1.5 `router/api-router.go`：注册 `POST /api/models/delete` 路由
- [x] 1.6 `web/.../model-delete-dialog.tsx`：onSuccess 对 result 做防御性兜底

## Bug 2 — 模型同步契约升级

- [x] 2.1 `controller/model_sync.go`：移植 `fetchMetadataCatalog`（llm-metadata 目录 + source.Version 计算）
- [x] 2.2 `controller/model_sync.go`：opencode-go 来源融合（FetchOpenCodeGoModelEntries + vendor 映射 + 富元数据）
- [x] 2.3 `controller/model_sync.go`：重写 `SyncUpstreamPreview` 返回 candidates（含 record_version/scope/fields 差异）
- [x] 2.4 `controller/model_sync.go`：重写 `SyncUpstreamModels` 接收 selections + source_version 校验，调 `model.ApplyMetadataSync`
- [x] 2.5 `model/model_metadata_sync.go`：`MetadataSyncUpdate` 增加 RichFields 透传，创建模型时写入富元数据（保留 opencode-go 能力）
- [x] 2.6 清理旧契约残留（overwriteField/buildSyncSourceInfo/missingRichFieldsMap/ensureVendorID 等，保留 GetMissingModels 接口）

## Bug 3 — 价格预设文案 i18n

- [x] 3.1 在 zh/zh-TW/fr/ru/ja/vi locale 补充 `OpenCode Go pricing preset` 翻译键并运行 `bun run i18n:sync`
- [x] 3.2 检查/更新 `upstream-ratio-sync-helpers.test.ts` 断言（t 为 identity，无需修改；3/3 通过）

## 验证

- [x] 4.1 后端：重写 controller 单测（同步契约 6 个新用例），`go build ./...` + `go test ./model/... ./controller/...` 通过
- [x] 4.2 前端：`bun run typecheck`、受影响测试 21/21、`bun run build` 通过
- [x] 4.3 根因消除检查：deleted_count 崩溃路径已容错、candidates/selections 契约对齐、7 语言文案键生效
