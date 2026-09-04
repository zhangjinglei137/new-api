# new-api fork 全面审查报告

- **日期**：2026-09-03
- **范围**：本地自定义代码为主（约 107 个提交、207 个文件差异、+25343/−5921），上游同源代码抽样巡检；全部 GitHub Actions 工作流；sync-upstream 冲突专项
- **方法**：4 个并行深度审查 lane（后端 / 前端 / CI+sync 方案 / 上游对比与测试清单）+ 安全构建与单测验证
- **结论**：本地代码整体质量**良好**（测试覆盖扎实、符合项目规范），但存在 **4 个高危问题**、若干可优化/可提炼项，以及一个**必然失败的 sync-upstream 流程**（已给出完整改造方案）。

> ## 实施状态（2026-09-03 更新）
>
> 全部 13 项 + 低风险优化已实施并提交（dev 分支本地，未 push），验证矩阵全绿。分组提交：
> 1. `feat(ci)` sync-upstream 自动化冲突解决与 CI 加固
> 2. `fix(relay)` 定价 ratio 上界与渠道模型更新行锁
> 3. `refactor(service)` 用量查询公共 JSON helper
> 4. `fix(web)` 渠道用量弹窗公共化与 SenseNova error_code
> 5. `fix(web)` 渠道编辑抽屉脏数据确认与健壮性修复
> 6. `refactor(web)` 合并 API 与渠道图表组件
> 7. `docs` 本报告
>
> **已实施**：sync-upstream 方案（.gitattributes + merge.ours.driver + rerere 缓存/种子 + vet 关卡 + issue 兜底）、CI 全部修复、后端 H1/H2/O1/O2/O4/O6/O7/O8/R1/R3/T1、前端 H1/H2/H3/R1/R2/O1-O4/P2/P4（P2 为 CHANNEL_TYPE_DEFAULTS 配置表）、上游 7 个提交手动合并、rerere 种子入库。
>
> **未实施（有明确理由）**：
> - P1（魔法键回显后端规范化）：需后端数据迁移，L 级，留作后续建议
> - P3（doubaoAccessMode 纯派生）：`endpoint_profile` 无法表达 standard/custom 差异，纯派生会破坏火山渠道回显链路，保留现状
> - O3（sensenova 锁内网络请求）：当前实现实际安全——锁内串行化恰好防止 token stampede，仅同凭证并发阻塞至登录完成（≤30s）的性能取舍，非正确性缺陷；如需优化可后续引入 singleflight
> - O9 打包优化：诊断确认 vchart 已有效分包，大 chunk 为图标库（按需加载），主包大头是 `lobe-icon.tsx` 全量导入；构建配置保持现状
> - O5（sensenova 凭证摘要分隔符）：推送前已修复（sha256 加 `\x00` 分隔）
>
> **验证结果**：根模块+relaykit 全量 build/vet/test 通过（567 前端用例全绿）；pg 实证裸 `group` 保留字报错（O8 修复必要）；SQLite T2 三态验证通过；sync 驱动 dry-run 实证有效。**MySQL 实例验证未做**（需起容器，待授权，按 AGENTS 要求如实标注、不声称 MySQL 兼容）。
>
> **council 全面验证（2026-09-03）**：3 位 councillor（ds/glm/qw）一致裁决 **GO（可推送）**，未发现阻塞性缺陷；gpt 两次空返回标记 FAILED。非阻塞跟进项：MySQL 实例验证、fr/ja/ru/vi/zh-TW 补 68 个 i18n key、`ClassifySenseNovaUsageError` 改 sentinel error、sync-upstream 补 setup-go（已修）。

---

## 0. 验证结论（实际执行，均通过）

| 验证项 | 结果 |
|---|---|
| `go build ./...`（根模块） | ✅ 通过 |
| `cd relaykit && GOWORK=off go build ./...`（relaykit 独立构建） | ✅ 通过 |
| `GOWORK=off go vet ./...`（relaykit） | ✅ 通过 |
| `go vet ./...`（根模块全量） | ✅ 通过 |
| `go test ./service/ ./controller/ -count=1` | ✅ 全部通过（service 1.9s / controller 3.7s） |
| `bun run build`（前端生产构建） | ✅ 通过（⚠️ 见 O9 打包体积） |
| 前端 `bun run typecheck`（ora-2 执行） | ✅ 通过 |

