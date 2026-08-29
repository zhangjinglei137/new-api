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
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import type {
  SubscriptionUsageData,
  SubscriptionUsageResponse,
} from '../../types'
import {
  formatSubscriptionUsageUpdatedAt,
  getSubscriptionPercentVariant,
  getSubscriptionWindowPercent,
  quotaToUsd,
  type SubscriptionPercentVariant,
  type SubscriptionWindowKey,
} from '../../lib/subscription-billing'

type SubscriptionUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  usage: SubscriptionUsageResponse | null
  onRefresh?: () => void | Promise<void>
  isRefreshing?: boolean
  isLoading?: boolean
}

const percentTextClassName: Record<SubscriptionPercentVariant, string> = {
  danger: 'text-destructive',
  warning: 'text-warning',
  info: 'text-info',
}

function formatUnixSeconds(unixSeconds: unknown): string {
  const v = Number(unixSeconds)
  return Number.isFinite(v) && v > 0 ? formatTimestampToDate(v) : '-'
}

/**
 * 格式化"将于以下时间重置"的剩余时长。窗口语义（对齐 opencode Go 官方）：
 * - 5h 滚窗：xx小时 xx分钟（不会超过 5h）
 * - 7d 自然周 / 31d 桶：>=1 天 → xx天 xx小时；<1 天 → xx小时 xx分钟
 */
function formatDurationSeconds(
  seconds: unknown,
  windowKey: SubscriptionWindowKey,
  t: (key: string) => string
): string {
  const s = Number(seconds)
  if (!Number.isFinite(s) || s <= 0) {
    return '-'
  }

  const total = Math.floor(s)
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const minutes = Math.floor((total % 3600) / 60)

  // 5h 滚窗：xx小时 xx分钟（窗口长度固定 5h，不会出现天数）
  if (windowKey === '5h') {
    return `${hours}${t('h')} ${minutes}${t('m')}`
  }

  // 7d/31d：>=1 天 → xx天 xx小时；否则 xx小时 xx分钟
  if (days >= 1) {
    return `${days}${t('d')} ${hours}${t('h')}`
  }
  return `${hours}${t('h')} ${minutes}${t('m')}`
}

const WINDOW_DEFINITIONS: Array<{
  key: SubscriptionWindowKey
  titleKey: string
}> = [
  { key: '5h', titleKey: '5-Hour Window' },
  { key: '7d', titleKey: 'Weekly Window' },
  { key: '31d', titleKey: 'Monthly Window' },
]

