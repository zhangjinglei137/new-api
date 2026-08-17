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
import { Activity, Hash, WalletCards, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { METRIC_OPTIONS } from '@/features/dashboard/constants'
import type { ChartMetric } from '@/features/dashboard/types'

const METRIC_ICONS: Record<ChartMetric, LucideIcon> = {
  quota: WalletCards,
  tokens: Hash,
  count: Activity,
}

interface MetricToggleProps {
  value: ChartMetric
  onChange: (metric: ChartMetric) => void
}

export function MetricToggle(props: MetricToggleProps) {
  const { t } = useTranslation()
  return (
    <Tabs
      value={props.value}
      onValueChange={(value) => props.onChange(value as ChartMetric)}
      className='shrink-0'
    >
      <TabsList aria-label={t('Metric')}>
        {METRIC_OPTIONS.map((option) => {
          const Icon = METRIC_ICONS[option.value]
          return (
            <TabsTrigger
              key={option.value}
              value={option.value}
              className='gap-1.5 px-2.5 text-xs'
            >
              <Icon data-icon='inline-start' aria-hidden='true' />
              {t(option.labelKey)}
            </TabsTrigger>
          )
        })}
      </TabsList>
    </Tabs>
  )
}
