---
comet_change: channel-affinity-retry-top-priority
role: technical-design
canonical_spec: openspec
archived-with: 2026-09-15-channel-affinity-retry-top-priority
status: final
---

# Design Doc: 修复渠道亲和性命中后重试跳过最高优先级层

## Context

见 `docs/openspec/changes/channel-affinity-retry-top-priority/proposal.md`（背景与范围）与同目录 `design.md`（open 阶段高层方案框架）。本 Design Doc 是对高层框架的深度技术细化。

核心代码事实（已核实）：

- `middleware/distributor.go` `Distribute()`：亲和性命中（`GetPreferredChannelByAffinity` 命中且渠道 Enabled + 通过过滤器）时直接 `channel = preferred`，调用 `MarkChannelAffinityUsed` 并把亲和元数据写入 context（`ginKeyChannelAffinityMeta`）。
- `controller/relay.go` `getChannel()`（controller/relay.go:334）：
  - `info.ChannelMeta == nil`（首轮 retry=0）→ 快捷路径：直接返回 context 中的渠道（亲和渠道或 distributor 选中的渠道），**不调用 `CacheGetRandomSatisfiedChannel`**；
  - `info.ChannelMeta != nil`（retry>=1）→ 调用 `service.CacheGetRandomSatisfiedChannel(retryParam)` 按优先级层重新选择。
- `model/channel_cache.go` `GetRandomSatisfiedChannel`：`targetPriority := sortedUniquePriorities[retry]`，retry 索引直接映射唯一优先级层；retry 越界 clamp 到最低层（不回绕）。
- `controller/relay.go` `shouldRetry()` 与 `shouldRetryTaskRelay()` 开头均检查 `ShouldSkipRetryAfterChannelAffinityFailure`：亲和规则 `skip_retry_on_failure=true` 时不进入重试。

缺陷根因：亲和渠道占用 retry=0（快捷路径不进入候选池选择），失败后重试从 retry=1 开始 → 优先级层 1，层 0（最高优先级渠道）结构性不可达。实测（数据库 channels 表 + logs 表）：渠道 1 priority=20（最高）且 24h 内 187 次成功调用（非亲和路径正常），但亲和命中链路（6→2→3→4→5→6→7→8）中从不出现。

## Goals / Non-Goals

**Goals:**
- 亲和渠道失败后，本次请求的重试链路回归正常优先级选择，最高优先级渠道（layer 0）参与候选。
- 保持亲和性命中仍优先使用亲和渠道、亲和渠道成功时不引入额外行为的既有语义。
- 保持重试总量语义：总尝试次数不增加（亲和 1 次 + 重试 N 次 ≤ RetryTimes+1）。
- 保持 `skip_retry_on_failure`、`keep_on_channel_disabled`、权重随机、优先级排序、clamp 不回绕等既有行为不变。

**Non-Goals:**
- 不修改渠道权重/优先级配置与选择逻辑本身。
- 不修改亲和缓存写入时机（仍仅成功时记录）与 TTL 语义。
- 不处理「亲和失败后缓存是否清除」决策（`keep_on_channel_disabled` 语义不变；请求失败不清缓存行为维持现状，作为后续独立 change）。
- 不解决「多渠道同上游导致 429 重试空转」（运维配置问题）。

## 方案：retry 索引偏移（retry-1 映射）

### 决策 1：在 getChannel 正常选择路径做有效 retry 偏移（rev.2，verify 集成审查修正）

`getChannel` 的 `info.ChannelMeta != nil` 分支中，若「亲和渠道已作为首轮尝试」（新增判定），将传给 `CacheGetRandomSatisfiedChannel` 的有效 retry 索引偏移 -1。

**rev.2 修正**（2026-09-15 verify 最终集成审查）：原实现「`cloned := *retryParam; cloned.SetRetry(effectiveRetry); CacheGetRandomSatisfiedChannel(&cloned)`」被证实有正确性缺陷——`CacheGetRandomSatisfiedChannel` 不是纯只读选择函数：

- service/channel_select.go:168 `param.SetRetry(0)` —— 切换到下一组时重置 retry 计数；
- service/channel_select.go:186-187 `param.SetRetry(0); param.ResetRetryNextTry()` —— 当前组优先级耗尽时重置计数并置 `resetNextTry` 标志。

这些变异作用于传入的 `*RetryParam` 指针，是 auto 跨组重试状态机的一部分。原实现传 `&cloned` 副本导致变异丢失：外层循环计数不被重置、`resetNextTry` 不置位，auto+crossGroupRetry+多组场景下后续组中间优先级层不可达（回归既有特性），且**无条件克隆**使未命中亲和（`effectiveRetry == retryParam.GetRetry()`）也受影响，违反 spec「亲和性未命中时不改变行为」。

**修正方案**：将「优先级层索引」与「循环计数/跨组状态机」解耦——新增显式优先级层入口：

```go
// service/channel_select.go — 新增变体入口（原函数成为转发包装）
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	return CacheGetRandomSatisfiedChannelWithPriority(param, param.GetRetry())
}

// CacheGetRandomSatisfiedChannelWithPriority 使用显式 priorityRetry 作为优先级层索引；
// 循环计数与跨组状态机变异（SetRetry(0)/ResetRetryNextTry）仍作用于原始 param。
func CacheGetRandomSatisfiedChannelWithPriority(param *RetryParam, priorityRetry int) (*model.Channel, string, error) {
	// ...原函数体，把 auto/非 auto 两处消费 param.GetRetry() 的地方改用 priorityRetry...
}
```

