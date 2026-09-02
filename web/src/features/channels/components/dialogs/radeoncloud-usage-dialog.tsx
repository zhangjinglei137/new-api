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
import {
  AlertTriangle,
  ChevronDown,
  ChevronUp,
  Info,
  RefreshCw,
} from 'lucide-react'
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
import { formatDateTimeStr, formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type {
  RadeonCloudUsageData,
  RadeonCloudUsageResponse,
} from '../../api'

export type { RadeonCloudUsageResponse } from '../../api'

type RadeonCloudUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  response: RadeonCloudUsageResponse | null
  onRefresh?: () => void | Promise<void>
  isRefreshing?: boolean
}

/** Coerce a value into a finite number, or null when missing or invalid. */
function toFiniteNumber(value: unknown): number | null {
  const v = Number(value)
  return Number.isFinite(v) ? v : null
}

/**
 * Clamp a percentage (0-100 scale) into [0, 100]. Returns null when the value
 * is missing or invalid so the UI can show "-" instead of a fake 0%.
 */
function clampPercent(value: unknown): number | null {
  const v = Number(value)
  return Number.isFinite(v) ? Math.max(0, Math.min(100, v)) : null
}

/** Always render percentages with two decimal places. */
function formatPercent(value: number): string {
  return value.toFixed(2)
}

/** Format an ISO timestamp into YYYY-MM-DD HH:mm:ss, or "-" when invalid. */
function formatDateTimeValue(value: unknown): string {
  if (typeof value !== 'string' || value.trim() === '') {
    return '-'
  }
  const d = dayjs(value)
  return d.isValid() ? formatDateTimeStr(d.toDate()) : '-'
}

/** Resolve the daily allowance reset time from the upstream payload. */
function formatResetsAt(data: RadeonCloudUsageData | undefined): string {
  const resetAt = data?.daily_reset_at
  if (typeof resetAt === 'string' && resetAt.trim() !== '') {
    const d = dayjs(resetAt)
    if (d.isValid()) {
      return formatDateTimeStr(d.toDate())
    }
  }
  const resetInSec = toFiniteNumber(data?.daily_reset_in_sec)
  if (resetInSec !== null) {
    return formatDateTimeStr(dayjs().add(resetInSec, 'second').toDate())
  }
  return '-'
}

/**
 * A higher daily usage percentage is more alarming.
 * Green below 80%, yellow from 80-90%, red above 90%.
 */
