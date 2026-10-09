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
  ClinePlanLimit,
  ClineUsageResponse,
} from '../../api'

import { UsageDialogShell } from './usage/usage-dialog-shell'
import {
  pickOrderedWindows,
  UsageWindowCard,
  type UsageWindowData,
} from './usage/usage-window-card'
import { useUsageDialogState } from './usage/use-usage-dialog-state'

export type { ClineUsageResponse } from '../../api'

type ClineUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  response: ClineUsageResponse | null
  onRefresh?: () => void | Promise<void>
  isRefreshing?: boolean
}

/** ClinePass 返回的 limit 周期固定顺序：5 小时 → 每周 → 每月。 */
const LIMIT_ORDER = ['five_hour', 'weekly', 'monthly'] as const
type LimitPeriod = (typeof LIMIT_ORDER)[number]

function getLimitTitle(
  period: LimitPeriod,
  t: (key: string) => string
): string {
  if (period === 'five_hour') {
    return t('Last 5 hours')
  }
  if (period === 'weekly') {
    return t('Weekly')
  }
  return t('Monthly')
}

/** 把 ClinePass 的 percentUsed/resetsAt 映射为通用窗口卡片字段。 */
function toUsageWindow(limit: ClinePlanLimit): UsageWindowData & {
  period?: string
} {
  return {
    period: limit.type,
    used_percent: limit.percentUsed,
    reset_at: limit.resetsAt,
  }
}

export function ClineUsageDialog(props: ClineUsageDialogProps) {
  const { t } = useTranslation()

  const windows = pickOrderedWindows<UsageWindowData & { period?: string }>(
    LIMIT_ORDER,
    props.response?.data?.limits?.map(toUsageWindow)
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
      title={t('Cline Coding Plan Usage')}
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
              title={getLimitTitle(window.period as LimitPeriod, t)}
              window={window}
            />
          ))}
        </div>
      ) : null}
    </UsageDialogShell>
  )
}
