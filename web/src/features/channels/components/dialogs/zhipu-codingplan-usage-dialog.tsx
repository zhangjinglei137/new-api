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
import { ChevronDown, ChevronUp, RefreshCw, AlertTriangle } from 'lucide-react'
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
import { Progress, ProgressIndicator } from '@/components/ui/progress'
import { ScrollArea } from '@/components/ui/scroll-area'
import dayjs from '@/lib/dayjs'
import { formatDateTimeStr } from '@/lib/format'
import { cn } from '@/lib/utils'

import type {
  ZhipuCodingPlanUsageResponse,
  ZhipuCodingPlanWindow,
} from '../../api'

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

/**
 * Clamp a percentage to [0, 100]. Returns null when the value is missing or
 * invalid so the UI can show "-" instead of pretending the failure is 0%.
 */
function clampPercent(value: unknown): number | null {
  const v = Number(value)
  return Number.isFinite(v) ? Math.max(0, Math.min(100, v)) : null
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

/** Uppercase the first letter of a plan level so "pro" renders as "Pro". */
function formatLevel(level: string): string {
  const trimmed = level.trim()
  return trimmed === ''
    ? trimmed
    : trimmed.charAt(0).toUpperCase() + trimmed.slice(1)
}

type UsageWindowCardProps = {
  title: string
  window: ZhipuCodingPlanWindow
}

function UsageWindowCard(props: UsageWindowCardProps) {
  const { t } = useTranslation()
  const usedPercent = clampPercent(props.window.used_percent)
  const usedVariant = usedPercent === null ? null : getUsedVariant(usedPercent)
  const resetsAt = formatResetsAt(props.window.reset_at)

  return (
    <Card size='sm' className='bg-muted/30 gap-0 py-0'>
      <CardHeader className='p-4 pb-2'>
        <CardTitle className='text-muted-foreground text-xs font-medium'>
          {props.title}
        </CardTitle>
      </CardHeader>
      <CardContent className='p-4 pt-0'>
        <div className='flex items-start justify-between gap-3'>
          <div
            className={cn(
              'text-3xl leading-none font-bold tabular-nums',
              usedPercent !== null && usedVariant
                ? usedVariantTextClass[usedVariant]
                : 'text-muted-foreground'
            )}
          >
            {usedPercent !== null ? `${formatPercent(usedPercent)}%` : '-'}
          </div>
        </div>
        {usedPercent !== null ? (
          <Progress
            value={usedPercent}
            aria-label={`${props.title}: ${formatPercent(usedPercent)}%`}
            className='mt-3'
          >
            <ProgressIndicator
              className={
                usedVariant ? usedVariantProgressClass[usedVariant] : undefined
              }
            />
          </Progress>
        ) : (
          <div className='text-muted-foreground mt-3 text-sm'>-</div>
        )}
        <div className='mt-3 grid grid-cols-1 gap-2 text-xs sm:grid-cols-2'>
          <div className='min-w-0'>
            <div className='text-muted-foreground text-[11px]'>
              {t('Used:')}
            </div>
            <div className='tabular-nums'>
              {usedPercent !== null ? `${formatPercent(usedPercent)}%` : '-'}
            </div>
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

type UsageErrorCopy = {
  title: string
  body: string
}

function getUsageErrorCopy(
  errorCode: string | undefined,
  message: string | undefined,
  t: (key: string) => string
): UsageErrorCopy {
  if (
    errorCode === 'credentials_not_configured' ||
    errorCode === 'credentials_expired'
  ) {
    return {
      title: t('Usage credentials not configured'),
      body: message?.trim() || t('Failed to fetch usage'),
    }
  }
  return {
    title: t('Unable to identify usage data'),
    body: message?.trim() || t('Failed to fetch usage'),
  }
}

export function ZhipuCodingPlanUsageDialog(
  props: ZhipuCodingPlanUsageDialogProps
) {
  const { t } = useTranslation()
  const [showRawJson, setShowRawJson] = useState(false)

  const level = props.response?.data?.level
  const dialogTitle =
    level && level.trim() !== ''
      ? t('GLM Coding Plan · {{level}}', { level: formatLevel(level) })
      : t('Zhipu Coding Plan Usage')

  // Render windows in a fixed order (session → weekly → monthly), only those
  // present in the response. Unknown periods are ignored.
  const windows = WINDOW_ORDER.map(
    (period) => props.response?.data?.windows?.find((w) => w?.period === period)
  ).filter((w): w is ZhipuCodingPlanWindow => Boolean(w))

  const hasUsageData = windows.length > 0

  const errorCopy =
    props.response?.success === false
      ? getUsageErrorCopy(
          props.response.error_code,
          props.response.message,
          t
        )
      : null

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
      title={dialogTitle}
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
        {errorCopy && (
          <Alert variant='destructive'>
            <AlertTriangle />
            <AlertTitle>{errorCopy.title}</AlertTitle>
            <AlertDescription>{errorCopy.body}</AlertDescription>
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
            {windows.map((window) => (
              <UsageWindowCard
                key={window.period}
                title={getWindowTitle(window.period, t)}
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
