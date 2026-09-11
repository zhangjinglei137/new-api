# Tasks：merge-upstream-rc36

## 任务清单（按执行顺序）

### Go 侧
- [x] 执行 `git merge --no-edit upstream/main`，接受默认三方合并（非冲突文件自动合并）
- [x] 修复 `common/crypto.go`：import 补 `strings`（保留本地 AES-GCM，吸收上游 argon2 分支）；确认 `common/account_password.go` 已合并
- [x] 移植上游 `resolveModelMetadata` + `MatchesName` 到本地 `model/model_meta.go`（保留本地富元数据列与 Insert/Update/Delete）
- [x] 融合 `controller/vendor_meta.go`：本地 `vendor_counts` + 上游 `vendorAPIError`/`recordManageAudit`/`association`；同步移植 `model.SearchVendors` 的 `association` 形参
- [x] 融合 `controller/channel_upstream_update.go`：本地 DB 保留字列引用修复 + 上游火山 `/api/v3/models` 修复
- [x] 融合 `controller/model_meta.go`：本地富元数据优先 + 吸收 `interface{}→any`/审计；放弃 square_state
- [x] 融合 `controller/model_sync.go`：本地富元数据同步映射优先 + 吸收上游非富元数据改进
- [x] `go build ./...` + `go vet ./...` 通过（root 模块）
- [x] `cd relaykit && GOWORK=off go build ./...` 通过（模块独立构建）

### 前端
- [x] 融合 `model-mutate-drawer.tsx`：上游骨架（initialSection 分段）+ 重挂本地 endpoint/CapabilityGroupsEditor；zod schema 逐字段比对
- [x] 融合 `models-dialogs.tsx`：上游 PriceSyncDialog/price-model + 本地 Endpoint/VendorManagement 注册
- [x] 取上游 `sync-wizard-dialog.tsx`、`upstream-conflict-dialog.tsx`、`upstream-ratio-sync-helpers.ts`、`models-columns.tsx`；处理本地测试改向（SyncDiffData→MetadataSyncField/Candidate）
- [x] 处理 vendor 命名碰撞：确认本地 `vendor-management-dialog` vs 上游 `vendors-management-dialog` 职责，弃一保一
- [x] 融合 `channels-columns.tsx`：本地优先 + 吸收上游错误通知统一/插件图标
- [x] `static-keys.ts` 并集 + `bun run i18n:sync` 补 zh 翻译
- [x] 重写/删除 2 个本地测试（upstream-conflict-dialog.test、sync-wizard-source-selection.test）
- [x] `cd web && bun install && bun run typecheck && bun run build && bun test` 通过

### 验证
- [x] 三数据库验证：SQLite/PostgreSQL 完成；MySQL 记录 blocker（用户确认先推进，待提供实例补验）
  - [x] SQLite：新鲜库 + 模拟旧 schema 升级 + 幂等 + 数据/唯一索引保留（临时测试，通过）
  - [x] PostgreSQL：生产库（14.19）备份库 `new_api_backup_20260911_202912` 上升级迁移 + 幂等 + 数据/唯一索引保留（通过）
  - [ ] MySQL：无可用实例（3306 不可达），验证 blocker（用户选择记录后推进，待提供实例补验）
- [x] 计费用例：`pkg/billingexpr` 测试 + `frontend_simulation.json`（全套通过，含 fixed-pricing 边界/Clamp 用例）
- [x] 记录数据库版本、命令、结果到 handoff（已写入 design doc 附录：PG 14.19/SQLite 版本、备份库名、命令与结果）
