---
change: channel-sort-field
design-doc: docs/superpowers/specs/2026-09-12-channel-sort-field-design.md
base-ref: 74d703b269e1c5a1b3f789f9b9d4e2556b25f091
---

# 渠道序号字段与排序 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为渠道新增独立于 Priority 的「序号」（sort）字段，支持列表按序号排序（列头点击 + 使用序号排序开关 + 默认升序）。

**Architecture:** 后端 `Channel` 模型加 `Sort *int64` 字段，`ChannelSortOptions` 加 `SortSort` 开关并调整 `Apply` 分支优先级（列头 > 序号开关 > ID开关 > 默认 sort ASC）；前端扩展类型/状态/列/开关/表单/卡片，复用既有 `ChannelFieldCell` 内联编辑与 `DataTableColumnHeader` 排序机制；迁移走 GORM AutoMigrate。

**Tech Stack:** Go 1.25 / GORM v2、React 19 / TypeScript / TanStack Table / Zod / i18next、Bun。

**Spec:** `docs/openspec/changes/channel-sort-field/specs/channels/spec.md`

## Global Constraints

- 后端 JSON 编解码一律走 `common.Marshal` / `common.Unmarshal`（`common/json.go`），不直接调 `encoding/json`
- 数据库代码必须兼容 SQLite / MySQL >= 5.7.8 / PostgreSQL >= 9.6；迁移改动需三库验证
- 前端 UI 文案必须走 i18n（`t('English key')`），key 为英文源串
- 前端复用既有组件：`ChannelFieldCell` / `NumericSpinnerInput` / `Switch` / `DropdownMenuCheckboxItem` / `DataTableColumnHeader`
- Go 代码遵循现代约定（`any`、`for i := range n`、`slices`、`strings.Cut` 等），修改文件用 `gofmt` 格式化并清理未用 import
- 序号字段不改变 `priority` 的负载均衡语义，不改 relay 路由逻辑
- tag 模式下 tag 聚合行的排序保持 `priority desc` 现状（子渠道仍按本规则）

---

## 1. 后端：模型与排序逻辑

### Task 1: Channel 模型新增 sort 字段

- [x] Task 1: Channel 模型新增 sort 字段

**Files:**
- Modify: `model/channel.go`（`Channel` 结构体，`Priority` 字段后）
- Test: `model/channel_constraint_test.go`（就近扩展）

**Interfaces:**
- Produces: `Channel.Sort *int64`（JSON key `sort`，gorm `bigint;default:0`）

- [x] **Step 1: 写失败测试（TDD 模式）**

在 `model/channel_constraint_test.go` 添加测试：断言 `Channel` 结构体包含 `Sort` 字段且默认值语义为 0。

```go
func TestChannelHasSortField(t *testing.T) {
	require := require.New(t)
	ch := &Channel{}
	require.Nil(ch.Sort) // 指针字段零值为 nil；新建渠道默认 0 由请求归一化处理
}
```