---

## 1. 高危问题

### 1.1 后端 H1 — OpenCodeGo 定价 ratio 无上界校验，可注入荒谬定价进入计费链路
- **定位**：`service/opencodego_pricing.go:38-92`（`convertOpenCodeGoRatioData`），调用方 `controller/ratio_sync.go:350`
- **问题**：将上游 `api.json` 的 `input` 价格转 `model_ratio` 时仅拒绝 NaN/Inf/负数，**无上界**。若上游数据被篡改或字段错位（如 `input=1e10`），`model_ratio` 可达 `6e7`，单次 1M token 请求扣约 $12 万配额。对比 `model_rich.go:499` 的 `deriveOpenAIModelCost` 有 `maxDisplayCostBound=1e9` 上界，同批代码却缺对称防护。
- **建议**：对 `input` 加上界（如 `maxRatioBound=1e6`，超界跳过并 `logger.LogWarn`）；换算前先做 allFinite 校验。
- **优先级：高 ｜ 工作量：S**

### 1.2 后端 H2 — 渠道模型更新 read-modify-write 无行锁，并发可丢失更新
- **定位**：`controller/channel_upstream_update.go:966-1014`（`applyChannelUpstreamModelUpdates`）、`510-568`（`checkAndPersistChannelUpstreamModelUpdates`）
- **问题**：两处均先读 `channel.GetModels()` → 计算 `nextModels` → 再写 DB，全程无 `lockForUpdate`/乐观锁。定时巡检与管理员手动操作并发时，后提交者覆盖前者对 `models`/`settings` 的写入，上游新增模型可能丢失或状态不一致。
- **建议**：DB 读阶段对 channel 行加 `lockForUpdate(tx)`（SQLite 自动跳过），或引入 `updated_time` 乐观锁。
- **优先级：高 ｜ 工作量：M**

### 1.3 前端 H1 — SenseNova 错误分类基于硬编码中文文案匹配
- **定位**：`web/src/features/channels/components/dialogs/sensenova-usage-dialog.tsx:308-317`（`getErrorTitle`）
- **问题**：用 `lower.includes('登录失败')`、`lower.includes('账号未配置')` 等中文字面量决定错误标题。后端调整措辞或切多语后分类立即失效；其余 7 个 dialog 均已改用 `error_code` 分支，唯独它是迁移遗留。
- **建议**：后端在 `SenseNovaUsageResponse` 返回稳定 `error_code`，前端按 code 分支，与 `getUsageErrorCopy` 统一。
- **优先级：高 ｜ 工作量：M**（需后端配合）

### 1.4 前端 H2 — 渠道编辑抽屉关闭无脏数据确认，改动静默丢失
- **定位**：`web/src/features/channels/components/drawers/channel-mutate-drawer.tsx:2002-2016`（`handleOpenChange`）
- **问题**：关闭时直接 `form.reset()`，不检查 `form.formState.isDirty`。渠道配置含数十字段，误点遮罩/ESC 即全部丢失，无二次确认。
- **建议**：`handleOpenChange(false)` 时若 dirty 且非 submitting，弹 ConfirmDialog 确认后才关闭；补回归测试覆盖「有改动→关闭→取消/确认」两条路径。
- **优先级：高 ｜ 工作量：M**

### 1.5 前端 H3 — `missingModelsResolveRef` 缺少卸载清理，Promise 可永久挂起
- **定位**：`channel-mutate-drawer.tsx:1721-1742`（`confirmMissingModelMappings` / `handleMissingModelsAction`）
- **问题**：`statusCodeRiskResolveRef` 在 cleanup 有兜底 resolve，`missingModelsResolveRef` 没有。弹窗打开期间组件卸载，`onSubmit` 中的 `await confirmMissingModelMappings(...)` 永不返回，保存静默失效。
- **建议**：为 `missingModelsResolveRef` 增加与 `statusCodeRiskResolveRef` 一致的 unmount cleanup（resolve `'cancel'` 置 null），`handleOpenChange` 关闭时同样兜底。
- **优先级：高 ｜ 工作量：S**

