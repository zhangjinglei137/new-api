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
import { AlertTriangle, ChevronDown, ChevronUp, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Progress } from '@/components/ui/progress'
import { ScrollArea } from '@/components/ui/scroll-area'
import { formatCurrencyFromUSD } from '@/lib/currency'
import dayjs from '@/lib/dayjs'
import { formatDateTimeStr } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { CommandCodeUsageResponse, CommandCodeWindow } from '../../api'

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

/**
 * Clamp a percentage to [0, 100]. Returns null when the value is missing,
 * invalid, or the -1 sentinel the backend uses for "no meaningful value", so
 * the UI can show "-" instead of pretending the failure is 0%.
 */
function clampPercent(value: unknown): number | null {
  const v = Number(value)
  if (!Number.isFinite(v) || v === -1) {
    return null
  }
  return Math.max(0, Math.min(100, v))
}

/** Always render percentages with two decimal places. */
function formatPercent(value: number): string {
  return value.toFixed(2)
}

function formatResetsAt(value: unknown): string {
  if (typeof value !== 'string' || value.trim() === '') {
    return '-'
  }
  const d = dayjs(value)
  return d.isValid() ? formatDateTimeStr(d.toDate()) : '-'
}

/** A higher used percentage is more alarming. */
function getUsedVariant(percent: number): 'success' | 'warning' | 'danger' {
  if (percent >= 80) {
    return 'danger'
  }
  if (percent >= 60) {
    return 'warning'
  }
  return 'success'
}

const usedVariantTextClass: Record<
  'success' | 'warning' | 'danger',
  string
> = {
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-destructive',
}

const usedVariantProgressClass: Record<
  'success' | 'warning' | 'danger',
  string
> = {
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-destructive',
}

/** Whether a numeric value is a usable non-negative amount. */
function isValidAmount(value: unknown): boolean {
  const v = Number(value)
  return Number.isFinite(v) && v >= 0
}

/**
 * Remaining USD amount for a non-metered window (limit - used). Returns null
 * when either side is missing/invalid so the UI can show "-".
 */
function getRemainingUSD(window: CommandCodeWindow): number | null {
  if (!isValidAmount(window.used) || !isValidAmount(window.limit)) {
    return null
  }
  return Math.max(0, Number(window.limit) - Number(window.used))
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
  if (period === 'monthly') {
    return t('Monthly')
  }
  return t('Top-up credits')
}

type UsageWindowCardProps = {
  title: string
  window: CommandCodeWindow
}