（若 `channel_constraint_test.go` 无合适的 require 导入模式，先按文件现有风格调整。）

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./model/ -run TestChannelHasSortField -count=1`
Expected: 编译失败或 `Sort` 未定义

- [x] **Step 3: 实现字段**

`model/channel.go` 的 `Channel` 结构体 `Priority` 字段后新增：

```go
Sort *int64 `json:"sort" gorm:"bigint;default:0"` // 序号，用于渠道列表展示排序，独立于 Priority
```

- [x] **Step 4: 运行测试确认通过**

Run: `go test ./model/ -run TestChannelHasSortField -count=1`
Expected: PASS

- [x] **Step 5: 提交**

```bash
git add model/channel.go model/channel_constraint_test.go
git commit -m "feat(channel): 新增 sort 序号字段"
```

### Task 2: 排序白名单与 ChannelSortOptions 扩展

- [x] Task 2: 排序白名单与 ChannelSortOptions 扩展

**Files:**
- Modify: `model/channel.go`（`channelSortColumns`、`ChannelSortOptions`、`NewChannelSortOptions`、`Apply`、`resolveChannelSortOptions`）
- Test: `model/channel_constraint_test.go`

**Interfaces:**
- Produces: `NewChannelSortOptions(sortBy string, sortOrder string, idSort bool, sortSort bool) ChannelSortOptions`；`ChannelSortOptions` 含 `SortSort bool`；`Apply` 分支：`SortBy` → `SortSort`(sort ASC) → `IDSort`(id DESC) → 默认 sort ASC

- [x] **Step 1: 写失败测试**

`model/channel_constraint_test.go` 添加表驱动测试：

```go
func TestChannelSortOptionsApplyPriority(t *testing.T) {
	// sort 白名单生效：sort_by=sort&sort_order=asc
	opts := NewChannelSortOptions("sort", "asc", false, false)
	// SortSort 优先于 IDSort
	opts2 := NewChannelSortOptions("", "", true, true)
	// 列头排序优先于开关
	opts3 := NewChannelSortOptions("name", "desc", false, true)
	// 默认无参数 -> sort ASC（替代 priority desc）
	opts4 := NewChannelSortOptions("", "", false, false)
	// 仅 IDSort -> id desc（保持现有行为）
	opts5 := NewChannelSortOptions("", "", true, false)

	// 断言：通过 Apply 生成的 SQL 含对应 ORDER BY 子句
	// 用 GORM 内存数据库（sqlite）执行 Apply 并检查 DryRun 语句
}
```

具体断言方式：`opts.Apply(DB.Session(&gorm.Session{DryRun: true}).Model(&Channel{})).Find(&[]Channel{})`，检查 `stmt.SQL.String()` 包含 `ORDER BY` 与目标列；默认分支断言 `sort` ASC、`opts2` 断言 `sort` ASC、`opts3` 断言 `name` DESC、`opts5` 断言 `id` DESC。

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./model/ -run TestChannelSortOptionsApplyPriority -count=1`
Expected: FAIL（`NewChannelSortOptions` 无 sortSort 参数 / 默认仍 priority desc）

- [x] **Step 3: 实现**

`model/channel.go`：

```go
var channelSortColumns = map[string]string{
	"id":            "id",
	"name":          "name",
	"priority":      "priority",
	"balance":       "balance",
	"response_time": "response_time",
	"test_time":     "test_time",
	"sort":          "sort",
}

type ChannelSortOptions struct {
	SortBy    string
	SortOrder string
	IDSort    bool
	SortSort  bool // 使用序号排序开关
}

func NewChannelSortOptions(sortBy string, sortOrder string, idSort bool, sortSort bool) ChannelSortOptions {
	normalizedSortBy := strings.ToLower(strings.TrimSpace(sortBy))
	normalizedSortOrder := strings.ToLower(strings.TrimSpace(sortOrder))
	if _, ok := channelSortColumns[normalizedSortBy]; !ok {
		normalizedSortBy = ""
		normalizedSortOrder = ""
	} else if normalizedSortOrder != "asc" {
		normalizedSortOrder = "desc"
	}
	return ChannelSortOptions{
		SortBy:    normalizedSortBy,
		SortOrder: normalizedSortOrder,
		IDSort:    idSort,
		SortSort:  sortSort,
	}
}

func (options ChannelSortOptions) Apply(query *gorm.DB) *gorm.DB {
	if columnName, ok := channelSortColumns[options.SortBy]; ok {
		return query.Order(clause.OrderByColumn{
			Column: clause.Column{Name: columnName},
			Desc:   options.SortOrder != "asc",
		})
	}
	if options.SortSort {
		return query.Order(clause.OrderByColumn{
			Column: clause.Column{Name: "sort"},
			Desc:   false,
		})
	}
	if options.IDSort {
		return query.Order(clause.OrderByColumn{
			Column: clause.Column{Name: "id"},
			Desc:   true,
		})
	}
	return query.Order(clause.OrderByColumn{
		Column: clause.Column{Name: "sort"},
		Desc:   false,
	})
}
```

`resolveChannelSortOptions` 默认分支改为 4 参：

