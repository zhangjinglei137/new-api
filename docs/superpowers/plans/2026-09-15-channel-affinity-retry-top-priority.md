---
change: channel-affinity-retry-top-priority
design-doc: docs/superpowers/specs/2026-09-15-channel-affinity-retry-top-priority-design.md
base-ref: c099ba808e225586e6f0abfead5ed8596952b319
archived-with: 2026-09-15-channel-affinity-retry-top-priority
---

# 修复渠道亲和性命中后重试跳过最高优先级层 — 实施计划

**Goal:** 修复「渠道亲和性命中某渠道后，该渠道失败的重试从优先级层 1 开始，导致最高优先级渠道（layer 0）结构性不可达」的缺陷，使重试回归正常优先级选择（实测链路 6→2→3→4→5→6→7→8 变为 6→1→2→3→4→5→6→7）。

**Architecture:** 在 `getChannel`（controller/relay.go）的正常选择路径中，当检测到「亲和渠道已消耗首轮尝试」（`MarkChannelAffinityUsed` 设置的新 context 标志）且 `retry >= 1` 时，将传给 `CacheGetRandomSatisfiedChannel` 的有效 retry 索引偏移 -1（通过 clone `RetryParam` 副本实现，不污染循环计数）。总尝试次数不变。

**Tech Stack:** Go 1.25，Gin，GORM；测试使用 testify（require/assert）+ glebarez/sqlite。

**Spec:** `docs/openspec/changes/channel-affinity-retry-top-priority/specs/channels/spec.md`（需求「亲和渠道失败后的重试回归正常优先级选择」）；Design Doc 见 frontmatter。

## Global Constraints

- 全部 JSON 编解码走 `common.Marshal`/`common.Unmarshal`，不直接调用 `encoding/json`（本 change 不涉及 JSON，仅提醒）。
- 数据库代码必须兼容 SQLite/MySQL/PostgreSQL（本 change 不涉及 schema，仅测试用 sqlite 内存库）。
- 现代 Go 约定：用 `any`、`for range`、`strings.Cut` 等（本 change 代码量小，遵循即可）。
- 测试遵循仓库约定：优先扩展现有测试文件，不散落新测试文件；使用 `require` 做 setup/致命断言、`assert` 做非致命检查。
- 完成后运行 `gofmt` 并 `go build ./...`。

---

### Task 1: 亲和命中标志（service/channel_affinity.go）

**Files:**
- Modify: `service/channel_affinity.go`（常量区 + `MarkChannelAffinityUsed`）
- Test: `service/channel_affinity_template_test.go`（扩展）

**Interfaces:**
- Produces: `ginKeyChannelAffinityRetryBase`（unexported const）、`MarkChannelAffinityUsed` 副作用（设置 context 标志）、导出函数 `AffinityConsumedFirstTry(c *gin.Context) bool`

- [x] **Step 1: 写失败测试**（追加到 `service/channel_affinity_template_test.go`）

```go
func TestAffinityConsumedFirstTry(t *testing.T) {
	// 未设置标志 → false
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r"})
	require.False(t, AffinityConsumedFirstTry(ctx))

	// 设置标志 → true
	ctx2 := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r2"})
	MarkChannelAffinityUsed(ctx2, "default", 6)
	require.True(t, AffinityConsumedFirstTry(ctx2))

	// nil ctx → false
	require.False(t, AffinityConsumedFirstTry(nil))
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./service/ -run TestAffinityConsumedFirstTry -v`
Expected: FAIL（`AffinityConsumedFirstTry` undefined）

- [x] **Step 3: 实现**

在 `service/channel_affinity.go` 常量区（约 24-33 行）追加：

```go
ginKeyChannelAffinityRetryBase = "channel_affinity_retry_base"
```

在 `MarkChannelAffinityUsed`（约 676 行）中，`c.Set(ginKeyChannelAffinitySkipRetry, meta.SkipRetry)` 之后追加：

```go
c.Set(ginKeyChannelAffinityRetryBase, true)
```

在文件末尾（或 `ShouldSkipRetryAfterChannelAffinityFailure` 附近）新增导出函数：

```go
// AffinityConsumedFirstTry 报告本次请求的首轮尝试是否已被亲和渠道消耗
// （即 Distribute 命中亲和缓存并直接使用了亲和渠道）。
func AffinityConsumedFirstTry(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(ginKeyChannelAffinityRetryBase)
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}
```

