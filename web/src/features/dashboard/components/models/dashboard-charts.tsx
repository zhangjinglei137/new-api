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
import type { LucideIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import {
  DEFAULT_TIME_GRANULARITY,
  MODEL_ANALYTICS_CHART_OPTIONS,
} from '@/features/dashboard/constants'
import { buildQueryParams, getDefaultDays } from '@/features/dashboard/lib'
import type {
  ChartMetric,
  DashboardFilters,
  ModelAnalyticsChartTab,
} from '@/features/dashboard/types'
import { useThemeRadiusPx } from '@/lib/theme-radius'
import { computeTimeRange } from '@/lib/time'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import {
  buildChartData,
  buildChartQueryKey,
  type ChartSpec,
  type DashboardChartQueryParams,
  type DashboardQuotaRow,
} from './dashboard-charts-data'

export interface DashboardChartsProps<
  TItem extends DashboardQuotaRow,
  TTrend extends DashboardQuotaRow,
> {
  filters?: DashboardFilters
  metric: ChartMetric
  queryKey: string
  fetchDates: (
    params: DashboardChartQueryParams,
    isAdmin: boolean
  ) => Promise<{ success: boolean; data?: TItem[]; message?: string }>
  fetchTrend: (
    params: DashboardChartQueryParams,
    isAdmin: boolean
  ) => Promise<{ success: boolean; data?: TTrend[]; message?: string }>
  getId: (row: TItem) => number
  getName: (row: TItem) => string | undefined
  getTrendId: (row: TTrend) => number
  getTrendName: (row: TTrend) => string | undefined
  titleKey: string
  unknownKey: string
  deletedKey: string
  icon: LucideIcon
  tone: IconBadgeTone
  // When provided, the query key and the fetchers carry the admin scope so an
  // admin view never leaks non-own quota data to regular users.
  isAdmin?: boolean
}

const CHART_SPEC_KEYS: Record<
  ModelAnalyticsChartTab,
  'spec_pie' | 'spec_rank' | 'spec_trend'
> = {
  trend: 'spec_trend',
  proportion: 'spec_pie',
  top: 'spec_rank',
}

export function DashboardCharts<
  TItem extends DashboardQuotaRow,
  TTrend extends DashboardQuotaRow,
>(props: DashboardChartsProps<TItem, TTrend>) {
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
  const chartIsAdmin = props.isAdmin ?? false

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
  const dateQueryKey = useMemo(
    () =>
      buildChartQueryKey(
        props.queryKey,
        'dates',
        queryParams,
        props.isAdmin
      ),
    [props.queryKey, props.isAdmin, queryParams]
  )
  const trendQueryKey = useMemo(
    () =>
      buildChartQueryKey(
        props.queryKey,
        'trend',
        queryParams,
        props.isAdmin
      ),
    [props.queryKey, props.isAdmin, queryParams]
  )

  const { data: rows, isLoading } = useQuery({
    queryKey: dateQueryKey,
    queryFn: () => props.fetchDates(queryParams, chartIsAdmin),
    select: (res) => res.data ?? [],
    staleTime: 60_000,
  })

  const { data: trendRows, isLoading: trendLoading } = useQuery({
    queryKey: trendQueryKey,
    queryFn: () => props.fetchTrend(queryParams, chartIsAdmin),
    select: (res) => res.data ?? [],
    staleTime: 60_000,
  })

  const chartData = useMemo(
    () =>
      buildChartData({
        rows: isLoading ? [] : (rows ?? []),
        trendRows: trendLoading ? [] : (trendRows ?? []),
        metric: props.metric,
        timeGranularity,
        t,
        deletedItemLabel: (id) => t(props.deletedKey, { id }),
        unknownItemKey: props.unknownKey,
        getId: props.getId,
        getName: props.getName,
        getTrendId: props.getTrendId,
        getTrendName: props.getTrendName,
        chartCornerRadius: chartRadius,
      }),
    [
      props.deletedKey,
      props.getId,
      props.getTrendId,
      props.getTrendName,
      props.metric,
      props.getName,
      props.unknownKey,
      rows,
      trendRows,
      isLoading,
      trendLoading,
      timeGranularity,
      t,
      chartRadius,
    ]
  )

  const spec: ChartSpec | undefined = chartData[CHART_SPEC_KEYS[activeTab]]
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
          <IconBadge tone={props.tone} size='sm'>
            <props.icon />
          </IconBadge>
          <div className='text-sm font-semibold'>{t(props.titleKey)}</div>
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