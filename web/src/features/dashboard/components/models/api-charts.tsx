import { KeyRound } from 'lucide-react'
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
  getTokenQuotaDates,
  getTokenQuotaTrendDates,
} from '@/features/dashboard/api'
import type {
  ChartMetric,
  DashboardFilters,
  TokenQuotaDataItem,
  TokenQuotaTrendItem,
} from '@/features/dashboard/types'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { DashboardCharts } from './dashboard-charts'

interface ApiChartsProps {
  filters?: DashboardFilters
  metric: ChartMetric
}

export function ApiCharts(props: ApiChartsProps) {
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = Boolean(user?.role && user.role >= ROLE.ADMIN)
  const getId = useCallback(
    (row: TokenQuotaDataItem) => Number(row.token_id) || 0,
    []
  )
  const getName = useCallback((row: TokenQuotaDataItem) => row.token_name, [])
  const getTrendId = useCallback(
    (row: TokenQuotaTrendItem) => Number(row.token_id) || 0,
    []
  )
  const getTrendName = useCallback(
    (row: TokenQuotaTrendItem) => row.token_name,
    []
  )

  return (
    <DashboardCharts<TokenQuotaDataItem, TokenQuotaTrendItem>
      filters={props.filters}
      metric={props.metric}
      queryKey='api-charts'
      fetchDates={getTokenQuotaDates}
      fetchTrend={getTokenQuotaTrendDates}
      getId={getId}
      getName={getName}
      getTrendId={getTrendId}
      getTrendName={getTrendName}
      titleKey='API Call Analytics'
      unknownKey='Unknown Token'
      deletedKey='Deleted token ({{id}})'
      icon={KeyRound}
      tone='info'
      isAdmin={isAdmin}
    />
  )
}