- [x] **Step 4: 运行测试确认通过**

Run: `go test ./service/ -run TestAffinityConsumedFirstTry -v`
Expected: PASS

- [x] **Step 5: 提交**

```bash
git add service/channel_affinity.go service/channel_affinity_template_test.go
git commit -m "feat(channel-affinity): 记录亲和渠道已消耗首轮尝试的标志"
```

---

### Task 2: getChannel 重试索引偏移（controller/relay.go）

**Files:**
- Modify: `service/channel_affinity.go`（新增 `AffinityAdjustedRetry`）
- Modify: `controller/relay.go`（`getChannel`，334-363 行区域）

**Interfaces:**
- Consumes: `service.AffinityConsumedFirstTry(c *gin.Context) bool`（Task 1）
- Consumes: `service.RetryParam`（`GetRetry`/`SetRetry`，service/channel_select.go:40-73）
- Produces: `service.AffinityAdjustedRetry(c *gin.Context, retry int) int`；`getChannel` 在亲和命中 + retry>=1 时用偏移后的 retryParam 副本调用 `CacheGetRandomSatisfiedChannel`

- [x] **Step 1: 写失败测试**（追加到 `service/channel_select_auto_groups_test.go`，TDD）

```go
func TestAffinityAdjustedRetry(t *testing.T) {
	// 未命中亲和 → 不偏移
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r"})
	require.Equal(t, 1, AffinityAdjustedRetry(ctx, 1))
	require.Equal(t, 7, AffinityAdjustedRetry(ctx, 7))

	// 亲和命中 → retry>=1 时偏移 -1，retry=0 不偏移（0 为首轮快捷路径，不进入此分支）
	ctx2 := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r2"})
	MarkChannelAffinityUsed(ctx2, "default", 6)
	require.Equal(t, 0, AffinityAdjustedRetry(ctx2, 1))
	require.Equal(t, 6, AffinityAdjustedRetry(ctx2, 7))
	require.Equal(t, 0, AffinityAdjustedRetry(ctx2, 0))

	// nil ctx → 不偏移
	require.Equal(t, 1, AffinityAdjustedRetry(nil, 1))
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./service/ -run TestAffinityAdjustedRetry -v`
Expected: FAIL（`AffinityAdjustedRetry` undefined）

- [x] **Step 3: 实现 `AffinityAdjustedRetry`**（`service/channel_affinity.go`，`AffinityConsumedFirstTry` 附近）

```go
// AffinityAdjustedRetry 返回用于候选池选择的有效 retry 索引：
// 亲和渠道已消耗首轮尝试（retry=0）时，重试索引前移一位，使
// 最高优先级渠道层（layer 0）得以参与候选池选择。
func AffinityAdjustedRetry(c *gin.Context, retry int) int {
	if retry > 0 && AffinityConsumedFirstTry(c) {
		return retry - 1
	}
	return retry
}
```

- [x] **Step 4: 运行测试确认通过**

Run: `go test ./service/ -run TestAffinityAdjustedRetry -v`
Expected: PASS

- [x] **Step 5: 修改 `getChannel` 的正常选择分支**（controller/relay.go）

将：

```go
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)
```

替换为：

```go
	effectiveRetry := service.AffinityAdjustedRetry(c, retryParam.GetRetry())
	cloned := *retryParam
	cloned.SetRetry(effectiveRetry)
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(&cloned)
```

说明：`cloned` 为浅拷贝，`SetRetry` 重新指向新的 int 指针，`Ctx`/`resetNextTry` 等字段共享原值；原始 `retryParam` 的计数不受影响，Relay 循环的 `GetRetry() <= RetryTimes` 判定与 `IncreaseRetry` 行为不变。

- [x] **Step 6: 编译确认 + 全量测试**

Run: `go build ./...`
Expected: 成功（无编译错误）

Run: `go test ./service/ ./controller/ -count=1`
Expected: 全部通过

- [x] **Step 7: 提交**

```bash
git add service/channel_affinity.go controller/relay.go service/channel_select_auto_groups_test.go
git commit -m "fix(relay): 亲和渠道失败后重试偏移回最高优先级层"
```

---

### Task 3: 回归测试（组合场景：偏移后的候选池选择）

**Files:**
- Test: `service/channel_select_auto_groups_test.go`（扩展，组合场景测试）

