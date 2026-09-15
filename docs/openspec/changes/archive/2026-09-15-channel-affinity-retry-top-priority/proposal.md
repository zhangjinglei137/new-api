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
