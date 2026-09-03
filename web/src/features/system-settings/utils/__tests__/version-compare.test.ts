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
import { describe, expect, it } from 'vitest'

import { compareVersions } from '../version-compare'

describe('compareVersions', () => {
  it('rc 序号更大视为新版本', () => {
    expect(compareVersions('v1.0.0-rc.31-202609032205', 'v1.0.0-rc.30-202609021952')).toBe(1)
    expect(compareVersions('v1.0.0-rc.30-202609021952', 'v1.0.0-rc.31-202609032205')).toBe(-1)
  })

  it('rc 序号相同时构建时间戳更新视为新版本', () => {
    expect(compareVersions('v1.0.0-rc.31-202609032205', 'v1.0.0-rc.31-202609021952')).toBe(1)
    expect(compareVersions('v1.0.0-rc.31-202609021952', 'v1.0.0-rc.31-202609032205')).toBe(-1)
  })

  it('完全相同的版本返回 0', () => {
    expect(compareVersions('v1.0.0-rc.31-202609032205', 'v1.0.0-rc.31-202609032205')).toBe(0)
  })

  it('major 版本差异优先于 rc 序号', () => {
    expect(compareVersions('v2.0.0-rc.1-202609032205', 'v1.0.0-rc.99-202609032205')).toBe(1)
    expect(compareVersions('v0.9.0-rc.99-202609032205', 'v1.0.0-rc.1-202609021952')).toBe(-1)
  })

  it('minor/patch 版本按序比较', () => {
    expect(compareVersions('v1.1.0', 'v1.0.9')).toBe(1)
    expect(compareVersions('v1.0.10', 'v1.0.9')).toBe(1)
  })

  it('正式版本视为比任何预发布新', () => {
    expect(compareVersions('v1.0.0', 'v1.0.0-rc.31')).toBe(1)
    expect(compareVersions('v1.0.0-rc.31', 'v1.0.0')).toBe(-1)
  })

  it('预发布类型 alpha < beta < rc', () => {
    expect(compareVersions('v1.0.0-rc.1', 'v1.0.0-beta.9')).toBe(1)
    expect(compareVersions('v1.0.0-beta.1', 'v1.0.0-alpha.9')).toBe(1)
  })

  it('兼容带连字符分隔的旧式时间戳 (v1.0.0-rc.24-20260817-1010)', () => {
    expect(compareVersions('v1.0.0-rc.24-20260817-1010', 'v1.0.0-rc.24-20260816-1010')).toBe(1)
    expect(compareVersions('v1.0.0-rc.25-20260817-1010', 'v1.0.0-rc.24-20260817-1010')).toBe(1)
  })

  it('无法解析的版本号返回 null', () => {
    expect(compareVersions('not-a-version', 'v1.0.0')).toBeNull()
    expect(compareVersions('v1.0.0', 'unknown')).toBeNull()
    expect(compareVersions('abc', 'def')).toBeNull()
  })

  it('版本号可省略 v 前缀', () => {
    expect(compareVersions('1.0.0-rc.31', 'v1.0.0-rc.30')).toBe(1)
  })
})