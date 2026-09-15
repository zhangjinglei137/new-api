# Design: 修复渠道亲和性命中后重试跳过最高优先级层

## Context

渠道选择的核心路径（见 proposal.md 的 Why）：

- `middleware/distributor.go` `Distribute()`：亲和性命中时直接 `channel = preferred`（跳过候选池选择），失败且未开启 keep_on_channel_disabled 时清除缓存。
- `controller/relay.go` `Relay()` 重试循环：`for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry()`；`getChannel` 中 `info.ChannelMeta == nil`（首轮）时直接返回 context 中已选渠道（快捷路径），retry>=1 才调用 `CacheGetRandomSatisfiedChannel`。
- `model/channel_cache.go` `GetRandomSatisfiedChannel`：`targetPriority := sortedUniquePriorities[retry]`，retry 索引直接映射唯一优先级层；retry 越界 clamp 到最低层（不回绕）。

缺陷根因：亲和渠道占用 retry=0（快捷路径不进入候选池选择），失败后 retry=1..N 从**优先级层 1** 开始逐层推进，**层 0（最高优先级渠道）在整个重试链中结构性不可达**。实测：渠道 1（priority=20 最高）在亲和命中的重试链路中从不出现，而非亲和路径下完全正常（24h 内 187 次成功调用）。

## Goals / Non-Goals

**Goals:**
- 亲和渠道失败后，本次请求的重试链路回归正常优先级选择，最高优先级渠道（layer 0）参与候选。
- 保持亲和性的既有语义：亲和命中仍优先使用亲和渠道；亲和渠道成功时不引入任何额外行为。
- 保持重试总量语义：总尝试次数不因修复而增加（亲和 1 次 + 重试 N 次，与 RetryTimes 语义一致）。
- 保持 `skip_retry_on_failure`、`keep_on_channel_disabled`、权重随机、优先级排序、clamp 不回绕等既有行为不变。

**Non-Goals:**
- 不修改渠道权重（weight）/优先级（priority）的配置与选择逻辑本身。
- 不修改亲和缓存的写入时机（仍仅成功时记录）与 TTL 语义。
- 不处理「亲和失败后缓存是否清除」的决策（`keep_on_channel_disabled` 语义不变；请求失败不清缓存的行为维持现状，可作为后续独立 change）。
- 不解决「多渠道同上游导致 429 重试空转」的问题（这是渠道配置层面的运维问题，非本 change 范围）。
- 不改变 `RetryTimes` 配置项及其含义。

## Decisions

### 决策 1：亲和渠道失败后的重试索引前移一位（retry-1 映射）

> **rev.2（verify 集成审查修正）**：最终实现放弃「浅拷贝 RetryParam 副本传偏移」方案（会破坏 `CacheGetRandomSatisfiedChannel` 内部跨组状态机变异 `SetRetry(0)`/`ResetRetryNextTry`，且无条件克隆违反「未命中不改变行为」），改为**新增 `CacheGetRandomSatisfiedChannelWithPriority(param, priorityRetry)` 变体入口**解耦「优先级层索引」与「循环计数/跨组状态机」：getChannel 直接传原始 retryParam + 显式 effectiveRetry（`AffinityAdjustedRetry` 结果），不克隆、不写回 `param.Retry`。详见 `docs/superpowers/specs/2026-09-15-channel-affinity-retry-top-priority-design.md` 决策 1 rev.2。

**方案**：在 `getChannel` 的正常选择路径中，若本次请求「亲和命中且亲和渠道已作为首轮尝试」（即 retry>=1 且 context 存在亲和命中标记），调用 `CacheGetRandomSatisfiedChannel` 时使用 `retryParam.GetRetry() - 1` 作为优先级层索引。

**效果**（RetryTimes=7、8 个渠道、亲和命中渠道 6 的场景）：