```go
func resolveChannelSortOptions(idSort bool, sortOptions []ChannelSortOptions) ChannelSortOptions {
	if len(sortOptions) == 0 {
		return NewChannelSortOptions("", "", idSort, false)
	}
	options := sortOptions[0]
	options.IDSort = options.IDSort || idSort
	return options
}
```

（`SortSort` 在调用方构造 `NewChannelSortOptions` 时已置位，`resolveChannelSortOptions` 无需再合并。）

- [x] **Step 4: 运行测试确认通过**

Run: `go test ./model/ -run 'TestChannelSortOptionsApplyPriority|TestChannelHasSortField' -count=1`
Expected: PASS

- [x] **Step 5: 运行既有渠道测试确认无回归**

Run: `go test ./model/ -run 'Channel' -count=1`
Expected: 全部 PASS

- [x] **Step 6: 提交**

```bash
git add model/channel.go model/channel_constraint_test.go
git commit -m "feat(channel): 排序白名单新增 sort 并扩展排序优先级"
```

### Task 3: Controller 解析 sort_sort 参数

- [x] Task 3: Controller 解析 sort_sort 参数

**Files:**
- Modify: `controller/channel.go`（`GetAllChannels` line ~107、`SearchChannels` line ~285）
- Test: `controller/channel_test_internal_test.go`（就近扩展，如已有排序测试模式）

**Interfaces:**
- Consumes: `model.NewChannelSortOptions(sortBy, sortOrder, idSort, sortSort)`
- Produces: `/api/channel` 与 `/api/channel/search` 支持 `sort_sort=true|false` 查询参数

- [x] **Step 1: 实现参数解析**

`controller/channel.go` 两处（`GetAllChannels` 与 `SearchChannels`）：

```go
idSort, _ := strconv.ParseBool(c.Query("id_sort"))
sortSort, _ := strconv.ParseBool(c.Query("sort_sort"))
sortOptions := model.NewChannelSortOptions(c.Query("sort_by"), c.Query("sort_order"), idSort, sortSort)
```

- [x] **Step 2: 验证编译与测试**

Run: `go build ./... && go vet ./controller/`
Expected: 成功

- [x] **Step 3: 手动验证（如 controller 测试环境可用）**

用 `curl` 或既有 controller 测试框架验证：`GET /api/channel/search?sort_sort=true` 返回按 sort 升序的数据；`GET /api/channel?sort_by=sort&sort_order=desc` 返回按 sort 降序。

- [x] **Step 4: 提交**

```bash
git add controller/channel.go
git commit -m "feat(channel): 渠道列表接口支持 sort_sort 排序参数"
```

---

## 2. 前端：类型与数据流

### Task 4: 前端类型扩展

- [x] Task 4: 前端类型扩展

**Files:**
- Modify: `web/src/features/channels/types.ts`（`channelSchema`、`ChannelSortBy`、`GetChannelsParams`、`SearchChannelsParams`）
- Test: `web/src/features/channels/lib/__tests__/`（类型层无独立测试则跳过，靠 typecheck）

**Interfaces:**
- Produces: `Channel.sort?: number | null`；`ChannelSortBy` 含 `'sort'`；两 Params 含 `sort_sort?: boolean`

- [x] **Step 1: 实现类型**

`web/src/features/channels/types.ts`：

```ts
// channelSchema 内，priority 行附近：
sort: z.number().nullish(),

// ChannelSortBy 联合类型加 'sort'
export type ChannelSortBy =
  | 'id'
  | 'name'
  | 'priority'
  | 'balance'
  | 'response_time'
  | 'test_time'
  | 'sort'

// GetChannelsParams / SearchChannelsParams 均加：
sort_sort?: boolean
```

- [x] **Step 2: 验证类型检查**

Run: `bun run typecheck`
Expected: 无类型错误

- [x] **Step 3: 提交**

```bash
git add web/src/features/channels/types.ts
git commit -m "feat(channels): 类型新增 sort 字段与 sort_sort 参数"
```

### Task 5: Provider 状态与表格接线

- [x] Task 5: Provider 状态与表格接线

