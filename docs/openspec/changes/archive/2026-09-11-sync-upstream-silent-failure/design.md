# Design：修复 Sync Upstream 静默失败

## 根因回顾

失败链条：

1. 上游对多个文件大幅改动（`common/crypto.go` 重写、`model/model_meta.go` / `model/pricing.go` 新增函数、前端 12 个文件等），与本地扩展形成 **17 个文件冲突**，默认三方 merge 失败。
2. 兜底 `-X ours` merge 成功，但产物**编译不过**：
   - `common/crypto.go`：上游在 `ValidatePasswordAndHash` 新增 argon2 分支（使用 `strings.HasPrefix`）；import 区冲突取本地侧（无 `strings`）→ `undefined: strings`。
   - `model/pricing.go`：上游新增对 `resolveModelMetadata` 的调用；该函数位于 `model/model_meta.go`，冲突取 ours 后丢失 → `undefined: resolveModelMetadata`。
3. `Verify build (fail-safe gate)` 步骤用 `set +e` 收集 vet 退出码后走 if 分支：失败分支只写 `verify=failed` 并 `git merge --abort`，**步骤整体退出码仍为 0** → GitHub 判定 run success。
4. 失败兜底 `Create issue on failure` 的条件是 `if: failure() && steps.check.outputs.skipped == 'false'`；run 中无任何 step 报错，`failure()` 恒为 false → **issue 永不创建**。

净效果：每天显示绿勾，但 merge 被 abort、不 push、无通知，上游版本静默落后。

## 方案：让失败真正可见（单一方案，最小改动）

只改 `.github/workflows/sync-upstream.yml`，不改产品代码、不改同步策略。

### 改动点

**1. verify 步骤失败分支末尾退出非零**

```yaml
if [ $VET_ROOT -ne 0 ] || [ $VET_RELAYKIT -ne 0 ]; then
  echo "verify=failed" >> "$GITHUB_OUTPUT"
  # ... 现有 step summary 输出 ...
  git merge --abort 2>/dev/null || true
  exit 1   # 新增：让 run 真正失败
fi
```

效果：vet 失败 → 步骤退出码 1 → run 标红 → `failure()` 为 true → 失败兜底 issue 创建生效；`Push merge`（条件 `verify == 'passed'`）保持跳过。

**2. verify 失败分支记录冲突文件列表**

merge 步骤在冲突失败时已生成 `/tmp/conflicted-files.txt`，verify 失败分支追加输出到 step summary：

```yaml
echo "### Conflicted files" >> "$GITHUB_STEP_SUMMARY"
echo '```' >> "$GITHUB_STEP_SUMMARY"
cat /tmp/conflicted-files.txt >> "$GITHUB_STEP_SUMMARY" 2>/dev/null || echo "unknown" >> "$GITHUB_STEP_SUMMARY"
echo '```' >> "$GITHUB_STEP_SUMMARY"
```

便于人工判断是哪些文件需要处理。

### 不做的改动（明确排除）

- **不做自动修复源码**：`-X ours` 造成的编译错误（缺 import、缺函数）无法通用自动修复；正确行为是明确失败并人工介入 merge。自动修补源码会引入不可控风险。
- **不改 `-X ours` 策略**：本地优先策略是设计决策，vet 关卡已能拦截坏结果，本次只修复「拦截后静默」的问题。
- **不改 `Create issue on failure` 的条件**：修复 verify 退出码后 `failure()` 自然生效，条件本身正确。

### 验证方式

- YAML 语法校验（actionlint 或 python yaml）。
- 逻辑推演：verify 失败 → exit 1 → run failure → issue 兜底触发（分支条件逐一核对）。
- 真实触发验证：推送后手动 `workflow_dispatch` 一次，观察 run 标红 + issue 创建（需在归档阶段确认后执行）。
