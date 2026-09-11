# Brainstorm Summary

- Change: merge-upstream-rc36
- Date: 2026-09-11

## 确认的技术方案

**合并上游 47 个提交（rc.32→rc.36），融合优先。** 已确认的决策：
1. 合并策略 = 融合优先（保留本地扩展 + 吸收上游新功能）
2. Go model 层 = 本地优先：保留本地富模型元数据架构（`Model` 11 列 + `model_rich.go`），放弃上游 SquareState/运行时聚合；只移植 `resolveModelMetadata`+`MatchesName` 两纯函数解 vet 错误
3. 前端 = oracle 决策：上游重构核心完整取上游 + 重挂本地扩展；channels-columns 本地优先；i18n 并集；2 个测试重写/删除
4. 计费 = 上游 fixed per-request 定价已合规（`QuotaRoundChecked`、乘法器 bound、task 路径隔离），融合后补跑上游新增用例，不额外加固

**执行顺序**（oracle 建议）：
1. `git merge --no-edit upstream/main`（非冲突自动合并）
2. 修 `common/crypto.go`（补 strings import）+ 确认 `account_password.go` 在位
3. 移植 `resolveModelMetadata`/`MatchesName` 到本地 `model/model_meta.go`
4. `go vet ./...` 确认无新错误
5. 处理 controller 层 4 文件（model_meta/model_sync 本地优先、vendor_meta 完整融合 + association 形参、channel_upstream 完整融合）
6. 前端 8 组融合（model-mutate-drawer 重挂、models-dialogs 并列、取上游 4 文件、vendor 命名碰撞、channels-columns、i18n 并集、2 测试、bun 验证）
7. 三数据库验证 + 计费用例

## 关键取舍与风险

- **取舍**：Go 富元数据 vs 上游 SquareState——选富元数据，SquareState 作为后续独立特性
- **风险**：
  - vendor 命名碰撞：本地 `vendor-management-dialog`（单数）vs 上游 `vendors-management-dialog`（复数）——弃一保一
  - drawer zod schema 合并是最大坑点（双方都改了 ExtendedModelFormValues/SideDrawerSection）
  - 废弃组件级联引用（upstream-conflict-dialog 掏空后测试/组件需改向 sync-wizard 新类型）
  - 上游 `migration_dialector.go` 与本地富元数据 `*bool` 列的「重启反复 ALTER」相关，需三库幂等验证
  - Go 依赖升级（sqlite v1.11.0、coreos/go-oidc/v3、oauth2）需三库验证

## 测试策略

1. `go build ./...` + `go vet ./...`
2. `cd relaykit && GOWORK=off go build ./...`
3. `cd web && bun install && bun run typecheck && bun run build && bun run i18n:sync && bun test`
4. 三库验证（SQLite/MySQL≥5.7.8/PG≥9.6）：新鲜库 + 升级库迁移、幂等性（`user.access_token_created_at` 列）
5. 计费：`pkg/billingexpr` 测试 + `testdata/frontend_simulation.json`

## Spec Patch

无（`skip_specs: true`，本 change 为上游代码同步/集成任务）
