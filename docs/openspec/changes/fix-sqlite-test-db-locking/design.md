# 修复测试 SQLite 并发写锁 — 修复方案

## 方案

修改 `controller/access_token_audit_test.go` 的 `newAuditTestDatabase` sqlite 分支，为测试连接追加与生产 `common.SQLitePath` 一致的并发配置：

```go
if kind == "sqlite" {
	path := t.TempDir() + "/audit.db"
	dsn := path + "?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	return db, path
}
```

- `_pragma=busy_timeout(30000)`：写者排队等待而非立即报锁（纯 Go 驱动只认 `_pragma=` 形式，见 `common/database.go` 注释）
- `_pragma=journal_mode(WAL)`：读者不被单写者阻塞
- `_txlock=immediate`：BEGIN IMMEDIATE 先取写锁，写者串行化，避免 SQLITE_BUSY_SNAPSHOT
- 返回路径保持原样（`path` 不含 DSN 参数，避免破坏调用方对路径的使用）

## 备选方案（不采用）

- 加 `t.Setenv` 提高测试并发时序容差：治标不治本，测试仍依赖时序
- 改测试为串行执行：削弱并发验证意图
- 修改生产 `common.SQLitePath`：生产已正确，不应为测试改生产

## 影响

- 该 helper 被 `security_enrollment_test.go`、`access_token_audit_test.go` 等多个安全/审计测试共用，统一获得并发正确性
- 只影响测试连接 DSN，不影响生产代码与测试断言
