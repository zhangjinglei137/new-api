# 渠道序号字段与排序 — 任务清单

## 1. 后端模型与排序逻辑

- [x] 1.1 在 `model/channel.go` 的 `Channel` 结构体中新增 `Sort *int64` 字段（gorm 标签 `bigint;default:0`，JSON `sort`），并在最接近字段位置补充注释说明；运行 `go build ./...` 验证编译通过
- [x] 1.2 在 `model/channel.go` 的 `channelSortColumns` 白名单中加入 `"sort": "sort"`，并新增 `SortSort` 字段到 `ChannelSortOptions`；调整 `NewChannelSortOptions` 签名与调用点，使 `Apply` 的优先级为：列头 `sort_by` > `sort_sort` > `id_sort` > 默认按 `sort` 升序；运行 `go test ./model/...`（或 `./model/ -run ChannelSort` 相关用例）验证排序行为
- [x] 1.3 检查后端所有调用 `NewChannelSortOptions` / `ChannelSortOptions` 的位置（`controller/channel.go`、`model/channel.go` 等），为 `SearchChannels` 增加 `sort_sort` 查询参数解析并透传；运行 `go build ./...` 与既有渠道相关测试验证

## 2. 前端类型与数据流

- [x] 2.1 在 `web/src/features/channels/types.ts` 的 `ChannelSortBy` 联合类型中加入 `'sort'`；在 `GetChannelsParams` / `SearchChannelsParams` 中加入可选 `sort_sort?: boolean`，并同步 `channelSchema`/`Channel` 类型中的 `sort` 字段；运行 `bun run typecheck` 验证
- [x] 2.2 在 `web/src/features/channels/components/channels-provider.tsx` 的 context 类型与 provider 状态中加入 `sortSort: boolean` / `setSortSort`（localStorage key `channels-sort-sort`，默认 false），与 `idSort` 对称；运行 `bun run typecheck` 验证
- [x] 2.3 在 `web/src/features/channels/components/channels-table.tsx` 的 `CHANNEL_SORTABLE_COLUMNS` 中加入 `'sort'`，并在列表查询参数中透传 `sort_sort: sortSort`；验证排序状态能触发后端 `sort_by=sort` 请求

## 3. 前端 UI：序号列、表单与开关

- [x] 3.1 在 `web/src/features/channels/components/channels-columns.tsx` 的 `PriorityCell` 附近新增 `sort` 列（`header: t('Sort')`，单元格复用内联编辑模式，`meta: { mobileHidden: true }`），并确认 `ChannelFieldCell` 的 `field` 类型或调用能覆盖 `'sort'`；运行 `bun run typecheck` 与 lint 验证
- [x] 3.2 在 `web/src/features/channels/components/channels-primary-buttons.tsx` 桌面工具栏与移动端下拉菜单中加入「使用序号排序」开关（复用 `SortAsc` 图标与 `Switch`/`DropdownMenuCheckboxItem`），绑定 `sortSort` / `setSortSort` 并持久化 localStorage；运行 lint 与既有渠道组件测试验证
- [x] 3.3 在 `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`（及相应的新增/编辑表单 schema）中加入「序号」（sort）输入字段，支持整数、默认 0；校验提交时携带 `sort` 字段；运行 `bun run typecheck` 验证

## 4. 国际化

- [x] 4.1 在 `web/src/i18n/locales/en.json` 添加 `Sort`（序号/排序）相关文案源串（如 `Sort`、`Sort by Sort`），并同步 `zh.json` 等既有语言文件；运行 `bun run i18n:sync`（如可用）验证 key 同步

## 5. 验证与交付

- [x] 5.1 补充/更新后端排序单元测试：覆盖 `sort` 白名单、默认按 `sort` 升序、`sort_sort` 优先级高于 `id_sort`、列头排序优先于开关；运行 `go test ./model/... ./controller/...` 验证通过
- [x] 5.2 补充/更新前端渠道列表与开关的回归测试（排序列交互、开关切换与持久化）；运行 `bun run test`（受影响用例）与 `bun run lint`、`bun run typecheck` 验证
- [x] 5.3 按项目数据库兼容规则对 SQLite、MySQL、PostgreSQL 三库执行迁移验证（启动 AutoMigrate 新增 `sort` 列，覆盖新建库与升级库并确认幂等），并记录数据库版本与结果；确认默认排序行为（无排序时按序号升序）