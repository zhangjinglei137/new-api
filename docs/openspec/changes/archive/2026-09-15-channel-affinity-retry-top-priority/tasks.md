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
