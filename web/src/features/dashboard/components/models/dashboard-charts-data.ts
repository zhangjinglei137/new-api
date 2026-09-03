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
import {
  formatChartMetricValue,
  getDashboardChartColors,
} from '@/features/dashboard/lib/charts'
import type { ChartMetric } from '@/features/dashboard/types'
import { formatChartTime, type TimeGranularity } from '@/lib/time'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type ChartSpec = Record<string, any>

const MAX_RANK_ITEMS = 20
const MAX_TREND_ITEMS = 20

type TFunction = (key: string) => string

// Shared slice of quota rows (per-token and per-channel) so the aggregation
// below stays dimension-agnostic; dimension access goes through getId/getName.
export interface DashboardQuotaRow {
  count?: number
  quota?: number
  token_used?: number
  created_at?: number
}

export interface DashboardChartQueryParams {
  start_timestamp: number
  end_timestamp: number
  default_time: string
  username?: string
}

function metricValueOf(
  row: DashboardQuotaRow,
  metric: ChartMetric
): number {
  if (metric === 'count') return Number(row.count) || 0
  if (metric === 'tokens') return Number(row.token_used) || 0
  return Number(row.quota) || 0
}

interface ChartData {
  spec_pie: ChartSpec
  spec_rank: ChartSpec
  spec_trend: ChartSpec
  totalDisplay: string
}

// Builds the react-query key for a chart query. When an admin scope is
// provided (only the token/API charts use one) it becomes part of the key, so
// the admin and non-admin views never share cached quota data.
export function buildChartQueryKey(
  queryKey: string,
  segment: 'dates' | 'trend',
  queryParams: DashboardChartQueryParams,
  isAdmin?: boolean
): unknown[] {
  if (isAdmin === undefined) {
    return ['dashboard', queryKey, segment, queryParams]
  }
  return ['dashboard', queryKey, segment, queryParams, isAdmin]
}

export function buildChartData<
  TItem extends DashboardQuotaRow,
  TTrend extends DashboardQuotaRow,
