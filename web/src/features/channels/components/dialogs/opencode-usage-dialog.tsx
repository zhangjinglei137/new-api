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
import { AlertTriangle, RefreshCw } from 'lucide-react'
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
import { Progress, ProgressIndicator } from '@/components/ui/progress'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { cn } from '@/lib/utils'

import type { OpenCodeGoUsageResponse } from '../../api'

export type { OpenCodeGoUsageResponse } from '../../api'

type OpenCodeGoUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  response: OpenCodeGoUsageResponse | null
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

function formatDurationSeconds(
  seconds: unknown,
  t: (key: string) => string
): string {
  const s = Number(seconds)
  if (!Number.isFinite(s) || s <= 0) {
    return '-'
  }

  const total = Math.floor(s)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const secs = total % 60

  if (hours > 0) {
    return `${hours}${t('h')} ${minutes}${t('m')}`
  }
  if (minutes > 0) {
    return `${minutes}${t('m')} ${secs}${t('s')}`
  }
  return `${secs}${t('s')}`
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

export function OpenCodeGoUsageDialog(props: OpenCodeGoUsageDialogProps) {
  const { t } = useTranslation()

  const remainingPercent = clampPercent(props.response?.data?.remaining_percent)
  const usedPercent = clampPercent(props.response?.data?.usage_percent)
  const remainingVariant =
    remainingPercent === null ? null : getRemainingVariant(remainingPercent)
  const resetsIn = formatDurationSeconds(
    props.response?.data?.reset_in_sec,
    t
  )
  const balance = props.response?.data?.balance
  const balanceText = Number.isFinite(Number(balance))
    ? formatCurrencyFromUSD(Number(balance), {
        digitsLarge: 2,
        digitsSmall: 2,
        abbreviate: false,
      })
    : '-'

  const errorMessage =
    props.response?.success === false
      ? props.response.message?.trim() || t('Failed to fetch usage')
      : ''

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('OpenCode Go Usage')}
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
            <div className='mt-3 grid grid-cols-1 gap-2 text-xs sm:grid-cols-3'>
              <div className='min-w-0'>
                <div className='text-muted-foreground text-[11px]'>
                  {t('Used:')}
                </div>
                <div className='tabular-nums'>
                  {usedPercent !== null ? `${usedPercent}%` : '-'}
                </div>
              </div>
              <div className='min-w-0'>
                <div className='text-muted-foreground text-[11px]'>
                  {t('Resets in:')}
                </div>
                <div className='tabular-nums'>{resetsIn}</div>
              </div>
              <div className='min-w-0 sm:text-right'>
                <div className='text-muted-foreground text-[11px]'>
                  {t('Balance')}
                </div>
                <div className='tabular-nums'>{balanceText}</div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </Dialog>
  )
}