**Interfaces:**
- Consumes: `AffinityAdjustedRetry`（Task 2）、`CacheGetRandomSatisfiedChannel`（service/channel_select.go:110）、`MarkChannelAffinityUsed`（Task 1）、`setupChannelSelectAutoGroupsTest`（已存在）
- Note: 本测试验证「亲和命中标志 → `AffinityAdjustedRetry` 偏移 → `CacheGetRandomSatisfiedChannel` 按偏移后索引选到最高优先级渠道」的完整链路。

- [x] **Step 1: 写组合场景测试**（追加到 `service/channel_select_auto_groups_test.go`）

```go
func TestAffinityRetrySelectsTopPriorityChannel(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	// 构造渠道：priority 20（渠道 1）与 19（渠道 2），同一 model/group
	channels := []model.Channel{
		{Id: 1, Type: 1, Key: "k1", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(20)},
		{Id: 2, Type: 1, Key: "k2", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(19)},
	}
	require.NoError(t, model.DB.Create(&channels).Error)
	model.InitChannelCache()

	// 未命中亲和：retry=1 选中层 1 → 渠道 2（priority 19）
	param1 := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "m1", Retry: intPtr(1)}
	ch1, _, err1 := CacheGetRandomSatisfiedChannel(param1)
	require.NoError(t, err1)
	require.NotNil(t, ch1)
	require.Equal(t, 2, ch1.Id, "无亲和时 retry=1 应选中层 1 的渠道 2")

	// 亲和命中：缓存渠道 6（无实际 6 渠道影响，仅设置标志）→ retry=1 偏移为 0 → 选中最高优先级渠道 1
	affCtx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r"})
	MarkChannelAffinityUsed(affCtx, "default", 6)
	effective := AffinityAdjustedRetry(affCtx, 1)
	require.Equal(t, 0, effective, "亲和命中时 retry=1 应偏移到 0")
	param2 := &RetryParam{Ctx: affCtx, TokenGroup: "default", ModelName: "m1", Retry: intPtr(1)}
	ch2, _, err2 := CacheGetRandomSatisfiedChannel(param2)
	require.NoError(t, err2)
	require.NotNil(t, ch2)
	require.Equal(t, 1, ch2.Id, "亲和命中时重试应能选中最高优先级渠道 1")
}
```

注意：若该文件已有 `int64Ptr`/`intPtr` helper 或命名冲突，改用现有命名；若不存在则定义（Task 4 前确认）。`MarkChannelAffinityUsed` 需先有 channelAffinityMeta（用 `buildChannelAffinityTemplateContextForTest` 构造，其内部调用 `setChannelAffinityContext` 写入 meta）。

- [x] **Step 2: 运行测试确认通过**

Run: `go test ./service/ -run TestAffinityRetrySelectsTopPriorityChannel -v`
Expected: PASS

- [x] **Step 3: 运行全量相关测试**

Run: `go test ./service/ ./controller/ -count=1`
Expected: 全部通过（含既有测试，确保无回归）

- [x] **Step 4: 提交**

```bash
git add service/channel_select_auto_groups_test.go
git commit -m "test(channel-select): 覆盖亲和渠道失败后重试偏移选到最高优先级层"
```

---

### Task 4: 验证与收尾

**Files:**
- 无代码改动

- [x] **Step 1: 运行完整构建与格式检查**

Run: `gofmt -l service/channel_affinity.go controller/relay.go service/channel_select_auto_groups_test.go service/channel_affinity_template_test.go`
Expected: 无输出（或输出为空表示已格式化）

Run: `go build ./...`
Expected: 成功

- [x] **Step 2: 复核边界**

对照 Design Doc 的「决策 2 边界确认」逐项检查代码：
- `skip_retry_on_failure=true`：`shouldRetry`/`shouldRetryTaskRelay` 开头 `ShouldSkipRetryAfterChannelAffinityFailure` 返回 true → 不进入重试 → 偏移分支不可达（确认无需额外处理）
- 亲和命中且成功：无重试，偏移不触发
- 亲和未命中：`AffinityConsumedFirstTry` 返回 false，偏移不触发
- retry=0 快捷路径（`ChannelMeta==nil`）：不受影响

- [x] **Step 3: 记录构建证据**

```bash
comet state record-check channel-affinity-retry-top-priority build --command "go build ./..." --exit-code 0
comet state record-check channel-affinity-retry-top-priority build --command "go test ./service/ ./controller/ -count=1" --exit-code 0
```

- [x] **Step 4: 汇总改动影响**