### 1.6 上游待合并安全/计费修复（非本地代码缺陷，但当前系统实际受影响）
本地 main 尚未合并的 7 个上游提交中，以下 3 个与本地改动区域重叠、风险最高：
- **057f71c23 fix(logs): isolate privileged metadata** — 日志敏感元数据隔离（`model/log_other.go` +281、`service/log_info_generate.go` +211）。**安全相关**，本地 AGENTS.md 的 `admin_info.quota_saturation` 与之相关。
- **bbd97446c / 0ed497f06 fix(relay): billing integrity and conversion completions** — 计费完整性与换算补全，触及 `relay/common/relay_info.go`、`relaykit/dto/{billing_usage,usage_merge}.go`、`model/channel.go`，与本地改动区域直接重叠。
- **b7017c251 fix(model): no-op system task state writes treated as lock loss** — 系统任务状态写修复（`model/system_task.go`）。
- **建议**：手动将这些提交 cherry-pick/合并进本地（避开 ours 策略覆盖），优先 057f71c23。
- **优先级：高 ｜ 工作量：M**

---

## 2. 可优化项

### 2.1 后端

| # | 定位 | 问题 | 建议 | 优先级/工作量 |
|---|---|---|---|---|
| O1 | `service/radeoncloud_usage.go:241,249`（及 zhipu/moonshot/commandcode/volc 同批 `*JSONInt64`） | 裸 `int()`/`int64()` 无上界 clamp，上游 `1e20` 展示为 max int | 抽 `jsonInt64Clamped` 统一 clamp（同时解决 O1 一致性） | 中 / S |
| O2 | `service/opencodego_usage.go:87`、`commandcode_usage.go:111` | 缺 nil channel 防御，与同批 zhipu/moonshot/radeoncloud 不一致（当前调用方已守卫，属 API 不一致隐患） | 补齐 `if channel == nil` 守卫 | 低 / S |
| O3 | `service/sensenova_usage.go:207-229` | 登录网络请求在 `entry.mu` 锁内执行，同凭证并发阻塞最长 30s，401 重试可触发 token stampede | singleflight 或移网络请求出锁 | 低 / M |
| O4 | `controller/model_sync.go:316-319` | `etagCache`/`bodyCache` 全局 map 无容量上限，长期运行内存增长 | 加容量上限（如 64 条）或 LRU | 低 / S |
| O5 | `service/sensenova_usage.go:143-146` | `sha256(username+password)` 无分隔符，理论上 `("abc","def")` 与 `("ab","cdef")` 缓存键碰撞 | 改 `sha256(username+"\x00"+password)` | 低 / S |
| O6 | `service/opencodego_usage.go:120` | `strings.Contains(body, "EntitlementError")` 判订阅状态脆弱 | 优先解析 JSON error 字段，字符串匹配仅兜底 | 低 / S |
| O7 | `service/commandcode_usage.go:430-446` | `commandCodeJSONFloat` 对负数不设防，helper 层不一致（当前下游有下界保护，实际安全） | 与 `radeonJSONFloatOK` 对齐 | 低 / S |
| O8 | `controller/channel_upstream_update.go:55` | Select 字段含保留字 `group`，GORM Select 不加引号，**MySQL 下可能语法错误**（需实测） | 用结构体字段映射或 common 保留字处理 | 中 / S |

### 2.2 前端

| # | 定位 | 问题 | 建议 | 优先级/工作量 |
|---|---|---|---|---|
| O1 | `dialogs/codex-usage-dialog.tsx:28-45` | 文件内重复 AGPL license 头（夹在 import 之间） | 删除重复块 | 低 / S |
| O2 | `drawers/channel-mutate-drawer.tsx:1467-1482` | `/v1` 结尾警告依赖 setTimeout 500ms 时序，`t` 未入依赖 | 去 setTimeout，用 ref 记录已警告 URL 去重 | 中 / S |
| O3 | `dashboard/components/models/channel-charts.tsx:132,189,279` | 复制自 api-charts 时残留 `Token` 字段名（实为渠道维度） | 统一改名 `Channel`/`Value`（spec 同步） | 低 / S |
| O4 | 7 个 `__tests__/*-usage-dialog.test.tsx` | 用后端中文 message 作完整断言，后端改措辞即批量红 | 测试描述注明「透传后端原文」语义或改关键子串断言 | 低 / S |
| **O9（新增）** | `bun run build` 产物 | **打包体积大**：主 index.js 3.7MB（gzip 1.06MB），多个 async chunk 5MB+（最大 6.8MB raw / 2.7MB gzip），疑似图表库（echarts 系）未有效分包 | 检查 ECharts 按需引入、大依赖分包、路由级 code-split | 中 / M |

