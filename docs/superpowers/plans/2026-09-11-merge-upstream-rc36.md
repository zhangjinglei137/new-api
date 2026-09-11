---
archived-with: 2026-09-11-merge-upstream-rc36
status: final
---
# merge-upstream-rc36 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 合并上游 QuantumNous/new-api main（47 提交，rc.32→rc.36）进本地 fork，融合优先解决 17 个冲突文件，通过编译/测试/三库验证。

**Architecture:** 保留本地富模型元数据架构（`model_rich.go` 数据源），放弃上游 SquareState 运行时聚合；前端完整取上游重构核心 + 重挂本地扩展；channels-columns 本地优先。

**Tech Stack:** Go 1.22+/Gin/GORM、React 19/Bun/Tailwind、SQLite/MySQL/PostgreSQL

**Spec:** `docs/superpowers/specs/2026-09-11-merge-upstream-rc36-design.md`（canonical: openspec change `merge-upstream-rc36`）

## Global Constraints

- relaykit/ 模块必须独立可构建：`cd relaykit && GOWORK=off go build ./...`
- JSON 必须用 `common/json.go` 包装函数（`common.Marshal/Unmarshal/...`），禁直接 `encoding/json`
- 三数据库兼容（SQLite/MySQL≥5.7.8/PostgreSQL≥9.6）：schema 变更必须三库验证（新鲜库+升级库+幂等）
- 计费不变量：quota 计算不得产生负扣费；转换用 `common.QuotaFromFloat/QuotaRound/QuotaFromDecimal`（及 `*Checked` 变体）；乘法器用 `types.PriceData.AddOtherRatio` bound
- `model/channel.go` 有 `merge=ours` 策略（.gitattributes），merge 时自动取本地侧
- 前端用 bun；i18n 用 `useTranslation()`/`t('English key')`，产物语言 zh-CN

---

### Task 1: 执行上游 merge 基底

**Files:**
- 全仓（`git merge --no-edit upstream/main`）

**Interfaces:**
- Consumes: 无
- Produces: 非冲突文件自动合并后的中间状态；17 个冲突文件处于冲突标记状态

- [x] **Step 1: 确认基线**

```bash
git status --short
git log --oneline -1
# 预期：worktree 干净，HEAD = 8e0a063a（main 最新）
```

- [x] **Step 2: 执行 merge**

```bash
git merge --no-edit upstream/main
# 预期：17 个文件冲突（Go 6 + 前端 12 中真实冲突部分）
```

- [x] **Step 3: 记录冲突状态**

```bash
git diff --name-only --diff-filter=U
# 预期输出：冲突文件清单
```

- [x] **Step 4: 不提交（冲突未解决，先逐个文件融合）**

---

### Task 2: 修复 common/crypto.go（完整融合）

**Files:**
- Modify: `common/crypto.go`

**Interfaces:**
- Consumes: 本地 AES-GCM envelope（`EncryptSecret/DecryptSecret`）+ 上游 argon2 分支（`ValidatePasswordAndHash` 内 `strings.HasPrefix`）
- Produces: import 含 `strings`；保留 AES 扩展 + argon2 分支共存

- [x] **Step 1: 打开冲突文件确认冲突区**

```bash
# 冲突区：import 块（本地有 aes/cipher/rand/base64/fmt，上游有 strings，无 AES）
# 非冲突区：ValidatePasswordAndHash 的 argon2 分支已被自动应用
```

- [x] **Step 2: 解决冲突——import 块取并集（保留本地 AES import + 补 `strings`）**

```go
import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)
```

- [x] **Step 3: 确认 `common/account_password.go`（上游新文件）已自动合并**

```bash
ls common/account_password.go
grep -n "argon2" common/account_password.go | head -3
```

- [x] **Step 4: 编译验证**

```bash
git add common/crypto.go
GOWORK=off go build ./common/ && GOWORK=off go vet ./common/
# 预期：exit 0
```

- [x] **Step 5: Commit**

```bash
git commit -m "merge: 融合上游 crypto.go（本地 AES 凭证加密 + 上游 argon2 分支）"
```

---

### Task 3: 移植 resolveModelMetadata + MatchesName 到 model/model_meta.go

**Files:**
- Modify: `model/model_meta.go`（本地富元数据版本）

**Interfaces:**
- Consumes: 上游 `model/pricing.go` 调用 `resolveModelMetadata(allMeta, names)`；本地 `NameRule/ModelName/Status` 字段
- Produces: `func resolveModelMetadata(allMeta []*Model, names []string) map[string]*Model`、`func (m *Model) MatchesName(name string) bool`（从上游移植，纯函数，不碰富元数据字段）

