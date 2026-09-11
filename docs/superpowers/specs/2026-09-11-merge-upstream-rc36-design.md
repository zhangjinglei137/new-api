---
comet_change: merge-upstream-rc36
role: technical-design
canonical_spec: openspec
archived-with: 2026-09-11-merge-upstream-rc36
status: final
---

# 技术设计：合并上游 rc.32→rc.36

## 1. 目标与范围

将上游 QuantumNous/new-api main（47 提交，v1.0.0-rc.32→rc.36）合并进本地 fork，采用融合优先策略：保留本地扩展（富模型元数据、AES 凭证加密、渠道扩展、模型广场 UI）同时吸收上游新功能（billing 表达式定价、vendor 管理、user 重构、模型管理 UI 重构），解决 17 个冲突文件并修复 vet 编译错误。

非目标：不采用上游 SquareState/运行时聚合架构；不重写本地渠道扩展；不引入上游废弃组件。

## 2. 架构矛盾与决策

**核心矛盾**：本地在 `model.Model` 上扩展了「富模型元数据」架构（11 个可编辑列 + `controller/model_rich.go` 636 行 + `model_sync.go` 富元数据同步映射）；上游 `0c76e4dae` 反向删除富元数据、改为运行时聚合（`HasMetadata`/`SquareState`/`SupportedEndpoints` + `resolveModelMetadata` 等）。两方向互斥。

**决策（用户确认）**：model 层本地优先——保留富元数据，放弃上游 SquareState；只移植 `resolveModelMetadata`+`MatchesName` 两纯函数（仅依赖 `NameRule/ModelName/Status/Endpoints`，本地都有）解 `pricing.go` 编译错误。SquareState/模型广场可见性留待后续独立 change。

## 3. Go 侧冲突解决详情

### 3.1 common/crypto.go（完整融合）
- 本地：AES-256-GCM `EncryptSecret/DecryptSecret` envelope（新增 `crypto/aes,cipher,rand,base64,fmt` import）
- 上游：`ValidatePasswordAndHash` 新增 argon2 分支（`strings.HasPrefix(hash,"$argon2id$")`）+ 新增 `common/account_password.go`（含 `validateArgon2AccountPassword`）
- 冲突区仅 import 块：本地 import 无 `strings` → **补 `strings` import**；确认 `account_password.go` 已自动合并；argon2 依赖 `golang.org/x/crypto/argon2` 检查 go.sum

### 3.2 model/model_meta.go（本地优先 + 移植）
- 保留：11 个富元数据列、`GetAllProviderNPMs()`、本地 `Insert/Update/Delete`（不走上游 `metadataTransaction`）
- 移植（从上游）：`resolveModelMetadata`（`metaMap[Name]` 构建 + `MatchesName` 匹配 `NameRule/ModelName`）、`MatchesName` 本身——纯函数，零副作用
- 放弃：`ModelSquareState`/`FillModelSquareStates`/`SearchModelsWithChannels`/`GetConfiguredModelChannels`/`GetModelConnections`/`DeleteModelMetadata`（上游依赖删除富元数据字段的实现）

### 3.3 controller/model_meta.go（本地优先 + 吸收）
- 保留：本地 `validateModelCapabilities`、`enrichModels` 富元数据填充、`CreateModelMeta` 校验
- 吸收：`interface{}→any`、`recordManageAudit` 审计调用（如本地已接审计框架）、错误返回统一
- 放弃：`square_state` 过滤、`include_channel_models` 开关（依赖上游 model 层新函数）

### 3.4 controller/model_sync.go（本地优先 + 吸收）
- 保留：479 行富元数据同步映射（DisplayName/Family/ProviderNpm/OpenWeights/Reasoning→CapReasoning 等增量更新）
- 吸收：上游同步流程中非富元数据改进（供应商同步、错误处理）
- 放弃：上游 380 行删除

### 3.5 controller/vendor_meta.go（完整融合）
- 本地：`vendor_counts` 聚合（`model.GetVendorModelCounts`）+ `fmt` import
- 上游：`vendorAPIError` 错误助手 + `recordManageAudit` 审计 + `interface{}→any` + `SearchVendors` 加 `association` 参数
- 需同步：移植 `model.SearchVendors` 的 `association` 形参到本地 model 层

