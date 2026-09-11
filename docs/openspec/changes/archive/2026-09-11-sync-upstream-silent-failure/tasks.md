# Tasks：修复 Sync Upstream 静默失败

## 任务清单

- [x] 修改 `.github/workflows/sync-upstream.yml`：`Verify build (fail-safe gate)` 步骤在 vet 失败分支末尾 `exit 1`，使 run 真正标红并触发 `failure()` 兜底
- [x] 修改 `.github/workflows/sync-upstream.yml`：verify 失败分支把 `/tmp/conflicted-files.txt`（存在时）写入 step summary
- [x] 校验 workflow YAML 语法（yaml.v3 解析通过）并逐分支核对逻辑（3 场景：vet 失败 exit 1 / relaykit 失败 exit 1 / 通过 exit 0）
- [x] 本地复现：模拟 merge 冲突 → `-X ours` → vet 失败（`common/crypto.go undefined: strings`、`model/pricing.go undefined: resolveModelMetadata`），确认失败路径与修复后预期一致

## 根因消除检查（build guard 前）

- [x] 静默失败机制已消除：verify 失败分支 `exit 1`（原退出码 0 被吞）
- [x] 另一条静默路径已消除：merge 完全失败分支 `exit 1`（原 `git merge --abort || true` 收尾退出码 0）
- [x] issue 兜底 `if: failure()` 恢复效力（run 中现在有真实失败步骤）
- [x] 冲突文件列表写入 step summary，人工可定位需处理的 merge 范围
