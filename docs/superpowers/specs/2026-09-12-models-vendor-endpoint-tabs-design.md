---
comet_change: models-vendor-endpoint-tabs
role: technical-design
canonical_spec: openspec
archived-with: 2026-09-12-models-vendor-endpoint-tabs
status: final
---

# Design Doc: 模型管理页导航重构与同步流程恢复

## Context

模型管理页（`web/src/features/models/`）当前导航结构：

```
/models/$section            section-registry（metadata / deployments）
   ├─ metadata    → ModelsTable
   └─ deployments → DeploymentsSection
右上角更多菜单（models-primary-buttons）：
   Missing Models / Sync Upstream / Prefill Groups / Manage Vendors / Manage Endpoints
```

「管理供应商」（`VendorManagementDialog`）与「管理端点」（`EndpointManagementDialog`）均为对话框组件，使用 `StaticDataTable` + 本地 state；同步入口（`sync-wizard`）在右侧菜单中。历史版本曾把供应商作为独立 tab（`VendorsTable`，`DataTablePage` + URL 状态）并把 `Sync metadata` 放在主操作区，本次变更恢复该导航结构并补齐端点 tab。

需求契约见 delta spec `specs/models-page-navigation/spec.md`。约束：`web/AGENTS.md`（i18n 全覆盖、bun 工具链、测试放 `__tests__/`、typecheck/lint/build 门槛）。

## Goals / Non-Goals

**Goals:**
- 通过 section-registry 声明四个 tab，URL 驱动导航与现有模式一致
- 复用现有对话框内部逻辑，抽为可内嵌表格组件，避免重复实现
- 保持同步向导、冲突处理流程不变，只调整入口与来源顺序/默认值
- 迁移并补充测试，确保行为回归可验证

**Non-Goals:**
- 不改后端 API 与数据模型
- 不改变 Deployments tab 行为
- 不引入新依赖、不更换前端状态管理机制
- 不将 `StaticDataTable` 迁移为 `DataTablePage` + URL 状态（保持改动最小）

## Decisions

### 1. Section 注册扩展

`section-registry.tsx` 的 `MODELS_SECTIONS` 顺序固定为：

```ts
sections: [metadata, vendors, endpoints, deployments]
```

`MODELS_SECTION_IDS` 数组顺序即 tab 渲染顺序，直接决定导航顺序。同步扩展：

- `types.ts`：`ModelTabCategory = 'metadata' | 'vendors' | 'endpoints' | 'deployments'`
- `$section.tsx` search schema：`metadata` 已有 `page/pageSize/filter/vendor/status/sync`；为 `vendors` 补充 `vPage`、`vPageSize`、`vFilter`（若沿用 URL 状态）或采用组件本地 state；`endpoints` 无分页需求，可不加参数

**备选**：`index.tsx` 手工渲染 Tabs——被否，section-registry 已承载 URL 风格与默认 section。

### 2. 对话框 → tab 内嵌组件

新建两个页面组件（放入 `web/src/features/models/components/`）：

- **`VendorsTabContent`**：将 `VendorManagementDialog` 内部逻辑（搜索、分页、表格列、删除确认、创建/编辑入口）抽入，移除 `Dialog` 外壳；`VendorMutateDialog`、`ConfirmDialog` 保留并在组件内挂载。数据加载从 `enabled: open` 改为挂载即启用（tab 激活时渲染）。
- **`EndpointsTabContent`**：将 `EndpointManagementDialog` 的端点表格、行编辑、校验、保存逻辑抽入；保存按钮放置页面级（主操作区或内容区底部）。

原 `vendor-management-dialog.tsx`、`endpoint-management-dialog.tsx` 删除（唯一引用点是菜单入口与 `models-dialogs.tsx`，一并清理）。

**备选**：对话框原样嵌入 tab——被否，遮罩/宽度语义破坏布局。

### 3. 菜单与常驻入口

`models-primary-buttons.tsx`：
- 移除 `handleManageVendors` / `handleManageEndpoints` / `handleSync`（Sync Upstream 菜单项）与对应 DropdownMenuItem
- 主按钮区（Add Model 前）新增常驻 `Sync metadata` 按钮：`variant='outline'`，`onClick={() => setOpen('sync-wizard')}`
- `models-provider.tsx` 的 `DialogType` 移除 `manage-vendors` / `manage-endpoints`
- `models-dialogs.tsx` 移除两个对话框挂载

### 4. 同步来源顺序与默认值

`constants.ts` `getSyncSourceOptions` 顺序改为：

1. `opencode-go`（label: OpenCode Go，disabled: false）
2. `official`（label: Official Repository，disabled: false，移除 Default 徽标或保留视设计）
3. `config`（label: Configuration File，disabled: true）

默认值：`sync-wizard-dialog.tsx` 初始 `source` 与 `models-provider.tsx` `syncWizardOptions` 初始值均改为 `'opencode-go'`。`syncWizardOptions` 记忆恢复逻辑保留（已有记忆时以记忆为准）。

### 5. i18n 与测试

- 主操作区/导航新增文案经 `t()` 使用，`bun run i18n:sync` 补齐 en/zh/zh-TW/fr/ru/ja/vi
- `sync-source-options.test.ts`：更新顺序与默认断言
- 迁移 `vendor-management-dialog.test.tsx` / `endpoint-management-dialog.test.tsx` 为挂载 `VendorsTabContent` / `EndpointsTabContent`
- 新增 tab 导航回归测试（四 tab 顺序、URL section 切换、菜单不含供应商/端点项）

## Risks / Trade-offs

- **[对话框抽取破坏既有测试]** → 迁移测试挂载形态，保留原有行为断言（搜索/分页/删除/保存）
- **[默认来源变更与用户记忆冲突]** → 仅改初始值，`syncWizardOptions` 记忆优先，不强制覆盖
- **[tab 内容滚动/分页容器回归]** → 沿用 `SectionPageLayout fixedContent` 与 `DataTablePagination` 块级占位约定（参考 dialog 内注释），验证各 tab 高度与分页交互
- **[tab 切换时数据请求]** → tab 激活才渲染组件，卸载即停；避免在非激活 tab 发起请求

## Migration Plan

纯前端导航与交互调整，无数据迁移。发布后用户直接看到四 tab 结构与新默认来源；旧菜单入口移除。

## Open Questions

无（specs/approach/tasks 均无未决项）。
