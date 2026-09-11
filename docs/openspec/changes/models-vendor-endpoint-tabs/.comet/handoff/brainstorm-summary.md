# Brainstorm Summary

- Change: models-vendor-endpoint-tabs
- Date: 2026-09-12

## 确认的技术方案

采用方案 A：section 扩展 + 对话框表格逻辑抽为页面组件。

- `section-registry.tsx` 新增 `vendors`、`endpoints`，顺序 `metadata → vendors → endpoints → deployments`；`ModelTabCategory` 扩展为四值联合；`$section.tsx` search schema 按需补充筛选参数。
- 新建 `VendorsTabContent`（沿用 `StaticDataTable` + 本地 state，保留搜索/分页/增删改/引用计数/删除确认，子对话框 `VendorMutateDialog`/`ConfirmDialog` 复用）与 `EndpointsTabContent`（端点定义表格 + 保存动作）。原对话框外壳删除（无其它引用点）。
- 移除右上角「Manage Vendors」「Manage Endpoints」「Sync Upstream」；主操作区新增常驻「Sync metadata」按钮，写死直接打开 sync-wizard。
- `getSyncSourceOptions` 顺序改为 `opencode-go` 置顶；向导默认 `source` 与 `syncWizardOptions` 初始值改 `opencode-go`（记忆逻辑保留）。
- 同步更新 i18n locale 文案。

## 关键取舍与风险

- 表格沿用 `StaticDataTable` + 本地 state（改动最小、保留测试基准），不用历史 `DataTablePage`+URL 状态。
- 对话框测试迁移到新页面组件；补充 tab 导航回归测试。
- 默认来源改 `opencode-go` 后，用户已记住的 `syncWizardOptions` 偏好不被覆盖。

## 测试策略

- 更新 `sync-source-options.test.ts`（顺序 + 默认值断言）。
- 迁移 `vendor-management-dialog.test.tsx` / `endpoint-management-dialog.test.tsx` 到新组件挂载形态。
- 新增 tab 导航测试：四 tab 顺序、URL section 切换、菜单不再含供应商/端点项。
- typecheck / lint / build 全绿。

## Spec Patch

无（delta spec 已覆盖全部需求场景）
