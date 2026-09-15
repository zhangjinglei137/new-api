# Comet Design Handoff

- Change: channel-affinity-retry-top-priority
- Phase: design
- Mode: compact
- Context hash: 0547d0a7ea1a4014986e6d6547800c1d01159ba3ea50f04484aa5c9b916b5568

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## docs/openspec/changes/channel-affinity-retry-top-priority/proposal.md

- Source: docs/openspec/changes/channel-affinity-retry-top-priority/proposal.md
- Lines: 1-29
- SHA256: 70c2b55d8e988e879b4d60ea3beb39e4b4389693cafd90580f1a58df4ce8c9f7

```md
# Proposal: 修复渠道亲和性命中后重试跳过最高优先级层

## Why

渠道亲和性（Channel Affinity）命中上次成功渠道后，请求会直接使用该渠道并占用 retry=0；一旦该渠道失败，重试从 retry=1 开始逐优先级层推进，导致候选池中**最高优先级渠道（位于 layer 0）在亲和命中期间结构性不可达**。实测案例：渠道 1（priority=20，最高）24 小时内完全不在亲和命中的重试链路中出现（链路表现为 6→2→3→4→5→6→7→8），而渠道 1 在非亲和路径下完全正常（24h 内 187 次成功调用），证明该缺陷与渠道本身可用性无关。

## What Changes

- 修复亲和渠道失败后的重试起点：当亲和渠道失败进入重试时，重试应从候选池正常选择重新开始，使最高优先级渠道（layer 0）能够参与重试，而不是被亲和渠道占用的 retry=0 永久跳过。
- 保持渠道亲和性的既有语义不变：亲和命中仍优先使用亲和渠道；仅当亲和渠道失败后，重试链路回归正常优先级选择。
- 保持 retry 索引的既有语义：不改变 `RetryTimes` 总量与优先级层 clamp 行为，只修正「retry=0 被亲和渠道独占导致 layer 0 不可达」这一边界。
- 不修改渠道权重（weight）随机选择逻辑与优先级（priority）排序逻辑。

## Capabilities

### New Capabilities

无新增 capability。

### Modified Capabilities

- `channels`: 渠道选择在「渠道亲和性命中且该渠道失败后重试」场景下的行为需求变更——重试 MUST 允许候选池最高优先级渠道（layer 0）参与，最高优先级渠道不得因亲和渠道占用 retry=0 而结构性不可达。

## Impact

- 后端代码：`middleware/distributor.go`（Distribute 中亲和性命中分支）、`controller/relay.go`（Relay 重试循环的 getChannel / retry 起点）、`service/channel_select.go`（RetryParam 语义，视方案而定）、`model/channel_cache.go`（GetRandomSatisfiedChannel 的 retry 参数消费方式，视方案而定）。
- 行为影响：亲和渠道失败后的重试顺序会变化（从「跳过 layer 0」变为「回归正常优先级选择」），可能增加最高优先级渠道的调用量。
- 配置影响：无需新增配置项；现有 `RetryTimes`、渠道亲和性规则（含 `skip_retry_on_failure`）语义保持不变。
- 测试影响：需要为「亲和命中 + 失败 + 重试回归最高优先级」路径补充回归测试。

```

## docs/openspec/changes/channel-affinity-retry-top-priority/design.md

- Source: docs/openspec/changes/channel-affinity-retry-top-priority/design.md
- Lines: 1-73
- SHA256: f51f51a48ce83202273f4a05d389e36c7d30942406cc40c20c317df6cd2e8b64

```md
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

```

## docs/openspec/changes/channel-affinity-retry-top-priority/tasks.md

- Source: docs/openspec/changes/channel-affinity-retry-top-priority/tasks.md
- Lines: 1-29
- SHA256: 375c05e1661746dd9d962de9b46f21162ae16cdc60128d72b77cf6ce37d7aad1

