# 验证报告：fix-sqlite-test-db-locking（修复测试 SQLite 并发写锁）

- Date: 2026-09-12
- Change: fix-sqlite-test-db-locking
- verify_mode: full（3 任务 / 0 delta spec / 13 变更文件——12 个为 change 产物，源码改动仅 1 个测试文件）
- Language: zh-CN

## 摘要

| 维度 | 状态 |
|------|------|
| Completeness | 3/3 任务完成（tasks.md 全部勾选）|
| Correctness | 根因消除验证通过；目标测试连跑 10 次全 PASS |
| Coherence | 实现与 design.md 方案一致（单文件 DSN 配置修改）|

## 1. Completeness

- tasks.md：1.1-1.3 全部 `[x]`
- 源码改动：`controller/access_token_audit_test.go` 的 `newAuditTestDatabase` sqlite 分支（1 处 DSN 配置）
- 其余 12 个变更文件为本 change 的 OpenSpec 产物（proposal/design/tasks/.comet 状态）

## 2. Correctness（根因消除）

- **根因**：测试 DB 连接（`sqlite.Open(path)` 裸路径）缺少生产 `common.SQLitePath` 的并发配置（`_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate`），CI 2 核环境并发写触发 `SQLITE_BUSY`，两个并发删除请求都失败 → `succeeded = 0`
- **修复**：测试 DSN 追加与生产一致的并发配置，写者串行化（`_txlock=immediate`），后者在 busy_timeout 内等待后因 `SECURITY_PROOF_CONSUMED` 失败 → `succeeded = 1`
- **消除验证**：修复前连跑 10 次出现失败（actual 0）；修复后连跑 10 次全 PASS；`go test ./controller/ -run 'TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner' -count=10` exit=0

## 3. Coherence

- 实现与 design.md 方案完全一致（唯一方案，未采用备选）
- 未触碰生产代码（`common/database.go` 已正确配置）、未改测试断言
- 另外两处裸 `sqlite.Open`（model_list_test.go / token_test.go）为内存共享缓存 DSN 且无并发写场景，非同源问题，未改动（已记录）

## 4. 验证证据（真实执行）

| 命令 | 结果 |
|------|------|
| `go test ./controller/ -run 'TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner' -count=10` | PASS（10/10，修复后）|
| `go test ./controller/ -run 'Security\|Audit\|Token' -count=1` | PASS（22.3s，覆盖所有 `newAuditTestDatabase` 使用方）|
| `go build ./... && go vet ./controller/` | PASS |
| `go test ./controller/ -run 'Channel' -count=1` | PASS（此前已验证无回归）|

record-check（verify）：目标测试、Security|Audit|Token 回归、build/vet 均 exit=0

## 5. 代码审查

`review_mode: off`（hotfix 预设默认）。跳过自动代码审查原因：单文件测试 DSN 配置修复（1 处改动），低风险、不涉及生产代码/安全边界/接口契约；根因消除与回归测试已提供充分验证证据。

## 6. 已知说明

- 本机全量 `go test ./controller/` 超时（含需要外部服务/DSN 的用例，CI 环境才有对应配置）；已用相关范围回归（Security|Audit|Token）替代覆盖本改动影响面
- 本 change 无 delta spec（未改变任何已有 spec 的验收场景，仅修复测试环境并发配置）

## 结论

全部检查项通过：3/3 任务完成、根因消除（修复前复现失败 → 修复后 10/10 通过）、相关范围回归全绿、build/vet 通过。无 CRITICAL / IMPORTANT 问题。验证通过，可进入归档。