### 2.3 其他 CI 优化项（详见第 6 节）

---

## 3. 可提炼项

### 3.1 后端
- **R1（中 / M）**：5 套 `*_usage.go` 的 JSON 宽松解析 helper（`*JSONMap/String/Float/Int64`，仅前缀不同）重复度极高，应抽 `service/usage_json.go` 公共函数（统一 NaN/clamp，顺带解决 O1）。涉及：commandcode/zhipu/moonshot/volc_codingplan/radeoncloud。
- **R2（中 / L）**：7 个 `Fetch*Usage` 的「取凭证→代理 client→超时→构造请求→Do→状态码分类→解析」样板同构，可抽 `usageFetcher` 基类。涉及 zhipu/moonshot/radeoncloud/opencodego/commandcode/codex_wham/volc_codingplan。
- **R3（低 / S）**：`codex_wham_usage.go` 三个函数参数校验 20 行完全重复，可抽 `doCodexWhamRequest`。
- **R4（低 / S）**：sensenova/commandcode/moonshot/volc 四处 epoch→RFC3339 独立实现，可抽公共函数。

### 3.2 前端
- **R1（高 / L，回报最大）**：8 个 `*-usage-dialog.tsx` 约 80% 重复（clamp/format 工具、`UsageWindowCard`、`WINDOW_ORDER` 派生、errorCopy/showRawPanel 逻辑、Dialog 外壳+Refresh+JSON 面板、footer）。建议抽到 `dialogs/usage/` 子目录：`usage-window-card.tsx`、`raw-response-collapsible.tsx`、`usage-dialog-shell.tsx`、`use-usage-dialog-state.ts`；各 provider dialog 仅保留数据形状适配 + error_code→文案映射，从 ~350 行降到 ~60-100 行，**预计 -1500 行重复**。codex（1354 行，含 reset credits 子流程）保留独立实现但复用外壳。测试已铺好安全网，重构风险可控。
- **R2（中 / M）**：`api-charts.tsx` 与 `channel-charts.tsx` 95% 重复（仅维度字段/API/queryKey/标题/图标不同），可抽 `dashboard-charts.tsx` 泛型组件。**注意**：合并时勿丢 api-charts 的 `isAdmin` 分支（越权数据风险）。

---

## 4. 页面流程问题

| # | 定位 | 问题 | 建议 | 优先级/工作量 |
|---|---|---|---|---|
| P1 | `channel-mutate-drawer.tsx:1309-1369` | 魔法键回显用 `base_url` 字面量（`doubao-coding-plan`、`endsWith('/api/coding')` 等）反推 endpoint_profile，分支互相覆盖，`shouldDirty:false` 静默改写 base_url | 后端返回规范化 `endpoint_profile`，前端只读不反推；短期补回显单测 | 中 / L |
| P2 | `channel-mutate-drawer.tsx:1388-1454` | 渠道类型默认值在 effect 里逐 type 硬编码（含 OpenCode Go 内联大段 header_override JSON） | 抽 `CHANNEL_TYPE_DEFAULTS` 配置表 | 中 / M |
| P3 | `channel-mutate-drawer.tsx:692-694` | `doubaoAccessMode` state 与 `endpoint_profile` 表单字段双轨，回显时未必同步，保存后三者可能不一致 | 让 mode 成为 profile 的派生视图或保存前校验 | 中 / M |
| P4 | `channel-mutate-drawer.tsx:1940-1989` | section 高亮用 `document.querySelector('#id')` 依赖全局唯一 id | 限定 `formElement.querySelector` + id 加前缀 | 低 / S |

---

## 5. sync-upstream 专项

