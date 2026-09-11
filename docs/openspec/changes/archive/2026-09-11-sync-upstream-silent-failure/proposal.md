## Why

Sync Upstream 任务（`.github/workflows/sync-upstream.yml`）自 2026-09-07 起每日 UTC 00:00 正常触发，但实际不再把上游 QuantumNous/new-api 的 main 合并进本仓库 main：上游已发布 `v1.0.0-rc.36`（09-08），origin/main 仍停留在 09-06 的 merge（`033a180fa`），落后 upstream/main 51 个提交。更严重的是每个 run 都显示 success 且无任何失败通知——这是一次完全静默的失败。

## What Changes

- 修复 `verify` 步骤：go vet 失败时以非零码退出（`exit 1`），使 run 真正标红并触发 `failure()` 兜底。
- 让「Create issue on failure」兜底步骤恢复效力：目前 `if: failure()` 永不触发，是因为 verify 步骤用 `set +e` + if 分支吞掉了失败退出码。
- 在 verify 失败分支把冲突文件列表写入 step summary，便于人工排查（merge abort 前已生成 `/tmp/conflicted-files.txt`，当前未使用）。
- 仅修改 CI workflow 文件，不改变产品代码、不改变同步策略（本地优先 + `-X ours` 兜底 + vet 关卡）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

（无 —— 纯 CI 工具修复，无 spec 级行为变更，`.openspec.yaml` 设置 `skip_specs: true`）

## Impact

- 文件：`.github/workflows/sync-upstream.yml`（唯一改动文件）
- 影响：sync 失败时从「静默绿」变为「标红 + 自动创建 `sync-upstream` label 的 issue」，人工可及时发现并介入处理 merge 冲突；成功路径行为不变
- 无 API / 依赖 / 数据库影响