- [x] **Step 1: 从上游取两函数源码**

```bash
git show upstream/main:model/model_meta.go | grep -n "func resolveModelMetadata" 
git show upstream/main:model/model_meta.go | sed -n '/func resolveModelMetadata/,/^}/p'
git show upstream/main:model/model_meta.go | grep -n "func.*MatchesName"
git show upstream/main:model/model_meta.go | sed -n '/func.*MatchesName/,/^}/p'
```

- [x] **Step 2: 将两函数追加到本地 `model/model_meta.go`（保留本地富元数据列、Insert/Update/Delete、GetAllProviderNPMs 等）**

- [x] **Step 3: 解决 model_meta.go 冲突——本地优先**

```bash
# 冲突区：结构体定义 + Insert/Update/Delete
# 处理：保留本地结构体（11 富元数据列），删掉上游冲突块；只保留移植的 resolveModelMetadata/MatchesName
```

- [x] **Step 4: 编译验证**

```bash
git add model/model_meta.go
GOWORK=off go build ./model/ && GOWORK=off go vet ./model/
# 预期：pricing.go 的 resolveModelMetadata vet 错误消除
```

- [x] **Step 5: Commit**

```bash
git commit -m "merge: 移植上游 resolveModelMetadata/MatchesName，保留本地富元数据架构"
```

---

### Task 4: 融合 controller/vendor_meta.go + model.SearchVendors association 形参

**Files:**
- Modify: `controller/vendor_meta.go`、`model/vendor_meta.go`

**Interfaces:**
- Consumes: 上游 `SearchVendors` 签名（加 `association` 参数）、`vendorAPIError`、`recordManageAudit`；本地 `vendor_counts` 聚合（`model.GetVendorModelCounts`）
- Produces: 融合版本——本地 `vendor_counts` + 上游错误助手/审计/`association` 形参

- [x] **Step 1: 查看上游改动**

```bash
git show upstream/main:controller/vendor_meta.go | grep -n "vendorAPIError\|recordManageAudit\|SearchVendors"
git show upstream/main:model/vendor_meta.go | grep -n "func SearchVendors"
```

- [x] **Step 2: 融合 controller/vendor_meta.go**：保留本地 `vendor_counts` 聚合逻辑，吸收上游 `vendorAPIError`/`recordManageAudit`/`interface{}→any`，适配 `SearchVendors` 新签名

- [x] **Step 3: 移植 `model.SearchVendors` 的 `association` 形参到本地 model 层**（若本地 model/vendor_meta.go 未自动合并上游版）

- [x] **Step 4: 编译验证**

```bash
git add controller/vendor_meta.go model/vendor_meta.go
GOWORK=off go build ./... && GOWORK=off go vet ./...
```

- [x] **Step 5: Commit**

```bash
git commit -m "merge: 融合 vendor_meta（本地 counts + 上游审计/association）"
```

---

### Task 5: 融合 controller/channel_upstream_update.go

**Files:**
- Modify: `controller/channel_upstream_update.go`

**Interfaces:**
- Consumes: 本地 DB 保留字列引用修复 + 上游火山 `/api/v3/models` 修复
- Produces: 融合版本（改动区不重叠，git 应自动合并大部分，手工确认）

- [x] **Step 1: 查看冲突区并解决**：保留本地 `channelUpstreamModelUpdateSelectFields` 方言兼容 + 吸收上游火山端点修复 + `interface{}→any`

- [x] **Step 2: 编译验证**

```bash
git add controller/channel_upstream_update.go
GOWORK=off go build ./... && GOWORK=off go vet ./...
```

- [x] **Step 3: Commit**

```bash
git commit -m "merge: 融合 channel_upstream_update（本地方言兼容 + 上游火山 404 修复）"
```

---

### Task 6: 融合 controller/model_meta.go

**Files:**
- Modify: `controller/model_meta.go`

**Interfaces:**
- Consumes: 本地 `validateModelCapabilities`/`enrichModels`/`CreateModelMeta` 富元数据校验
- Produces: 本地优先版本，吸收 `interface{}→any`/`recordManageAudit`；放弃 `square_state`/`include_channel_models`

- [x] **Step 1: 解决冲突——本地优先**：保留本地富元数据列表/编辑逻辑；吸收上游非破坏性改进；删除依赖上游 model 层新函数（`SearchModelsWithChannels`/`ModelSquareState`）的代码块

- [x] **Step 2: 编译验证**