### 5.1 根因
本地对上游核心文件有大量自定义（`model/channel.go`、`relaykit/dto/channel_settings.go` 等），上游也在持续改同一批文件，每日 merge 必冲突；当前 workflow 无任何冲突策略，冲突即硬失败。

### 5.2 冲突热点与策略表（已实测 `git merge-tree`，当前待合并共 5 个文件）

| 文件 | 本地改动性质 | 冲突类型 | 建议策略 |
|---|---|---|---|
| `.gitignore` | 末尾追加忽略项 | 纯追加 | 默认三方 merge（可自动） |
| `model/channel.go` | 核心扩展（事务化删除/能力清理） | 函数区域重叠 | **`merge=ours`** |
| `relaykit/dto/channel_settings.go` | 大量扩展字段 + ResolveProxy | 结构体尾部追加重叠 | **`merge=union`**（并集保留，避免丢上游 `ToolLossPolicy` 导致编译失败） |
| `relay/channel/zhipu_4v/adaptor.go` | GetRequestURL 重构 | 不同函数 | 默认三方 merge |
| `relay/common/relay_info.go` | streamSupportedChannels 扩展 | import/结构体区 | 默认三方 merge + rerere |

### 5.3 完整改造方案（可落地，写入报告不实际应用）

**分层冲突解决**：
1. `.gitattributes` 按文件策略：`model/channel.go merge=ours`、`relaykit/dto/channel_settings.go merge=union`
2. 全局 `git merge -X ours` 兜底（仅未配置文件的冲突块取本地）
3. `git rerere` 缓存（优先级高于 `-X ours`，自动重放已人工解决的冲突）
4. 失败兜底：`go vet` 编译验证关卡 → 仍失败则 abort + 自动创建 issue（含上游提交清单与冲突文件），**不标红硬失败、不 push 坏代码**

**关键落地细节**：
- rerere 持久化：`actions/cache` 存 `.git/rr-cache`（`key: rerene-${{ github.run_id }}` + `restore-keys: rerene-` 实现 always-update）；仓库内置 `.github/rerere-cache/` 种子兜底（cache 有 7 天过期）
- 短路优化：`git merge-base --is-ancestor upstream/main HEAD` 判断上游已包含 → 跳过 merge/push（修复当前"永远不触发"的无效短路）
- 触发链：sync push main → build-image.yml 保留；**release.yml 移除 `push: branches: [main]`** 避免每天重建 3 个 OS 产物+发 Release
- 上游变更告警：`Report upstream changes` 步骤在 step-summary 列出被 ours 可能覆盖的上游提交，建议每周查看 + 每月一次人工深度同步刷新 rerere

**风险评估**：ours 丢弃上游 `ToolLossPolicy` 字段 → 用 `merge=union` 规避（union 产生重复字段时 go vet 捕获）；`ValidateToolLossPolicy` 调用丢失 → 影响小 + 告警兜底；rerere 缓存损坏 → 可删除 key 重置 + vet 关卡。

> 完整 yaml 已在审查会话产出，需实施时提供。

---

## 6. 其他 CI 问题（按严重度）

| 优先级 | 文件:行 | 问题 | 建议 |
|---|---|---|---|
| **P0** | `docker-image-branch.yml:40` | 仍引用已废弃的 `Calcium-Ion/new-api`（上游已迁 QuantumNous）→ VERSION 永远回退 v0.0.0 | 改 `QuantumNous/new-api` |
| **P1** | `release.yml:12-16` | `push: branches: [main]` 使 sync 每次 push 触发 3 OS release 构建 + 创建 Release | 只保留 tags + workflow_dispatch |
| **P1** | `ci.yml:4-5` | 只在 PR 触发，push-main 不跑 CI → sync 合并上游后未验证就进镜像 | 加 `push: branches: [main]`（注意 `repository` 表达式在非 PR 事件为空，需兜底） |
| **P1** | `build-image.yml:61-62` / `docker-build.yml:87-88` / `docker-image-branch.yml:109-110` | `type=gha` cache 无 arch scope，amd64/arm64 缓存互相污染 | 加 `scope: ${{ matrix.arch }}` |
| **P1** | `electron-build.yml:5-8` | tag 过滤 `!*-*` 排除所有含连字符 tag（含 v1.0.0-rc.24 等正常版本） | 改精确排除 `!*-alpha*`/`!*-beta*` |
| **P1** | `build-image.yml:33-42` | 查上游版本号无 token 认证，共享 IP 被限流回退 v0.0.0 | 加 `Authorization: Bearer ${{ github.token }}` |
| **P1** | `sync-upstream.yml:33-37` | 短路判断 HEAD≠upstream/main 永不成立（本地领先 107 提交） | 改 `merge-base --is-ancestor` |
| P2 | `sync-upstream.yml:27` | `git remote add upstream` 未处理 remote 已存在 | `remote remove 2>/dev/null || true` 后 add |
| P2 | 各 workflow | 无 `concurrency`/`timeout-minutes`（tag 触发多 workflow 重叠、卡死跑满 6h） | 补 concurrency + timeout |
| P2 | `docker-build.yml:140` | cosign-installer@v3 未固定 SHA | 固定 SHA |
| P2 | `release.yml` | 三 job 的 Determine Version / Fetch upstream 逻辑重复约 18 行 × 3 | 抽 composite action |
| P3 | `electron-build.yml:15-16` | macOS 构建被注释 | 删除或加说明 |

