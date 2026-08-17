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

import { formatCompactMetric } from '@/lib/format'

describe('formatCompactMetric', () => {
  it('keeps values whose formatted string is shorter than 9 characters unchanged', () => {
    expect(formatCompactMetric(0)).toBe('0')
    expect(formatCompactMetric(999)).toBe('999')
    expect(formatCompactMetric(1234)).toBe('1,234')
    expect(formatCompactMetric(999999)).toBe('999,999')
  })

  it('abbreviates 9+ character values with 1000-based suffixes', () => {
    expect(formatCompactMetric(1000000)).toBe('1M')
    expect(formatCompactMetric(1234567)).toBe('1.2M')
    expect(formatCompactMetric(1234567890)).toBe('1.2G')
    expect(formatCompactMetric(123456789012)).toBe('123.5G')
  })

  it('keeps at most one decimal and strips trailing .0', () => {
    expect(formatCompactMetric(12000000)).toBe('12M')
    expect(formatCompactMetric(12345678)).toBe('12.3M')
    expect(formatCompactMetric(999000000)).toBe('999M')
    expect(formatCompactMetric(1200000000)).toBe('1.2G')
  })

  it('handles null, NaN and negative values', () => {
    expect(formatCompactMetric(null)).toBe('-')
    expect(formatCompactMetric(undefined)).toBe('-')
    expect(formatCompactMetric(Number.NaN)).toBe('-')
    expect(formatCompactMetric(-10000000)).toBe('-10M')
  })
})
