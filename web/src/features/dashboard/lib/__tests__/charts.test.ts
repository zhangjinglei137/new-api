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
import { describe, expect, test } from 'vitest'

import type { QuotaDataItem } from '../../types'
import { processChartData } from '../charts'

const rows: QuotaDataItem[] = [
  {
    model_name: 'gpt-4.1',
    created_at: 1700000000,
    quota: 1000,
    token_used: 500,
    count: 3,
  },
  {
    model_name: 'claude-4-sonnet',
    created_at: 1700000000,
    quota: 500,
    token_used: 200,
    count: 1,
  },
]

type PieDatum = { type: string; value: number }
type RankDatum = { Model: string; Value: number }
type LineDatum = { Model: string; rawValue: number; Usage: number }

describe('processChartData metric switching', () => {
  test('quota mode aggregates pie/rank/trend by quota and titles reflect amount', () => {
    const result = processChartData(rows, 'day')

    const pie = new Map(
      (result.spec_pie.data[0].values as PieDatum[]).map((v) => [
        v.type,
        v.value,
      ])
    )
    expect(pie.get('gpt-4.1')).toBe(1000)
    expect(pie.get('claude-4-sonnet')).toBe(500)

    const rank = (result.spec_rank_bar.data[0].values as RankDatum[]).find(
      (v) => v.Model === 'gpt-4.1'
    )
    expect(rank?.Value).toBe(1000)

    expect(result.spec_pie.title.text).toBe('Quota Distribution')
    expect(result.spec_rank_bar.title.text).toBe('Quota Ranking')
    expect(result.spec_model_line.title.text).toBe('Quota Trend')
  })

  test('tokens mode aggregates pie/rank/trend by token_used and titles reflect tokens', () => {
    const result = processChartData(rows, 'day', undefined, undefined, 'tokens')

    const pie = new Map(
      (result.spec_pie.data[0].values as PieDatum[]).map((v) => [
        v.type,
        v.value,
      ])
    )
    expect(pie.get('gpt-4.1')).toBe(500)
    expect(pie.get('claude-4-sonnet')).toBe(200)

    const rank = (result.spec_rank_bar.data[0].values as RankDatum[]).find(
      (v) => v.Model === 'gpt-4.1'
    )
    expect(rank?.Value).toBe(500)

    expect(result.spec_pie.title.text).toBe('Token Distribution')
    expect(result.spec_rank_bar.title.text).toBe('Token Ranking')
    expect(result.spec_model_line.title.text).toBe('Token Trend')
  })

  test('consumption distribution Usage tracks the active metric', () => {
    const quotaResult = processChartData(rows, 'day', undefined, undefined, 'quota')
    const tokenResult = processChartData(rows, 'day', undefined, undefined, 'tokens')

    const sumByModel = (values: LineDatum[]) => {
      const map = new Map<string, { rawValue: number; Usage: number }>()
      values.forEach((v) => {
        const prev = map.get(v.Model) ?? { rawValue: 0, Usage: 0 }
        map.set(v.Model, {
          rawValue: prev.rawValue + v.rawValue,
          Usage: prev.Usage + v.Usage,
        })
      })
      return map
    }

    const quotaTotals = sumByModel(
      quotaResult.spec_line.data[0].values as LineDatum[]
    )
    const tokenTotals = sumByModel(
      tokenResult.spec_line.data[0].values as LineDatum[]
    )

    // Raw metric value differs by mode.
    expect(quotaTotals.get('gpt-4.1')?.rawValue).toBe(1000)
    expect(tokenTotals.get('gpt-4.1')?.rawValue).toBe(500)
    // Tokens mode keeps Usage as the raw count; quota mode converts to USD.
    expect(tokenTotals.get('gpt-4.1')?.Usage).toBe(500)
    expect((quotaTotals.get('gpt-4.1')?.Usage ?? 0) > 0).toBeTruthy()
    expect((quotaTotals.get('gpt-4.1')?.Usage ?? 0) < 1000).toBeTruthy()
  })

  test('empty data still produces metric-specific specs', () => {
    const result = processChartData([], 'day', undefined, undefined, 'tokens')
    expect(result.spec_pie.title.text).toBe('Token Distribution')
    expect(result.spec_rank_bar.title.text).toBe('Token Ranking')
    expect(result.spec_pie.data[0].values).toStrictEqual([])
  })
})
