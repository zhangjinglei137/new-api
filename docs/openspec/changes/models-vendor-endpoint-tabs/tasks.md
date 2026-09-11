# Tasks: models-vendor-endpoint-tabs

## 1. Section 与导航结构

- [x] 1.1 在 `section-registry.tsx` 的 `MODELS_SECTIONS` 中新增 `vendors`、`endpoints`，顺序固定为 `metadata → vendors → endpoints → deployments`；验证 `MODELS_SECTION_IDS` 输出该顺序
- [x] 1.2 扩展 `types.ts` 的 `ModelTabCategory` 为四值联合（metadata/vendors/endpoints/deployments）；验证 typecheck 通过
- [ ] 1.3 在 `$section.tsx` search schema 中为 vendors/endpoints 补充所需筛选参数（如 `vPage`、`vPageSize`、`vFilter`、`eFilter` 等，按实际需要）；验证 `/models/vendors`、`/models/endpoints` 路由可解析且非法 section 仍回退默认

## 2. tab 内容组件抽取

- [x] 2.1 将 `VendorManagementDialog` 表格主体抽为页面内嵌组件（参照历史 `VendorsTable`），支持分页/搜索/创建/编辑/删除/引用计数；验证 `/models/vendors` 渲染供应商表格且增删改查可用
- [x] 2.2 将 `EndpointManagementDialog` 表格主体抽为页面内嵌组件，支持端点定义列表/编辑/保存；验证 `/models/endpoints` 渲染端点表格且编辑保存生效
- [ ] 2.3 在 `index.tsx` 按 section 挂载对应内容并调整主操作区（metadata 保留现有按钮组，vendors 显示 Add Vendor，endpoints/deployments 显示对应动作）；验证四个 tab 内容切换正确
- [ ] 2.4 迁移 `vendor-management-dialog.test.tsx`、`endpoint-management-dialog.test.tsx` 到新组件挂载形态并保持既有行为断言；验证相关测试通过

## 3. 菜单与常驻入口

- [ ] 3.1 从 `models-primary-buttons.tsx` 更多菜单移除「Manage Vendors」「Manage Endpoints」项及对应 handler；验证菜单不再含两项
- [ ] 3.2 在主操作区新增常驻「Sync metadata」按钮打开 sync-wizard，并移除菜单中 Sync Upstream 项；验证点击按钮打开同步向导
- [ ] 3.3 同步更新 `models-dialogs.tsx` 挂载（若对话框保留）与 i18n locale（en/zh/zh-TW/fr/ru/ja/vi）文案；验证 `bun run i18n:sync` 无缺失键

## 4. 同步来源顺序与默认值

- [ ] 4.1 调整 `getSyncSourceOptions`：`opencode-go` 置顶，随后 `official`，最后 `config`（禁用）；更新 `sync-source-options.test.ts` 断言顺序，验证测试通过
- [ ] 4.2 将 `sync-wizard-dialog.tsx` 默认 `source` 与 `models-provider.tsx` `syncWizardOptions` 初始值改为 `opencode-go`；验证向导打开时默认选中 OpenCode Go

## 5. 集成验证

- [ ] 5.1 运行 `bun run typecheck` 且无错误
- [ ] 5.2 运行受影响测试文件（models 相关 `__tests__`）全部通过
- [ ] 5.3 运行涉及文件的 lint 无 error
- [ ] 5.4 运行 `bun run build` 生产构建成功
