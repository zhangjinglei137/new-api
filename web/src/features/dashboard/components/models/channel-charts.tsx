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
import { useQuery } from '@tanstack/react-query'
import { VChart } from '@visactor/react-vchart'
import { Network } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import {
  getChannelQuotaDates,
  getChannelQuotaTrendDates,
} from '@/features/dashboard/api'
import {
  DEFAULT_TIME_GRANULARITY,
  MODEL_ANALYTICS_CHART_OPTIONS,
} from '@/features/dashboard/constants'
import { buildQueryParams, getDefaultDays } from '@/features/dashboard/lib'
import {
  formatChartMetricValue,
  getDashboardChartColors,
} from '@/features/dashboard/lib/charts'
import type {
  ChannelQuotaDataItem,
  ChannelQuotaTrendItem,
  ChartMetric,
  DashboardFilters,
  ModelAnalyticsChartTab,
} from '@/features/dashboard/types'
import { useThemeRadiusPx } from '@/lib/theme-radius'
import {
  computeTimeRange,
  formatChartTime,
  type TimeGranularity,
} from '@/lib/time'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type ChannelChartSpec = Record<string, any>

const MAX_RANK_CHANNELS = 20
const MAX_TREND_CHANNELS = 20

type TFunction = (key: string) => string

function metricValueOf(
  row: Pick<ChannelQuotaDataItem, 'count' | 'quota' | 'token_used'>,
  metric: ChartMetric
): number {
  if (metric === 'count') return Number(row.count) || 0
  if (metric === 'tokens') return Number(row.token_used) || 0
  return Number(row.quota) || 0
}

function channelLabel(
  row: Pick<ChannelQuotaDataItem, 'channel_id' | 'channel_name'>,
  deletedChannelLabel: (id: number) => string,
  t: TFunction
): string {
  const id = Number(row.channel_id) || 0
  if (row.channel_name) return row.channel_name
  return id > 0 ? deletedChannelLabel(id) : t('Unknown Channel')
}

interface ChannelChartData {
  spec_pie: ChannelChartSpec
  spec_rank: ChannelChartSpec
  spec_trend: ChannelChartSpec
  totalDisplay: string
}

