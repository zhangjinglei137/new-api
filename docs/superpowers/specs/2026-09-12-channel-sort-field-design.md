---
comet_change: channel-sort-field
role: technical-design
canonical_spec: openspec
archived-with: 2026-09-12-channel-sort-field
status: final
---

# 渠道序号字段与排序 — Design Doc

## Context

open 阶段已确认需求与高层方案（见 `docs/openspec/changes/channel-sort-field/proposal.md` 与 `design.md`，spec 见 `docs/openspec/changes/channel-sort-field/specs/channels/spec.md`）：为渠道新增独立于 `priority`（负载均衡优先级）的「序号」（sort）字段，仅用于渠道管理列表展示排序；新增与「使用ID排序」平级的「使用序号排序」开关；无显式排序时默认按序号升序。

本 Design Doc 对高层方案做深度技术细化：逐文件实现细节、前后端接线、边界条件、测试策略与三库迁移验证。改动不触碰 relay 渠道选择逻辑与 `priority` 语义。

## Goals / Non-Goals

**Goals:**
- 完整定义后端字段、排序优先级、查询参数与调用点改动的实现细节
- 完整定义前端类型、状态、列、开关、表单与卡片视图的接线细节
- 明确 i18n 文案、三库迁移验证与测试策略

**Non-Goals:**
- 不改变 `priority` 行为与 relay 路由逻辑
- 不做拖拽排序、批量重排号、导入导出格式变更
- tag 模式下 tag 聚合行的排序保持 `priority desc` 现状（用户已确认）；tag 聚合行内子渠道仍按本规则排序
- 不新增 API 协议字段（仅扩展既有 `sort_by` 白名单与新增查询参数 `sort_sort`）

## Decisions

### D1: 后端字段定义

`model/channel.go` 的 `Channel` 结构体在 `Priority` 字段后新增：

```go
Sort *int64 `json:"sort" gorm:"bigint;default:0"` // 序号，用于渠道列表展示排序，独立于 Priority
```

- 指针类型 + `default:0`：与 `Priority` 一致，保留显式零值语义，JSON 序列化时 0 不被省略
- `channelSortColumns` 白名单加 `"sort": "sort"`
- 后端 JSON 编码一律走 `common.Marshal` / `common.Unmarshal`（项目规则），本字段无特殊处理

### D2: 排序选项扩展与优先级

`model/channel.go`：

```go
type ChannelSortOptions struct {
    SortBy    string
    SortOrder string
    IDSort    bool
    SortSort  bool // 使用序号排序开关
}

func NewChannelSortOptions(sortBy string, sortOrder string, idSort bool, sortSort bool) ChannelSortOptions
```

`Apply` 分支顺序（优先级从高到低）：

1. `sort_by` ∈ 白名单（列头点击）→ `ORDER BY <column> <asc|desc>`
2. `SortSort`（使用序号排序开关）→ `ORDER BY sort ASC`
3. `IDSort`（使用ID排序开关）→ `ORDER BY id DESC`（现有行为不变）
4. 默认 → `ORDER BY sort ASC`（替代现有 `priority DESC`）

`resolveChannelSortOptions` 同步合并：`options.SortSort = options.SortSort || sortSort`（与 `IDSort` 合并方式一致）。

调用点梳理（需同步改 4 参签名）：
- `controller/channel.go:SearchChannels` — `NewChannelSortOptions(c.Query("sort_by"), c.Query("sort_order"), idSort, sortSort)`，新增 `sortSort, _ := strconv.ParseBool(c.Query("sort_sort"))`
- `model/channel.go:resolveChannelSortOptions` — `NewChannelSortOptions("", "", idSort, sortSort)`，签名相应加 `sortSort bool`
- 其他调用 `GetAllChannels` / `GetChannelsByTag` / `SearchChannels` 的位置经由 `resolveChannelSortOptions`，无需逐点改

### D3: 前端类型与数据流

- `web/src/features/channels/types.ts`：
  - `ChannelSortBy` 加 `'sort'`
  - `GetChannelsParams` / `SearchChannelsParams` 加 `sort_sort?: boolean`
  - `channelSchema` / `Channel` 类型加 `sort` 字段（zod 数字，默认 0）
- `web/src/features/channels/components/channels-provider.tsx`：
  - context 类型加 `sortSort: boolean` / `setSortSort`
  - state：`useState(() => localStorage.getItem('channels-sort-sort') === 'true')`
  - 加入 memo 依赖与 value
- `web/src/features/channels/components/channels-table.tsx`：
  - `CHANNEL_SORTABLE_COLUMNS` 加 `'sort'`
  - 三个查询（search / getChannels / 主查询）透传 `sort_sort: sortSort`
  - `sortParams` 复用现有逻辑（列头点击 → `sort_by`），无需改动

