import { Network } from 'lucide-react'
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
import { useCallback } from 'react'

import {
  getChannelQuotaDates,
  getChannelQuotaTrendDates,
} from '@/features/dashboard/api'
import type {
  ChannelQuotaDataItem,
  ChannelQuotaTrendItem,
  ChartMetric,
  DashboardFilters,
} from '@/features/dashboard/types'

import { DashboardCharts } from './dashboard-charts'

interface ChannelChartsProps {
  filters?: DashboardFilters
  metric: ChartMetric
}

export function ChannelCharts(props: ChannelChartsProps) {
  const getId = useCallback(
    (row: ChannelQuotaDataItem) => Number(row.channel_id) || 0,
    []
  )
  const getName = useCallback(
    (row: ChannelQuotaDataItem) => row.channel_name,
    []
  )
  const getTrendId = useCallback(
    (row: ChannelQuotaTrendItem) => Number(row.channel_id) || 0,
    []
  )
  const getTrendName = useCallback(
    (row: ChannelQuotaTrendItem) => row.channel_name,
    []
  )

  return (
    <DashboardCharts<ChannelQuotaDataItem, ChannelQuotaTrendItem>
      filters={props.filters}
      metric={props.metric}
      queryKey='channel-charts'
      fetchDates={getChannelQuotaDates}
      fetchTrend={getChannelQuotaTrendDates}
      getId={getId}
      getName={getName}
      getTrendId={getTrendId}
      getTrendName={getTrendName}
      titleKey='Channel Call Analytics'
      unknownKey='Unknown Channel'
      deletedKey='Deleted channel ({{id}})'
      icon={Network}
      tone='chart-2'
    />
  )
}
