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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

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
    assert.equal(pie.get('gpt-4.1'), 1000)
    assert.equal(pie.get('claude-4-sonnet'), 500)

    const rank = (result.spec_rank_bar.data[0].values as RankDatum[]).find(
      (v) => v.Model === 'gpt-4.1'
    )
    assert.equal(rank?.Value, 1000)

    assert.equal(result.spec_pie.title.text, 'Quota Distribution')
    assert.equal(result.spec_rank_bar.title.text, 'Quota Ranking')
    assert.equal(result.spec_model_line.title.text, 'Quota Trend')
  })

  test('tokens mode aggregates pie/rank/trend by token_used and titles reflect tokens', () => {
    const result = processChartData(rows, 'day', undefined, undefined, 'tokens')

    const pie = new Map(
      (result.spec_pie.data[0].values as PieDatum[]).map((v) => [
        v.type,
        v.value,
      ])
    )
    assert.equal(pie.get('gpt-4.1'), 500)
    assert.equal(pie.get('claude-4-sonnet'), 200)

    const rank = (result.spec_rank_bar.data[0].values as RankDatum[]).find(
      (v) => v.Model === 'gpt-4.1'
    )
    assert.equal(rank?.Value, 500)

    assert.equal(result.spec_pie.title.text, 'Token Distribution')
    assert.equal(result.spec_rank_bar.title.text, 'Token Ranking')
    assert.equal(result.spec_model_line.title.text, 'Token Trend')
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
    assert.equal(quotaTotals.get('gpt-4.1')?.rawValue, 1000)
    assert.equal(tokenTotals.get('gpt-4.1')?.rawValue, 500)
    // Tokens mode keeps Usage as the raw count; quota mode converts to USD.
    assert.equal(tokenTotals.get('gpt-4.1')?.Usage, 500)
    assert.ok((quotaTotals.get('gpt-4.1')?.Usage ?? 0) > 0)
    assert.ok((quotaTotals.get('gpt-4.1')?.Usage ?? 0) < 1000)
  })

  test('empty data still produces metric-specific specs', () => {
    const result = processChartData([], 'day', undefined, undefined, 'tokens')
    assert.equal(result.spec_pie.title.text, 'Token Distribution')
    assert.equal(result.spec_rank_bar.title.text, 'Token Ranking')
    assert.deepEqual(result.spec_pie.data[0].values, [])
  })
})
