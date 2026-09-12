# Comet Design Handoff

- Change: channel-sort-field
- Phase: design
- Mode: compact
- Context hash: d41deb3be9ac01e6ecf1d9e67dc2518297f84ddfa8503706628c437c418fe549

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## docs/openspec/changes/channel-sort-field/proposal.md

- Source: docs/openspec/changes/channel-sort-field/proposal.md
- Lines: 1-36
- SHA256: 84914c2c889caf6f904d5e771cc362fe2aedaa6208958b36e2a910d3ecc1cb69

```md
# 渠道序号字段与排序

## Why

渠道管理列表目前只能按优先级（Priority）、余额、响应时间等字段排序，缺少一个可手工控制的展示顺序字段。管理员希望为渠道设置一个独立于负载均衡优先级的「序号」，并在渠道列表上方按序号排序，以便按自己的业务顺序浏览和管理渠道。

现状：渠道已有 `priority`（优先级）字段且列头可排序，但 Priority 承担负载均衡语义（影响请求路由权重），不适合直接当作纯展示顺序使用。需要一个与 Priority 语义分离的「序号」字段，仅用于渠道列表的展示排序。

## What Changes

- 后端 `Channel` 模型新增独立「序号」字段（`sort`，bigint，默认 0），不影响 `priority` 的负载均衡语义
- 渠道列表 API（`/api/channel`、`/api/channel/search`）的排序白名单新增 `sort` 字段，支持按序号升/降序
- 无显式排序时，渠道列表默认按序号升序（序号小在上）；保持列头点击排序与「使用ID排序」开关现有行为
- 前端渠道列表新增「序号」列：单元格可内联编辑（复用现有 `ChannelFieldCell` 模式），列头可点击排序
- 前端渠道新增/编辑表单增加「序号」输入字段
- 前端新增与「使用ID排序」平级的「使用序号排序」开关（桌面工具栏 + 移动端下拉菜单），状态持久化到 localStorage
- 数据库迁移：SQLite / MySQL / PostgreSQL 三库新增 `sort` 列（AutoMigrate 或迁移逻辑）

## Capabilities

### New Capabilities

- `channels`: 渠道管理能力，覆盖渠道序号字段的存储、列表排序与编辑交互

### Modified Capabilities

无（`priority` 语义与行为保持不变）

## Impact

- **模型/数据库**：`model/channel.go`（`Channel` 结构体新增 `Sort` 字段、`channelSortColumns` 白名单、`ChannelSortOptions` 排序逻辑与默认排序）、迁移逻辑（三数据库兼容）
- **API**：`controller/channel.go`（`SearchChannels` / `GetAllChannels` 排序参数透传已支持，无需协议变更，仅白名单扩展）
- **前端**：`web/src/features/channels/components/channels-columns.tsx`（序号列 + 内联编辑）、`channels-table.tsx`（排序接线）、`channels-primary-buttons.tsx`（使用序号排序开关）、`channels-provider.tsx`（开关状态）、`channels-dialogs.tsx` / 渠道表单（序号字段）、`types.ts`（`ChannelSortBy` 扩展）、`web/src/i18n/locales/*.json`（文案）
- **i18n**：新增 `Sort` / `Sort by Sort` 等英文源文案并同步各语言文件
- **测试**：后端排序白名单/默认排序单测；前端序号列与开关交互测试（如适用）
- **不涉及**：relay 渠道选择逻辑、计费、权限模型

```

## docs/openspec/changes/channel-sort-field/design.md

- Source: docs/openspec/changes/channel-sort-field/design.md
- Lines: 1-77
- SHA256: dabf8f151fb807d1b5539cb06d4057dfa3f01e94997e5d41a0465033d9c7c355

```md
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

```

## docs/openspec/changes/channel-sort-field/tasks.md

- Source: docs/openspec/changes/channel-sort-field/tasks.md
- Lines: 1-28
- SHA256: c4f908652b69e80492f780049662b6c4e941b08b66c69d77ea66e53deba837fb

```md
# 渠道序号字段与排序 — 任务清单

## 1. 后端模型与排序逻辑

- [ ] 1.1 在 `model/channel.go` 的 `Channel` 结构体中新增 `Sort *int64` 字段（gorm 标签 `bigint;default:0`，JSON `sort`），并在最接近字段位置补充注释说明；运行 `go build ./...` 验证编译通过
- [ ] 1.2 在 `model/channel.go` 的 `channelSortColumns` 白名单中加入 `"sort": "sort"`，并新增 `SortSort` 字段到 `ChannelSortOptions`；调整 `NewChannelSortOptions` 签名与调用点，使 `Apply` 的优先级为：列头 `sort_by` > `sort_sort` > `id_sort` > 默认按 `sort` 升序；运行 `go test ./model/...`（或 `./model/ -run ChannelSort` 相关用例）验证排序行为
- [ ] 1.3 检查后端所有调用 `NewChannelSortOptions` / `ChannelSortOptions` 的位置（`controller/channel.go`、`model/channel.go` 等），为 `SearchChannels` 增加 `sort_sort` 查询参数解析并透传；运行 `go build ./...` 与既有渠道相关测试验证

## 2. 前端类型与数据流

- [ ] 2.1 在 `web/src/features/channels/types.ts` 的 `ChannelSortBy` 联合类型中加入 `'sort'`；在 `GetChannelsParams` / `SearchChannelsParams` 中加入可选 `sort_sort?: boolean`，并同步 `channelSchema`/`Channel` 类型中的 `sort` 字段；运行 `bun run typecheck` 验证
- [ ] 2.2 在 `web/src/features/channels/components/channels-provider.tsx` 的 context 类型与 provider 状态中加入 `sortSort: boolean` / `setSortSort`（localStorage key `channels-sort-sort`，默认 false），与 `idSort` 对称；运行 `bun run typecheck` 验证
- [ ] 2.3 在 `web/src/features/channels/components/channels-table.tsx` 的 `CHANNEL_SORTABLE_COLUMNS` 中加入 `'sort'`，并在列表查询参数中透传 `sort_sort: sortSort`；验证排序状态能触发后端 `sort_by=sort` 请求

## 3. 前端 UI：序号列、表单与开关

- [ ] 3.1 在 `web/src/features/channels/components/channels-columns.tsx` 的 `PriorityCell` 附近新增 `sort` 列（`header: t('Sort')`，单元格复用内联编辑模式，`meta: { mobileHidden: true }`），并确认 `ChannelFieldCell` 的 `field` 类型或调用能覆盖 `'sort'`；运行 `bun run typecheck` 与 lint 验证
- [ ] 3.2 在 `web/src/features/channels/components/channels-primary-buttons.tsx` 桌面工具栏与移动端下拉菜单中加入「使用序号排序」开关（复用 `SortAsc` 图标与 `Switch`/`DropdownMenuCheckboxItem`），绑定 `sortSort` / `setSortSort` 并持久化 localStorage；运行 lint 与既有渠道组件测试验证
- [ ] 3.3 在 `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`（及相应的新增/编辑表单 schema）中加入「序号」（sort）输入字段，支持整数、默认 0；校验提交时携带 `sort` 字段；运行 `bun run typecheck` 验证

## 4. 国际化

- [ ] 4.1 在 `web/src/i18n/locales/en.json` 添加 `Sort`（序号/排序）相关文案源串（如 `Sort`、`Sort by Sort`），并同步 `zh.json` 等既有语言文件；运行 `bun run i18n:sync`（如可用）验证 key 同步

## 5. 验证与交付

- [ ] 5.1 补充/更新后端排序单元测试：覆盖 `sort` 白名单、默认按 `sort` 升序、`sort_sort` 优先级高于 `id_sort`、列头排序优先于开关；运行 `go test ./model/... ./controller/...` 验证通过
- [ ] 5.2 补充/更新前端渠道列表与开关的回归测试（排序列交互、开关切换与持久化）；运行 `bun run test`（受影响用例）与 `bun run lint`、`bun run typecheck` 验证
- [ ] 5.3 按项目数据库兼容规则对 SQLite、MySQL、PostgreSQL 三库执行迁移验证（启动 AutoMigrate 新增 `sort` 列，覆盖新建库与升级库并确认幂等），并记录数据库版本与结果；确认默认排序行为（无排序时按序号升序）
```

## docs/openspec/changes/channel-sort-field/specs/channels/spec.md

- Source: docs/openspec/changes/channel-sort-field/specs/channels/spec.md
- Lines: 1-57
- SHA256: 825bcc0da34eeca361550100371b01d110a6d14a6c2e03c64c660cda6a1a95d9

```md
## Purpose

定义渠道管理列表的「序号」字段能力：管理员可以为每个渠道设置独立于负载均衡优先级的展示序号，并按序号对渠道列表排序，从而按自定义业务顺序浏览和管理渠道。

## ADDED Requirements

### Requirement: 渠道具有序号字段

渠道 SHALL 具有一个可编辑的「序号」（sort）字段，用于渠道管理列表的展示排序。序号 SHALL 独立于渠道的负载均衡优先级（priority）字段，改变序号 MUST NOT 影响渠道的请求路由权重。新创建的渠道序号默认值为 0。渠道序号 MUST 支持任意整数，且编辑序号后渠道列表 MUST 按最新值刷新。

#### Scenario: 创建渠道时序号默认为 0

- **WHEN** 管理员创建一个新渠道且未填写序号
- **THEN** 该渠道的序号为 0

#### Scenario: 编辑渠道序号

- **WHEN** 管理员将某渠道的序号从 5 修改为 3 并保存
- **THEN** 渠道列表展示的该渠道序号变为 3，且列表按最新排序规则重新排序

#### Scenario: 序号不影响负载均衡优先级

- **WHEN** 管理员修改某渠道的序号
- **THEN** 该渠道的 priority 字段保持不变，其请求路由权重不受影响

### Requirement: 渠道列表按序号排序

渠道管理列表 SHALL 支持按序号（sort）升序或降序排序。未指定任何排序条件时，列表 SHALL 默认按序号升序排列（序号小的在上）。用户点击序号列头 SHALL 在升序与降序之间切换。

#### Scenario: 默认按序号升序

- **WHEN** 管理员打开渠道列表且未设置任何排序条件
- **THEN** 渠道按序号从小到大排列，序号小的渠道显示在上方

#### Scenario: 点击序号列头切换排序方向

- **WHEN** 管理员点击渠道列表的「序号」列头
- **THEN** 列表按序号降序排列；再次点击后恢复升序

### Requirement: 使用序号排序开关

渠道列表 SHALL 提供与「使用ID排序」平级的「使用序号排序」开关。开启后，渠道列表 SHALL 按序号升序排列；开关状态 SHALL 在浏览器本地持久化，刷新页面后保持。开启「使用序号排序」时，若管理员点击其他列头排序，MUST 以列头排序为准；开启「使用ID排序」与「使用序号排序」同时开启时，MUST 以「使用序号排序」为准。

#### Scenario: 开启使用序号排序

- **WHEN** 管理员开启「使用序号排序」开关
- **THEN** 渠道列表立即按序号升序排列，且刷新页面后该开关仍为开启状态

#### Scenario: 使用序号排序与列头排序冲突

- **WHEN** 管理员开启「使用序号排序」后点击「名称」列头按名称排序
- **THEN** 列表按名称排序，序号排序不再生效

#### Scenario: 使用序号排序优先于使用ID排序

- **WHEN** 管理员同时开启「使用序号排序」和「使用ID排序」
- **THEN** 列表按序号升序排列

```
