# 模型管理页导航重构与同步流程恢复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将模型管理页顶部 tab 扩展为「模型列表 → 供应商 → 管理端点 → 部署」，移除右上角菜单中的供应商/端点/同步入口，主操作区新增常驻同步按钮，并把同步来源默认值改为 OpenCode Go。

**Architecture:** 扩展 `section-registry.tsx` 的 section 数组（顺序即 tab 顺序）；把 `VendorManagementDialog` / `EndpointManagementDialog` 的表格逻辑抽为页面组件 `VendorsTabContent` / `EndpointsTabContent`，删除对话框外壳；`models-primary-buttons.tsx` 移除三项菜单、新增常驻 Sync metadata 按钮；`getSyncSourceOptions` 调整顺序、默认值改 `opencode-go`。

**Tech Stack:** React 19, TypeScript, TanStack Router, TanStack Query, Base UI, Tailwind, i18next, Bun

**Spec:** `docs/openspec/changes/models-vendor-endpoint-tabs/specs/models-page-navigation/spec.md`
**Design Doc:** `docs/superpowers/specs/2026-09-12-models-vendor-endpoint-tabs-design.md`

## Global Constraints

- 全部前端工作目录：`web/`，包管理器为 `bun`
- 所有用户可见文案必须经 `t()` 支持 i18n；flat JSON locale 在 `web/src/i18n/locales/{lang}.json`（en 为基准，zh 为 fallback，另有 zh-TW/fr/ru/ja/vi）
- 修改 TS/TSX 后必须 `bun run typecheck` 无错误；涉及文件 lint 无 error
- 组件测试放各自模块的 `__tests__/` 目录；测试用 Vitest + React Testing Library；测试必须等待明确界面状态，禁止固定 sleep
- tab 顺序必须严格为：metadata → vendors → endpoints → deployments
- 提交信息用中文、Conventional Commits 结构（feat/fix/refactor 等）
- 不要修改后端代码；不要引入新依赖

---

## 文件结构

| 文件 | 操作 | 职责 |
|---|---|---|
| `web/src/features/models/section-registry.tsx` | 修改 | section 数组新增 vendors/endpoints |
| `web/src/features/models/types.ts` | 修改 | `ModelTabCategory` 扩展四值 |
| `web/src/features/models/index.tsx` | 修改 | 按 section 挂载内容与主操作区按钮 |
| `web/src/features/models/components/models-provider.tsx` | 修改 | `DialogType` 移除 manage-vendors/manage-endpoints；`syncWizardOptions` 默认 opencode-go |
| `web/src/features/models/components/models-primary-buttons.tsx` | 修改 | 移除三项菜单；新增常驻 Sync metadata |
| `web/src/features/models/components/models-dialogs.tsx` | 修改 | 移除两个对话框挂载 |
| `web/src/features/models/components/vendors-tab-content.tsx` | 新建 | 供应商表格（抽取自 VendorManagementDialog） |
| `web/src/features/models/components/endpoints-tab-content.tsx` | 新建 | 端点定义表格（抽取自 EndpointManagementDialog） |
| `web/src/features/models/components/dialogs/vendor-management-dialog.tsx` | 删除 | 外壳移除 |
| `web/src/features/models/components/dialogs/endpoint-management-dialog.tsx` | 删除 | 外壳移除 |
| `web/src/features/models/constants.ts` | 修改 | `getSyncSourceOptions` 顺序调整 |
| `web/src/features/models/components/dialogs/sync-wizard-dialog.tsx` | 修改 | 默认 source 改 opencode-go |
| `web/src/routes/_authenticated/models/$section.tsx` | 修改 | search schema 按需扩展 |
| `web/src/features/models/components/dialogs/__tests__/vendor-management-dialog.test.tsx` | 迁移 | 改为挂载 VendorsTabContent |
| `web/src/features/models/components/dialogs/__tests__/endpoint-management-dialog.test.tsx` | 迁移 | 改为挂载 EndpointsTabContent |
| `web/src/features/models/__tests__/sync-source-options.test.ts` | 修改 | 顺序断言更新 |
| `web/src/features/models/components/__tests__/models-navigation.test.tsx` | 新建 | tab 导航回归测试 |
| `web/src/i18n/locales/*.json` | 修改 | 新文案补齐 |

---

### Task 1: Section 注册与类型扩展

**Files:**
- Modify: `web/src/features/models/section-registry.tsx`
- Modify: `web/src/features/models/types.ts`
- Test: `web/src/features/models/components/__tests__/models-navigation.test.tsx`（本任务创建骨架，Task 4 补全断言）

**Interfaces:**
- Produces: `ModelsSectionId = 'metadata' | 'vendors' | 'endpoints' | 'deployments'`；`MODELS_SECTION_IDS` 输出四值且顺序固定

