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
import { useTranslation } from 'react-i18next'

import { formatCurrencyFromUSD } from '@/lib/currency'

import type { CommandCodeUsageResponse, CommandCodeWindow } from '../../api'

import { UsageDialogShell } from './usage/usage-dialog-shell'
import {
  isValidAmount,
  pickOrderedWindows,
  UsageWindowCard,
  type UsageWindowData,
} from './usage/usage-window-card'
import { useUsageDialogState } from './usage/use-usage-dialog-state'

export type { CommandCodeUsageResponse } from '../../api'

type CommandCodeUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  response: CommandCodeUsageResponse | null
  onRefresh?: () => void | Promise<void>
  isRefreshing?: boolean
}

const WINDOW_ORDER = ['session', 'weekly', 'monthly', 'topup'] as const
type WindowPeriod = (typeof WINDOW_ORDER)[number]

function getWindowTitle(
  period: WindowPeriod,
  t: (key: string) => string
): string {
  if (period === 'session') {
    return t('Last 5 hours')
  }
  if (period === 'weekly') {
    return t('Weekly')
  }
  if (period === 'monthly') {
    return t('Monthly')
  }
  return t('Top-up credits')
}

function formatUSD(value: number): string {
  return formatCurrencyFromUSD(value)
}

/**
 * "Used:" row for metered windows: "$used / $limit" (or "$used" when the
 * limit is missing). Non-metered windows let the shared card render the
 * amount itself.
 */
function formatUsedRow(window: CommandCodeWindow): string | undefined {
  if (window.metered !== true) {
    return undefined
  }
  const usedValid = isValidAmount(window.used)
  const limitValid = isValidAmount(window.limit)
  if (usedValid && limitValid) {
    return `${formatUSD(Number(window.used))} / ${formatUSD(Number(window.limit))}`
  }
  return usedValid ? formatUSD(Number(window.used)) : '-'
}

export function CommandCodeUsageDialog(props: CommandCodeUsageDialogProps) {
  const { t } = useTranslation()

  const windows = pickOrderedWindows<CommandCodeWindow>(
    WINDOW_ORDER,
    props.response?.data?.windows
  )
  const { errorCopy, showDegraded, rawJsonText, showRawPanel } =
    useUsageDialogState({
      response: props.response,
      hasUsageData: windows.length > 0,
    })

  return (
    <UsageDialogShell
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Command Code Usage')}
      errorCopy={errorCopy}
      showDegraded={showDegraded}
      onRefresh={props.onRefresh}
      isRefreshing={props.isRefreshing}
      showRawPanel={showRawPanel}
      rawJsonText={rawJsonText}
    >
      {windows.length > 0 ? (
        <div className='flex flex-col gap-3'>
          {windows.map((window) => (
            <UsageWindowCard
              key={window.period}
              title={getWindowTitle(window.period ?? 'topup', t)}
              window={window as UsageWindowData}
              variant={window.metered === true ? 'percent' : 'currency'}
              formatValue={formatUSD}
              usedRow={formatUsedRow(window)}
            />
          ))}
        </div>
      ) : null}
    </UsageDialogShell>
  )
}