# 渠道序号字段与排序 — 技术设计

## Context

现有渠道列表排序链路已完整：前端表格 `sorting` 状态 → `sort_by`/`sort_order` 查询参数 → 后端 `controller.SearchChannels`/`model.SearchChannels` → `ChannelSortOptions.Apply` 生成 `ORDER BY`。排序白名单 `channelSortColumns` 目前包含 `id/name/priority/balance/response_time/test_time`，默认（无任何排序条件）为 `ORDER BY priority DESC`。渠道模型已有 `priority` 字段承担负载均衡语义，不宜复用作纯展示序号。

本 change 新增独立 `sort`（序号）字段，仅用于列表展示排序，不触碰 `priority` 的路由语义。改动横跨后端模型/排序逻辑、前端列表/表单/开关，但均为已有模式的扩展，无新依赖、无协议变更。

## Goals / Non-Goals

**Goals:**
- 后端 `Channel` 新增 `sort` 序号字段（bigint，默认 0），三数据库（SQLite/MySQL/PostgreSQL）可迁移
- 排序白名单加入 `sort`，支持按序号升/降序；新增「使用序号排序」开关状态，优先级高于「使用ID排序」
- 无任何显式排序条件时，列表默认按序号升序（替代现有 `priority DESC` 默认）
- 前端渠道列表新增「序号」列（列头可排序、单元格内联编辑，复用 `ChannelFieldCell` 模式）
- 前端新增/编辑渠道表单增加序号输入；工具栏新增「使用序号排序」开关（桌面开关 + 移动端下拉菜单项）
- 序号不影响 relay 渠道选择与负载均衡（`priority` 语义不变）

**Non-Goals:**
- 不改动 `priority` 字段行为、不改 relay 路由逻辑
- 不做拖拽排序、不做批量重排号
- 不改动渠道导入/导出格式
- 标签聚合模式（tag mode）下 tag 行的排序行为不在需求范围（子渠道仍按排序规则）

## Decisions

### D1: 字段命名为 `sort`（Go `Sort *int64`，JSON `sort`，DB 列 `sort`）

- **选择**：`sort`，类型 `*int64`，`gorm:"bigint;default:0"`，与 `priority` 的指针类型一致（保留零值语义，避免 `omitempty` 丢 0）。
- **备选**：`sort_order`（与前端 `ChannelSortOrder` 类型名混淆，放弃）；`sequence`/`order`（语义冗余，且 `order` 是 SQL 关键字，放弃）。
- **理由**：与现有 `channelSortColumns` 键命名风格一致；前端 `ChannelSortBy` 联合类型直接加 `'sort'`。

### D2: 排序优先级：列头点击 > 使用序号排序 > 使用ID排序 > 默认序号升序

`ChannelSortOptions` 增加 `SortSort bool` 字段，`Apply` 分支顺序调整为：

```
if SortBy ∈ channelSortColumns          → ORDER BY <SortBy> <asc|desc>   // 列头点击
else if SortSort                        → ORDER BY sort ASC              // 使用序号排序开关
else if IDSort                          → ORDER BY id DESC               // 使用ID排序开关（现有行为）
else                                    → ORDER BY sort ASC              // 新默认
```

- **理由**：列头点击是用户最显式的意图，保持最高优先级（与现状一致）；「使用序号排序」开关语义上应覆盖「使用ID排序」；默认改为序号升序是用户明确要求（序号小的在上）。
- **备选**：开关与列头互斥/联动清除——复杂且破坏现有交互，放弃。
- **影响**：`controller.SearchChannels` 需读取 `sort_sort` 查询参数传入 `NewChannelSortOptions`；`GetAllChannels`/`GetChannelsByTag`/`SearchChannels` 的 `resolveChannelSortOptions` 同步合并开关状态。

### D3: 前端开关状态与 idSort 平级，持久化到 localStorage

`channels-provider.tsx` 新增 `sortSort: boolean` + `setSortSort`，与 `idSort` 完全对称（localStorage key `channels-sort-sort`）。`channels-primary-buttons.tsx` 在「使用ID排序」开关旁新增同款开关（桌面 `Switch` + 移动端 `DropdownMenuCheckboxItem`）。列表请求同时携带 `id_sort` 与 `sort_sort`，后端按 D2 优先级裁决。

### D4: 序号列内联编辑复用 `ChannelFieldCell`

前端「序号」列 `accessorKey: 'sort'`，`header: t('Sort')`（字符串 header 自动获得 `DataTableColumnHeader` 排序能力），`cell` 复用 `ChannelFieldCell`。`ChannelFieldCell` 的 `field` 联合类型从 `'priority' | 'weight'` 扩展为含 `'sort'`；`handleUpdateChannelField`（已泛化）无需改动。表单（`channel-mutate-drawer.tsx`）在 priority 字段附近新增序号数字输入，接入现有 React Hook Form + Zod 表单 schema。

### D5: 迁移走 GORM AutoMigrate，无显式迁移脚本

`Channel` 结构体加字段后，启动时 AutoMigrate 自动 `ADD COLUMN`（SQLite 支持，MySQL/PostgreSQL 有既有 dialector 适配器）。新列 `default:0`，存量行自动回填 0。无需数据回填逻辑。验证需覆盖三数据库（见 Risks）。

## Risks / Trade-offs

- **[默认排序行为变更]** 默认 `ORDER BY priority DESC` → `ORDER BY sort ASC` 会改变现有部署的渠道列表初始顺序 → 用户明确要求；`sort` 默认 0 时等价于按 id 稳定序，影响可接受，且在 proposal 审视中已声明。
- **[三数据库迁移]** AutoMigrate 新增 bigint 列在 SQLite/MySQL/PostgreSQL 的默认值处理存在差异 → 按项目 AGENTS.md 数据库兼容规则，对三种真实数据库各跑一次迁移验证（新建 + 升级存量），并记录版本与结果。
- **[开关语义混淆]** 「使用序号排序」与「使用ID排序」同时开启时以序号为准，可能让部分用户困惑 → 开关 UI 文案明确（`Sort by Sort` / 序号），spec 已定义优先级。
- **[前端列空间]** 渠道表格列已较多，新增序号列可能挤压空间 → `meta: { mobileHidden: true }`，桌面默认显示，用户可经列显隐控制。

## Migration Plan

1. 后端：加字段 + 排序逻辑 → `go build ./...` 通过
2. 前端：类型/列/开关/表单 + i18n → `bun run typecheck` + lint 通过
3. 三数据库迁移验证：SQLite/MySQL/PostgreSQL 各新建库跑启动迁移 + 升级存量库跑启动迁移，各两次确认幂等
4. 后端排序单测（白名单含 sort、默认升序、开关优先级）；前端序号列与开关交互测试
5. 回滚：还原 `Apply` 默认分支与字段即可，无数据迁移残留

## Open Questions

无（影响 specs、方案或任务拆分的未知项均已在上方解决；字段命名、默认排序、开关优先级均已确定）。