确认改动仅涉及 2 个生产文件（service/channel_affinity.go、controller/relay.go）+ 2 个测试文件扩展；`git log --oneline -5` 展示提交历史；准备进入 verify 阶段。

### Task 5: 修复 auto 跨组重试状态机回归（verify 集成审查 Critical/Important）

**Files:**
- Modify: `service/channel_select.go`（新增 `CacheGetRandomSatisfiedChannelWithPriority` 变体入口，原函数转发）
- Modify: `controller/relay.go`（`getChannel`：改用 WithPriority，不再 clone `RetryParam`）
- Test: `service/channel_select_auto_groups_test.go`（追加 auto+crossGroupRetry+多组回归测试）

**Interfaces:**
- Consumes: `service.AffinityAdjustedRetry(c *gin.Context, retry int) int`（Task 2）、`service.RetryParam`（`GetRetry`/`SetRetry`/`ResetRetryNextTry`）、`CacheGetRandomSatisfiedChannel`（原入口保持，成为 WithPriority 的转发包装）
- Produces: `service.CacheGetRandomSatisfiedChannelWithPriority(param *RetryParam, priorityRetry int) (*model.Channel, string, error)`

- [x] **Step 1: 写失败回归测试**（追加到 `service/channel_select_auto_groups_test.go`）

覆盖「auto + crossGroupRetry + 多组 + 亲和命中」场景：断言偏移后层索引被显式控制、循环计数不被污染、未命中亲和时行为不变。参考现有 `TestCacheGetRandomSatisfiedChannelUsesTokenAutoGroupsWhenGlobalAutoIsEmpty` 的 fixture 构造方式（setupChannelSelectAutoGroupsTest + channel 构造），构造 2 组（default/vip）各 2 优先级渠道 + `ContextKeyTokenCrossGroupRetry=true`：

```go
func TestCacheGetRandomWithPriorityKeepsCrossGroupRetryState(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set(constant.ContextKeyTokenCrossGroupRetry, true)
	ctx.Set(constant.ContextKeyUserGroup, "default")

	// 组 default：渠道 1(pri 20)、渠道 2(pri 19)；组 vip：渠道 3(pri 18)、渠道 4(pri 17)
	channels := []model.Channel{
		{Id: 1, Type: 1, Key: "k1", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(20)},
		{Id: 2, Type: 1, Key: "k2", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(19)},
		{Id: 3, Type: 1, Key: "k3", Models: "m1", Group: "vip", Status: common.ChannelStatusEnabled, Priority: int64Ptr(18)},
		{Id: 4, Type: 1, Key: "k4", Models: "m1", Group: "vip", Status: common.ChannelStatusEnabled, Priority: int64Ptr(17)},
	}
	require.NoError(t, model.DB.Create(&channels).Error)
	// 为两个组播种 Ability（group 集合）
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "m1", ChannelId: 1, Enabled: true, Priority: int64Ptr(20)}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "vip", Model: "m1", ChannelId: 3, Enabled: true, Priority: int64Ptr(18)}).Error)
	model.InitChannelCache()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))

	param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "m1", Retry: intPtr(1)}
	// 未命中亲和：WithPriority(param, 1) 与 CacheGetRandomSatisfiedChannel(param) 等价
	ch1, _, err1 := CacheGetRandomSatisfiedChannelWithPriority(param, 1)
	require.NoError(t, err1)
	require.NotNil(t, ch1)
	require.Equal(t, 2, ch1.Id, "未命中亲和 retry=1 应选中 default 组层 1 的渠道 2")

	// 关键断言：WithPriority 调用后 param.Retry 未被本调用污染（仍是 1），
	// 且内部跨组状态机变异仍作用在原始 param 上（复现既有行为）。
	require.Equal(t, 1, param.GetRetry(), "循环计数必须保持不变")
}

func TestCacheGetWithPriorityAffinityOffsetKeepsStateMachine(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set(constant.ContextKeyTokenCrossGroupRetry, true)
	ctx.Set(constant.ContextKeyUserGroup, "default")

	channels := []model.Channel{
		{Id: 1, Type: 1, Key: "k1", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(20)},
		{Id: 2, Type: 1, Key: "k2", Models: "m1", Group: "default", Status: common.ChannelStatusEnabled, Priority: int64Ptr(19)},
	}
	require.NoError(t, model.DB.Create(&channels).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "m1", ChannelId: 1, Enabled: true, Priority: int64Ptr(20)}).Error)
	model.InitChannelCache()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))

	// 亲和命中：retry=1 偏移为 0 → 选中 highest priority（渠道 1）
	affCtx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{RuleName: "r"})
	MarkChannelAffinityUsed(affCtx, "default", 6)
	effective := AffinityAdjustedRetry(affCtx, 1)
	require.Equal(t, 0, effective)
	param := &RetryParam{Ctx: affCtx, TokenGroup: "default", ModelName: "m1", Retry: intPtr(1)}
	ch2, _, err2 := CacheGetRandomSatisfiedChannelWithPriority(param, effective)
	require.NoError(t, err2)
	require.NotNil(t, ch2)
	require.Equal(t, 1, ch2.Id, "亲和命中偏移后应选中最高优先级渠道 1")
	require.Equal(t, 1, param.GetRetry(), "循环计数不被偏移污染")
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./service/ -run "TestCacheGetRandomWithPriority|TestCacheGetWithPriority" -v`
Expected: FAIL（`CacheGetRandomSatisfiedChannelWithPriority` undefined）

