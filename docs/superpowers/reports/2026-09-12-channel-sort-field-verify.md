# 验证报告：channel-sort-field（渠道序号字段与排序）

- Date: 2026-09-12
- Change: channel-sort-field
- verify_mode: full（13 任务 / 1 delta spec / 27 变更文件）
- Language: zh-CN

## 摘要

| 维度 | 状态 |
|------|------|
| Completeness | 13/13 任务完成（plan + tasks.md 全部勾选）|
| Correctness | 3 需求 / 8 场景全部实现并经最终集成审查核验 |
| Coherence | 实现符合 design.md 高层决策与 Design Doc；1 处文档字面偏差已裁决 |

## 1. Completeness

- plan `docs/superpowers/plans/2026-09-12-channel-sort-field.md`：Task 1-13 全部 `[x]`
- tasks.md：1.1-5.3 全部 `[x]`
- 变更文件（27 个）与 tasks 描述一致：后端 model/controller/authz + 迁移测试；前端 types/provider/table/columns/开关/表单/卡片 + i18n

## 2. Correctness（spec 场景覆盖，经最终集成审查 ora-7 逐项核验）

### Requirement: 渠道具有序号字段 — ✅
- Scenario: 创建渠道时序号默认为 0 ✅（DB default:0 + 表单默认值）
- Scenario: 编辑渠道序号 ✅（表单 + 内联编辑双入口，保存后 invalidateQueries 刷新）
- Scenario: 序号不影响负载均衡优先级 ✅（无 relay 路由改动）

### Requirement: 渠道列表按序号排序 — ✅
- Scenario: 默认按序号升序 ✅（Apply 默认分支 ORDER BY sort ASC）
- Scenario: 点击序号列头切换排序方向 ✅（DataTableColumnHeader asc/desc）

### Requirement: 使用序号排序开关 — ✅
- Scenario: 开启使用序号排序 ✅（开关 + localStorage 持久化 + queryKey 含 sort_sort 触发重新请求）
- Scenario: 使用序号排序与列头排序冲突 ✅（SortBy 分支优先）
- Scenario: 使用序号排序优先于使用ID排序 ✅（SortSort 分支在 IDSort 之前）

## 3. Coherence（design 一致性）

- 实现符合 design.md D1-D5 与 Design Doc：字段命名 sort（*int64）、排序优先级（列头 > sort_sort > id_sort > 默认 sort ASC）、开关平级 idSort、复用 ChannelFieldCell、AutoMigrate 迁移
- 已知裁决：`resolveChannelSortOptions` 不合并 SortSort 与 design.md D2 字面表述偏差——调用方构造时已置位，Task2 审查确认语义正确（deferred minor）
- Design Doc `docs/superpowers/specs/2026-09-12-channel-sort-field-design.md` 可定位且与 change 相关

## 4. 验证证据（真实执行）

| 命令 | 结果 |
|------|------|
| `go test ./model/... ./controller/... -run 'Channel' -count=1` | PASS（model 0.103s / controller 0.426s）|
| `bun run test`（web/）| PASS（149 文件 / 1330 测试）|
| `bun run typecheck`（web/）| PASS |
| `go build ./... && go vet ./...`（build 阶段）| PASS |
| `go test ./model/ -run 'TestChannelSortMigration' -count=1` | PASS（SQLite 新建/升级/幂等）|

record-check（verify）：`go test ./model/... ./controller/... -run 'Channel' -count=1` exit=0；`bun run test` exit=0

## 5. 最终集成代码审查（review_mode=standard，唯一集成审查）

结论：**整体通过**。排序优先级三层（前端透传 → controller → model.Apply）一致，authz 分类正确（fail-closed 守卫强制），无注入面/硬编码密钥，边界条件合理（nil/0/负数、tag 行不编辑、开关切换触发请求）。

### Deferred Minor（不阻塞，记录备查）
1. `resolveChannelSortOptions` 不合并 SortSort 与 design.md D2 字面偏差（语义正确，Task2 已裁决）
2. 内联编辑 `min={-999}` 与 spec「任意整数」不完全一致（表单入口无限制，业务足够）
3. sort create 用 `?? null` 与 priority `|| null` 操作符不一致（`?? null` 更正确，功能无差异）
4. zh-TW/fr/ru/ja/vi 新 i18n key 为英文回退（i18n:sync 约定，zh-TW 建议后续补繁体）
5. channel-card sort 行缩进与周围不一致（纯风格）

### 信息项
- 默认排序变更（priority DESC → sort ASC）影响内部调用方（GetTagModels/计费/同步等）——业务逻辑不依赖顺序，design.md 已声明 intended

## 6. 已知 Blocker（用户已确认接受）

- **MySQL/PostgreSQL 实库迁移未实测**：当前环境无 docker/mysql/psql 二进制、3306/5432 端口关闭，按全局规则不得自行启动数据库服务。SQLite 迁移实测通过（新建/升级/幂等）。测试代码已按三库兼容写法准备（`TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN` 环境变量钩子），待具备 DSN 环境运行 `TEST_MYSQL_DSN=... TEST_POSTGRES_DSN=... go test ./model/ -run TestChannelSortMigration -count=1` 补齐矩阵。用户已明确选择「记录 blocker 接受缺口」，不声称三库验证完整。

## 结论

全部检查项通过：13/13 任务完成、spec 3 需求 8 场景覆盖、最终集成审查整体通过、后端/前端测试全绿。无 CRITICAL / IMPORTANT 未决问题。验证通过，可进入归档。
