# Comet Design Handoff

- Change: merge-upstream-rc36
- Phase: design
- Mode: compact
- Context hash: ebd2738c707b686abf5de0bf2ab0c68f42ac23a27b3a406f839e6fbf023d2cef

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## docs/openspec/changes/merge-upstream-rc36/proposal.md

- Source: docs/openspec/changes/merge-upstream-rc36/proposal.md
- Lines: 1-43
- SHA256: 26f32ff29c73026b2898ed53f5897eaa4f98630205a15c63ef584edf634cfc46

```md
# Proposal：merge-upstream-rc36

## Why

本地 fork 落后上游 QuantumNous/new-api main 47 个提交（v1.0.0-rc.32 → rc.36，含 billing 表达式定价、vendor 管理、user 重构、模型管理 UI 全面重构等）。sync-upstream 自动任务因 17 个文件冲突 + vet 编译错误已连续失败（静默失败问题已在单独 change 修复，本次解决冲突本体）。需要人工完成这次合并，让本地跟上上游功能与修复。

## What Changes

采用**融合优先**策略合并上游 47 个提交，逐个解决 17 个冲突文件：

- **Go 侧（6 文件）**：
  - `common/crypto.go`：完整融合——保留本地 AES-GCM 凭证加密，吸收上游 argon2 密码哈希分支（补 `strings` import）
  - `model/model_meta.go`：本地优先——保留本地 11 列富模型元数据架构，移植上游 `resolveModelMetadata`/`MatchesName` 两函数（解 `pricing.go` vet 错误）；放弃上游 SquareState/运行时聚合重构
  - `controller/model_meta.go`、`controller/model_sync.go`：本地优先——保留富元数据列表/同步逻辑，吸收上游无关改进（`interface{}→any`、审计、错误统一）
  - `controller/vendor_meta.go`：完整融合——本地 `vendor_counts` 聚合 + 上游 `vendorAPIError`/审计/`association` 参数（同步移植 `model.SearchVendors` 形参）
  - `controller/channel_upstream_update.go`：完整融合——本地 DB 保留字列引用修复 + 上游火山 `/api/v3/models` 404 修复
- **前端（12 文件 + 2 测试）**（oracle 已决策）：
  - 完整取上游重构核心：`sync-wizard-dialog`、`upstream-conflict-dialog`（废弃为 re-export）、`upstream-ratio-sync-helpers`、`models-columns`
  - 上游为骨架 + 重挂本地扩展：`model-mutate-drawer`（endpoint/capability 扩展挂到上游 `initialSection` 分段）
  - 并列融合：`models-dialogs`（上游 PriceSyncDialog 等 + 本地 Endpoint/VendorManagement 注册）
  - 本地优先：`channels-columns`（吸收上游错误通知统一 + 插件图标两处）
  - i18n `static-keys` 并集；2 个本地测试重写/删除
- **验证**：`go build`/`vet`、`relaykit` 独立构建（GOWORK=off）、`bun typecheck`/`build`/`test`/`i18n:sync`、三数据库兼容验证、上游计费表达式用例

## Capabilities

### New Capabilities

（无——本 change 为上游代码同步/集成任务，行为契约由上游定义，本项目不新增 spec 契约）

### Modified Capabilities

（无——保留本地扩展，不改变本地 spec 契约；`.openspec.yaml` 设置 `skip_specs: true`）

## Impact

- 合并上游 47 个提交（rc.32→rc.36），含：
  - **billing**：fixed per-request 表达式定价（`064ed943e`）、time-based 定价编辑器（`d52bdc0b4`）——计费安全审查结论：合规（`QuotaRoundChecked`、乘法器 bound、task 路径隔离），需补跑新增用例
  - **model**：上游 `user.go` 新增 `AccessTokenCreatedAt` 列（AutoMigrate `ADD COLUMN`，三库验证）；vendor 管理重构
  - **前端**：模型管理 UI 重构（定价解耦到 price-sync-dialog、冲突解决并入 sync-wizard）、无新 npm 依赖、无新路由
- 影响的代码：Go 6 个冲突文件 + 上游自动合并的新文件（`account_password.go`、`model_metadata_sync.go`、`migration_dialector.go` 等）；前端 12 个冲突文件 + 2 测试
- 数据库：`user` 表新增 1 列；需三库（SQLite/MySQL≥5.7.8/PostgreSQL≥9.6）验证升级与幂等
- 不引入新的公共 API 变更（保留本地扩展 API）

```

## docs/openspec/changes/merge-upstream-rc36/design.md