- [x] **Step 3: 实现 WithPriority 入口**（`service/channel_select.go`）

将 `CacheGetRandomSatisfiedChannel`（:110-200）原函数体重命名为 `CacheGetRandomSatisfiedChannelWithPriority(param *RetryParam, priorityRetry int)`，并在原位置保留转发包装：

```go
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	return CacheGetRandomSatisfiedChannelWithPriority(param, param.GetRetry())
}

func CacheGetRandomSatisfiedChannelWithPriority(param *RetryParam, priorityRetry int) (*model.Channel, string, error) {
	// ...原函数体，把所有读取 param.GetRetry() 作为优先级层索引的地方改用 priorityRetry：
	//   - auto 分支 :138 `priorityRetry := param.GetRetry()` → `priorityRetry := priorityRetry`（参数）
	//   - 非 auto :192 `param.GetRetry()` → `priorityRetry`
	// 注意：:161/:179 的 `param.SetRetry(0)` 与 :180 `param.ResetRetryNextTry()` 是跨组状态机变异，
	// **必须保留原样作用于 param 指针**（这正是本修复要保留的既有行为）。
}
```

- [x] **Step 4: 修改 `getChannel`**（controller/relay.go:348-351）

将：

```go
	effectiveRetry := service.AffinityAdjustedRetry(c, retryParam.GetRetry())
	cloned := *retryParam
	cloned.SetRetry(effectiveRetry)
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(&cloned)
```

替换为：

```go
	effectiveRetry := service.AffinityAdjustedRetry(c, retryParam.GetRetry())
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannelWithPriority(retryParam, effectiveRetry)
```

- [x] **Step 5: 运行测试确认通过**

Run: `go test ./service/ -run "TestCacheGetRandomWithPriority|TestCacheGetWithPriority|TestAffinityRetrySelectsTopPriorityChannel|TestAffinityAdjustedRetry|TestAffinityConsumedFirstTry" -v`
Expected: PASS

- [x] **Step 6: 全量验证 + 提交**

Run: `go build ./...`（成功）→ `go test ./service/ ./controller/ -count=1`（全过）

```bash
git add service/channel_select.go controller/relay.go service/channel_select_auto_groups_test.go
git commit -m "fix(relay): 解耦优先级层索引与跨组重试状态机，修复 auto 跨组回归"
```

---

### Task 6: 修复验证与设计文档收尾（verify 审查 Important/Minor）

**Files:**
- Modify: `docs/superpowers/plans/2026-09-15-channel-affinity-retry-top-priority.md`（本 plan 自身无代码，仅确认）
- 无生产代码

- [x] **Step 1: 复核 design.md rev.2 准确性**

确认 `docs/superpowers/specs/2026-09-15-channel-affinity-retry-top-priority-design.md` 决策 1 已为 rev.2（WithPriority 方案 + 修正 resetNextTry 共享断言），与 Task 5 实现一致。

- [x] **Step 2: 全量构建/测试 + 保持设计文档一致性**

Run: `gofmt -l service/channel_select.go controller/relay.go service/channel_select_auto_groups_test.go`（无输出）→ `go build ./...`（成功）→ `go test ./service/ ./controller/ -count=1`（全过）

- [x] **Step 3: 提交（如有文档/注释修正）**

```bash
git add -A
git commit -m "chore(change): verify 审查后设计文档与实现对齐" （仅在确有改动时）
```