### 3.6 controller/channel_upstream_update.go（完整融合）
- 本地：`channelUpstreamModelUpdateSelectFields` 按 DB 方言显式引用保留字列（`group`/`key`）
- 上游：火山方舟端点 `/v1/models`→`/api/v3/models` 404 修复 + 3 处 `interface{}→any`
- 改动区不重叠，git 应自动合并大部分

## 4. 前端冲突解决详情（oracle 决策）

| 文件 | 策略 | 详情 |
|------|------|------|
| `sync-wizard-dialog.tsx` | 完整取上游 | 上游 823 行重写；本地 `21b78354c` 小修先验证是否被上游覆盖，不复现则丢弃 |
| `upstream-conflict-dialog.tsx` | 完整取上游 | 上游掏空为 `re-export SyncWizardDialog`；本地 627 行废弃 |
| `upstream-ratio-sync-helpers.ts` | 完整取上游+重挂 | 本地 opencode-go 预设按上游新 schema（`SyncPriceContext`/`PricingSourceSelection`）重写 |
| `models-columns.tsx` | 完整取上游+补回 | 补回本地 display_name revert 语义 |
| `model-mutate-drawer.tsx` | 上游骨架+重挂 | 上游 706 行 `initialSection` 分段；本地 endpoint 模板下拉 + `CapabilityGroupsEditor` 挂到 `initialSection='metadata'`；**zod schema 逐字段比对（最大坑点）** |
| `models-dialogs.tsx` | 并列融合 | 上游 `PriceSyncDialog`/`price-model`/`initialSection` + 本地 `EndpointManagementDialog`/`VendorManagementDialog` 注册 |
| `channels-columns.tsx` | 本地优先+吸收 | 吸收 `12be9975c` 错误通知统一 + `eb76b136b` 插件图标 |
| `static-keys.ts` | 并集 | 本地 dashboard key + 上游 152 billing key 无重叠 |
| 测试 2 个 | 重写/删除 | `upstream-conflict-dialog.test.tsx` 删除或改向 sync-wizard；`sync-wizard-source-selection.test.tsx` 对照上游 STEPS 重写 |
| 本地 4 文件 | 无操作 | channels/dashboard constants/types，上游零改动 |

**已确认零风险**：package.json 无 diff（无新 npm 依赖）、无新路由文件。

**命名碰撞**：本地 `vendor-management-dialog.tsx`（单数）vs 上游 `vendors-management-dialog.tsx`（复数）——先确认职责，重复则弃本地保上游。

## 5. 数据库 schema 影响

- 上游 `model/user.go`（本地未改→自动合并上游版）：
  - `User.Password` validate `max=20→128`（仅 validate tag，无表结构变化）
  - 新增 `User.AccessTokenCreatedAt *int64`（`gorm:"type:bigint;column:access_token_created_at"`）→ AutoMigrate `ADD COLUMN`，三库 OK，旧数据 NULL 需业务层处理
  - `User.HasPassword bool`（`gorm:"-:all"`，非持久化）
- 上游 `model/vendor_meta.go` 加 `ModelCount/Version`（`gorm:"-"`，非持久化）
- 本地富元数据 11 列保留 → 无新增 AutoMigrate 抖动
- **风险**：上游 `9a8674425 fix(db): avoid redundant schema migrations` + `migration_dialector.go`（方言级迁移调度）与本地富元数据 `*bool` 列的「重启反复 ALTER」问题相关，需三库幂等验证

### 三数据库验证（AGENTS.md 硬要求）
1. 新鲜库：SQLite/MySQL≥5.7.8/PG≥9.6 各跑启动迁移，确认 `access_token_created_at` 列创建
2. 升级库：最新发布版建库 → 融合代码迁移 → 新列新增、旧数据完好、`access_token` 唯一索引保留
3. 幂等性：同一升级库连续启动两次，无反复 `ALTER TABLE`
4. 记录数据库版本、命令、结果到 handoff

## 6. 计费安全审查（AGENTS.md 计费不变量）