- [x] **Step 1: 扩展 section-registry**

在 `section-registry.tsx` 的 `MODELS_SECTIONS` 数组中加入 `vendors`、`endpoints`，最终顺序：

```tsx
const MODELS_SECTIONS = [
  { id: 'metadata', titleKey: 'Models', build: () => null },
  { id: 'vendors', titleKey: 'Vendors', build: () => null },
  { id: 'endpoints', titleKey: 'Endpoints', build: () => null },
  { id: 'deployments', titleKey: 'Deployments', build: () => null },
] as const
```

`titleKey` 使用英文源文案（`Models` / `Vendors` / `Endpoints` / `Deployments`），i18n locale 已含这些键（en.json 已有 `"Vendors"`、`"All Vendors"` 等，需确认精确键 `Vendors`/`Endpoints` 存在，缺失时 Task 4 补 i18n）。

- [x] **Step 2: 扩展 ModelTabCategory 类型**

在 `types.ts` 中将 `ModelTabCategory` 改为：

```ts
export type ModelTabCategory = 'metadata' | 'vendors' | 'endpoints' | 'deployments'
```

- [x] **Step 3: 验证类型与构建**

Run: `cd web && bun run typecheck`
Expected: PASS（此时 index.tsx 仍引用旧结构，若 typecheck 报 `ModelTabCategory` 相关错，属预期中间态，Task 2 完成后消除）

---

### Task 2: 对话框表格逻辑抽为 tab 页面组件

**Files:**
- Create: `web/src/features/models/components/vendors-tab-content.tsx`
- Create: `web/src/features/models/components/endpoints-tab-content.tsx`
- Delete: `web/src/features/models/components/dialogs/vendor-management-dialog.tsx`
- Delete: `web/src/features/models/components/dialogs/endpoint-management-dialog.tsx`

**Interfaces:**
- Consumes: `useModels()`（`setOpen`/`setCurrentVendor`）、`VendorMutateDialog`、`ConfirmDialog`、api（`getVendors`/`searchVendors`/`deleteVendor`/`getEndpointDefinitions`/`updateEndpointDefinitions`）、queryKeys
- Produces: `<VendorsTabContent />`、`<EndpointsTabContent />`（无 props，挂载即启用数据加载）

- [x] **Step 1: 创建 VendorsTabContent**

新建 `vendors-tab-content.tsx`：将 `vendor-management-dialog.tsx` 的 Dialog 外壳移除（去掉 `open`/`onOpenChange` props、`<Dialog>` 包裹、标题/描述/宽度类），保留：keyword/page/pageSize state、`useQuery`（去掉 `enabled: open`，改为 `enabled: true`）、`VendorRow` 映射、搜索输入框与清除按钮、`Add Vendor`/`Refresh` 按钮、`StatusBadge` 计数、错误 `Alert`、`StaticDataTable` 各列、`DataTablePagination`（保持块级流布局）、loading 指示、`VendorMutateDialog` 与 `ConfirmDialog` 挂载。组件导出 `export function VendorsTabContent()`。

- [x] **Step 2: 创建 EndpointsTabContent**

新建 `endpoints-tab-content.tsx`：将 `endpoint-management-dialog.tsx` 的 Dialog 外壳移除，保留 rows state、`useQuery`（`enabled: true`）、`updateRow`、`buildNpmOptions`、`validate`、`handleSave`、`StaticDataTable`（含 overflow 处理注释）、错误/loading 分支。保存动作由父级主操作区按钮触发：通过 `useModels()` 上下文或组件内自持 `isSaving` 并暴露保存方法。**决策：** 保存按钮放在 tab 内容组件内部顶部操作行（与 VendorsTabContent 的 Add/Refresh 行对齐），避免父级回调复杂度。导出 `export function EndpointsTabContent()`。

- [x] **Step 3: 删除旧对话框文件**

删除 `vendor-management-dialog.tsx`、`endpoint-management-dialog.tsx`。

- [x] **Step 4: 运行既有测试迁移验证**

Run: `cd web && bun run typecheck`
Expected: PASS

---

### Task 3: 页面挂载与主操作区调整

**Files:**
- Modify: `web/src/features/models/index.tsx`
- Modify: `web/src/features/models/components/models-dialogs.tsx`
- Modify: `web/src/features/models/components/models-primary-buttons.tsx`
- Modify: `web/src/features/models/components/models-provider.tsx`
- Modify: `web/src/routes/_authenticated/models/$section.tsx`

**Interfaces:**
- Consumes: `VendorsTabContent`、`EndpointsTabContent`、`MODELS_SECTION_IDS`、`useModels`
- Produces: 四 tab 页面；主操作区常驻 Sync metadata 按钮