```bash
git add controller/model_meta.go
GOWORK=off go build ./... && GOWORK=off go vet ./...
```

- [x] **Step 3: Commit**

```bash
git commit -m "merge: 融合 controller/model_meta（本地富元数据优先）"
```

---

### Task 7: 融合 controller/model_sync.go

**Files:**
- Modify: `controller/model_sync.go`

**Interfaces:**
- Consumes: 本地 479 行富元数据同步映射（DisplayName/Family/ProviderNpm/OpenWeights/Reasoning→CapReasoning）
- Produces: 本地优先版本，吸收上游非富元数据改进

- [x] **Step 1: 解决冲突——本地优先**：保留富元数据同步映射；吸收上游同步流程中非富元数据改进（供应商同步/错误处理）；放弃上游 380 行删除

- [x] **Step 2: 编译验证**

```bash
git add controller/model_sync.go
GOWORK=off go build ./... && GOWORK=off go vet ./...
```

- [x] **Step 3: Commit**

```bash
git commit -m "merge: 融合 controller/model_sync（本地富元数据同步优先）"
```

---

### Task 8: Go 全量编译 + relaykit 独立构建验证

**Files:**（无）

**Interfaces:**
- Consumes: Task 2-7 全部融合完成
- Produces: Go 侧编译绿

- [x] **Step 1: 全量编译验证**

```bash
git add -A
GOWORK=off go build ./...
GOWORK=off go vet ./...
# 预期：exit 0
```

- [x] **Step 2: relaykit 独立构建**

```bash
cd relaykit && GOWORK=off go build ./... && GOWORK=off go vet ./...
# 预期：exit 0（模块独立构建硬要求）
cd ..
```

- [x] **Step 3: 确认无残留冲突标记**

```bash
grep -rn "<<<<<<<" --include="*.go" . | head
# 预期：无输出
```

- [x] **Step 4: Commit（若有遗漏改动）**

```bash
git commit -m "merge: Go 侧融合完成，编译/vet/relaykit 通过" 2>/dev/null || echo "无未提交改动"
```

---

### Task 9: 融合 model-mutate-drawer.tsx（上游骨架 + 重挂本地扩展）

**Files:**
- Modify: `web/src/features/models/components/drawers/model-mutate-drawer.tsx`

**Interfaces:**
- Consumes: 上游 `initialSection` prop（metadata/pricing 分段导航）、`PriceSyncDialog`；本地 endpoint 模板下拉（`endpointDefinitionsQueryKeys`）+ `CapabilityGroupsEditor`/`capabilityFields`
- Produces: 融合版本——上游 706 行骨架 + 本地 endpoint/capability 扩展挂到 `initialSection='metadata'` 分段

- [x] **Step 1: 查看上游版本结构**

```bash
git show upstream/main:web/src/features/models/components/drawers/model-mutate-drawer.tsx | head -80
# 确认 initialSection、SideDrawerSection 布局、form schema
```

- [x] **Step 2: 取上游骨架 + 重挂本地扩展**：以 `upstream/main` 版本为基础，把本地 `endpoints` 字段、端点模板下拉、`CapabilityGroupsEditor` 挂入 metadata 分段；**zod schema 逐字段比对**（`ExtendedModelFormValues` 双方都改过）

- [x] **Step 3: 类型检查**

```bash
cd web && bun run typecheck
# 预期：无新增类型错误（若上游未改 package.json 依赖）
cd ..
```

- [x] **Step 4: Commit**

```bash
git add web/src/features/models/components/drawers/model-mutate-drawer.tsx
git commit -m "merge: 融合 model-mutate-drawer（上游 initialSection 骨架 + 本地 endpoint/capability 扩展）"
```

---

### Task 10: 融合 models-dialogs.tsx（并列融合）

**Files:**
- Modify: `web/src/features/models/components/models-dialogs.tsx`

**Interfaces:**
- Consumes: 上游 `PriceSyncDialog`/`price-model`/`initialSection` 传参；本地 `EndpointManagementDialog`/`VendorManagementDialog` 注册
- Produces: 上游结构 + 本地对话框注册

- [x] **Step 1: 取上游结构**（含 PriceSyncDialog + price-model open 状态 + initialSection 传参 + VendorMutateDialog key）

- [x] **Step 2: 追加本地对话框注册**：`EndpointManagementDialog` + `VendorManagementDialog`

- [x] **Step 3: 处理 vendor 命名碰撞**：确认本地 `vendor-management-dialog.tsx`（单数）vs 上游 `vendors-management-dialog.tsx`（复数）职责——若重复弃本地保上游，删除本地文件并改用上游 import

