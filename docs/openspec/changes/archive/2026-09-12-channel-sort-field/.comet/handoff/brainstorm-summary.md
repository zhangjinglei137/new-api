# Brainstorm Summary

- Change: channel-sort-field
- Date: 2026-09-12

## 确认的技术方案

- 后端 `Channel` 新增 `Sort *int64` 序号字段（`gorm:"bigint;default:0"`，JSON `sort`），独立于 `priority`，不影响负载均衡语义
- `channelSortColumns` 白名单加 `"sort": "sort"`；`ChannelSortOptions` 加 `SortSort bool`；`NewChannelSortOptions` 扩展为 4 参
- `Apply` 排序优先级：列头 `sort_by` > `sort_sort` > `id_sort` > 默认 `sort ASC`（替代现有 `priority DESC` 默认）
- `controller.SearchChannels` 解析 `sort_sort` 查询参数并透传
- 前端 `types.ts`：`ChannelSortBy` 加 `'sort'`；`GetChannelsParams`/`SearchChannelsParams` 加 `sort_sort?: boolean`；`channelSchema`/`Channel` 加 `sort` 字段
- 前端 `channels-provider`：加 `sortSort`/`setSortSort`（localStorage key `channels-sort-sort`，默认 false），与 `idSort` 对称
- 前端 `channels-primary-buttons`：桌面 Switch + 移动端 DropdownMenuCheckboxItem 加「使用序号排序」开关
- 前端 `channels-columns`：序号列放在 **Priority 列旁边**，`header: t('Sort')`（字符串 header 自动获得排序能力），cell 复用 `ChannelFieldCell`（field 联合类型扩展 `'sort'`），`meta: { mobileHidden: true }`
- 前端 `channels-table`：`CHANNEL_SORTABLE_COLUMNS` 加 `'sort'`；列表查询参数透传 `sort_sort: sortSort`
- 前端 `channel-card`（卡片视图）：`renderCell('sort')` 显示序号并支持内联编辑，与表格一致
- 前端 `channel-mutate-drawer`：priority 旁加「序号」整数输入（React Hook Form + Zod，默认 0）
- i18n：`en.json` 加 `Sort`、`Sort by Sort` 源串，同步 `zh.json` 等语言文件
- 迁移：GORM AutoMigrate 三库（SQLite/MySQL/PostgreSQL）自动加列，无显式脚本，存量回填 0

## 关键取舍与风险

- 默认排序从 `priority DESC` 改为 `sort ASC`，改变现有部署列表初始顺序 → 用户明确要求；sort 默认 0 时等价按 id 稳定序
- `NewChannelSortOptions` 签名扩展波及 controller/model 内约 3-4 处调用点 → 统一改 4 参签名
- 「使用序号排序」与「使用ID排序」同时开启时以序号为准 → spec 已定义优先级，后端裁决
- 三库迁移默认值处理差异 → 按 AGENTS.md 数据库兼容规则各跑新建 + 升级迁移验证
- tag 模式 tag 聚合行排序保持 `priority desc` 现状（子渠道仍按排序规则）→ 用户确认保持现状
- 前端列空间：序号列 `mobileHidden`，桌面默认显示

## 测试策略

- 后端：`model` 排序单测覆盖 sort 白名单、默认 sort ASC、sort_sort 优先于 id_sort、列头排序优先于开关；`go test ./model/... ./controller/...`
- 前端：序号列交互（排序列点击、内联编辑）、开关切换与 localStorage 持久化；`bun run test` 受影响用例 + `bun run lint` + `bun run typecheck`
- 数据库：SQLite/MySQL/PostgreSQL 各新建库 + 升级存量库跑启动迁移，各两次确认幂等，记录版本与结果

## Spec Patch

无（spec 三需求已覆盖全部确认行为；序号列位置、卡片视图、tag 行排序为 UI/实现细节，不改变 spec 行为契约）
