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
import { KeyRound } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { getTokenQuotaDates } from '@/features/dashboard/api'
import { MODEL_DISTRIBUTION_CHART_OPTIONS } from '@/features/dashboard/constants'
import { buildQueryParams, getDefaultDays } from '@/features/dashboard/lib'
import {
  formatChartMetricValue,
  getDashboardChartColors,
} from '@/features/dashboard/lib/charts'
import type {
  ChartMetric,
  DashboardFilters,
  ModelDistributionChartTab,
  TokenQuotaDataItem,
} from '@/features/dashboard/types'
import { ROLE } from '@/lib/roles'
import { useThemeRadiusPx } from '@/lib/theme-radius'
import { computeTimeRange } from '@/lib/time'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'
import { useAuthStore } from '@/stores/auth-store'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type TokenChartSpec = Record<string, any>

const MAX_RANK_TOKENS = 20

interface TokenDistributionData {
  spec_pie: TokenChartSpec
  spec_rank: TokenChartSpec
  totalDisplay: string
}

function buildTokenDistributionData(
  rows: TokenQuotaDataItem[],
  metric: ChartMetric,
  t: (key: string) => string,
  deletedTokenLabel: (id: number) => string,
  chartCornerRadius?: number
): TokenDistributionData {
  const isTokens = metric === 'tokens'
  const otherLabel = t('Other')
  const pieTitle = isTokens ? t('Token Distribution') : t('Quota Distribution')
  const rankTitle = isTokens ? t('Token Ranking') : t('Quota Ranking')
  const metricValue = (row: TokenQuotaDataItem) =>
    isTokens ? Number(row.token_used) || 0 : Number(row.quota) || 0
  const formatValue = (value: number) => formatChartMetricValue(value, metric)

  // Aggregate by token id; token_name is empty for deleted keys.
  const totals = new Map<number, { name: string; value: number }>()
  rows.forEach((row) => {
    const id = Number(row.token_id) || 0
    const prev = totals.get(id)
    if (prev) {
      prev.value += metricValue(row)
      return
    }
    const name =
      row.token_name || (id > 0 ? deletedTokenLabel(id) : t('Unknown Token'))
    totals.set(id, { name, value: metricValue(row) })
  })

  const entries = [...totals.values()].sort((a, b) => b.value - a.value)
  const totalValue = entries.reduce((sum, entry) => sum + entry.value, 0)

  const pieValues = entries.map((entry) => ({
    type: entry.name,
    value: entry.value,
  }))

  let rankValues: Array<{ Token: string; Value: number }>
  if (entries.length > MAX_RANK_TOKENS) {
    const top = entries
      .slice(0, MAX_RANK_TOKENS)
      .map((entry) => ({ Token: entry.name, Value: entry.value }))
    const otherValue = entries
      .slice(MAX_RANK_TOKENS)
      .reduce((sum, entry) => sum + entry.value, 0)
    rankValues = [...top, { Token: otherLabel, Value: otherValue }]
  } else {
    rankValues = entries.map((entry) => ({
      Token: entry.name,
      Value: entry.value,
    }))
  }

  const colorDomain = [
    ...new Set([...entries.map((entry) => entry.name), otherLabel]),
  ]
  const color = {
    type: 'ordinal',
    domain: colorDomain,
    range: getDashboardChartColors(colorDomain.length),
  }

  const empty = entries.length === 0
  const subtext = empty ? t('No data available') : undefined

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
      color,
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
      color,
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
    totalDisplay: formatChartMetricValue(totalValue, metric, 2),
  }
}

interface TokenDistributionChartProps {
  filters: DashboardFilters
  metric: ChartMetric
}

export function TokenDistributionChart(props: TokenDistributionChartProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const { customization } = useThemeCustomization()
  const chartRadius = useThemeRadiusPx(
    '--radius-md',
    `${customization.preset}:${customization.radius}`
  )
  const [activeTab, setActiveTab] = useState<ModelDistributionChartTab>(
    'proportion'
  )
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = Boolean(user?.role && user.role >= ROLE.ADMIN)

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

  const { data: tokenRows, isLoading } = useQuery({
    queryKey: ['dashboard', 'tokens', queryParams, isAdmin],
    queryFn: () => getTokenQuotaDates(queryParams, isAdmin),
    select: (res) => res.data ?? [],
    staleTime: 60_000,
  })

  const chartData = useMemo(
    () =>
      buildTokenDistributionData(
        isLoading ? [] : (tokenRows ?? []),
        props.metric,
        t,
        (id) => t('Deleted token ({{id}})', { id }),
        chartRadius
      ),
    [tokenRows, isLoading, props.metric, t, chartRadius]
  )

  const spec =
    activeTab === 'proportion' ? chartData.spec_pie : chartData.spec_rank
  const specType = typeof spec?.type === 'string' ? spec.type : activeTab
  const chartKey = [
    activeTab,
    specType,
    isLoading ? 'loading' : 'ready',
    tokenRows?.length ?? 0,
    props.metric,
    resolvedTheme,
    customization.preset,
  ].join('-')

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex w-full flex-col gap-1.5 border-b px-3 py-2 sm:gap-3 sm:px-5 sm:py-3 lg:flex-row lg:items-center lg:justify-between'>
        <div className='flex items-center gap-2'>
          <IconBadge tone='info' size='sm'>
            <KeyRound />
          </IconBadge>
          <div className='text-sm font-semibold'>
            {t('API Key Distribution')}
          </div>
          <span className='text-muted-foreground text-xs'>
            {t('Total:')} {chartData.totalDisplay}
          </span>
        </div>

        <div className='bg-muted/60 inline-flex h-7 w-full overflow-x-auto rounded-lg border p-0.5 sm:h-8 sm:w-auto'>
          {MODEL_DISTRIBUTION_CHART_OPTIONS.map((tab) => (
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
