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
