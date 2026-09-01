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
import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
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
  VolcCodingPlanUsageErrorCode,
  VolcCodingPlanUsageResponse,
} from '../../api'

export type { VolcCodingPlanUsageResponse } from '../../api'

type VolcCodingPlanUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  response: VolcCodingPlanUsageResponse | null
  onRefresh?: () => void | Promise<void>
  isRefreshing?: boolean
}

/**
 * Clamp a percentage to [0, 100]. Returns null when the value is missing or
 * invalid so the UI can show "-" instead of pretending the failure is 0%.
 */
function clampPercent(value: unknown): number | null {
  const v = Number(value)
  return Number.isFinite(v) ? Math.max(0, Math.min(100, v)) : null
}

function formatResetsAt(value: unknown): string {
  if (typeof value !== 'string' || value.trim() === '') {
    return '-'
  }
  const d = dayjs(value)
  return d.isValid() ? formatDateTimeStr(d.toDate()) : '-'
}

function getRemainingVariant(percent: number): 'success' | 'warning' | 'danger' {
  if (percent >= 50) {
    return 'success'
  }
  if (percent >= 20) {
    return 'warning'
  }
  return 'danger'
}

const remainingVariantTextClass: Record<
  'success' | 'warning' | 'danger',
  string
> = {
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-destructive',
}

const remainingVariantProgressClass: Record<
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
  errorCode: VolcCodingPlanUsageErrorCode | undefined,
  message: string | undefined,
  t: (key: string) => string
): UsageErrorCopy {
  if (errorCode === 'credentials_not_configured') {
    return {
      title: t('Usage credentials not configured'),
      body: t(
        'Configure the CSRF token and session cookie in the channel settings before querying usage.'
      ),
    }
  }
  if (errorCode === 'credentials_expired') {
    return {
      title: t('Session expired, please update Cookie and CSRF token'),
      body: t(
        'Re-login to your VolcEngine account and paste the fresh Cookie and CSRF token in the channel settings.'
      ),
    }
  }
  return {
    title: t('Unable to identify usage data'),
    body: message?.trim() || t('Failed to fetch usage'),
  }
}

export function VolcCodingPlanUsageDialog(
  props: VolcCodingPlanUsageDialogProps
) {
  const { t } = useTranslation()
  const [showRawJson, setShowRawJson] = useState(false)

  const remainingPercent = clampPercent(props.response?.data?.remaining_percent)
  const usedPercent = clampPercent(props.response?.data?.used_percent)
  const remainingVariant =
    remainingPercent === null
      ? null
      : getRemainingVariant(remainingPercent)
  const resetsAt = formatResetsAt(props.response?.data?.reset_at)

  const hasUsageData =
    props.response?.data != null &&
    typeof props.response.data === 'object' &&
    Object.keys(props.response.data).length > 0

  const errorCopy =
    props.response?.success === false
      ? getUsageErrorCopy(
          props.response.error_code,
          props.response.message,
          t
        )
      : null

  const rawJsonText = props.response
    ? JSON.stringify(props.response, null, 2)
    : ''

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('VolcEngine Coding Plan Usage')}
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

        <Card size='sm' className='bg-muted/30 gap-0 py-0'>
          <CardHeader className='p-4 pb-2'>
            <CardTitle className='text-muted-foreground text-xs font-medium'>
              {t('Monthly remaining')}
            </CardTitle>
            {props.onRefresh ? (
              <CardAction>
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
              </CardAction>
            ) : null}
          </CardHeader>
          <CardContent className='p-4 pt-0'>
            <div className='flex items-start justify-between gap-3'>
              <div
                className={cn(
                  'text-3xl leading-none font-bold tabular-nums',
                  remainingPercent !== null && remainingVariant
                    ? remainingVariantTextClass[remainingVariant]
                    : 'text-muted-foreground'
                )}
              >
                {remainingPercent !== null ? `${remainingPercent}%` : '-'}
              </div>
              {remainingPercent !== null && remainingVariant ? (
                <StatusBadge
                  label={t('Monthly')}
                  variant={remainingVariant}
                  copyable={false}
                />
              ) : null}
            </div>
            {remainingPercent !== null ? (
              <Progress
                value={remainingPercent}
                aria-label={`${t('Monthly remaining')}: ${remainingPercent}%`}
                className='mt-3'
              >
                <ProgressIndicator
                  className={
                    remainingVariant
                      ? remainingVariantProgressClass[remainingVariant]
                      : undefined
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
                  {usedPercent !== null ? `${usedPercent}%` : '-'}
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

        {props.response && hasUsageData && rawJsonText ? (
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
