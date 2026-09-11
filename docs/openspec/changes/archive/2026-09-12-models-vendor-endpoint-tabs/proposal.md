# Proposal: 模型管理页导航重构与同步流程恢复

## Why

模型管理页的导航结构在历史重构中发生了变化：供应商管理从独立 tab 变成了右上角「更多」菜单中的对话框入口，同步入口也收进了菜单，导致核心管理操作层级变深、不符合原有操作流程。需要恢复「模型列表 → 供应商 → 管理端点 → 部署」的顶部 tab 结构，并把同步模型资料入口提升为常驻按钮，同时让 OpenCode Go 成为默认同步来源。

## What Changes

- **新增「供应商」tab**：将右上角菜单中的「管理供应商」（VendorManagementDialog）提升为模型页顶部 tab，参照历史版本 `VendorsTable` 的表格形态嵌入页面，替换右上角菜单入口。
- **新增「管理端点」tab**：将右上角菜单中的「管理端点」（EndpointManagementDialog）提升为模型页顶部 tab，插在供应商与部署之间；移除菜单入口。
- **顶部 tab 顺序**：`Metadata`（模型列表）→ `Vendors`（供应商）→ `Endpoints`（管理端点）→ `Deployments`（部署）。
- **同步入口常驻**：在主操作区新增常驻「同步模型资料」按钮（历史版本 `Sync metadata` 按钮形态），点击直接打开同步向导；右上角菜单保留或移除 Sync Upstream 视实现而定。
- **同步来源调整**：来源选项顺序改为 OpenCode Go → Official → Configuration File（禁用），且 OpenCode Go 为默认选中项。

## Capabilities

### New Capabilities

- `models-page-navigation`: 模型管理页顶部 tab 导航结构（模型列表 / 供应商 / 管理端点 / 部署）与各 tab 内容挂载，以及供应商、管理端点从对话框到 tab 页的嵌入行为。

### Modified Capabilities

无（specs 体系当前为空目录，本 change 首次建立 models 相关 spec）。

## Impact

- **前端**：`web/src/features/models/` 下的 `section-registry.tsx`（新增 sections）、`index.tsx`（tab 内容挂载与操作区）、`models-primary-buttons.tsx`（移除菜单项、新增常驻同步按钮）、`models-provider.tsx`（tab 状态类型扩展）、`models-dialogs.tsx`（对话框挂载调整）、`constants.ts`（来源选项顺序与默认值）、`types.ts`（`ModelTabCategory` 扩展）、i18n locale 文件（en/zh 等新增文案）。
- **组件复用**：`vendor-management-dialog.tsx`、`endpoint-management-dialog.tsx` 的内部表格逻辑需要抽为可内嵌组件（参照历史 `VendorsTable`）。
- **测试**：更新 `web/src/features/models/__tests__/sync-source-options.test.ts`（来源顺序/默认值），新增或更新 tab 导航相关回归测试。
- **后端**：无改动（纯前端导航与交互调整）。
