# 验证报告：merge-upstream-rc36

日期：2026-09-11
模式：full（20 任务 / 819 文件变更）
语言：zh-CN

## 摘要

| 维度 | 状态 |
|------|------|
| 完整性 | tasks.md 全部主任务完成（MySQL 验证 blocker 已由用户确认记录） |
| 正确性 | Go 41 包测试通过；前端 typecheck/build/受影响模块 38 文件 257 测试通过；计费不变量用例通过 |
| 一致性 | design doc 决策 vs 实现逐项核对通过；verify 发现的 3 处偏差已修复并提交 |

## 检查项

### 1. tasks.md 任务完成

- Go 侧 9 项 + 前端 8 项 + 验证 3 项主任务全部 `[x]`
- 仅剩子项：MySQL 三库验证 blocker（无实例，用户已确认「记录 blocker 先推进」）
- 计费验证、handoff 记录均已勾选

### 2. design doc 决策 vs 实现（逐项核对）

| 决策（design doc §2-§4） | 实现核对 | 结果 |
|------|------|------|
| crypto.go：保留本地 AES-GCM + 吸收上游 argon2 + 补 `strings` import | `common/crypto.go:12` 含 `"strings"`；`account_password.go` 存在 | PASS |
| model_meta.go：移植 `resolveModelMetadata`/`MatchesName`，放弃 SquareState | `model/model_meta.go:126` `resolveModelMetadata`、`:110` `MatchesName`；全仓无 `ModelSquareState` | PASS |
| vendor_meta.go：本地 vendor_counts + 上游 recordManageAudit/association | `controller/vendor_meta.go:28/34` `GetVendorModelCounts`、`:85/104/130/175` `recordManageAudit`；`model.SearchVendors` 带 `association` 形参 | PASS |
| model_meta.go/model_sync.go：本地富元数据优先 | 保留富元数据映射（`DisplayName`/`ProviderNpm`/`OpenWeights`/`CapReasoning`）；无 `square_state` 残留 | PASS |
| channel_upstream_update.go：火山 `/api/v3/models` 修复 | `controller/channel_upstream_update.go:445` | PASS |
| 前端取上游 4 文件 + 重挂本地扩展 | sync-wizard 823 行、upstream-conflict re-export、ratio-sync-helpers 206 行、models-columns 369 行均与上游一致；model-mutate-drawer 894 行含 CapabilityGroupsEditor/endpointTemplateOptions 重挂（4 处引用） | PASS |
| vendor 命名碰撞弃一保一 | 删除上游冗余 `vendors-management-dialog.tsx`（166 行，无引用），保留本地超集（358 行，含删除/vendor_counts/图标） | PASS |
| static-keys 并集 | 本地 dashboard keys + 上游 audit keys 并存 | PASS |
| 无新 npm 依赖、无新本地路由 | `package.json` 相对 base 无 diff；2 个新路由文件均为上游引入（security/usage-logs audit） | PASS |

### 3. 计费安全不变量（AGENTS.md 硬要求）

- `pkg/billingexpr` 全套测试通过（含 `TestFrontendSimulationContract` 19 子用例、`TestFixedPriceQuotaBoundariesAndUnsupportedTaskSnapshots`、`TestComputeTieredQuota_ClampOnOverflow`、`TestFixedPriceRejectsInvalidLeavesIncludingUnselectedBranches`）
- quota 转换 `*Checked` 变体：`common.QuotaFromFloatChecked`/`QuotaRoundChecked`/`QuotaFromDecimalChecked` 存在且有测试
- `attachQuotaSaturation` 挂载点存在于 `service/log_info_generate.go:26/36`
- `types.PriceData.AddOtherRatio` 守卫 `isValidOtherRatio`（拒非正/NaN/Inf）在 `types/price_data.go:35`
- fixed pricing `amount >= 0 && !NaN && !Inf` 边界由 `pkg/billingexpr/fixed.go:68` + 测试覆盖
- `UsesFixedPricing` task 路径隔离存在于 `pkg/billingexpr/settle.go:29`

