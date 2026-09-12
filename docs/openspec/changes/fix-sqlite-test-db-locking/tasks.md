# 修复测试 SQLite 并发写锁 — 任务清单

## 1. 修复测试 DB 连接

- [x] 1.1 修改 `controller/access_token_audit_test.go` 的 `newAuditTestDatabase` sqlite 分支：`sqlite.Open(path)` 改为 `sqlite.Open(path+"?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate")`，返回路径保持原样；运行 `go build ./...` 验证编译通过
- [x] 1.2 复现验证：先确认失败测试在修复前可复现（如可多次运行 `go test ./controller/ -run TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner -count=5` 观察偶发失败），修复后该测试连跑 5 次全通过
- [x] 1.3 回归验证：运行 `go test ./controller/ -run 'Security|Audit|Token' -count=1`（覆盖所有使用 `newAuditTestDatabase` 的测试）与 `go test ./controller/ -run 'Channel' -count=1` 确认无回归；`go vet ./controller/` 通过