- [x] **Step 4: 类型检查**

```bash
cd web && bun run typecheck
cd ..
```

- [x] **Step 5: Commit**

```bash
git add web/src/features/models/components/models-dialogs.tsx web/src/features/models/components/dialogs/
git commit -m "merge: 融合 models-dialogs（上游 PriceSync + 本地对话框注册）"
```

---

### Task 11: 取上游重构核心 4 文件 + 处理类型改向

**Files:**
- 完整取上游: `web/src/features/models/components/dialogs/sync-wizard-dialog.tsx`、`web/src/features/models/components/dialogs/upstream-conflict-dialog.tsx`、`web/src/features/system-settings/models/upstream-ratio-sync-helpers.ts`、`web/src/features/models/components/models-columns.tsx`

**Interfaces:**
- Consumes: 上游 `MetadataSyncField`/`MetadataSyncCandidate`/`STEPS`/`PAGE_SIZE`、`SyncPriceContext`/`PricingSourceSelection`
- Produces: 本地组件若 import 旧类型（`SyncDiffData['conflicts']` 等）需改向 sync-wizard 新类型

- [x] **Step 1: 对这 4 个文件执行 `git checkout --theirs`（取上游版本）**

```bash
git checkout --theirs -- \
  web/src/features/models/components/dialogs/sync-wizard-dialog.tsx \
  web/src/features/models/components/dialogs/upstream-conflict-dialog.tsx \
  web/src/features/system-settings/models/upstream-ratio-sync-helpers.ts \
  web/src/features/models/components/models-columns.tsx
git add <上述文件>
```

- [x] **Step 2: 处理 models-columns 本地语义补回**：确认本地 display_name revert（不在列里显示 display_name）是否需要在新列定义上重应用

- [x] **Step 3: 全局搜索旧类型引用并改向**

```bash
grep -rn "SyncDiffData\|UpstreamConflictDialog" web/src --include="*.tsx" --include="*.ts" | grep -v upstream-conflict-dialog.tsx
```

- [x] **Step 4: 类型检查**

```bash
cd web && bun run typecheck
cd ..
```

- [x] **Step 5: Commit**

```bash
git commit -m "merge: 取上游模型管理重构核心（sync-wizard/conflict-dialog/ratio-sync/columns）"
```

---

### Task 12: 融合 channels-columns.tsx（本地优先 + 吸收）

**Files:**
- Modify: `web/src/features/channels/components/channels-columns.tsx`

**Interfaces:**
- Consumes: 本地 6 类渠道用量弹窗（Command/SenseNova/Moonshot/Zhipu/Volc/Radeon）+ opencode 徽标；上游 `handleServerError`/`createServerError` 错误通知统一 + `TaskPluginChannelBadge` 插件图标
- Produces: 本地 1579 行渠道扩展 + 上游两处小改

- [x] **Step 1: 解决冲突——本地优先**：保留本地渠道分支；手工补上游 `12be9975c`（错误通知统一）与 `eb76b136b`（插件图标）两处改动到 import 区与相关单元格

- [x] **Step 2: 类型检查**

```bash
cd web && bun run typecheck
cd ..
```

- [x] **Step 3: Commit**

```bash
git add web/src/features/channels/components/channels-columns.tsx
git commit -m "merge: 融合 channels-columns（本地渠道扩展 + 上游错误通知/插件图标）"
```

---

### Task 13: static-keys.ts 并集 + i18n 同步

**Files:**
- Modify: `web/src/i18n/static-keys.ts`

**Interfaces:**
- Consumes: 本地 dashboard key（Quota/Tokens/Call Trend 等 10 行）+ 上游 billing 表达式仿真 key（152 行）
- Produces: 并集（无重叠）

- [x] **Step 1: 解决 static-keys.ts 冲突**：取双方新增 key 并集

- [x] **Step 2: i18n 同步（补 zh 翻译）**

```bash
cd web && bun run i18n:sync
# 确认上游 152 个 billing key 进入 locales/{en,zh}.json，补 zh 翻译
cd ..
```

- [x] **Step 3: Commit**

```bash
git add web/src/i18n/
git commit -m "merge: 合并 i18n key 并集（本地 dashboard + 上游 billing 表达式）"
```

---

### Task 14: 重写/删除 2 个本地测试

**Files:**
- Modify/Delete: `web/src/features/models/components/dialogs/__tests__/upstream-conflict-dialog.test.tsx`、`web/src/features/models/components/dialogs/__tests__/sync-wizard-source-selection.test.tsx`