- Source: docs/openspec/changes/merge-upstream-rc36/design.md
- Lines: 1-55
- SHA256: 8da6ecf2f5392b6f02da9a06bd6bea74decb16f2e7471100f43e6acba229852a

```md
# Design：merge-upstream-rc36

## 高层决策

1. **合并策略 = 融合优先**：同时保留本地扩展与上游新功能，逐个冲突文件精细融合（用户确认）。
2. **Go model 层 = 本地优先**：保留本地富模型元数据架构（`Model` 11 列 + `model_rich.go` 636 行），**放弃**上游 SquareState/运行时聚合重构；只移植 `resolveModelMetadata` + `MatchesName` 两个纯函数解 vet 错误（用户确认）。
3. **前端 = oracle 决策**：上游重构核心完整取上游 + 重挂本地扩展；channels-columns 本地优先；i18n 并集。
4. **计费安全不变量**：上游 fixed per-request 定价已合规（`QuotaRoundChecked`、乘法器 bound、task 路径隔离），融合后补跑上游新增用例验证，不额外加固。

## 冲突解决矩阵

### Go 侧

| 文件 | 策略 | 要点 |
|------|------|------|
| `common/crypto.go` | 完整融合 | 保留本地 AES-GCM envelope + 吸收上游 argon2 分支；import 补 `strings`；确认 `common/account_password.go`（上游新文件）自动合并 |
| `model/model_meta.go` | 本地优先+吸收 | 保留富元数据列；移植 `resolveModelMetadata`/`MatchesName`（纯函数，依赖 `NameRule/ModelName/Status` 本地已有）；Insert/Update/Delete 保留本地现状（不走 `metadataTransaction`） |
| `controller/model_meta.go` | 本地优先+吸收 | 保留富元数据列表/编辑；吸收 `interface{}→any`、`recordManageAudit`（如本地已接审计）；放弃 `square_state`/`include_channel_models` |
| `controller/model_sync.go` | 本地优先+吸收 | 保留 479 行富元数据同步映射；吸收上游同步流程中非富元数据改进；放弃上游 380 行删除 |
| `controller/vendor_meta.go` | 完整融合 | 本地 `vendor_counts` + 上游 `vendorAPIError`/`recordManageAudit`/`association` 参数；同步移植 `model.SearchVendors` 的 `association` 形参 |
| `controller/channel_upstream_update.go` | 完整融合 | 本地 DB 保留字列引用修复 + 上游火山 `/api/v3/models` 404 修复 + `interface{}→any`；改动区不重叠 |

### 前端

| 文件 | 策略 | 要点 |
|------|------|------|
| `sync-wizard-dialog.tsx` | 完整取上游 | 上游 823 行重写；本地 `21b78354c` 小修先验证是否已被上游覆盖 |
| `upstream-conflict-dialog.tsx` | 完整取上游 | 上游掏空为 re-export；本地 627 行废弃，语义已被 sync-wizard 吸收 |
| `upstream-ratio-sync-helpers.ts` | 完整取上游+重挂 | 本地 opencode-go 预设按上游新 schema（SyncPriceContext）重写 |
| `models-columns.tsx` | 完整取上游+补回 | 补回本地 display_name revert 语义 |
| `model-mutate-drawer.tsx` | 上游骨架+重挂 | 本地 endpoint 模板 + CapabilityGroupsEditor 挂到上游 `initialSection='metadata'`；zod schema 逐字段比对（最大坑点） |
| `models-dialogs.tsx` | 并列融合 | 上游 PriceSyncDialog/price-model + 本地 Endpoint/VendorManagement 注册 |
| `channels-columns.tsx` | 本地优先+吸收 | 吸收 `12be9975c` 错误通知统一 + `eb76b136b` 插件图标 |
| `static-keys.ts` | 并集 | 本地 dashboard key + 上游 152 billing key 无重叠 |
| 测试 2 个 | 重写/删除 | `upstream-conflict-dialog.test` 删除或改为 sync-wizard 冲突选择测试；`sync-wizard-source-selection.test` 对照上游 STEPS 重写 |
| 本地 4 文件（channels/dashboard constants/types） | 无操作 | 上游零改动，非真实冲突 |

## 风险与对策

- **vendor 命名碰撞**：本地 `vendor-management-dialog.tsx`（单数）vs 上游 `vendors-management-dialog.tsx`（复数）——build 阶段先确认职责，若重复弃本地保上游
- **drawer zod schema 合并**：本地/上游都改了 `ExtendedModelFormValues` 与 `SideDrawerSection`——逐字段比对，typecheck 第一道关卡
- **废弃组件级联引用**：`upstream-conflict-dialog` 掏空后，本地测试/组件若 import 其内部类型需改向 sync-wizard 新类型
- **上游自动合并的新文件**（`model_pricing_config.go`、`model_metadata_sync.go`、`migration_dialector.go`）：自洽编译（已核查自带定义），低风险；但 `migration_dialector.go` 与本地富元数据 `*bool` 列的「重启反复 ALTER」问题相关，需三库幂等验证
- **Go 依赖升级**：上游升级 `glebarez/sqlite v1.11.0`、新增 `coreos/go-oidc/v3`、`oauth2` 等——三库验证覆盖

## 验证方案

1. `go build ./...` + `go vet ./...`
2. `cd relaykit && GOWORK=off go build ./...`（模块独立构建硬要求）
3. `cd web && bun install && bun run typecheck && bun run build && bun run i18n:sync && bun test`
4. 三数据库验证（AGENTS.md 硬要求）：
   - 新鲜库：SQLite/MySQL≥5.7.8/PG≥9.6 各跑启动迁移，确认 `user.access_token_created_at` 创建
   - 升级库：最新发布版建库 → 融合代码迁移 → 确认新列新增、旧数据完好、唯一索引保留
   - 幂等性：同一升级库连续启动两次，无反复 `ALTER TABLE`
5. 计费用例：`pkg/billingexpr` 测试 + `testdata/frontend_simulation.json`

```

