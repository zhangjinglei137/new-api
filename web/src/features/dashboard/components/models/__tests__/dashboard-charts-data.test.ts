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

import { buildChartData, buildChartQueryKey } from '../dashboard-charts-data'

// Covers the dimension-neutral aggregation shared by the API (per-token) and
// channel charts. Channel-shaped fixtures prove the aggregation never reads
// dimension fields itself: the accessors are the only channel-specific part.
type ChannelRow = {
  channel_id?: number
  channel_name?: string
  count?: number
  quota?: number
  token_used?: number
  created_at?: number
}

function t(key: string): string {
  return key
}

function channelId(row: ChannelRow): number {
  return Number(row.channel_id) || 0
}

function channelName(row: ChannelRow): string | undefined {
  return row.channel_name
}

const queryParams = {
  start_timestamp: 1000,
  end_timestamp: 2000,
  default_time: 'hour',
}

describe('buildChartQueryKey', () => {
  it('omits the admin scope from the key when isAdmin is not configured', () => {
    expect(buildChartQueryKey('channel-charts', 'dates', queryParams)).toEqual([
      'dashboard',
      'channel-charts',
      'dates',
      queryParams,
    ])
    expect(
      buildChartQueryKey('channel-charts', 'trend', queryParams)
    ).toEqual(['dashboard', 'channel-charts', 'trend', queryParams])
  })

  it('includes the admin scope in the key when isAdmin is configured', () => {
    expect(buildChartQueryKey('api-charts', 'dates', queryParams, true)).toEqual(
      ['dashboard', 'api-charts', 'dates', queryParams, true]
    )
    expect(
      buildChartQueryKey('api-charts', 'trend', queryParams, false)
    ).toEqual(['dashboard', 'api-charts', 'trend', queryParams, false])
  })
})

describe('buildChartData', () => {
  it('aggregates repeated dimension ids into a single ranked entry', () => {
    const rows: ChannelRow[] = [
      { channel_id: 1, channel_name: 'Alpha', count: 1, quota: 100 },
      { channel_id: 1, channel_name: 'Alpha', count: 2, quota: 200 },
      { channel_id: 2, channel_name: 'Beta', count: 5, quota: 50 },
    ]
    const result = buildChartData({
      rows,
      trendRows: [],
      metric: 'count',
      timeGranularity: 'day',
      t,
      deletedItemLabel: (id) => `Deleted #${id}`,
      unknownItemKey: 'Unknown Channel',
      getId: channelId,
      getName: channelName,
      getTrendId: channelId,
      getTrendName: channelName,
    })

    const rankValues = result.spec_rank.data[0].values as Array<{
      Series: string
      Value: number
    }>
    expect(rankValues).toEqual([
      { Series: 'Beta', Value: 5 },
      { Series: 'Alpha', Value: 3 },
    ])
    expect(result.spec_pie.data[0].values).toEqual([
      { type: 'Beta', value: 5 },
      { type: 'Alpha', value: 3 },
    ])
    expect(result.totalDisplay).toBeTruthy()
  })

  it('merges entries beyond the ranking cap into an Other bucket', () => {
    const rows: ChannelRow[] = Array.from({ length: 25 }, (_, i) => ({
      channel_id: i + 1,
      channel_name: `Ch-${i + 1}`,
      count: i + 1,
    }))
    const result = buildChartData({
      rows,
      trendRows: [],
      metric: 'count',
      timeGranularity: 'day',
      t,
      deletedItemLabel: (id) => `Deleted #${id}`,
      unknownItemKey: 'Unknown Channel',
      getId: channelId,
      getName: channelName,
      getTrendId: channelId,
      getTrendName: channelName,
    })

    const rankValues = result.spec_rank.data[0].values as Array<{
      Series: string
      Value: number
    }>
    expect(rankValues).toHaveLength(21)
    expect(rankValues[0].Series).toBe('Ch-25')
    expect(rankValues[20].Series).toBe('Other')
    // The 20 capped entries are Ch-25..Ch-6; the smallest five (1..5) merge
    // into Other.
    expect(rankValues[20].Value).toBe(15)
  })

  it('labels rows without a name via the deleted or unknown label', () => {
    const rows: ChannelRow[] = [
      { channel_id: 7, count: 1 },
      { channel_id: 0, count: 1 },
    ]
    const result = buildChartData({
      rows,
      trendRows: [],
      metric: 'quota',
      timeGranularity: 'day',
      t,
      deletedItemLabel: (id) => `Deleted #${id}`,
      unknownItemKey: 'Unknown Channel',
      getId: channelId,
      getName: channelName,
      getTrendId: channelId,
      getTrendName: channelName,
    })

    const rankValues = result.spec_rank.data[0].values as Array<{
      Series: string
      Value: number
    }>
    expect(rankValues.find((item) => item.Series === 'Deleted #7')).toBeTruthy()
    expect(
      rankValues.find((item) => item.Series === 'Unknown Channel')
    ).toBeTruthy()
  })

  it('builds a trend series per time bucket and keeps the top series', () => {
    // Two distinct days so the trend aggregation sees separate time buckets.
    const dayMs = 24 * 60 * 60 * 1000
    const rows: ChannelRow[] = [
      { channel_id: 1, channel_name: 'Ch-1', created_at: 1000, count: 1 },
      {
        channel_id: 1,
        channel_name: 'Ch-1',
        created_at: 1000 + dayMs,
        count: 2,
      },
      { channel_id: 2, channel_name: 'Ch-2', created_at: 1000, count: 3 },
    ]
    const result = buildChartData({
      rows,
      trendRows: rows,
      metric: 'count',
      timeGranularity: 'day',
      t,
      deletedItemLabel: (id) => `Deleted #${id}`,
      unknownItemKey: 'Unknown Channel',
      getId: channelId,
      getName: channelName,
      getTrendId: channelId,
      getTrendName: channelName,
    })

    const trendValues = result.spec_trend.data[0].values as Array<{
      Time: string
      Series: string
      Value: number
    }>
    // Two series across two time buckets, with missing buckets filled as 0.
    expect(trendValues).toHaveLength(4)
    expect(new Set(trendValues.map((item) => item.Time))).toHaveLength(2)
    const seriesTotal = (series: string) =>
      trendValues
        .filter((item) => item.Series === series)
        .reduce((sum, item) => sum + item.Value, 0)
    expect(seriesTotal('Ch-1')).toBe(3)
    expect(seriesTotal('Ch-2')).toBe(3)
  })

  it('shows a no-data subtext when there are no rows', () => {
    const result = buildChartData({
      rows: [],
      trendRows: [],
      metric: 'quota',
      timeGranularity: 'day',
      t,
      deletedItemLabel: (id) => `Deleted #${id}`,
      unknownItemKey: 'Unknown Channel',
      getId: channelId,
      getName: channelName,
      getTrendId: channelId,
      getTrendName: channelName,
    })

    expect(result.spec_pie.title?.subtext).toBe('No data available')
    expect(result.spec_rank.title?.subtext).toBe('No data available')
  })
})