**Interfaces:**
- Consumes: 上游 sync-wizard 新结构（`STEPS`/`MetadataSyncField`）
- Produces: 有效的前端测试

- [x] **Step 1: 删除或重写 `upstream-conflict-dialog.test.tsx`**：该 dialog 已掏空为 re-export，原 UI 断言全部失效——删除，或在可行时改为针对 sync-wizard 冲突选择步骤的测试

- [x] **Step 2: 对照上游 `STEPS` 重写或删除 `sync-wizard-source-selection.test.tsx`**：若上游已覆盖该场景则删除

- [x] **Step 3: 前端测试验证**

```bash
cd web && bun test
cd ..
```

- [x] **Step 4: Commit**

```bash
git add web/src/features/models/components/dialogs/__tests__/
git commit -m "merge: 重写/删除失效的上游同步向导测试"
```

---

### Task 15: 前端全量构建验证

**Files:**（无）

**Interfaces:**
- Consumes: Task 9-14 全部前端融合
- Produces: 前端构建绿

- [x] **Step 1: 依赖安装 + 类型检查 + 构建 + 测试**

```bash
cd web && bun install && bun run typecheck && bun run build && bun test
# 预期：全绿；typecheck 第一道关卡（drawer schema、vendor 命名、类型改向）
cd ..
```

- [x] **Step 2: 确认无残留冲突标记**

```bash
grep -rn "<<<<<<<\|=======" web/src --include="*.tsx" --include="*.ts" | head
# 预期：无输出
```

- [x] **Step 3: Commit（若有遗漏）**

```bash
git commit -m "merge: 前端融合完成，typecheck/build/test 通过" 2>/dev/null || echo "无未提交改动"
```

---

### Task 16: 三数据库兼容验证

**Files:**（无）

**Interfaces:**
- Consumes: 上游 `user.AccessTokenCreatedAt` 列（AutoMigrate ADD COLUMN）、`migration_dialector.go`
- Produces: 三库验证证据（记录版本/命令/结果到 handoff）

- [x] **Step 1: 新鲜库迁移验证**（SQLite/MySQL≥5.7.8/PostgreSQL≥9.6）

```bash
# SQLite：启动服务连 SQLite 库，观察 AutoMigrate 日志确认 access_token_created_at 列创建
# MySQL/PG：用可用实例跑启动迁移
```

- [x] **Step 2: 升级库验证**：最新发布版建库 → 融合代码启动迁移 → 新列新增、旧数据完好、`access_token` 唯一索引保留

- [x] **Step 3: 幂等性**：同一升级库连续启动两次，确认无反复 `ALTER TABLE`（尤其 PG/MySQL `*bool` 列 default 处理）

- [x] **Step 4: 记录数据库版本、命令、结果**

---

### Task 17: 计费安全验证

**Files:**（无）

**Interfaces:**
- Consumes: 上游 `pkg/billingexpr` fixed per-request 定价、`BillingUnit` request-unit
- Produces: 计费不变量确认证据

- [x] **Step 1: 运行上游新增计费用例**

```bash
GOWORK=off go test ./pkg/billingexpr/...
GOWORK=off go test ./relay/... -run "Quota" 2>/dev/null
```

- [x] **Step 2: 检查 `BillingUnit` request-unit 消费方**（`service/log_info_generate.go` 的 `attachQuotaSaturation`）确认走 `*Checked` 转换

- [x] **Step 3: 确认无负扣费路径**：fixed pricing 的 `amount * 1_000_000`（amount≥0 已 bound）、task 路径隔离（`UsesFixedPricing` 阻止）

---

### Task 18: 全量回归 + guard build

**Files:**（无）

**Interfaces:**
- Consumes: Task 1-17 全部完成
- Produces: tasks.md 全勾选、build 阶段退出

- [x] **Step 1: 全量测试**

```bash
GOWORK=off go build ./... && GOWORK=off go vet ./...
(cd relaykit && GOWORK=off go build ./...)
(cd web && bun run typecheck && bun run build && bun test)
```

- [x] **Step 2: 勾选 tasks.md 全部任务 + 提交**

```bash
git add -A
git commit -m "merge: 合并上游 rc.32→rc.36 完成" 2>/dev/null || echo "无未提交改动"
```

- [x] **Step 3: 记录 build 验证证据**

```bash
comet state record-check merge-upstream-rc36 build --command "GOWORK=off go build ./... && GOWORK=off go vet ./... && (cd relaykit && GOWORK=off go build ./...) && (cd web && bun run typecheck && bun run build && bun test)" --exit-code 0
```
