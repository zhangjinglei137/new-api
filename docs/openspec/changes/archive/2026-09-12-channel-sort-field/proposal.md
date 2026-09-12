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
