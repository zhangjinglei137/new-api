# Brainstorm Summary

- Change: channel-affinity-retry-top-priority
- Date: 2026-09-15
- Status: 已定稿（用户确认设计方案 2026-09-15）

## 问题根因（已实证，方案已确认）

- 亲和渠道占用 retry=0（`getChannel` 中 `info.ChannelMeta == nil` 快捷路径直接返回 context 渠道，不进入候选池选择）
- 失败重试从 retry=1 开始逐优先级层推进（`GetRandomSatisfiedChannel` 的 `sortedUniquePriorities[retry]`），layer 0（最高优先级渠道）结构性不可达
- 数据库实测：渠道 1（priority=20 最高、status=1、group 含 default）24h 187 次成功调用（非亲和路径正常），亲和命中链路（6→2→3→4→5→6→7→8）中从不出现

## 确认的技术方案（候选）

### 方案：retry 索引偏移（推荐）

- 在 `getChannel`（controller/relay.go:334）正常选择路径中，若检测到「亲和渠道已作为首轮尝试」（新导出函数判断 context 亲和标志），将传给 `CacheGetRandomSatisfiedChannel` 的有效 retry 索引偏移 -1（通过 clone RetryParam 副本实现，不污染循环计数）
- 效果（RetryTimes=7、8 渠道、亲和命中渠道 6）：链路 6→1→2→3→4→5→6→7（channel 1 参与；总数不变）
- skip_retry_on_failure=true 时 shouldRetry 返回 false → 偏移分支不可达（无需处理）
- 修改位置：
  - `service/channel_affinity.go`：`MarkChannelAffinityUsed` 设置新 context 标志；新增导出判定函数
  - `controller/relay.go`：`getChannel` 偏移逻辑
  - 测试：service 层（扩展现有测试文件，偏移逻辑做成可测单元）
- 备选（已否决）：
  - A. 完整覆盖 layer 0..RetryTimes（9 次尝试）→ 突破 RetryTimes 语义
  - B. 亲和失败后清缓存+重置 retry → 只影响后续请求，未修复本次重试链
  - C. 修改 GetRandomSatisfiedChannel 取模回绕 → 副作用大，改变所有路径

## 关键取舍与风险

- [最低优先级渠道在亲和失败场景少一次覆盖（总数不变）] → 语义权衡，运维可调 RetryTimes
- [偏移依赖 context 标志] → 显式标志 + 单元测试锁定
- [最高优先级渠道调用量上升（可能放大 429）] → 预期效果，管理员调优先级/权重
- [仅修 layer 0 跳过，未修「亲和失败不清缓存下一请求仍钉渠道 6」] → 非目标，后续独立 change

## 测试策略（候选）

- service 层偏移单元测试 + controller 场景回归（亲和失败含最高优先级 / 未命中不偏移 / 成功无重试 / skip_retry 不进入）
- `go test ./service/... ./controller/...` + `go build ./...`

## Spec Patch

- 无（open 阶段 delta spec 已覆盖需求与验收场景；如需补充边界场景再回写）