function buildChannelChartData(
  rows: ChannelQuotaDataItem[],
  trendRows: ChannelQuotaTrendItem[],
  metric: ChartMetric,
  timeGranularity: TimeGranularity,
  t: TFunction,
  deletedChannelLabel: (id: number) => string,
  chartCornerRadius?: number
): ChannelChartData {
  const otherLabel = t('Other')
  const pieTitle = t('Call Distribution')
  const rankTitle = t('Call Ranking')
  const trendTitle = t('Call Trend')
  const formatValue = (value: number) => formatChartMetricValue(value, metric)
  const formatTotal = (value: number) =>
    formatChartMetricValue(value, metric, 2)

  // Distribution (pie) and ranking (bar) share the per-channel aggregation.
  const totals = new Map<number, { name: string; value: number }>()
  rows.forEach((row) => {
    const id = Number(row.channel_id) || 0
    const prev = totals.get(id)
    if (prev) {
      prev.value += metricValueOf(row, metric)
      return
    }
    totals.set(id, {
      name: channelLabel(row, deletedChannelLabel, t),
      value: metricValueOf(row, metric),
    })
  })

  const entries = [...totals.values()].sort((a, b) => b.value - a.value)
  const totalValue = entries.reduce((sum, entry) => sum + entry.value, 0)
  const empty = entries.length === 0
  const subtext = empty ? t('No data available') : undefined

  const pieValues = entries.map((entry) => ({
    type: entry.name,
    value: entry.value,
  }))

  let rankValues: Array<{ Token: string; Value: number }>
  if (entries.length > MAX_RANK_CHANNELS) {
    const top = entries
      .slice(0, MAX_RANK_CHANNELS)
      .map((entry) => ({ Token: entry.name, Value: entry.value }))
    const otherValue = entries
      .slice(MAX_RANK_CHANNELS)
      .reduce((sum, entry) => sum + entry.value, 0)
    rankValues = [...top, { Token: otherLabel, Value: otherValue }]
  } else {
    rankValues = entries.map((entry) => ({
      Token: entry.name,
      Value: entry.value,
    }))
  }

  const pieColorDomain = entries.map((entry) => entry.name)
  const pieColor = {
    type: 'ordinal',
    domain: pieColorDomain,
    range: getDashboardChartColors(pieColorDomain.length),
  }

  const rankColorDomain = [...new Set(rankValues.map((v) => v.Token))]
  const rankColor = {
    type: 'ordinal',
    domain: rankColorDomain,
    range: getDashboardChartColors(rankColorDomain.length),
  }

  // Trend (area): aggregate by (time bucket, channel) with a Map, then keep
  // the top channels by total and merge the rest into "Other".
  const timeChannelMap = new Map<string, Map<string, number>>()
  const channelTotals = new Map<string, number>()
  trendRows.forEach((row) => {
    const timeKey = formatChartTime(Number(row.created_at), timeGranularity)
    const name = channelLabel(row, deletedChannelLabel, t)
    const value = metricValueOf(row, metric)
    let channelMap = timeChannelMap.get(timeKey)
    if (!channelMap) {
      channelMap = new Map()
      timeChannelMap.set(timeKey, channelMap)
    }
    channelMap.set(name, (channelMap.get(name) || 0) + value)
    channelTotals.set(name, (channelTotals.get(name) || 0) + value)
  })

  const rankedTrendChannels = [...channelTotals.entries()].sort(
    (a, b) => b[1] - a[1]
  )
  const topTrendChannels = rankedTrendChannels
    .slice(0, MAX_TREND_CHANNELS)
    .map(([name]) => name)
  const topTrendChannelSet = new Set(topTrendChannels)
  const hasOtherChannels = rankedTrendChannels.length > MAX_TREND_CHANNELS
  const sortedTimes = [...timeChannelMap.keys()].sort()

  const trendValues: Array<{ Time: string; Token: string; Value: number }> = []
  sortedTimes.forEach((time) => {
    const channelMap = timeChannelMap.get(time) ?? new Map()
    topTrendChannels.forEach((name) => {
      trendValues.push({ Time: time, Token: name, Value: channelMap.get(name) || 0 })
    })
    if (hasOtherChannels) {
      let otherValue = 0
      channelMap.forEach((value, name) => {
        if (!topTrendChannelSet.has(name)) otherValue += value
      })
      trendValues.push({ Time: time, Token: otherLabel, Value: otherValue })
    }
  })

  const trendColorDomain = [...topTrendChannels, otherLabel]
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
          chartCornerRadius == null
            ? {}
            : { cornerRadius: chartCornerRadius },
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
      xField: 'Token',
      yField: 'Value',
      seriesField: 'Token',
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
              key: (datum: Record<string, unknown>) => datum?.Token,
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
      seriesField: 'Token',
      stack: false,
      legends: { visible: true, selectMode: 'single' },
      color: trendColor,
      title: { visible: true, text: trendTitle, subtext },
      tooltip: {
        mark: {
          content: [
            {
              key: (datum: Record<string, unknown>) => datum?.Token,
              value: (datum: Record<string, unknown>) =>
                formatValue(Number(datum?.Value) || 0),
            },
          ],
        },
        dimension: {
          content: [
            {
              key: (datum: Record<string, unknown>) => datum?.Token,
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
              key: t('Total:'),
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

const CHART_SPEC_KEYS: Record<
  ModelAnalyticsChartTab,
  'spec_pie' | 'spec_rank' | 'spec_trend'
> = {
  trend: 'spec_trend',
  proportion: 'spec_pie',
  top: 'spec_rank',
}

interface ChannelChartsProps {
  filters?: DashboardFilters
  metric: ChartMetric
}

export function ChannelCharts(props: ChannelChartsProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const { customization } = useThemeCustomization()
  const chartRadius = useThemeRadiusPx(
    '--radius-md',
    `${customization.preset}:${customization.radius}`
  )
  const [activeTab, setActiveTab] = useState<ModelAnalyticsChartTab>('trend')
  const timeGranularity =
    props.filters?.time_granularity ?? DEFAULT_TIME_GRANULARITY

  const timeRange = useMemo(
    () =>
      computeTimeRange(
        getDefaultDays(props.filters?.time_granularity),
        props.filters?.start_timestamp,
        props.filters?.end_timestamp
      ),
    [
      props.filters?.end_timestamp,
      props.filters?.start_timestamp,
      props.filters?.time_granularity,
    ]
  )
  const queryParams = useMemo(
    () =>
      buildQueryParams(timeRange, {
        time_granularity: props.filters?.time_granularity,
      }),
    [timeRange, props.filters?.time_granularity]
  )

  const { data: rows, isLoading } = useQuery({
    queryKey: ['dashboard', 'channel-charts', 'dates', queryParams],
    queryFn: () => getChannelQuotaDates(queryParams),
    select: (res) => res.data ?? [],
    staleTime: 60_000,
  })

  const { data: trendRows, isLoading: trendLoading } = useQuery({
    queryKey: ['dashboard', 'channel-charts', 'trend', queryParams],
    queryFn: () => getChannelQuotaTrendDates(queryParams),
    select: (res) => res.data ?? [],
    staleTime: 60_000,
  })

  const chartData = useMemo(
    () =>
      buildChannelChartData(
        isLoading ? [] : (rows ?? []),
        trendLoading ? [] : (trendRows ?? []),
        props.metric,
        timeGranularity,
        t,
        (id) => t('Deleted channel ({{id}})', { id }),
        chartRadius
      ),
    [rows, trendRows, isLoading, trendLoading, props.metric, timeGranularity, t, chartRadius]
  )

  const spec = chartData[CHART_SPEC_KEYS[activeTab]]
  const specType = typeof spec?.type === 'string' ? spec.type : activeTab
  const chartKey = [
    activeTab,
    specType,
    isLoading || trendLoading ? 'loading' : 'ready',
    (rows?.length ?? 0) + (trendRows?.length ?? 0),
    props.metric,
    resolvedTheme,
    customization.preset,
  ].join('-')

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex w-full flex-col gap-1.5 border-b px-3 py-2 sm:gap-3 sm:px-5 sm:py-3 lg:flex-row lg:items-center lg:justify-between'>
        <div className='flex items-center gap-2'>
          <IconBadge tone='chart-2' size='sm'>
            <Network />
          </IconBadge>
          <div className='text-sm font-semibold'>{t('Channel Call Analytics')}</div>
          <span className='text-muted-foreground text-xs'>
            {t('Total:')} {chartData.totalDisplay}
          </span>
        </div>

        <div className='bg-muted/60 inline-flex h-7 w-full overflow-x-auto rounded-lg border p-0.5 sm:h-8 sm:w-auto'>
          {MODEL_ANALYTICS_CHART_OPTIONS.map((tab) => (
            <button
              key={tab.value}
              type='button'
              onClick={() => setActiveTab(tab.value)}
              className={`shrink-0 rounded-md px-3 text-xs font-medium transition-colors ${
                activeTab === tab.value
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {t(tab.labelKey)}
            </button>
          ))}
        </div>
      </div>

      <div className='h-[300px] p-1.5 sm:h-96 sm:p-2'>
        {themeReady && spec && (
          <VChart
            key={chartKey}
            spec={{
              ...spec,
              theme: resolvedTheme === 'dark' ? 'dark' : 'light',
              background: 'transparent',
            }}
            option={VCHART_OPTION}
          />
        )}
      </div>
    </div>
  )
}