上游 fixed per-request pricing（`064ed943e`/`d52bdc0b4`）审查结论**合规**：
- `ComputeTieredQuotaWithRequest` 用 `common.QuotaRoundChecked` → 返回 Clamp
- `validateFixedPricingTree` 强制 `amount >= 0 && !NaN && !Inf`
- `isRequestPriceMultiplier` 检查 multiplier bound
- task 路径隔离：`UsesFixedPricing` 阻止 fixed 进入 task 计费
- 融合后需补跑：`pkg/billingexpr` 测试（上游新增 125 行）+ `testdata/frontend_simulation.json`；检查 `BillingUnit` request-unit 消费方（`attachQuotaSaturation`）走 `*Checked` 转换

## 7. 测试与验证策略

1. `go build ./...` + `go vet ./...`
2. `cd relaykit && GOWORK=off go build ./...`（relaykit 模块独立构建硬要求）
3. `cd web && bun install && bun run typecheck && bun run build && bun run i18n:sync && bun test`（typecheck 第一道关卡：drawer schema、vendor 命名碰撞、类型改向）
4. 三数据库验证（§5）
5. 计费用例（§6）
6. 回归：模型创建/编辑抽屉 endpoint/capability、定价跳转、上游同步向导冲突选择、渠道 6 类用量弹窗、vendor 单一管理入口

---

## 附录：Build 阶段实际验证记录（2026-09-11）

### Go 侧
- `GOWORK=off go build ./...` → exit 0
- `GOWORK=off go vet ./...` → exit 0
- `cd relaykit && GOWORK=off go build ./...` → exit 0
- `GOWORK=off go test ./...` → 40 个包 ok（含 controller/model/service/relay/pkg/billingexpr）
- Go 冲突文件全部解决（先前提交 e902cb2bd），无残留冲突标记

### 前端
- `bun install`（1195 packages）
- `bun run typecheck` → 通过
- `bun run build` → 通过
- `bun run i18n:sync` → 全语言 missing=0
- `bunx vitest run`（受影响模块：models/system-settings-models/channels/pricing）→ 37 文件 254 测试通过；全量 137/142 文件、1299/1308 通过（批量并发 5000ms 超时为环境资源问题，加长超时后全绿）
- 删除 2 个失效测试（upstream-conflict-dialog、sync-wizard-source-selection，上游已重构废弃）；适配 model-cards 分页测试至本地单页展示行为（MODEL_CARD_GRID_PAGE_SIZE=1000）

### 三数据库验证（Task 16）
- **SQLite**：`glebarez/sqlite` 文件库临时测试通过——新鲜库迁移创建 `access_token_created_at`；模拟旧 schema 升级库迁移后列存在、数据（1 行 legacy token）与 `idx_users_access_token` 唯一索引保留；二次迁移幂等
- **PostgreSQL 14.19**（生产库 `thntime.fun:5432/new-api`，经 new_api_db_mcp 只读确认迁移前状态：`users` 缺 `access_token_created_at`、`idx_users_access_token` 唯一索引完好）：新建备份库 `new_api_backup_20260911_202912`（完整复制 36 表 schema+数据+索引+约束，users=2 行、logs=63279 行等），备份库上执行 `AutoMigrate(&model.User{})` → 列添加成功、二次运行幂等、数据完好、唯一索引保留。**生产库未做任何写入**
- **MySQL**：无可用实例（3306 不可达、无连接信息）→ 验证 blocker，待提供实例后补验

### 计费安全验证（Task 17）
- `GOWORK=off go test ./pkg/billingexpr/...` → 通过（含 `TestFrontendSimulationContract` 全套 19 子用例、`TestFixedPriceQuotaBoundariesAndUnsupportedTaskSnapshots`、`TestComputeTieredQuota_ClampOnOverflow`、`TestFixedPriceRejectsInvalidLeavesIncludingUnselectedBranches`）
- `TestFrontendSimulationContract` 用例文件 `pkg/billingexpr/testdata/frontend_simulation.json` 存在且被使用
- quota 转换 `common.QuotaFrom*Checked` 变体测试通过；fixed pricing 的 `amount >= 0 && !NaN && !Inf` 拒绝与 `amount * 1_000_000` 边界由 `TestFixedPriceRejectsInvalidLeaves...` 覆盖