```md
# Tasks: 修复渠道亲和性命中后重试跳过最高优先级层

## 1. 亲和命中标记与重试基准备

- [x] 1.1 在 `service/channel_affinity.go` 中新增亲和命中重试基线标志：`MarkChannelAffinityUsed` 被调用时记录「亲和渠道已消耗首轮尝试」（context 标志，如 `ginKeyChannelAffinityRetryBase`），并验证该标志随请求生命周期存在
- [x] 1.2 在 `controller/relay.go` 的 `getChannel` 正常选择路径中读取该标志，结合 `retryParam.GetRetry() >= 1` 判定本次重试需要 retry-1 偏移，并验证：亲和未命中时无标志、不偏移，行为与现状一致

## 2. 重试索引偏移实现

- [x] 2.1 实现偏移逻辑：亲和渠道已作为首轮尝试且失败进入重试时，调用 `CacheGetRandomSatisfiedChannel` 使用 `retryParam.GetRetry() - 1` 作为优先级层索引；验证 RetryTimes=7、8 个渠道、亲和命中渠道 6 时链路变为 6→1→2→3→4→5→6→7（渠道 1 参与重试且总数不变）
- [x] 2.2 确认 `retry=0` 快捷路径（`info.ChannelMeta == nil` 直接返回 context 渠道）不受影响：首轮仍走亲和渠道，只为后续重试启用偏移
- [x] 2.3 确认 `skip_retry_on_failure=true` 的亲和规则不进入偏移分支：`shouldRetry` 返回 false 时重试循环不执行，行为与现状一致

## 3. 回归测试

- [x] 3.1 扩展现有测试（优先 `service/channel_select_auto_groups_test.go` 或 controller 层对应测试文件）：构造「亲和命中 context + retry=1..RetryTimes」序列，断言传入 `CacheGetRandomSatisfiedChannel` 的 retry 索引为 `GetRetry()-1`
- [x] 3.2 添加关键场景回归：亲和命中渠道失败 → 重试包含最高优先级渠道；亲和未命中 → retry 索引不偏移；亲和渠道成功 → 无重试；`skip_retry_on_failure=true` → 不进入重试；运行 `go test ./service/... ./controller/...` 验证全部通过
- [x] 3.3 运行 `go build ./...` 与 `gofmt` 检查，确认无编译错误与格式问题

## 4. 验证与收尾

- [x] 4.1 结合本 change 场景复核 `middleware/distributor.go` 亲和命中分支与 `controller/relay.go` 重试循环的边界：亲和缓存清除（`keep_on_channel_disabled=false` 时渠道禁用场景）与 retry 基线标志无冲突
- [x] 4.2 汇总改动影响（渠道选择顺序变化）并确认 proposal/design/specs 与实际实现一致，准备进入 verify 阶段

## 5. verify 审查修复（auto 跨组重试状态机回归）

- [x] 5.1 新增 `CacheGetRandomSatisfiedChannelWithPriority(param, priorityRetry)` 变体入口（原函数转发），getChannel 改用显式层索引，不再克隆 `RetryParam`；并验证 `go test ./service/ -run "TestCacheGetRandomWithPriority|TestCacheGetWithPriority" -v` 通过
- [x] 5.2 修复后验证未命中亲和（auto+crossGroupRetry+多组）行为与现状一致：循环计数不被污染、后续组中间优先级层可达；运行 `go build ./...` 与 `go test ./service/ ./controller/ -count=1` 全部通过
- [x] 5.3 复核 design.md 决策 1 已更新为 rev.2（WithPriority 方案 + 修正 resetNextTry 共享性断言），与实现一致

```

## docs/openspec/changes/channel-affinity-retry-top-priority/specs/channels/spec.md

- Source: docs/openspec/changes/channel-affinity-retry-top-priority/specs/channels/spec.md
- Lines: 1-20
- SHA256: 778089f8e800dec72faf06ab6ae4aef4059f929f58a55162d421ee3eb42ba21a

```md
## ADDED Requirements

### Requirement: 亲和渠道失败后的重试回归正常优先级选择

当渠道亲和性命中某渠道且该渠道处理请求失败进入重试时，系统 MUST 允许候选池中最高优先级渠道（优先级层级 layer 0）参与后续重试。亲和渠道占用首次尝试（retry=0）MUST NOT 导致最高优先级渠道在整个重试链路中结构性不可达；重试链路 MUST 从候选池正常优先级选择重新开始。

#### Scenario: 亲和渠道失败后重试包含最高优先级渠道

- **WHEN** 渠道亲和性命中渠道 X（非最高优先级），渠道 X 失败后系统进入重试
- **THEN** 后续重试依次按候选池正常优先级降序选择渠道，最高优先级渠道（如渠道 A）参与候选，而不是直接从亲和渠道所在优先级层的下一层开始

#### Scenario: 亲和命中且渠道成功时不改变行为

- **WHEN** 渠道亲和性命中渠道 X 且该渠道处理成功
- **THEN** 请求直接使用渠道 X，不触发额外选择，重试行为与现状一致

#### Scenario: 亲和性未命中时不改变行为

- **WHEN** 渠道亲和性未命中（无缓存、TTL 过期或规则不匹配）
- **THEN** 首次尝试与重试均按现状从候选池最高优先级渠道开始选择

```
