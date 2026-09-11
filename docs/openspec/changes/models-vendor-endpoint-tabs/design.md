# Design: 模型管理页导航重构与同步流程恢复

## Context

当前模型管理页（`web/src/features/models/`）使用 `section-registry.tsx` + `/models/$section` 路由实现顶部 tab：只有 `metadata`（模型列表）和 `deployments`（部署）两个 section。「管理供应商」和「管理端点」是两个对话框组件（`VendorManagementDialog` / `EndpointManagementDialog`），入口在右上角更多菜单（`models-primary-buttons.tsx`）。同步入口（`sync-wizard`）同样藏在菜单中。

历史版本（`284412899` 之前的本地实现）曾将「供应商」作为独立 tab（`VendorsTable`），并把「Sync metadata」按钮放在主操作区常驻。本次变更目标是恢复该导航结构并补齐「管理端点」tab。

约束：前端遵循 `web/AGENTS.md`（i18n 全覆盖、`bun` 工具链、typecheck/lint/测试门槛、测试放 `__tests__/`）；同步来源选项定义在 `constants.ts` 的 `getSyncSourceOptions`，有对应单测 `sync-source-options.test.ts`。

## Goals / Non-Goals

**Goals:**
- 通过 section-registry 声明四个 tab，保证 URL 驱动导航与现有模式一致
- 复用现有对话框内部逻辑，抽为可内嵌表格组件，避免重复实现
- 保持同步向导、冲突处理等既有流程不变，只调入口与来源顺序/默认值

**Non-Goals:**
- 不改动后端 API 与数据模型
- 不改变部署（Deployments）tab 的行为
- 不引入新的第三方依赖或新的前端状态管理机制

## Decisions

### 1. Section 注册扩展：新增 `vendors` 与 `endpoints` section
在 `section-registry.tsx` 的 `MODELS_SECTIONS` 中新增 `vendors`、`endpoints`，并把顺序固定为 `metadata → vendors → endpoints → deployments`；同步扩展 `ModelTabCategory` 类型与 `$section.tsx` 的 search schema（如需独立筛选参数）。`MODELS_SECTION_IDS` 数组顺序即 tab 渲染顺序，直接决定导航顺序。

**备选**：在 `index.tsx` 中手工渲染 Tabs。被否——section-registry 已承载 URL 风格与默认 section，沿用可保持一致。

### 2. 对话框 → tab 页内嵌组件
将 `VendorManagementDialog`、`EndpointManagementDialog` 的内部表格逻辑抽为页面级组件 `VendorsTabContent` / `EndpointsTabContent`（或以 `VendorsTable` 命名，与历史一致），接收同样的状态与动作入口（`useModels` 的 `setOpen`/`setCurrentVendor` 等）。原对话框组件可保留为包装层或移除，视组件复用面决定；菜单入口一律删除。

**备选**：把对话框原样嵌入 tab。被否——对话框自带遮罩/宽度语义，内嵌会破坏布局与滚动行为。

### 3. 主操作区常驻同步入口
在 `models-primary-buttons.tsx` 主按钮区（Add Model 之前或之后）新增常驻「Sync metadata」按钮，`onClick` 打开 `sync-wizard`，与历史版本按钮形态一致；右上角菜单中的 Sync Upstream 项移除，避免双入口。

### 4. 同步来源顺序与默认值
调整 `getSyncSourceOptions`：`opencode-go` 置顶且 `disabled: false`，随后 `official`，最后 `config`（禁用）。同步向导 `sync-wizard-dialog.tsx` 的默认 state `source` 从 `'official'` 改为 `'opencode-go'`；`models-provider.tsx` 中 `syncWizardOptions` 初始值同步改为 `{ locale: 'zh', source: 'opencode-go' }`。同步更新 `sync-source-options.test.ts` 断言顺序与默认值。

## Risks / Trade-offs

- **[对话框抽取破坏既有测试]** → `vendor-management-dialog.test.tsx`、`endpoint-management-dialog.test.tsx` 针对对话框渲染；抽取为 tab 组件后同步迁移测试挂载方式，并补充 tab 导航回归测试。
- **[来源默认值变更影响用户既有选择]** → 仅改变向导初始选中项，用户可在向导内改选；`syncWizardOptions` 记忆逻辑仍保留，不强制覆盖已记住的偏好。
- **[tab 内容滚动/布局回归]** → 复用现有 `SectionPageLayout fixedContent` 与表格容器结构，验证各 tab 高度与分页交互。