**Files:**
- Modify: `web/src/features/channels/components/channels-provider.tsx`、`web/src/features/channels/components/channels-table.tsx`
- Test: `web/src/features/channels/components/__tests__/`（开关持久化交互在 Task 8 覆盖）

**Interfaces:**
- Produces: context `sortSort: boolean` / `setSortSort`；localStorage key `channels-sort-sort`；查询参数透传 `sort_sort`

- [x] **Step 1: provider 加状态**

`channels-provider.tsx`（仿 `idSort`）：

```tsx
// ContextType 加：
sortSort: boolean
setSortSort: (enabled: boolean) => void

// Provider 内：
const [sortSort, setSortSort] = useState(() => {
  return localStorage.getItem('channels-sort-sort') === 'true'
})
```

加入 `value` 与 memo 依赖数组。

- [x] **Step 2: 表格接线**

`channels-table.tsx`：
- `const { sortSort } = useChannels()`
- `CHANNEL_SORTABLE_COLUMNS` 加 `'sort'`
- 三处查询参数（`searchChannels` / `getChannels` / 主 `useQuery` queryKey）加 `sort_sort: sortSort`

- [x] **Step 3: 验证**

Run: `bun run typecheck && bun run lint`
Expected: 无错误

- [x] **Step 4: 提交**

```bash
git add web/src/features/channels/components/channels-provider.tsx web/src/features/channels/components/channels-table.tsx
git commit -m "feat(channels): provider 新增序号排序状态并接入表格查询"
```

---

## 3. 前端：序号列、表单与开关

### Task 6: 序号列（内联编辑 + 列头排序）

- [x] Task 6: 序号列（内联编辑 + 列头排序）

**Files:**
- Modify: `web/src/features/channels/components/channels-columns.tsx`
- Test: `web/src/features/channels/components/__tests__/sort-column.test.tsx`（新建）

**Interfaces:**
- Consumes: `ChannelFieldCell`（field 类型扩展 `'sort'`）；`NumericSpinnerInput`
- Produces: 序号列 `accessorKey: 'sort'`，位于 Priority 列旁，`header: t('Sort')`，`meta: { mobileHidden: true }`；tag 行返回 null

- [x] **Step 1: 扩展 ChannelFieldCell field 类型**

`channels-columns.tsx`：

```tsx
field: 'priority' | 'weight' | 'sort'
```

- [x] **Step 2: 新增 SortCell 组件与列**

在 `PriorityCell` 附近：

```tsx
/**
 * Sort cell component with inline editing
 */
function SortCell({ channel }: { channel: Channel }) {
  if (isTagAggregateRow(channel)) {
    return null // tag 行不展示/编辑序号，tag 聚合行排序保持现状
  }
  return (
    <ChannelFieldCell
      channelId={channel.id}
      value={channel.sort}
      field='sort'
      min={-999}
    />
  )
}
```

在 Priority 列（`accessorKey: 'priority'`）旁边新增：

```tsx
// Sort column
{
  accessorKey: 'sort',
  header: t('Sort'),
  meta: { mobileHidden: true },
  cell: ({ row }) => <SortCell channel={row.original} />,
  size: 100,
},
```

- [x] **Step 3: 写前端测试**

`web/src/features/channels/components/__tests__/sort-column.test.tsx`（参照仓库既有 RTL 测试模式，如 `data-table-row-actions-reset.test.tsx` 的 fixture 与渲染方式）：断言普通渠道行渲染序号编辑控件、tag 行不渲染序号编辑。

- [x] **Step 4: 验证**

Run: `bun run typecheck && bun run lint && bun run test sort-column`
Expected: 全部通过

- [x] **Step 5: 提交**

```bash
git add web/src/features/channels/components/channels-columns.tsx web/src/features/channels/components/__tests__/sort-column.test.tsx
git commit -m "feat(channels): 新增序号列并支持内联编辑"
```

### Task 7: 使用序号排序开关

- [x] Task 7: 使用序号排序开关

**Files:**
- Modify: `web/src/features/channels/components/channels-primary-buttons.tsx`
- Test: `web/src/features/channels/components/__tests__/sort-toggle.test.tsx`（新建）

