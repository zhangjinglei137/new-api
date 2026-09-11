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