function getUsedVariant(percent: number): 'success' | 'warning' | 'danger' {
  if (percent > 90) {
    return 'danger'
  }
  if (percent >= 80) {
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

type UsageErrorCopy = {
  title: string
  body: string
}

function getUsageErrorCopy(
  errorCode: string | undefined,
  message: string | undefined,
  t: (key: string) => string
): UsageErrorCopy {
  if (errorCode === 'credentials_expired') {
    return {
      title: t('Usage credentials not configured'),
      body: message?.trim() || t('Failed to fetch usage'),
    }
  }
  if (errorCode === 'fetch_failed') {
    return {
      title: t('Failed to fetch usage'),
      body: message?.trim() || t('Failed to fetch usage'),
    }
  }
  if (errorCode === 'usage_schema_unknown') {
    return {
      title: t('Unable to identify usage data'),
      body: message?.trim() || t('Failed to fetch usage'),
    }
  }
  return {
    title: t('Unable to identify usage data'),
    body: message?.trim() || t('Failed to fetch usage'),
  }
}

/** A response carries usable data when at least one core numeric field is set. */
function hasRadeonCloudData(data: RadeonCloudUsageData | undefined): boolean {
  if (!data) {
    return false
  }
  return [
    data.daily_limit_points,
    data.daily_remaining_points,
    data.daily_used_points,
    data.rpm_limit,
    data.today_requests,
  ].some((value) => typeof value === 'number' && Number.isFinite(value))
}

type StatItemProps = {
  label: string
  value: string
}

function StatItem(props: StatItemProps) {
  return (
    <div className='min-w-0'>
      <div className='text-muted-foreground text-[11px]'>{props.label}</div>
      <div className='mt-0.5 tabular-nums'>{props.value}</div>
    </div>
  )
}

export function RadeonCloudUsageDialog(
  props: RadeonCloudUsageDialogProps
) {
  const { t } = useTranslation()
  const [showRawJson, setShowRawJson] = useState(false)

  const data = props.response?.data
  const hasUsageData = hasRadeonCloudData(data)

  const remainingPoints = toFiniteNumber(data?.daily_remaining_points)
  const usedPoints = toFiniteNumber(data?.daily_used_points)
  const allowancePoints = toFiniteNumber(data?.daily_limit_points)
  const rpmLimit = toFiniteNumber(data?.rpm_limit)
  const todayRequests = toFiniteNumber(data?.today_requests)
  const todayTokens = toFiniteNumber(data?.today_tokens)
  const last24hRequests = toFiniteNumber(data?.last_24h_requests)
  const last24hTokens = toFiniteNumber(data?.last_24h_tokens)

  const usedPercent = (() => {
    const fromPercent = clampPercent(Number(data?.daily_used_percent) * 100)
    if (fromPercent !== null) {
      return fromPercent
    }
    if (usedPoints !== null && allowancePoints !== null && allowancePoints > 0) {
      return clampPercent((usedPoints / allowancePoints) * 100)
    }
    return null
  })()
  const usedVariant = usedPercent === null ? null : getUsedVariant(usedPercent)

  const resetsAt = formatResetsAt(data)
  const lastRequestAt = formatDateTimeValue(data?.last_24h_last_request_at)

  const formatRequestsTokens = (
    requests: number | null,
    tokens: number | null
  ): string => {
    const req =
      requests === null ? '-' : t('{{count}} requests', { count: requests })
    const tok = tokens === null ? '-' : t('{{count}} tokens', { count: tokens })
    return `${req} · ${tok}`
  }

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
      title={t('AMD Radeon Cloud Usage')}
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
        {!errorCopy && (
          <Alert>
            <Info />
            <AlertTitle>
              {t(
                'Free to use. Points track API usage against your daily limit—they are not charges.'
              )}
            </AlertTitle>
          </Alert>
        )}

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
                'The upstream response did not include recognizable usage data.'
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
            <Card size='sm' className='bg-muted/30 gap-0 py-0'>
              <CardContent className='p-4'>
                <div className='text-muted-foreground text-xs font-medium'>
                  {t('Today Remaining')}
                </div>
                <div className='mt-1 text-3xl leading-none font-bold tabular-nums'>
                  {remainingPoints !== null
                    ? `${formatNumber(remainingPoints)} pts`
                    : '-'}
                </div>

                <div className='mt-4'>
                  {usedPercent !== null ? (
                    <Progress
                      value={usedPercent}
                      aria-label={`${t('Today Used')}: ${formatPercent(usedPercent)}%`}
                      className='h-2'
                    >
                      <ProgressIndicator
                        className={
                          usedVariant
                            ? usedVariantProgressClass[usedVariant]
                            : undefined
                        }
                      />
                    </Progress>
                  ) : (
                    <div className='text-muted-foreground text-sm'>-</div>
                  )}
                  <div className='mt-1 flex justify-end'>
                    <span
                      className={cn(
                        'text-xs font-medium tabular-nums',
                        usedPercent !== null && usedVariant
                          ? usedVariantTextClass[usedVariant]
                          : 'text-muted-foreground'
                      )}
                    >
                      {usedPercent !== null
                        ? `${formatPercent(usedPercent)}%`
                        : '-'}
                    </span>
                  </div>
                </div>

                <div className='mt-4 flex items-center justify-between gap-2 text-sm'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Today Used')} / {t('Daily Allowance')}
                  </div>
                  <div className='tabular-nums'>
                    {formatNumber(usedPoints)} / {formatNumber(allowancePoints)}{' '}
                    pts
                  </div>
                </div>

                <div className='mt-2 flex items-center justify-between gap-2 text-sm'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Today Requests')} · {t('Today Tokens')}
                  </div>
                  <div className='tabular-nums'>
                    {formatRequestsTokens(todayRequests, todayTokens)}
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card size='sm' className='bg-muted/30 gap-0 py-0'>
              <CardHeader className='p-4 pb-2'>
                <CardTitle className='text-muted-foreground text-xs font-medium'>
                  {t('Last 24h')}
                </CardTitle>
              </CardHeader>
              <CardContent className='p-4 pt-0'>
                <div className='grid grid-cols-1 gap-x-4 gap-y-2 text-xs sm:grid-cols-2'>
                  <StatItem
                    label={t('RPM Limit')}
                    value={rpmLimit !== null ? String(rpmLimit) : '-'}
                  />
                  <StatItem label={t('Resets at:')} value={resetsAt} />
                  <StatItem label={t('Last Request')} value={lastRequestAt} />
                  <StatItem
                    label={`${t('Requests')} · ${t('Tokens')}`}
                    value={formatRequestsTokens(last24hRequests, last24hTokens)}
                  />
                </div>
              </CardContent>
            </Card>
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
