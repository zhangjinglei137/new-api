# Comet Design Handoff

- Change: models-vendor-endpoint-tabs
- Phase: design
- Mode: compact
- Context hash: 9f6e24599e6d095efc28dbbed4a25936ffc8b564cc19cf7685b93a18f7ebab10

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## docs/openspec/changes/models-vendor-endpoint-tabs/proposal.md

- Source: docs/openspec/changes/models-vendor-endpoint-tabs/proposal.md
- Lines: 1-30
- SHA256: 984e2f151f2ac2a72d2033f093214ef164529a026ba610b7ab2310d0dc005bc8

```md
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

```

## docs/openspec/changes/models-vendor-endpoint-tabs/design.md

- Source: docs/openspec/changes/models-vendor-endpoint-tabs/design.md
- Lines: 1-45
- SHA256: 15e1441ead6e38f0781c1ffb3b2af0462138c8c1528645888a0c83bea36619fc

```md
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

```

## docs/openspec/changes/models-vendor-endpoint-tabs/tasks.md

- Source: docs/openspec/changes/models-vendor-endpoint-tabs/tasks.md
- Lines: 1-32
- SHA256: 22c15d2b62b1b854c77f1c8c7080176ebd505d93ad2ec9c381500b1b92814e89

```md
# Tasks: models-vendor-endpoint-tabs

## 1. Section 与导航结构

- [ ] 1.1 在 `section-registry.tsx` 的 `MODELS_SECTIONS` 中新增 `vendors`、`endpoints`，顺序固定为 `metadata → vendors → endpoints → deployments`；验证 `MODELS_SECTION_IDS` 输出该顺序
- [ ] 1.2 扩展 `types.ts` 的 `ModelTabCategory` 为四值联合（metadata/vendors/endpoints/deployments）；验证 typecheck 通过
- [ ] 1.3 在 `$section.tsx` search schema 中为 vendors/endpoints 补充所需筛选参数（如 `vPage`、`vPageSize`、`vFilter`、`eFilter` 等，按实际需要）；验证 `/models/vendors`、`/models/endpoints` 路由可解析且非法 section 仍回退默认

## 2. tab 内容组件抽取

- [ ] 2.1 将 `VendorManagementDialog` 表格主体抽为页面内嵌组件（参照历史 `VendorsTable`），支持分页/搜索/创建/编辑/删除/引用计数；验证 `/models/vendors` 渲染供应商表格且增删改查可用
- [ ] 2.2 将 `EndpointManagementDialog` 表格主体抽为页面内嵌组件，支持端点定义列表/编辑/保存；验证 `/models/endpoints` 渲染端点表格且编辑保存生效
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

```

## docs/openspec/changes/models-vendor-endpoint-tabs/specs/models-page-navigation/spec.md

- Source: docs/openspec/changes/models-vendor-endpoint-tabs/specs/models-page-navigation/spec.md
- Lines: 1-69
- SHA256: 4983861cd9dddf94482c24b0f79f6846f9a7e46a8f96b2a8f77015355e7ea10b

```md
# Delta Spec: models-page-navigation

## Purpose

Defines the model management page navigation contract: the top-level tab structure (models list, vendors, endpoints, deployments), how vendor and endpoint management are embedded as page tabs instead of menu dialogs, and the persistent model metadata sync entry with its source ordering and default.

## ADDED Requirements

### Requirement: Top-level tab navigation
The model management page SHALL expose a top-level tab bar containing exactly four tabs in this order: Models (model list), Vendors (supplier management), Endpoints (endpoint management), and Deployments (deployment management). The default landing tab SHALL be Models.

#### Scenario: Landing on the models page
- **WHEN** an admin opens the model management page without a section
- **THEN** the Models tab is active and the model list is displayed

#### Scenario: Switching tabs
- **WHEN** the admin clicks the Vendors, Endpoints, or Deployments tab
- **THEN** the URL section changes to the corresponding section and the matching content is rendered below the tab bar

### Requirement: Vendor management as a page tab
The vendor management capability SHALL be accessible as the Vendors page tab rather than through the top-right overflow menu. The tab SHALL render a vendor table with the same behaviors as the previous vendor management dialog: paginated listing, keyword search, create/edit, delete with reference counts, and vendor mutation via the shared vendor form.

#### Scenario: Accessing vendor management from the tab
- **WHEN** an admin clicks the Vendors tab
- **THEN** the vendor table is rendered inline and the "Manage Vendors" entry is no longer present in the top-right overflow menu

#### Scenario: Creating a vendor from the Vendors tab
- **WHEN** the admin clicks the Add Vendor action on the Vendors tab
- **THEN** the vendor create dialog opens and a new vendor can be saved

#### Scenario: Vendors tab with pagination and search
- **WHEN** the vendor list exceeds one page or the admin enters a search keyword
- **THEN** the table paginates results and filters by keyword server-side

### Requirement: Endpoint management as a page tab
The endpoint management capability SHALL be accessible as the Endpoints page tab positioned between the Vendors tab and the Deployments tab. The tab SHALL render the endpoint definition table with the same behaviors as the previous endpoint management dialog: listing endpoint definitions, editing fields (HTTP method, path, NPM), and saving updates.

#### Scenario: Accessing endpoint management from the tab
- **WHEN** an admin clicks the Endpoints tab
- **THEN** the endpoint definition table is rendered inline and the "Manage Endpoints" entry is no longer present in the top-right overflow menu

#### Scenario: Editing an endpoint definition
- **WHEN** the admin modifies an endpoint definition field on the Endpoints tab and saves
- **THEN** the update is persisted and the table refreshes with the new values

### Requirement: Overflow menu cleanup
The top-right overflow menu of the model management page SHALL no longer offer "Manage Vendors" or "Manage Endpoints" entries, since both capabilities are now exposed as tabs.

#### Scenario: Menu no longer shows vendor/endpoint entries
- **WHEN** the admin opens the top-right overflow menu on the Models tab
- **THEN** neither "Manage Vendors" nor "Manage Endpoints" appears among the menu items

### Requirement: Persistent model metadata sync entry
The model management page SHALL provide a persistent, directly selectable model metadata sync entry in the primary action area (next to the Add Model button). Clicking it SHALL open the sync wizard with source selection visible first.

#### Scenario: Triggering sync from the primary action area
- **WHEN** the admin clicks the sync entry button in the primary action area
- **THEN** the sync wizard opens and the source selection step is shown before language selection

### Requirement: Sync source ordering and default
The sync source options SHALL be ordered OpenCode Go first, Official Repository second, and Configuration File last (Configuration File remains disabled). OpenCode Go SHALL be the default selected source when the sync wizard opens.

#### Scenario: Default source is OpenCode Go
- **WHEN** the sync wizard opens
- **THEN** OpenCode Go is the pre-selected source option

#### Scenario: Source options ordered with OpenCode Go first
- **WHEN** the sync source options are rendered
- **THEN** OpenCode Go appears first, Official Repository second, and Configuration File last in a disabled state

```