---

## 7. 测试质量评估

### 后端：优秀 ✅
- 24 个新增测试文件、~190 个 Test 函数，全部 testify `require`+`assert`，确定性表驱动、mock transport（无真实网络）、`t.Cleanup` 清理全局缓存、无 sleep/随机/凑覆盖率。
- 亮点回归测试：凭证变更触发完整登录、401 重试、14 个错误分支表驱动、真实上游响应快照 fixture。
- 小问题：
  - **T1（低/S）**：`volc_codingplan_usage.go:140` `volcRoundPercent` 无 NaN 守卫（对比同批均有），测试未覆盖。
  - **T2（中/M）**：富模型字段 `*bool` 三态仅在 SQLite memory 测试，**未在三数据库真实验证**（受服务管理规则限制，本次仅静态核对）。

### 前端：良好 ✅
- 21 个新增测试文件，断言真实行为（窗口顺序、clamp、degraded 态、error_code 分支、raw JSON 面板显隐），符合规范。
- 小问题见 2.2 O4（后端中文 message 作完整断言）。

---

## 8. i18n 缺口

- en/zh 完整（missing=0），但 **fr/ja/ru/vi/zh-TW 各缺 68 个 key**（其中约 33 条来自本次新增的 7 个用量弹窗，其余为历史遗留）。
- 后端独立问题：SenseNova 错误分类硬编码中文（见 1.3）。

---

## 9. 上游待合并风险面（详见 1.6）

7 个上游提交本地未合并；3 个高优先（日志敏感隔离、billing 完整性 ×2），均与本地改动区域重叠，建议手动合并而非依赖 ours 策略。

---

## 10. 行动优先级清单（汇总）

| 顺序 | 行动 | 类别 | 工作量 |
|---|---|---|---|
| 1 | 实施 sync-upstream 改造（B 方案完整 yaml） | sync | M |
| 2 | 修复 `docker-image-branch.yml` Calcium-Ion 旧仓库名 | CI | S |
| 3 | 后端 H1 定价 ratio 上界 | 高危 | S |
| 4 | 后端 H2 渠道更新行锁 | 高危 | M |
| 5 | 手动合并上游 057f71c23（日志敏感隔离）等 3 个安全/计费修复 | 上游 | M |
| 6 | 前端 H2 脏数据确认 + H3 ref cleanup | 高危 | M/S |
| 7 | release.yml 移除 push-main 触发 | CI | S |
| 8 | 前端 H1 SenseNova error_code 迁移 | 高危 | M |
| 9 | 前端 R1 抽公共 usage-dialog 组件 | 提炼 | L |
| 10 | 后端 R1 JSON helper 抽取（含 O1 clamp 统一） | 提炼 | M |
| 11 | 前端 R2 合并 api/channel-charts | 提炼 | M |
| 12 | O9 前端打包体积优化 | 优化 | M |
| 13 | 三数据库真实验证（T2 富字段 + O8 MySQL 保留字） | 验证 | M |

---

*本报告为只读审查交付物，未修改任何代码。实施建议经确认后另行执行。*