**Interfaces:**
- Consumes: context `sortSort` / `setSortSort`
- Produces: 桌面 `Switch` + 移动端 `DropdownMenuCheckboxItem`「使用序号排序」，持久化 `channels-sort-sort`

- [x] **Step 1: 实现开关**

`channels-primary-buttons.tsx`（仿 `handleIdSortToggle` / id-sort 开关）：

```tsx
const handleSortSortToggle = (checked: boolean) => {
  localStorage.setItem('channels-sort-sort', String(checked))
  setSortSort(checked)
}
```

桌面在「使用ID排序」开关块后复制一块（`SortAsc` 图标、`htmlFor='sort-sort'`、`t('Sort by Sort')`）；移动端下拉菜单同理加 `DropdownMenuCheckboxItem`。

- [x] **Step 2: 写前端测试**

`web/src/features/channels/components/__tests__/sort-toggle.test.tsx`：渲染 `ChannelsPrimaryButtons`（包 Provider），点击开关断言 `localStorage` 写入 `channels-sort-sort` 且 `sortSort` 状态翻转。

- [x] **Step 3: 验证**

Run: `bun run typecheck && bun run lint && bun run test sort-toggle`
Expected: 全部通过

- [x] **Step 4: 提交**

```bash
git add web/src/features/channels/components/channels-primary-buttons.tsx web/src/features/channels/components/__tests__/sort-toggle.test.tsx
git commit -m "feat(channels): 新增使用序号排序开关"
```

### Task 8: 渠道表单与卡片视图

- [x] Task 8: 渠道表单与卡片视图

**Files:**
- Modify: `web/src/features/channels/lib/channel-form.ts`（schema + 默认值 + 提交映射）、`web/src/features/channels/lib/channel-form-errors.ts`、`web/src/features/channels/constants.ts`（默认值）、`web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`（表单字段）、`web/src/features/channels/components/channel-card.tsx`（卡片显示）

**Interfaces:**
- Consumes: `ChannelFieldCell`（卡片经 `renderCell('sort')` 复用）
- Produces: 表单「序号」输入（默认 0，整数）；提交载荷含 `sort`；卡片视图显示序号并可内联编辑

- [x] **Step 1: 表单 schema 与默认值**

`web/src/features/channels/lib/channel-form.ts`（仿 `priority`）：
- schema：`sort: z.number().optional()`
- `defaultValues`（~line 498）：`sort: 0`
- 编辑回填（~line 678）：`sort: channel.sort || 0`
- 提交映射（~line 1001、1049）：`sort: formData.sort ?? null` / `sort: formData.sort ?? 0`

`web/src/features/channels/lib/channel-form-errors.ts` 字段列表加 `'sort'`。
`web/src/features/channels/constants.ts` 的 `channelTypeDefaults`（~line 315）加 `sort: 0`。

- [x] **Step 2: 抽屉表单字段**

`channel-mutate-drawer.tsx` 在 priority `FormField`（~line 4310）后新增：

```tsx
<FormField
  control={form.control}
  name='sort'
  render={({ field }) => (
    <FormItem>
      <FormLabel>{t('Sort')}</FormLabel>
      <FormControl>
        <Input
          type='number'
          placeholder='0'
          {...field}
          onChange={(e) => field.onChange(Number(e.target.value))}
        />
      </FormControl>
      <FormDescription>{t('Used to order channels in the list')}</FormDescription>
      <FormMessage />
    </FormItem>
  )}
/>
```

（`grid gap-4 sm:grid-cols-2` 布局内新增一个字段即可，或按需调整 grid 结构。）

- [x] **Step 3: 卡片视图**

`channel-card.tsx`：
- `const sortCell = renderCell('sort')`
- 在 Priority/Weight 网格加一行：`<span className={labelClass}>{t('Sort')}</span>` 与 `<div className='flex justify-start'>{sortCell}</div>`（放在 priority 行附近）

- [x] **Step 4: 验证**

Run: `bun run typecheck && bun run lint`
Expected: 无错误

- [x] **Step 5: 提交**