## docs/openspec/changes/merge-upstream-rc36/tasks.md

- Source: docs/openspec/changes/merge-upstream-rc36/tasks.md
- Lines: 1-29
- SHA256: 49dc4f92cab01e39560582a1b73e6d8b166f6c5bc716adc6ead3a462cef03b2a

```md
# Tasks：merge-upstream-rc36

## 任务清单（按执行顺序）

### Go 侧
- [ ] 执行 `git merge --no-edit upstream/main`，接受默认三方合并（非冲突文件自动合并）
- [ ] 修复 `common/crypto.go`：import 补 `strings`（保留本地 AES-GCM，吸收上游 argon2 分支）；确认 `common/account_password.go` 已合并
- [ ] 移植上游 `resolveModelMetadata` + `MatchesName` 到本地 `model/model_meta.go`（保留本地富元数据列与 Insert/Update/Delete）
- [ ] 融合 `controller/vendor_meta.go`：本地 `vendor_counts` + 上游 `vendorAPIError`/`recordManageAudit`/`association`；同步移植 `model.SearchVendors` 的 `association` 形参
- [ ] 融合 `controller/channel_upstream_update.go`：本地 DB 保留字列引用修复 + 上游火山 `/api/v3/models` 修复
- [ ] 融合 `controller/model_meta.go`：本地富元数据优先 + 吸收 `interface{}→any`/审计；放弃 square_state
- [ ] 融合 `controller/model_sync.go`：本地富元数据同步映射优先 + 吸收上游非富元数据改进
- [ ] `go build ./...` + `go vet ./...` 通过（root 模块）
- [ ] `cd relaykit && GOWORK=off go build ./...` 通过（模块独立构建）

### 前端
- [ ] 融合 `model-mutate-drawer.tsx`：上游骨架（initialSection 分段）+ 重挂本地 endpoint/CapabilityGroupsEditor；zod schema 逐字段比对
- [ ] 融合 `models-dialogs.tsx`：上游 PriceSyncDialog/price-model + 本地 Endpoint/VendorManagement 注册
- [ ] 取上游 `sync-wizard-dialog.tsx`、`upstream-conflict-dialog.tsx`、`upstream-ratio-sync-helpers.ts`、`models-columns.tsx`；处理本地测试改向（SyncDiffData→MetadataSyncField/Candidate）
- [ ] 处理 vendor 命名碰撞：确认本地 `vendor-management-dialog` vs 上游 `vendors-management-dialog` 职责，弃一保一
- [ ] 融合 `channels-columns.tsx`：本地优先 + 吸收上游错误通知统一/插件图标
- [ ] `static-keys.ts` 并集 + `bun run i18n:sync` 补 zh 翻译
- [ ] 重写/删除 2 个本地测试（upstream-conflict-dialog.test、sync-wizard-source-selection.test）
- [ ] `cd web && bun install && bun run typecheck && bun run build && bun test` 通过

### 验证
- [ ] 三数据库验证：SQLite/MySQL/PostgreSQL 新鲜库 + 升级库迁移、幂等性（user.access_token_created_at 列）
- [ ] 计费用例：`pkg/billingexpr` 测试 + `frontend_simulation.json`
- [ ] 记录数据库版本、命令、结果到 handoff

```