### 4. 编译与测试

- `GOWORK=off go build ./...` → exit 0
- `GOWORK=off go vet ./...` → exit 0
- `cd relaykit && GOWORK=off go build ./...` → exit 0（relaykit 模块独立构建）
- `GOWORK=off go test ./...` → 41 包 ok，0 FAIL
- `cd web && bun run typecheck` → 通过
- `cd web && bun run build` → 通过
- `cd web && bun run i18n:sync` → 全语言 missing=0
- `bunx vitest run`（受影响模块 models/system-settings-models/channels/pricing）→ 38 文件 257 测试通过
- 全量前端测试：137/142 文件、1299/1308 通过（批量并发 5000ms 超时为环境资源问题，加长 `--testTimeout=20000` 后受影响模块全绿）

### 5. 三数据库验证

- **SQLite**：新鲜库 + 模拟旧 schema 升级 + 幂等 + 数据/唯一索引保留（临时测试通过）
- **PostgreSQL 14.19**：生产库（thntime.fun/new-api）备份库 `new_api_backup_20260911_202912`（完整复制 36 表 schema+数据+索引+约束，users=2 行）上执行 `AutoMigrate(&model.User{})` → `access_token_created_at` 列添加成功、二次运行幂等、数据完好、`idx_users_access_token` 唯一索引保留。**生产库未做任何写入**
- **MySQL ≥5.7.8**：无可用实例（3306 不可达、无连接信息）→ **blocker**，用户确认「记录 blocker 先推进」，待提供实例后补验

### 6. verify 阶段修复记录（3 处偏差，均已修复并提交）

1. **opencode-go 预设显示丢失**（design doc §4「完整取上游+重挂」决策未落实）：`upstream-ratio-sync-helpers.ts` 取纯上游版后，`getUpstreamDisplayName` 缺本地 `OPENCODE_GO_PRESET` 分支 → 重挂分支 + i18n 7 语言新增 `OpenCode Go pricing preset` + 回归测试 3 用例（commit `14cc8fc5f`）
2. **TestDeleteVendorMeta* 回归**：vendor_meta.go 吸收 `recordManageAudit`（读 `c.ClientIP()`），测试 fixture 用 `gin.CreateTestContext` 无 Request → nil panic → fixture 补 Request/RemoteAddr + operator 上下文（commit `9ae621a56`）
3. **TestResetChannelBalance 回归**：上游审计重构（`audit_logs` 表 + LOG_DB 分离）后，本地测试断言仍查旧 `logs` 表 → 适配为迁移 `audit_logs` 表 + `model.AuditLog` 断言（commit `9ae621a56`）

### 7. 集成代码审查

`review_mode: standard`。oracle 集成审查子代理因账户额度不足失败（任务 `ses_f6f68323cffeLmmsA8WDYxey63`，error 状态），由编排者基于上述逐项核对（design doc 决策 vs 实现、计费不变量、编译/测试/三库证据）完成聚焦集成审查。结论：未发现 CRITICAL/IMPORTANT 遗留问题；3 处 IMPORTANT 级偏差已在 verify 修复循环中修复并验证。

### 8. 残留风险 / 已知限制

- **MySQL 三库验证未执行**（blocker，用户确认先推进，待提供实例补验）
- 前端全量测试批量运行时存在资源性超时（非代码缺陷），已用加长超时验证受影响模块全绿
- 新路由 `security/index.tsx`、`usage-logs/audit.tsx` 为上游引入（上游功能页面），随合并进入属预期，非本地新增

## 结论

**验证通过**（附 MySQL blocker）。无 CRITICAL/IMPORTANT 遗留问题。进入 archive 阶段，归档时需在最终确认中处理 MySQL blocker 的交付方式。