- [x] **Step 1: index.tsx 按 section 挂载内容与操作区**

将 `SECTION_META` 扩展为四 section（titleKey 用 `Models`/`Vendors`/`Endpoints`/`Deployments`）。`ModelsContent` 中：

```tsx
let actions: React.ReactNode
let content: React.ReactNode
switch (activeSection) {
  case 'vendors':
    actions = <Button size='sm' onClick={() => { setCurrentVendor(null); setOpen('create-vendor') }}>
      <Plus className='h-4 w-4' />{t('Add Vendor')}</Button>
    content = <VendorsTabContent />
    break
  case 'endpoints':
    actions = <Button size='sm' onClick={/* 触发 EndpointsTabContent 保存，见 Step 3 */}>{t('Save')}</Button>
    content = <EndpointsTabContent />
    break
  case 'deployments':
    actions = <Button onClick={() => setCreateDeploymentOpen(true)} size='sm'><Plus className='h-4 w-4' />{t('Create deployment')}</Button>
    content = <DeploymentsSection />
    break
  default:
    actions = <ModelsPrimaryButtons />
    content = <ModelsTable />
}
```

注意：`models-provider.tsx` 的 `open` state 需新增 `endpoints-save` 触发（或改用 context 传递保存函数，见 Step 3 决策），确保 `EndpointsTabContent` 能收到保存指令。

- [x] **Step 2: models-dialogs.tsx 移除两个挂载**

删除 `VendorManagementDialog` / `EndpointManagementDialog` 的 import 与 JSX 挂载块。

- [x] **Step 3: models-primary-buttons.tsx 移除菜单项 + 新增常驻按钮**

- 删除 `handleManageVendors`、`handleManageEndpoints`、`handleSync` 与对应 `DropdownMenuItem`（Sync Upstream / Manage Vendors / Manage Endpoints 三项），保留 Missing Models、Prefill Groups
- 删除 `RefreshCw`、`Building2`、`Cable` 图标 import（不再使用）
- 在主按钮区新增（Add Model 之前）：

```tsx
<Button onClick={() => setOpen('sync-wizard')} variant='outline' size='sm'>
  {t('Sync metadata')}
</Button>
```

- 保存触发决策：为 `EndpointsTabContent` 增加对外保存能力，采用 context 通道——在 `ModelsContextType` 增加 `endpointsSaveSignal: number` 与 `triggerEndpointsSave: () => void`，`EndpointsTabContent` 用 `useEffect` 监听信号执行 `handleSave`；index.tsx 的 endpoints actions 按钮调用 `triggerEndpointsSave()`。

- [x] **Step 4: models-provider.tsx 类型清理**

`DialogType` 移除 `'manage-vendors' | 'manage-endpoints'`；新增 `endpointsSaveSignal` state（初始 0）与 `triggerEndpointsSave`（`setEndpointsSaveSignal((s) => s + 1)`）；`syncWizardOptions` 初始值改为 `{ locale: 'zh', source: 'opencode-go' }`。

- [x] **Step 5: $section.tsx search schema 扩展**

为 vendors 补充 `vPage`/`vPageSize`/`vFilter`（Zod：`z.number().optional().catch(1)` / `z.number().optional().catch(10)` / `z.string().optional().catch('')`）；endpoints 无需参数。若 VendorsTabContent 采用本地 state（Task 2 决策），此步仅保留 schema 空扩展说明，不引入未用参数——**最终以 Task 2 实现的 state 形态为准，二者保持一致**。

- [x] **Step 6: 验证**

Run: `cd web && bun run typecheck`
Expected: PASS

---

### Task 4: 同步来源顺序与默认值 + i18n

**Files:**
- Modify: `web/src/features/models/constants.ts`
- Modify: `web/src/features/models/components/dialogs/sync-wizard-dialog.tsx`
- Modify: `web/src/features/models/__tests__/sync-source-options.test.ts`
- Modify: `web/src/i18n/locales/en.json`、`zh.json`、`zh-TW.json`、`fr.json`、`ru.json`、`ja.json`、`vi.json`（按需）

**Interfaces:**
- Consumes: `getSyncSourceOptions(t)`、`SyncSource`
- Produces: 来源顺序 `opencode-go → official → config(禁用)`；向导默认选中 opencode-go

- [x] **Step 1: 调整来源顺序**

`constants.ts` `getSyncSourceOptions` 返回数组顺序改为：