function UsageWindowCard(props: UsageWindowCardProps) {
  const { t } = useTranslation()
  // Metered windows surface percentages; non-metered windows (topup, or a
  // monthly that fell back to the remaining balance) surface USD amounts.
  const isMetered = props.window.metered === true
  const usedPercent = isMetered ? clampPercent(props.window.used_percent) : null
  const usedVariant =
    usedPercent === null ? null : getUsedVariant(usedPercent)
  const resetsAt = formatResetsAt(props.window.reset_at)

  const usedValid = isValidAmount(props.window.used)
  const limitValid = isValidAmount(props.window.limit)
  const remainingUSD = getRemainingUSD(props.window)

  let bigNumber: string
  if (usedPercent !== null) {
    bigNumber = `${formatPercent(usedPercent)}%`
  } else if (remainingUSD !== null) {
    bigNumber = formatCurrencyFromUSD(remainingUSD)
  } else {
    bigNumber = '-'
  }

  let usedRowText = '-'
  if (isMetered && usedValid && limitValid) {
    usedRowText = `${formatCurrencyFromUSD(props.window.used)} / ${formatCurrencyFromUSD(props.window.limit)}`
  } else if (usedValid) {
    usedRowText = formatCurrencyFromUSD(props.window.used)
  }

  return (
    <Card size='sm' className='bg-muted/30 gap-0 py-0'>
      <CardHeader className='p-4 pb-2'>
        <CardTitle className='text-muted-foreground text-xs font-medium'>
          {props.title}
        </CardTitle>
      </CardHeader>
      <CardContent className='p-4 pt-0'>
        <div
          className={cn(
            'text-3xl leading-none font-bold tabular-nums',
            usedPercent !== null && usedVariant
              ? usedVariantTextClass[usedVariant]
              : 'text-muted-foreground'
          )}
        >
          {bigNumber}
        </div>
        {usedPercent !== null ? (
          <Progress
            value={usedPercent}
            aria-label={`${props.title}: ${formatPercent(usedPercent)}%`}
            className='mt-3'
            indicatorClassName={
              usedVariant ? usedVariantProgressClass[usedVariant] : undefined
            }
          />
        ) : (
          <div className='text-muted-foreground mt-3 text-sm'>-</div>
        )}
        <div className='mt-3 grid grid-cols-1 gap-2 text-xs sm:grid-cols-2'>
          <div className='min-w-0'>
            <div className='text-muted-foreground text-[11px]'>
              {t('Used:')}
            </div>
            <div className='tabular-nums'>{usedRowText}</div>
          </div>
          <div className='min-w-0 sm:text-right'>
            <div className='text-muted-foreground text-[11px]'>
              {t('Resets at:')}
            </div>
            <div className='tabular-nums'>{resetsAt}</div>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

export function CommandCodeUsageDialog(props: CommandCodeUsageDialogProps) {
  const { t } = useTranslation()
  const [showRawJson, setShowRawJson] = useState(false)

  // Render windows in a fixed order (session → weekly → monthly → topup), only
  // those present in the response. Unknown periods are ignored.
  const windows = WINDOW_ORDER.map((period) => ({
    period,
    window: props.response?.data?.windows?.find((w) => w?.period === period),
  })).filter(
    (entry): entry is { period: WindowPeriod; window: CommandCodeWindow } =>
      Boolean(entry.window)
  )

  const hasUsageData = windows.length > 0

  const errorMessage =
    props.response?.success === false
      ? props.response.message?.trim() || t('Failed to fetch usage')
      : ''

  const showDegraded =
    props.response != null &&
    props.response.success !== false &&
    !hasUsageData

  const rawJsonText = props.response
    ? JSON.stringify(props.response, null, 2)
    : ''
  const showRawPanel = Boolean(
    props.response && props.response.success !== false && rawJsonText
  )

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Command Code Usage')}
      contentHeight='auto'
      bodyClassName='flex flex-col gap-4'
      footer={
        <Button
          type='button'
          variant='outline'
          onClick={() => props.onOpenChange(false)}
        >
          {t('Close')}
        </Button>
      }
    >
      <div className='flex flex-col gap-4'>
        {errorMessage && (
          <Alert variant='destructive'>
            <AlertTriangle />
            <AlertTitle>{t('Unable to identify usage data')}</AlertTitle>
            <AlertDescription>{errorMessage}</AlertDescription>
          </Alert>
        )}

        {showDegraded && (
          <Alert>
            <AlertTriangle />
            <AlertTitle>{t('Unable to identify usage data')}</AlertTitle>
            <AlertDescription>
              {t(
                'The upstream response did not include recognizable usage windows.'
              )}
            </AlertDescription>
          </Alert>
        )}

        {props.onRefresh ? (
          <div className='flex justify-end'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={props.onRefresh}
              disabled={Boolean(props.isRefreshing)}
            >
              <RefreshCw data-icon='inline-start' />
              {t('Refresh usage')}
            </Button>
          </div>
        ) : null}

        {hasUsageData ? (
          <div className='flex flex-col gap-3'>
            {windows.map(({ period, window }) => (
              <UsageWindowCard
                key={period}
                title={getWindowTitle(period, t)}
                window={window}
              />
            ))}
          </div>
        ) : null}

        {showRawPanel ? (
          <Collapsible
            open={showRawJson}
            onOpenChange={setShowRawJson}
            className='rounded-lg border'
          >
            <CollapsibleTrigger
              render={
                <button
                  type='button'
                  className='hover:bg-muted/40 flex w-full items-center justify-between gap-2 p-3 text-left transition-colors'
                  aria-expanded={showRawJson}
                />
              }
            >
              <div className='text-sm font-medium'>
                {t('Show raw upstream response')}
              </div>
              {showRawJson ? (
                <ChevronUp className='text-muted-foreground h-4 w-4' />
              ) : (
                <ChevronDown className='text-muted-foreground h-4 w-4' />
              )}
            </CollapsibleTrigger>
            <CollapsibleContent>
              <ScrollArea className='max-h-[50vh]'>
                <pre className='bg-muted/30 m-0 p-3 text-xs break-words whitespace-pre-wrap'>
                  {rawJsonText}
                </pre>
              </ScrollArea>
            </CollapsibleContent>
          </Collapsible>
        ) : null}
      </div>
    </Dialog>
  )
}