>(args: {
  rows: TItem[]
  trendRows: TTrend[]
  metric: ChartMetric
  timeGranularity: TimeGranularity
  t: TFunction
  deletedItemLabel: (id: number) => string
  unknownItemKey: string
  getId: (row: TItem) => number
  getName: (row: TItem) => string | undefined
  getTrendId: (row: TTrend) => number
  getTrendName: (row: TTrend) => string | undefined
  chartCornerRadius?: number
}): ChartData {
  const otherLabel = args.t('Other')
  const pieTitle = args.t('Call Distribution')
  const rankTitle = args.t('Call Ranking')
  const trendTitle = args.t('Call Trend')
  const formatValue = (value: number) =>
    formatChartMetricValue(value, args.metric)
  const formatTotal = (value: number) =>
    formatChartMetricValue(value, args.metric, 2)
  const itemLabel = (
    id: number,
    name: string | undefined
  ): string => {
    if (name) return name
    return id > 0 ? args.deletedItemLabel(id) : args.t(args.unknownItemKey)
  }

  // Distribution (pie) and ranking (bar) share the per-item aggregation.
  const totals = new Map<number, { name: string; value: number }>()
  args.rows.forEach((row) => {
    const id = args.getId(row)
    const prev = totals.get(id)
    if (prev) {
      prev.value += metricValueOf(row, args.metric)
      return
    }
    totals.set(id, {
      name: itemLabel(id, args.getName(row)),
      value: metricValueOf(row, args.metric),
    })
  })

  const entries = [...totals.values()].sort((a, b) => b.value - a.value)
  const totalValue = entries.reduce((sum, entry) => sum + entry.value, 0)
  const empty = entries.length === 0
  const subtext = empty ? args.t('No data available') : undefined

  const pieValues = entries.map((entry) => ({
    type: entry.name,
    value: entry.value,
  }))

  // The series field name is dimension-neutral ("Series") so the shared spec
  // never assumes the rows are tokens or channels.
  let rankValues: Array<{ Series: string; Value: number }>
  if (entries.length > MAX_RANK_ITEMS) {
    const top = entries
      .slice(0, MAX_RANK_ITEMS)
      .map((entry) => ({ Series: entry.name, Value: entry.value }))
    const otherValue = entries
      .slice(MAX_RANK_ITEMS)
      .reduce((sum, entry) => sum + entry.value, 0)
    rankValues = [...top, { Series: otherLabel, Value: otherValue }]
  } else {
    rankValues = entries.map((entry) => ({
      Series: entry.name,
      Value: entry.value,
    }))
  }

  const pieColorDomain = entries.map((entry) => entry.name)
  const pieColor = {
    type: 'ordinal',
    domain: pieColorDomain,
    range: getDashboardChartColors(pieColorDomain.length),
  }

  const rankColorDomain = [...new Set(rankValues.map((v) => v.Series))]
  const rankColor = {
    type: 'ordinal',
    domain: rankColorDomain,
    range: getDashboardChartColors(rankColorDomain.length),
  }

  // Trend (area): aggregate by (time bucket, item) with a Map, then keep the
  // top items by total and merge the rest into "Other".
  const timeItemMap = new Map<string, Map<string, number>>()
  const itemTotals = new Map<string, number>()
  args.trendRows.forEach((row) => {
    const timeKey = formatChartTime(Number(row.created_at), args.timeGranularity)
    const id = args.getTrendId(row)
    const name: string | undefined = args.getTrendName(row)
    const itemKey = itemLabel(id, name)
    let itemMap = timeItemMap.get(timeKey)
    if (!itemMap) {
      itemMap = new Map()
      timeItemMap.set(timeKey, itemMap)
    }
    const value = metricValueOf(row, args.metric)
    itemMap.set(itemKey, (itemMap.get(itemKey) || 0) + value)
    itemTotals.set(itemKey, (itemTotals.get(itemKey) || 0) + value)
  })

  const rankedTrendItems = [...itemTotals.entries()].sort(
    (a, b) => b[1] - a[1]
  )
  const topTrendItems = rankedTrendItems
    .slice(0, MAX_TREND_ITEMS)
    .map(([name]) => name)
  const topTrendItemSet = new Set(topTrendItems)
  const hasOtherItems = rankedTrendItems.length > MAX_TREND_ITEMS
  const sortedTimes = [...timeItemMap.keys()].sort()

  const trendValues: Array<{ Time: string; Series: string; Value: number }> =
    []
  sortedTimes.forEach((time) => {
    const itemMap = timeItemMap.get(time) ?? new Map()
    topTrendItems.forEach((name) => {
      trendValues.push({
        Time: time,
        Series: name,
        Value: itemMap.get(name) || 0,
      })
    })
    if (hasOtherItems) {
      let otherValue = 0
      itemMap.forEach((value, name) => {
        if (!topTrendItemSet.has(name)) otherValue += value
      })
      trendValues.push({ Time: time, Series: otherLabel, Value: otherValue })
    }
  })

  const trendColorDomain = [...topTrendItems, otherLabel]
  const trendColor = {
    type: 'ordinal',
    domain: trendColorDomain,
    range: getDashboardChartColors(trendColorDomain.length),
  }

  return {
    spec_pie: {
      type: 'pie',
      data: [{ id: 'id0', values: pieValues }],
      outerRadius: 0.8,
      innerRadius: 0.5,
      padAngle: 0.6,
      valueField: 'value',
      categoryField: 'type',
      pie: {
        style:
          args.chartCornerRadius == null
            ? {}
            : { cornerRadius: args.chartCornerRadius },
        state: {
          hover: { outerRadius: 0.85, stroke: '#000', lineWidth: 1 },
          selected: { outerRadius: 0.85, stroke: '#000', lineWidth: 1 },
        },
      },
      title: { visible: true, text: pieTitle, subtext },
      legends: { visible: true, orient: 'left' },
      label: { visible: true },
      color: pieColor,
      tooltip: {
        mark: {
          content: [
            {
              key: (datum: Record<string, unknown>) => datum?.type,
              value: (datum: Record<string, unknown>) =>
                formatValue(Number(datum?.value) || 0),
            },
          ],
        },
      },
      background: { fill: 'transparent' },
      animation: true,
    },
    spec_rank: {
      type: 'bar',
      data: [{ id: 'rankData', values: rankValues }],
      xField: 'Series',
      yField: 'Value',
      seriesField: 'Series',
      legends: { visible: false },
      color: rankColor,
      title: { visible: true, text: rankTitle, subtext },
      bar: {
        state: { hover: { stroke: '#000', lineWidth: 1 } },
      },
      tooltip: {
        mark: {
          content: [
            {
              key: (datum: Record<string, unknown>) => datum?.Series,
              value: (datum: Record<string, unknown>) =>
                formatValue(Number(datum?.Value) || 0),
            },
          ],
        },
      },
      background: { fill: 'transparent' },
      animation: true,
    },
    spec_trend: {
      type: 'area',
      data: [{ id: 'trendData', values: trendValues }],
      xField: 'Time',
      yField: 'Value',
      seriesField: 'Series',
      stack: false,
      legends: { visible: true, selectMode: 'single' },
      color: trendColor,
      title: { visible: true, text: trendTitle, subtext },
      tooltip: {
        mark: {
          content: [
            {
              key: (datum: Record<string, unknown>) => datum?.Series,
              value: (datum: Record<string, unknown>) =>
                formatValue(Number(datum?.Value) || 0),
            },
          ],
        },
        dimension: {
          content: [
            {
              key: (datum: Record<string, unknown>) => datum?.Series,
              value: (datum: Record<string, unknown>) =>
                Number(datum?.Value) || 0,
            },
          ],
          updateContent: (
            array: Array<{
              key: string
              value: string | number
            }>
          ) => {
            array.sort(
              (a, b) => (Number(b.value) || 0) - (Number(a.value) || 0)
            )
            let sum = 0
            for (let i = 0; i < array.length; i++) {
              const v = Number(array[i].value) || 0
              sum += v
              array[i].value = formatValue(v)
            }
            array.unshift({
              key: args.t('Total:'),
              value: formatValue(sum),
            })
            return array
          },
        },
      },
      area: {
        style: {
          fillOpacity: 0.08,
          curveType: 'monotone',
        },
      },
      line: {
        style: {
          lineWidth: 2,
          curveType: 'monotone',
        },
      },
      point: { visible: false },
      background: { fill: 'transparent' },
      animation: true,
    },
    totalDisplay: formatTotal(totalValue),
  }
}