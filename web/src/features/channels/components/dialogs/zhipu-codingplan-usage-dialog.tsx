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

import type {
  ZhipuCodingPlanUsageResponse,
  ZhipuCodingPlanWindow,
} from '../../api'

import { UsageDialogShell } from './usage/usage-dialog-shell'
import {
  pickOrderedWindows,
  UsageWindowCard,
} from './usage/usage-window-card'
import { useUsageDialogState } from './usage/use-usage-dialog-state'

export type { ZhipuCodingPlanUsageResponse } from '../../api'

type ZhipuCodingPlanUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  response: ZhipuCodingPlanUsageResponse | null
  onRefresh?: () => void | Promise<void>
  isRefreshing?: boolean
}

const WINDOW_ORDER = ['session', 'weekly', 'monthly'] as const
type WindowPeriod = (typeof WINDOW_ORDER)[number]

/** Uppercase the first letter of a plan level so "pro" renders as "Pro". */
function formatLevel(level: string): string {
  const trimmed = level.trim()
  return trimmed === ''
    ? trimmed
    : trimmed.charAt(0).toUpperCase() + trimmed.slice(1)
}

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
  return t('Monthly')
}

export function ZhipuCodingPlanUsageDialog(
  props: ZhipuCodingPlanUsageDialogProps
) {
  const { t } = useTranslation()

  const level = props.response?.data?.level
  const dialogTitle =
    level && level.trim() !== ''
      ? t('GLM Coding Plan · {{level}}', { level: formatLevel(level) })
      : t('Zhipu Coding Plan Usage')

  const windows = pickOrderedWindows<ZhipuCodingPlanWindow>(
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
      title={dialogTitle}
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
              title={getWindowTitle(window.period, t)}
              window={window}
            />
          ))}
        </div>
      ) : null}
    </UsageDialogShell>
  )
}