```ts
{
  label: t('OpenCode Go'),
  value: 'opencode-go' as SyncSource,
  description: t('Sync from the official opencode-go catalog.'),
  disabled: false,
},
{
  label: t('Official Repository'),
  value: 'official' as SyncSource,
  description: t('Sync from the public upstream metadata repository.'),
  disabled: false,
},
{
  label: t('Configuration File'),
  value: 'config' as SyncSource,
  description: t('Upload or reference a local configuration file.'),
  disabled: true,
},
```

将「Official」项的 `Default` 徽标逻辑（sync-wizard-dialog.tsx 中 `option.value === 'official'`）改为 `option.value === 'opencode-go'`。

- [x] **Step 2: 默认来源改 opencode-go**

`sync-wizard-dialog.tsx` 初始 `useState<SyncSource>('official')` 改为 `useState<SyncSource>('opencode-go')`；`models-provider.tsx`（Task 3 已改）保持一致。

- [x] **Step 3: 更新单测**

`sync-source-options.test.ts`：
- 新测试「orders opencode-go first, official second, config last」：断言 `options.map((o) => o.value)` 等于 `['opencode-go', 'official', 'config']`
- 保留 opencode-go selectable 断言（更新：官方项不再断言为第一）
- 保留 config disabled 断言

- [x] **Step 4: 补充 i18n 键**

检查 `web/src/i18n/locales/en.json` 是否存在 `Sync metadata`、`Vendors`、`Endpoints` 键；缺失则补 en，运行 `cd web && bun run i18n:sync` 补齐其余语言。

- [x] **Step 5: 验证测试与构建**

Run: `cd web && bun run typecheck && bun run test sync-source-options`
Expected: PASS

---

### Task 5: 测试迁移与回归测试

**Files:**
- Modify: `web/src/features/models/components/dialogs/__tests__/vendor-management-dialog.test.tsx`（改为 `components/__tests__/vendors-tab-content.test.tsx`）
- Modify: `web/src/features/models/components/dialogs/__tests__/endpoint-management-dialog.test.tsx`（改为 `components/__tests__/endpoints-tab-content.test.tsx`）
- Create: `web/src/features/models/components/__tests__/models-navigation.test.tsx`

**Interfaces:**
- Consumes: `VendorsTabContent`、`EndpointsTabContent`、`Models`、section-registry、`getSyncSourceOptions`
- Produces: 回归测试覆盖 tab 顺序、URL 切换、菜单清理、来源顺序/默认

- [x] **Step 1: 迁移供应商测试**

将 `vendor-management-dialog.test.tsx` 复制为 `components/__tests__/vendors-tab-content.test.tsx`，替换 import 为 `VendorsTabContent`，`renderDialog` 改为 `render(<QueryClientProvider client={queryClient}><VendorsTabContent /></QueryClientProvider>)`，删除 `open onOpenChange` props。保留分页/搜索/删除断言。删除旧测试文件。

- [x] **Step 2: 迁移端点测试**

同理迁移为 `components/__tests__/endpoints-tab-content.test.tsx`：import `EndpointsTabContent`，无 props 渲染，保留保存/校验断言。删除旧测试文件。

- [x] **Step 3: 新建导航回归测试**

`components/__tests__/models-navigation.test.tsx`：
- 测试 1「section 顺序」：断言 `MODELS_SECTION_IDS` 等于 `['metadata', 'vendors', 'endpoints', 'deployments']`
- 测试 2「菜单不再含供应商/端点」：渲染 `ModelsPrimaryButtons`（mock api），打开菜单，断言 `Manage Vendors`、`Manage Endpoints`、`Sync Upstream` 不在菜单中，且 `Sync metadata` 常驻按钮存在
- 测试 3「来源顺序与默认」：断言 `getSyncSourceOptions(t)[0].value === 'opencode-go'`（t 为 identity）

遵循 mock 模式：mock `react-i18next`、`../../api`（按相对路径），等待明确界面状态。

- [x] **Step 4: 运行全部相关测试**

Run: `cd web && bun run test -- features/models`
Expected: 全部通过

---

### Task 6: 集成验证

**Files:** 无（验证任务）

- [ ] **Step 1: typecheck**

Run: `cd web && bun run typecheck`
Expected: 无错误

- [ ] **Step 2: lint**

Run: `cd web && bun run lint`（若全仓 lint 过慢，仅对涉及文件 `bunx oxlint web/src/features/models web/src/routes/_authenticated/models web/src/i18n`）
Expected: 无 error

- [ ] **Step 3: 生产构建**

Run: `cd web && bun run build`
Expected: 构建成功

- [ ] **Step 4: 勾选 tasks.md 并提交**

在 `<classic-change-dir>/tasks.md` 勾选全部任务，然后：

```bash
git add -A
git commit -m "feat(models): 模型页 tab 增加供应商/管理端点并恢复常驻同步入口"
```