```go
// controller/relay.go getChannel 的 ChannelMeta != nil 分支
effectiveRetry := service.AffinityAdjustedRetry(c, retryParam.GetRetry())
channel, selectGroup, err := service.CacheGetRandomSatisfiedChannelWithPriority(retryParam, effectiveRetry)
```

要点：
- 不再克隆 `RetryParam`：`CacheGetRandomSatisfiedChannelWithPriority` 内部对 `param` 的 `SetRetry(0)`/`ResetRetryNextTry()` 变异直接作用于原始 `retryParam`，跨组状态机行为与现状一致。
- 优先级层索引完全由显式 `priorityRetry` 参数控制：offset 只影响层索引，不影响循环计数（外层 `for retryParam.GetRetry() <= RetryTimes` 判定与 `IncreaseRetry` 不受影响）。
- 未命中亲和时 `AffinityAdjustedRetry` 返回 `retryParam.GetRetry()` 原值，`WithPriority(param, retry)` 与 `CacheGetRandomSatisfiedChannel(param)` 等价 → 「未命中不改变行为」成立（含 auto+crossGroupRetry 场景）。
- dispatcher 现有调用 `retryParam{Retry: 0}` 走转发包装，行为不变。

效果（RetryTimes=7、8 渠道、亲和命中渠道 6）：

| retry | 现状 | 修复后 |
|---|---|---|
| 0 | 快捷路径 → 渠道 6 | 同左 |
| 1 | 层 1 → 渠道 2 | 层 0 → 渠道 1 |
| 2 | 层 2 → 渠道 3 | 层 1 → 渠道 2 |
| ... | ... | ... |
| 7 | 层 7 → 渠道 8 | 层 6 → 渠道 7 |

### 决策 2：亲和命中标记的显式传递

`MarkChannelAffinityUsed`（service/channel_affinity.go:677）在 Distributor 亲和命中被调用，此时设置新 context 标志：

```go
ginKeyChannelAffinityRetryBase = "channel_affinity_retry_base" // 新常量

func MarkChannelAffinityUsed(c *gin.Context, selectedGroup string, channelID int) {
    // ...existing...
    c.Set(ginKeyChannelAffinityRetryBase, true)
}
```

判定链路：`Distribute` 亲和命中 → `MarkChannelAffinityUsed` → 写标志 → `getChannel` 读取。亲和未命中、亲和命中但不可用（`ClearCurrentChannelAffinityCache` 路径）均不设置标志 → 不偏移。

边界确认：
- `skip_retry_on_failure=true`：`shouldRetry`/`shouldRetryTaskRelay` 在开头 `ShouldSkipRetryAfterChannelAffinityFailure` 返回 true → 不进入重试循环 → 偏移分支不可达。无需额外处理，但需回归测试锁定。
- 亲和命中且成功：无重试，偏移不触发。
- 亲和未命中：`AffinityConsumedFirstTry` 返回 false，偏移不触发，行为与现状完全一致。
- retry=0 快捷路径（`ChannelMeta==nil`）：不受影响，首轮仍走亲和渠道（context 渠道）。
- auto 组 + 亲和：`CacheGetRandomSatisfiedChannel` 的 auto 分支内部按 `priorityRetry`（=传入 retry）计算，跨组时每组从 `priorityRetry=0` 开始；偏移作用于传入值后，auto 分支的开始组仍从层 0 开始，逻辑保持一致。

### 决策 3：测试策略

- service 层：在现有 `service/channel_select_auto_groups_test.go` 或对应 controller 层测试中新增偏移单元测试（构造 `RetryParam` + context 标志，断言 `CacheGetRandomSatisfiedChannel` 接收的有效 retry = `GetRetry()-1`；未命中标志时不偏移）。
- controller 场景回归：亲和命中失败 → 重试含最高优先级渠道；亲和未命中 → 不偏移；亲和成功 → 无重试；`skip_retry_on_failure=true` → 不进入重试；`shouldRetryTaskRelay` 同检查。
- 验证命令：`go test ./service/... ./controller/...`、`go build ./...`、gofmt。

## Risks / Trade-offs

- [亲和失败场景最低优先级渠道少一次覆盖（总次数不变）] → 语义权衡（亲和消耗一次尝试）；如需完整覆盖可调高 RetryTimes，管理员可配置。
- [偏移依赖 context 标志，未来重构可能破坏] → 显式常量 + 单元测试锁定；标志位于既有 channelAffinityMeta/Reusable context 体系内。
- [渠道 1 调用量上升（可能放大其 429）] → 修复的预期效果；管理员按需调整 priority/weight。
- [未修复「亲和失败不清缓存 → 下一请求仍命中渠道 6」] → 非目标，记录为后续独立 change。

## Migration Plan

- 纯代码修复，无 schema/配置迁移；`RetryTimes`、亲和规则配置语义不变。
- 回滚：还原 `getChannel` 偏移行 + `MarkChannelAffinityUsed` 标志行即可，无数据迁移。

## Open Questions

- 无（offset 方案、边界处理、测试范围已在 brainstorming 中与用户确认；「亲和失败后清缓存」留作后续 change）。