```bash
git add web/src/features/channels/lib/channel-form.ts web/src/features/channels/lib/channel-form-errors.ts web/src/features/channels/constants.ts web/src/features/channels/components/drawers/channel-mutate-drawer.tsx web/src/features/channels/components/channel-card.tsx
git commit -m "feat(channels): 表单与卡片视图支持序号字段"
```

---

## 4. 国际化

### Task 9: i18n 文案

- [x] Task 9: i18n 文案

**Files:**
- Modify: `web/src/i18n/locales/en.json`、`web/src/i18n/locales/zh.json`（及 zh-TW、fr、ru、ja、vi 如 i18n:sync 支持）

**Interfaces:**
- Produces: `Sort`（列头/字段名）、`Sort by Sort`（开关）、`Used to order channels in the list`（表单描述）key

- [x] **Step 1: 添加源串**

`web/src/i18n/locales/en.json` 添加：

```json
"Sort": "Sort",
"Sort by Sort": "Sort by Sort",
"Used to order channels in the list": "Used to order channels in the list"
```

`zh.json` 添加：

```json
"Sort": "序号",
"Sort by Sort": "使用序号排序",
"Used to order channels in the list": "用于在列表中排列渠道的顺序"
```

- [x] **Step 2: 同步并验证**

Run: `bun run i18n:sync`（如可用）
Expected: key 无缺失

- [x] **Step 3: 提交**

```bash
git add web/src/i18n/locales/
git commit -m "feat(i18n): 渠道序号相关文案"
```

---

## 5. 验证与交付

### Task 10: 后端排序回归测试

- [x] Task 10: 后端排序回归测试

**Files:**
- Modify: `model/channel_constraint_test.go`（Task 2 已覆盖核心；此处补充 controller 参数解析或收尾）

**Interfaces:**
- 验证：sort 白名单、默认 sort ASC、SortSort 优先于 IDSort、列头排序优先于开关

- [x] **Step 1: 运行后端全部渠道相关测试**

Run: `go test ./model/... ./controller/... -run 'Channel' -count=1`
Expected: 全部 PASS

- [x] **Step 2: 确认排序语义断言完整**

检查 Task 2 的表驱动测试已覆盖：默认分支 `sort ASC`、`SortSort` 优先于 `IDSort`、`SortBy` 优先于开关；如 controller 层已有排序测试模式，补充 `sort_sort` 参数解析断言。

### Task 11: 前端回归测试

- [x] Task 11: 前端回归测试

**Files:**
- Modify: `web/src/features/channels/components/__tests__/`（Task 6/7 已建）

- [x] **Step 1: 运行前端受影响测试**

Run: `bun run test`
Expected: 全部通过

- [x] **Step 2: 类型与 lint**

Run: `bun run typecheck && bun run lint`
Expected: 无错误

### Task 12: 三库迁移验证

- [x] Task 12: 三库迁移验证

**Files:**
- 验证：SQLite / MySQL / PostgreSQL

- [x] **Step 1: SQLite 迁移验证**

用项目启动逻辑（或 `common.SetupDB` 测试路径）执行 AutoMigrate：新建库 + 存量库各跑一次启动迁移，各两次确认幂等；确认 `channels` 表含 `sort` 列且默认 0。

- [x] **Step 2: MySQL / PostgreSQL 迁移验证**

在可用实例上重复 Step 1；如无可用实例，记录 blocker 并如实上报，不声称完成。

- [x] **Step 3: 记录结果**

在最终 handoff / PR 记录：数据库版本、迁移命令、新建/升级/幂等验证结果。

### Task 13: 收尾与阶段守卫

- [x] Task 13: 收尾与阶段守卫

- [x] **Step 1: 全量构建**

Run: `go build ./... && go vet ./...`
Expected: 成功

- [x] **Step 2: 阶段守卫**

```bash
comet state record-check channel-sort-field build --command "go build ./... && go vet ./..." --exit-code 0
comet guard channel-sort-field build --apply
```

Expected: guard 全部 PASS，phase 推进到 verify

- [x] **Step 3: 汇总改动**

列出改动文件与测试结果，供 verify 阶段使用。