### D4: 前端 UI

- `channels-columns.tsx`：在 Priority 列旁边新增：

```tsx
{
  accessorKey: 'sort',
  header: t('Sort'),
  meta: { mobileHidden: true },
  cell: ({ row }) => <SortCell channel={row.original} />, // 或直接复用 ChannelFieldCell
  size: 100,
}
```

  - `ChannelFieldCell` 的 `field` 联合类型 `'priority' | 'weight'` 扩展为 `'priority' | 'weight' | 'sort'`；现有 `handleUpdateChannelField`（已泛化 `fieldName: string`）无需改动
  - 字符串 header 经 `DataTableHeader.renderHeaderContent` 自动用 `DataTableColumnHeader` 渲染，获得点击排序能力（无需 `enableSorting` 显式配置）
- `channels-primary-buttons.tsx`：桌面在「使用ID排序」开关旁新增同款开关（`SortAsc` 图标 + `Switch`），移动端下拉菜单加 `DropdownMenuCheckboxItem`；`handleSortSortToggle` 写入 localStorage `channels-sort-sort`
- `channel-card.tsx`（卡片视图）：`const sortCell = renderCell('sort')`，在 Priority/Weight 网格中加一行「序号」标签与单元格（与表格共用 cell 渲染器，内联编辑一致）
- `channel-mutate-drawer.tsx`：表单 schema 加 `sort`（zod number，默认 0），在 priority 输入附近加「序号」数字输入（复用现有 NumericSpinnerInput 或数字输入模式）；提交载荷携带 `sort`

### D5: 国际化

- `web/src/i18n/locales/en.json` 加 `"Sort"`（列头/字段名）与 `"Sort by Sort"`（开关标签）源串
- 同步 `zh.json`（`序号` / `使用序号排序`）及 zh-TW、fr、ru、ja、vi
- 运行 `bun run i18n:sync` 校验 key 一致性

### D6: 数据库迁移

- 无显式迁移脚本：启动时 GORM AutoMigrate 对三库自动 `ADD COLUMN sort`（SQLite 支持 ADD COLUMN；MySQL/PostgreSQL 已有 dialector 适配器）
- 存量行回填 0（列默认值）
- 迁移验证（AGENTS.md 强制）：SQLite / MySQL / PostgreSQL 各执行（a）新建库启动迁移、（b）由最近发布版本创建的存量库升级迁移，各两次确认幂等；记录数据库版本与结果

### D7: 测试策略

- 后端（`model/channel_test.go` 或就近测试文件，遵循 AGENTS.md 单文件合并原则）：
  - `NewChannelSortOptions` 白名单含 `sort`、非法值回退
  - `Apply`：默认 `sort ASC`；`SortSort` 优先于 `IDSort`；`SortBy` 优先于两者
  - `controller/channel.go` 的 `sort_sort` 参数解析（如已有 controller 测试模式则就近补充）
- 前端（`web/src/features/channels/components/__tests__/`）：
  - 序号列头点击触发 `sort_by=sort` 请求
  - 「使用序号排序」开关切换写入 localStorage 并触发 `sort_sort` 参数
  - 序号单元格内联编辑保存
- 使用 `github.com/stretchr/testify/require` + `assert`（后端）、Vitest + React Testing Library（前端）

## Risks / Trade-offs

- [默认排序行为变更] 默认从 `priority DESC` 改为 `sort ASC` 会改变现有部署的初始列表顺序 → 用户明确要求；`sort` 全为 0 时退化为按 id 稳定序，影响可接受
- [签名扩散] `NewChannelSortOptions` 加参波及 controller/model 调用点 → 统一改 4 参签名，编译期捕获遗漏
- [开关语义] 「使用序号排序」与「使用ID排序」同开时以序号为准（spec 已定）→ 后端 `Apply` 分支顺序保证，前端不额外互斥
- [三库迁移] AutoMigrate 默认值处理差异 → 三库迁移矩阵验证 + 记录版本
- [列空间] 序号列挤压表格空间 → `meta: { mobileHidden: true }` + 列显隐控制

## Migration Plan

1. 后端：字段 + 白名单 + `ChannelSortOptions` + `SearchChannels` 参数 → `go build ./...`、`go vet ./...`
2. 前端：类型 → provider → 表格接线 → 列 → 开关 → 表单 → 卡片 → i18n → `bun run typecheck`、lint
3. 后端单测 + 前端回归测试
4. 三库迁移验证矩阵（新建 + 升级 + 幂等）
5. 回滚：还原 `Apply` 默认分支与新增字段即可，无数据迁移残留

## Open Questions

无。