| retry | 现状行为 | 修复后行为 |
|---|---|---|
| 0 | 快捷路径 → 渠道 6（亲和） | 同左 |
| 1 | 层 1 → 渠道 2 | **层 0 → 渠道 1** |
| 2 | 层 2 → 渠道 3 | 层 1 → 渠道 2 |
| 3 | 层 3 → 渠道 4 | 层 2 → 渠道 3 |
| ... | ... | ... |
| 7 | 层 7 → 渠道 8 | 层 6 → 渠道 7 |

总尝试次数不变（1 次亲和 + 7 次重试 = 8 次）；最坏情况下最低优先级渠道（渠道 8）在亲和失败场景中不再被覆盖——这是「亲和渠道额外消耗一次尝试」的合理权衡，与 RetryTimes 的「最多尝试 N+1 次」语义保持一致。

**备选方案（已否决）**：
- A. 让 retry=1 额外从 layer 0 开始并完整覆盖 layer 0..RetryTimes：总尝试次数变为 1+8=9 次，突破 RetryTimes 语义，否决。
- B. 亲和失败后清除亲和缓存并重置 retry：只影响后续请求，本次请求的重试链仍跳过 layer 0，未修复核心缺陷，否决。
- C. 修改 `GetRandomSatisfiedChannel` 的索引算法（取模回绕）：会改变所有路径（含非亲和）的重试语义，副作用大，否决。

### 决策 2：亲和命中标记的传递

**方案**：亲和命中发生在 `middleware/distributor.go` 的 `Distribute()` 中，此时调用 `MarkChannelAffinityUsed` 已把亲和元数据写入 context（`ginKeyChannelAffinityMeta`）。`getChannel`（controller/relay.go）读取该 context 判定「本次请求亲和命中」，并结合 `retryParam.GetRetry() >= 1`（说明亲和渠道已作为首轮尝试且失败）决定是否做 retry-1 偏移。

**要点**：
- 亲和未命中时 context 无亲和元数据 → 不偏移，行为与现状完全一致。
- 亲和命中但渠道成功（retry 循环第一次就返回）→ 无重试，不进入偏移分支。
- 亲和命中但 `skip_retry_on_failure=true` → `shouldRetry` 返回 false，不进入重试，偏移分支不可达。
- 需要新增一个轻量 context 标志（如 `ginKeyChannelAffinityRetryBase`）或在现有 meta 中附加字段，明确记录「亲和渠道已消耗首轮尝试」，避免与「亲和命中但首轮走正常选择」的边界混淆（当前实现中亲和命中必定走快捷路径，该边界理论不冲突，但仍以显式标志为准，防未来重构破坏）。

### 决策 3：测试策略

- 单元测试覆盖 `getChannel` 的 retry 偏移逻辑：构造亲和命中 context + retry=1..RetryTimes 序列，断言 `CacheGetRandomSatisfiedChannel` 收到的 retry 索引为 `GetRetry()-1`。
- 集成/回归测试覆盖关键场景：亲和命中失败 → 重试包含最高优先级渠道；亲和未命中 → 行为不变；亲和成功 → 无重试；`skip_retry_on_failure=true` → 不进入重试。
- 遵循后端测试质量约定：优先扩展现有测试文件（如 `service/channel_select_auto_groups_test.go` 相关测试或 controller 层测试），不散落新测试文件。

## Risks / Trade-offs

- [亲和失败场景下最低优先级渠道不再被覆盖（retry 总次数不变）] → 属于「亲和渠道消耗一次尝试」的语义结果，在 design 中明确记录；如需完整覆盖可提高 RetryTimes 配置（运维可调，不改变代码语义）。
- [retry 偏移依赖 context 标志，未来 relay 重构可能破坏] → 决策 2 使用显式标志 + 单元测试锁定行为。
- [最高优先级渠道调用量增加（可能放大其 429/负载）] → 这是修复的预期效果（渠道 1 本应参与候选）；权重/优先级配置由管理员按需调整。
- [仅修复「层 0 跳过」，未修复「亲和失败不清缓存导致下请求仍钉在渠道 6」] → 明确为非目标，可在后续独立 change 处理；本次改动不与缓存清除逻辑冲突。
