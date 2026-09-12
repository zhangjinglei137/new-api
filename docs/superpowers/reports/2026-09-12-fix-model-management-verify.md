# 验证报告：fix-model-management

日期：2026-09-12
Change：fix-model-management（hotfix）
验证模式：full（17 任务，超 3 阈值）
产物语言：zh-CN

## 摘要

| 维度 | 状态 |
|------|------|
| 完整性 | 17/17 任务完成，无 delta spec（hotfix 不要求） |
| 正确性 | 三个 bug 的根因路径均已消除，契约对齐，测试全绿 |
| 一致性 | 实现与 design.md 方案一致，无 Design Doc（hotfix 预设） |

## 检查项结果

| # | 检查项 | 结果 | 证据 |
|---|--------|------|------|
| 1 | tasks.md 全部任务完成 | PASS | 17 项全部 `[x]` |
| 2 | 实现符合 design.md 高层设计决策 | PASS | 见下方逐项对照 |
| 3 | 实现符合 Design Doc | N/A | hotfix 预设无 Design Doc（docs/superpowers/specs/ 无关联文档） |
| 4 | 能力规格场景全部通过 | N/A | 无 delta spec |
| 5 | proposal.md 目标已满足 | PASS | 三项目标全部达成 |
| 6 | delta spec 与 design doc 无矛盾 | N/A | 无 delta spec |
| 7 | 关联设计文档可定位 | N/A | 无 |

## 逐项对照（design.md 方案 ↔ 实现）

### 方案 1：删除模型（单个 + 批量）

- `model/ability.go`：`DeleteAbilitiesByModel` 按模型删除 abilities 并统计去重渠道数 ✓
- `model/model_pricing_config.go`：`DeleteModelPricingConfig` 事务内删除模型价格配置 ✓
- `controller/model_meta.go`：`DeleteModelMeta` 重写（解析 remove_from_channels/remove_pricing，返回 `{deleted_count, updated_channels}`）；`BatchDeleteModels` 新增（model_ids + 两布尔参数，累计统计）✓
- `router/api-router.go`：`POST /api/models/delete` 已注册 ✓
- `model-delete-dialog.tsx`：`result?.deleted_count ?? props.models.length` 防御性兜底 ✓

### 方案 2：模型同步契约升级

- `controller/model_sync.go`：`fetchMetadataCatalog`（llm-metadata + opencode-go 双来源，source.Version sha256 指纹）✓
- `normalizeLocale` 支持 `zh`（前端默认 locale，旧实现拒绝导致预览失败——隐藏第二根因）✓
- `SyncUpstreamPreview` 返回 `{source, candidates}`（create/update/unchanged/blocked/missing_upstream/missing_vendor + scope/record_version/fields）✓
- `SyncUpstreamModels` 接收 `{locale, source, source_version, selections}`，版本指纹校验（不匹配 409），调 `model.ApplyMetadataSync` ✓
- `model/model_metadata_sync.go`：`MetadataSyncUpdate.RichFields` 透传，opencode-go 来源创建时写入富元数据 ✓
- 旧契约残留（overwriteField/buildSyncSourceInfo/missingRichFieldsMap/ensureVendorID/coalesce/chooseStatus/containsField）已清理 ✓
- 前端：`MetadataSyncRequest` 增加 `source`，`sync-wizard-dialog.tsx` apply 透传 source ✓

### 方案 3：价格预设文案 i18n

- 7 个 locale（en/zh/zh-TW/fr/ru/ja/vi）补充 `OpenCode Go pricing preset` 键，zh 为「OpenCode Go 价格预设」✓
- `bun run i18n:sync` 幂等 ✓
- `upstream-ratio-sync-helpers.test.ts` t 为 identity，无需修改（3/3 通过）✓

## 代码审查

`review_mode: off`（hotfix 预设默认）：跳过自动代码审查。原因记录——改动经并行 fixer 车道（Bug 1 后端、前端容错+i18n）+ 主线（Bug 2 契约升级）执行，并由 orchestrator 统一集成验证；最终 diff 的编译、测试、构建证据齐全（见下）。

## 验证证据（record-check 已记录）

| 命令 | 结果 |
|------|------|
| `go build ./...` | exit 0 |
| `go test ./model/... ./controller/...` | exit 0（model 8s / controller 23s 全绿） |
| `bun run typecheck` | exit 0（tsgo -b） |
| `bun run build` | exit 0（前端生产构建成功） |
| 前端受影响测试 4 文件 | 21/21 通过（metadata-sync、model-deletion、upstream-ratio-sync-helpers、sync-wizard-source-selection、sync-source-options） |

## 根因消除确认

1. Bug 1（`Cannot read properties of null (reading 'deleted_count')`）：后端 `DeleteModelMeta` 不再返回 `data: null`，改为返回 `{deleted_count, updated_channels}`；前端 `result?.deleted_count` 兜底；批量删除路由补齐。崩溃路径已消除。
2. Bug 2（同步预览「没有符合筛选条件的模型」）：`SyncUpstreamPreview` 返回前端期望的 `candidates`（旧实现缺失该字段导致恒空）；`normalizeLocale` 支持 `zh`；apply 契约 `selections/source_version` 对齐。功能断链已修复。
3. Bug 3（`OpenCode Go pricing preset` 英文回退）：7 语言补键，zh 显示「OpenCode Go 价格预设」。

## 结论

无 CRITICAL / IMPORTANT 问题。全部检查通过，可归档（ready for archive）。
