# 修复测试 SQLite 并发写锁导致的 CI flaky 失败

## Why

GitHub CI 的 `TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner`（`controller/security_account_test.go:231`）间歇性失败：并发发送 2 个账号删除请求时期望恰好 1 个成功，实际 0 个成功。日志关键错误：`auth session internal error (DELETE /api/user/self): database is locked (5) (SQLITE_BUSY)`。

## 根因分析

- 测试 DB 由 `newAuditTestDatabase`（`controller/access_token_audit_test.go:441`）创建：`sqlite.Open(path)` 使用**裸路径**，未带并发写配置
- 生产配置 `common.SQLitePath`（`common/database.go:52-64`）明确带 `_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate`，其注释说明：缺省时并发写会立即报 "database is locked"（历史 issue #6805）
- CI 机器（2 核）并发调度激进，两个 goroutine 几乎同时写临时文件 SQLite，第二个写事务立即 `SQLITE_BUSY` → 两个请求都失败 → `succeeded = 0`
- 本地连跑 3 次全部 PASS，属 CI 环境触发的 flaky，与本 change 无关

## 修复目标

给 `newAuditTestDatabase` 的 sqlite 分支加与生产一致的并发配置（`busy_timeout(30000)` + `journal_mode(WAL)` + `_txlock=immediate`），使两个并发删除请求串行化：一个成功消费 proof、另一个因 `SECURITY_PROOF_CONSUMED` 失败 → `succeeded = 1`，测试通过。该 helper 被 `security_enrollment_test.go`、`access_token_audit_test.go` 等多个安全测试共用，一并受益。

## 非目标

- 不改动生产代码（`common/database.go` 已正确配置）
- 不改动测试断言逻辑
- 不处理其他数据库方言（MySQL/PostgreSQL 连接未受影响）