export function SubscriptionWindowCards({
  windows,
}: {
  windows?: SubscriptionUsageData['windows']
}) {
  const { t } = useTranslation()

  return (
    <div className='grid grid-cols-1 gap-3 md:grid-cols-3'>
      {WINDOW_DEFINITIONS.map((definition) => {
        const window = windows?.[definition.key]
        const hasData =
          !!window &&
          typeof window === 'object' &&
          Object.keys(window).length > 0
        const percent = hasData ? getSubscriptionWindowPercent(window) : 0
        const variant = getSubscriptionPercentVariant(percent)

        return (
          <Card key={definition.key} size='sm' className='gap-0 py-0'>
            <CardHeader className='p-3 pb-2'>
              <div className='flex items-start justify-between gap-3'>
                <div className='min-w-0'>
                  <CardTitle className='text-sm font-semibold'>
                    {t(definition.titleKey)}
                  </CardTitle>
                </div>
                <div className='shrink-0 text-right'>
                  <div
                    className={cn(
                      'text-xl leading-none font-semibold tabular-nums',
                      percentTextClassName[variant]
                    )}
                  >
                    {hasData ? `${Math.round(percent)}%` : '-'}
                  </div>
                  <div className='text-muted-foreground mt-1 text-[11px]'>
                    {t('Used')}
                  </div>
                </div>
              </div>
            </CardHeader>
            <CardContent className='p-3 pt-0'>
              {hasData ? (
                <Progress
                  value={percent}
                  aria-label={`${t(definition.titleKey)} usage: ${Math.round(
                    percent
                  )}%`}
                  className='mt-1'
                />
              ) : (
                <div className='text-muted-foreground mt-1 text-sm'>-</div>
              )}
              <div className='mt-3 grid grid-cols-1 gap-2 text-xs sm:grid-cols-2'>
                <div className='min-w-0'>
                  <div className='text-muted-foreground text-[11px]'>
                    {t('Reset at:')}
                  </div>
                  <div className='break-all tabular-nums'>
                    {hasData ? formatUnixSeconds(window.reset_at) : '-'}
                  </div>
                </div>
                <div className='min-w-0 sm:text-right'>
                  <div className='text-muted-foreground text-[11px]'>
                    {t('Resets in:')}
                  </div>
                  <div className='tabular-nums'>
                    {hasData
                      ? formatDurationSeconds(
                          window.reset_after_seconds,
                          definition.key,
                          t
                        )
                      : '-'}
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}

function SectionHeading(props: { title: string; description?: string }) {
  return (
    <div className='min-w-0'>
      <div className='text-sm font-semibold'>{props.title}</div>
      {props.description ? (
        <div className='text-muted-foreground mt-1 text-xs leading-5'>
          {props.description}
        </div>
      ) : null}
    </div>
  )
}

function PerModelUsageTable({
  perModel,
}: {
  perModel: SubscriptionUsageData['per_model']
}) {
  const { t } = useTranslation()
  const rows = perModel ?? []

  if (rows.length === 0) {
    return (
      <Empty className='min-h-32 border'>
        <EmptyHeader>
          <EmptyTitle>{t('No model usage yet')}</EmptyTitle>
          <EmptyDescription>
            {t('No per-model usage details are available yet.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <div className='overflow-x-auto rounded-lg border'>
      <table className='w-full min-w-[560px] text-sm'>
        <thead>
          <tr className='border-b bg-muted/30 text-left text-xs'>
            <th className='px-3 py-2 font-medium'>{t('Model')}</th>
            <th className='px-3 py-2 font-medium'>{t('Used')}</th>
            <th className='px-3 py-2 font-medium'>{t('Limit')}</th>
            <th className='px-3 py-2 font-medium'>{t('Used %')}</th>
            <th className='px-3 py-2 font-medium'>{t('Status')}</th>
          </tr>
        </thead>
        <tbody className='divide-y'>
          {rows.map((item, index) => {
            const percent = Number(item.used_percent)
            const percentText = Number.isFinite(percent)
              ? `${Math.round(percent)}%`
              : '-'
            const variant = getSubscriptionPercentVariant(percent)
            return (
              <tr key={item.model || `model-${index}`} className='hover:bg-muted/30'>
                <td className='px-3 py-2 font-mono text-xs break-all'>
                  {item.model || '-'}
                </td>
                <td className='px-3 py-2 tabular-nums'>
                  {item.used_quota != null
                    ? formatBillingCurrencyFromUSD(quotaToUsd(item.used_quota))
                    : '-'}
                </td>
                <td className='px-3 py-2 tabular-nums'>
                  {item.limit_quota != null
                    ? formatBillingCurrencyFromUSD(quotaToUsd(item.limit_quota))
                    : '-'}
                </td>
                <td
                  className={cn(
                    'px-3 py-2 tabular-nums',
                    percentTextClassName[variant]
                  )}
                >
                  {percentText}
                </td>
                <td className='px-3 py-2'>
                  {item.over_limit ? (
                    <StatusBadge
                      label={t('Over limit')}
                      variant='danger'
                      size='sm'
                      copyable={false}
                    />
                  ) : (
                    <StatusBadge
                      label={t('OK')}
                      variant='success'
                      size='sm'
                      copyable={false}
                    />
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

export function SubscriptionUsageDialog({
  open,
  onOpenChange,
  channelName,
  usage,
  onRefresh,
  isRefreshing,
  isLoading,
}: SubscriptionUsageDialogProps) {
  const { t } = useTranslation()
  const data = usage?.data
  const errorMessage =
    usage?.success === false
      ? usage?.message?.trim() || t('Failed to fetch usage')
      : ''

  const channelLabel = channelName?.trim() || '-'

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Subscription Usage')}
      contentClassName='sm:max-w-[900px]'
      contentHeight='auto'
      bodyClassName='flex flex-col gap-4'
      footer={
        <Button
          type='button'
          variant='outline'
          onClick={() => onOpenChange(false)}
        >
          {t('Close')}
        </Button>
      }
    >
      <div className='flex flex-col gap-4'>
        {errorMessage ? (
          <div className='border-destructive/40 bg-destructive/10 text-destructive rounded-lg border px-3 py-2 text-sm'>
            {errorMessage}
          </div>
        ) : null}

        <Card size='sm' className='bg-muted/30 gap-0 py-0'>
          <CardHeader className='p-4 pb-2'>
            <CardTitle className='text-muted-foreground text-xs font-medium'>
              {t('Subscription Usage')}
            </CardTitle>
            {onRefresh ? (
              <CardAction>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={onRefresh}
                  disabled={Boolean(isRefreshing) || Boolean(isLoading)}
                >
                  <RefreshCw data-icon='inline-start' />
                  {t('Refresh now')}
                </Button>
              </CardAction>
            ) : null}
          </CardHeader>
          <CardContent className='p-4 pt-0'>
            <div className='flex flex-wrap items-center gap-2'>
              <StatusBadge
                label={channelLabel}
                variant='neutral'
                copyable={false}
              />
              {data?.partial ? (
                <StatusBadge
                  label={t('Partial data')}
                  variant='warning'
                  copyable={false}
                />
              ) : null}
            </div>
            <div className='text-muted-foreground mt-2 text-xs'>
              {t('Last updated:')}{' '}
              {formatSubscriptionUsageUpdatedAt(data?.updated_at)}
            </div>
          </CardContent>
        </Card>

        {isLoading ? (
          <div className='grid grid-cols-1 gap-3 md:grid-cols-3'>
            <Skeleton className='h-36 w-full' />
            <Skeleton className='h-36 w-full' />
            <Skeleton className='h-36 w-full' />
          </div>
        ) : (
          <SubscriptionWindowCards windows={data?.windows} />
        )}

        <div className='flex flex-col gap-3'>
          <SectionHeading
            title={t('Per-model monthly usage')}
            description={t('Monthly quota usage per model.')}
          />
          {isLoading ? (
            <Skeleton className='h-32 w-full' />
          ) : (
            <PerModelUsageTable perModel={data?.per_model} />
          )}
        </div>
      </div>
    </Dialog>
  )
}
