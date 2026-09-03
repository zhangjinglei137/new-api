/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export type VersionCompareResult = -1 | 0 | 1

type ParsedVersion = {
  major: number
  minor: number
  patch: number
  // 预发布类型（alpha/beta/rc），空字符串表示正式版本
  pre: string
  // 预发布序号；正式版本视为 +Infinity（比任何预发布都要新）
  preNum: number
  // 构建时间戳（8-14 位数字）；无时间戳视为 0
  ts: number
}

const PRE_RELEASE_RANK: Record<string, number> = {
  alpha: 0,
  beta: 1,
  rc: 2,
  '': 3,
}

/**
 * 解析形如 v1.0.0-rc.31-202609032205 / v1.0.0-rc.24-20260817-1010 / v1.0.0 的版本号。
 * 时间戳支持两种形态：
 *   - 12 位无分隔串（202609032205）
 *   - 「8 位日期-4 位时间」合成串（20260817-1010）
 * 解析失败返回 null。
 */
function parseVersion(value: string): ParsedVersion | null {
  const match =
    /^v?(\d+)\.(\d+)\.(\d+)(?:-(rc|alpha|beta)\.(\d+))?(?:[-.](\d{8})[-.](\d{4})|[-.](\d{12}))?$/i.exec(
      value.trim()
    )
  if (!match) return null
  const [, major, minor, patch, pre, preNum, ts8, ts4, ts12] = match
  let ts = 0
  if (ts8) {
    ts = Number(ts8 + ts4)
  } else if (ts12) {
    ts = Number(ts12)
  }
  return {
    major: Number(major),
    minor: Number(minor),
    patch: Number(patch),
    pre: pre ? pre.toLowerCase() : '',
    preNum: pre ? Number(preNum) : Infinity,
    ts,
  }
}

/**
 * 比较两个版本号：a > b 返回 1，a === b 返回 0，a < b 返回 -1。
 * 任一版本号无法解析时返回 null（由调用方决定兜底策略）。
 *
 * 比较顺序：major → minor → patch → 预发布类型（alpha < beta < rc < 正式）→
 * 预发布序号 → 构建时间戳。在 rc 序号相同时按时间戳区分（如
 * rc.31-20260903 与 rc.31-20260902），确保晚构建的镜像不被误判为旧版。
 */
export function compareVersions(a: string, b: string): VersionCompareResult | null {
  const pa = parseVersion(a)
  const pb = parseVersion(b)
  if (!pa || !pb) return null

  for (const key of ['major', 'minor', 'patch'] as const) {
    if (pa[key] !== pb[key]) return pa[key] > pb[key] ? 1 : -1
  }
  const rankA = PRE_RELEASE_RANK[pa.pre]
  const rankB = PRE_RELEASE_RANK[pb.pre]
  if (rankA !== rankB) return rankA > rankB ? 1 : -1
  if (pa.pre !== pb.pre) return pa.pre > pb.pre ? 1 : -1
  if (pa.preNum !== pb.preNum) return pa.preNum > pb.preNum ? 1 : -1
  if (pa.ts !== pb.ts) return pa.ts > pb.ts ? 1 : -1
  return 0
}