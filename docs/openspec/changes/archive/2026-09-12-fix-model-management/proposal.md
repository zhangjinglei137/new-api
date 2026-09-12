# Proposal: 修复模型管理三个问题

## 问题描述

用户报告模型管理相关三个问题：

### 1. 删除模型报错 `Cannot read properties of null (reading 'deleted_count')`

在模型列表删除单个模型（勾选或不勾选「同时从渠道移除」「同时删除价格」）后，前端在成功回调中访问 `result.deleted_count` 崩溃。

### 2. 模型同步资料报错「没有符合筛选条件的模型」

打开「同步模型资料」向导，无论选择 OpenCode Go 还是 Official 来源、任何语言，点击「Load metadata preview」后都提示「没有符合筛选条件的模型 / 当前条件下没有可同步的模型」。

### 3. 价格同步预设文案

「模型价格同步」中 opencode-go 预设显示 `OpenCode Go pricing preset` 英文原文，期望中文环境显示「OpenCode Go 价格预设」。

## 根因分析

### 1. 删除模型崩溃

- 前端 `web/src/features/models/components/dialogs/model-delete-dialog.tsx:83` 在 mutation `onSuccess` 中读取 `result.deleted_count`（`result = response.data`）。
- 后端单删接口 `DELETE /api/models/:id`（`controller/model_meta.go` 的 `DeleteModelMeta`）执行成功后返回 `common.ApiSuccess(c, nil)`，即 `data: null` → 前端 `result` 为 `null` → 访问 `result.deleted_count` 抛出 TypeError。
- 前端批量删除调用 `POST /api/models/delete`（`web/src/features/models/api.ts` 的 `deleteModels`），但后端 router（`router/api-router.go:391-402`）**没有注册该路由** → 批量删除会 404。
- 后端 `DeleteModelMeta` 完全未解析前端传递的 `remove_from_channels`、`remove_pricing` 查询参数，勾选对应选项不会生效（功能断链）。

### 2. 模型同步资料异常

- 前端 `sync-wizard-dialog.tsx`（融合自上游重构版）期望预览接口返回 `{ source: {..., version}, candidates: MetadataSyncCandidate[] }`，并提交 `{ locale, source_version, selections }`。
- 后端 `SyncUpstreamPreview`（`controller/model_sync.go:930`）仍返回旧契约 `{ missing, conflicts, source }`，**没有 `candidates` 字段** → 前端 `preview.candidates` 恒为 `undefined` → 候选列表永远为空 → 提示「没有符合筛选条件的模型」。
- 后端 `SyncUpstreamModels` 接收旧契约 `{ overwrite, locale, source }`，与前端提交的 `{ locale, source_version, selections }` 错位 → 即使预览通过，应用阶段同样失败。
- 本地 `model/model_metadata_sync.go` 已完整实现上游契约（`ApplyMetadataSync`、`GetMetadataSyncState`、`MetadataRecordVersion`、`MetadataSyncFields` 等），仅 controller 层未升级。

### 3. 价格预设文案

- `web/src/features/system-settings/models/upstream-ratio-sync-helpers.ts:58` 使用 `t('OpenCode Go pricing preset')`。
- 7 个 locale 文件（en/zh/zh-TW/fr/ru/ja/vi）均无该翻译键 → 回退显示英文原文。

## 修复目标

1. 删除模型（单个与批量）正常完成，成功提示正确显示删除数量；勾选「同时从渠道移除」「同时删除价格」时后端真正执行对应操作。
2. 模型同步向导能正确加载上游模型预览（candidates 列表），并能提交 selections 完成创建/更新；保留本地 opencode-go 来源扩展能力。
3. 价格同步预设文案按语言正确翻译（zh 显示「OpenCode Go 价格预设」）。
