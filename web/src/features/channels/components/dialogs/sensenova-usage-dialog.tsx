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
  Gift,
  Info,
  Loader2,
  RefreshCw,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
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
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import type {
  SenseNovaUsagePool,
  SenseNovaUsageResponse,
  SenseNovaUsageWindow,
} from '../../api'

export type { SenseNovaUsageResponse } from '../../api'

export type SenseNovaUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  response: SenseNovaUsageResponse | null
  onRefresh?: () => void | Promise<void>
  isRefreshing?: boolean
}

type WindowKey = 'window_5h' | 'window_7d'

function validAmount(value: unknown): number | null {
  const number = Number(value)
  return Number.isFinite(number) && number >= 0 ? number : null
}

function formatPoints(value: unknown, locale?: string): string {
  const number = validAmount(value)
  if (number === null) return '-'
  return new Intl.NumberFormat(locale, {
    maximumFractionDigits: 2,
  }).format(number)
}

function formatTime(value: unknown): string {
  if (typeof value !== 'string' || !value.trim()) return '-'
  const date = dayjs(value)
  return date.isValid() ? formatDateTimeStr(date.toDate()) : '-'
}

function getRemaining(window: SenseNovaUsageWindow | null | undefined) {
  if (!window) return null
  const remaining = validAmount(window.remaining)
  if (remaining !== null) return remaining
  const limit = validAmount(window.limit)
  const used = validAmount(window.used)
  return limit !== null && used !== null ? Math.max(0, limit - used) : null
}

function getRemainingPercent(window: SenseNovaUsageWindow | null | undefined) {
  if (!window) return null
  const limit = validAmount(window.limit)
  const remaining = getRemaining(window)
  if (limit === null || remaining === null) return null
  if (limit === 0) return remaining === 0 ? 100 : 0
  return Math.max(0, Math.min(100, (remaining / limit) * 100))
}

function getUsedColor(remainingPercent: number | null): string {
  if (remainingPercent === null) return 'bg-muted-foreground/40'
  const usedPercent = 100 - remainingPercent
  if (usedPercent >= 80) return 'bg-destructive'
  if (usedPercent >= 60) return 'bg-warning'
  return 'bg-success'
}

function isExpiringSoon(value: unknown): boolean {
  if (typeof value !== 'string' || !value.trim()) return false
  const expiry = dayjs(value)
  if (!expiry.isValid()) return false
  return expiry.diff(dayjs(), 'day', true) <= 7
}

function windowTitle(key: WindowKey, t: (key: string) => string): string {
  return key === 'window_5h' ? t('5h window') : t('Weekly window')
}

function PoolTypeBadge(props: { pool: SenseNovaUsagePool }) {
  const { t } = useTranslation()
  const isDedicated = props.pool.pool_type === 'dedicated'
  return (
    <Badge variant={isDedicated ? 'secondary' : 'outline'}>
      {isDedicated ? t('Dedicated pool') : t('General pool')}
    </Badge>
  )
}

function UsageWindowCard(props: {
  window: SenseNovaUsageWindow | null | undefined
  title: string
  locale?: string
}) {
  const { t } = useTranslation()
  const remaining = getRemaining(props.window)
  const limit = validAmount(props.window?.limit)
  const remainingPercent = getRemainingPercent(props.window)
  const usedPercent = remainingPercent === null ? null : 100 - remainingPercent
  const remainingText = formatPoints(remaining, props.locale)
  const limitText = formatPoints(limit, props.locale)

  return (
    <Card size='sm' className='bg-muted/20 gap-0 py-0'>
      <CardHeader className='p-4 pb-2'>
        <CardTitle className='text-muted-foreground flex items-center justify-between gap-2 text-xs font-medium'>
          <span>{props.title}</span>
          <span className='text-[11px] font-normal'>
            {t('Resets at:')} {formatTime(props.window?.reset_at)}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent className='p-4 pt-0'>
        <div className='flex items-baseline justify-between gap-2'>
          <div className='text-2xl leading-none font-bold tabular-nums'>
            {remainingText}
            <span className='text-muted-foreground ml-1 text-sm font-normal'>
              / {limitText}
            </span>
          </div>
          <div
            className={cn(
              'text-sm font-semibold tabular-nums',
              remainingPercent !== null ? 'text-foreground' : 'text-muted-foreground'
            )}
          >
            {remainingPercent === null ? '-' : `${remainingPercent.toFixed(1)}%`}
          </div>
        </div>
        {usedPercent === null ? (
          <div className='text-muted-foreground mt-3 text-xs'>
            {t('Usage percentage unavailable')}
          </div>
        ) : (
          <Progress
            value={usedPercent}
            aria-label={`${props.title}: ${remainingPercent?.toFixed(1)}% ${t('remaining')}`}
            className='mt-3'
          >
            <ProgressIndicator className={getUsedColor(remainingPercent)} />
          </Progress>
        )}
        <div className='text-muted-foreground mt-2 flex items-center justify-between text-[11px]'>
          <span>{t('Remaining available')}</span>
          <span>{t('Bar shows used')}</span>
        </div>
      </CardContent>
    </Card>
  )
}

function PoolCard(props: {
  pool: SenseNovaUsagePool
  locale?: string
}) {
  const { t } = useTranslation()
  const [modelsOpen, setModelsOpen] = useState(false)
  const models = Array.isArray(props.pool.model_ids)
    ? props.pool.model_ids.filter((model): model is string => Boolean(model))
    : []
  const previewModels = models.slice(0, 3)
  const weeklyBalance = getRemaining(props.pool.window_7d)
  const grantBalance = validAmount(props.pool.grant_balance)
  const hasGrant = grantBalance !== null && grantBalance > 0
  const expiry = formatTime(props.pool.nearest_grant_expiry)
  const expiringBalance = formatPoints(
    props.pool.nearest_grant_expiring_balance,
    props.locale
  )

  return (
    <Card className='overflow-hidden'>
      <CardHeader className='border-b bg-muted/20 p-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0'>
            <CardTitle className='flex flex-wrap items-center gap-2 text-base'>
              <span className='truncate'>{props.pool.name || t('Unnamed pool')}</span>
              <PoolTypeBadge pool={props.pool} />
            </CardTitle>
            <div className='mt-2 flex flex-wrap items-center gap-1.5'>
              {previewModels.map((model) => (
                <Badge key={model} variant='outline' className='font-mono text-[10px]'>
                  {model}
                </Badge>
              ))}
              {models.length > previewModels.length && (
                <button
                  type='button'
                  className='text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-xs underline-offset-4 hover:underline'
                  onClick={() => setModelsOpen((open) => !open)}
                  aria-expanded={modelsOpen}
                >
                  {modelsOpen
                    ? t('Hide models')
                    : t('+{{count}} more models', {
                        count: models.length - previewModels.length,
                      })}
                  {modelsOpen ? (
                    <ChevronUp className='size-3' />
                  ) : (
                    <ChevronDown className='size-3' />
                  )}
                </button>
              )}
            </div>
            {modelsOpen && (
              <div className='mt-2 flex flex-wrap gap-1.5'>
                {models.slice(3).map((model) => (
                  <Badge key={model} variant='outline' className='font-mono text-[10px]'>
                    {model}
                  </Badge>
                ))}
              </div>
            )}
          </div>
          <div className='shrink-0 text-right'>
            <div className='text-muted-foreground text-[11px]'>{t("This week's balance")}</div>
            <div className='text-primary text-2xl leading-none font-bold tabular-nums'>
              {formatPoints(weeklyBalance, props.locale)}
            </div>
          </div>
        </div>
      </CardHeader>
      <CardContent className='space-y-3 p-4'>
        <div className='grid gap-3 sm:grid-cols-2'>
          <UsageWindowCard
            title={windowTitle('window_5h', t)}
            window={props.pool.window_5h}
            locale={props.locale}
          />
          <UsageWindowCard
            title={windowTitle('window_7d', t)}
            window={props.pool.window_7d}
            locale={props.locale}
          />
        </div>
        {hasGrant && (
          <div className='border-primary/20 bg-primary/5 rounded-lg border p-3'>
            <div className='flex items-start gap-2'>
              <Gift className='text-primary mt-0.5 size-4 shrink-0' aria-hidden='true' />
              <div className='min-w-0 flex-1'>
                <div className='text-sm font-medium'>{t('Activity credits')}</div>
                <div className='mt-1 grid gap-1 text-xs sm:grid-cols-2'>
                  <span>
                    {t('Total balance')}: <strong className='tabular-nums'>{formatPoints(grantBalance, props.locale)}</strong>
                  </span>
                  <span className={cn(isExpiringSoon(props.pool.nearest_grant_expiry) && 'text-warning font-medium')}>
                    {t('Next expiry')}: {expiry} · {expiringBalance}
                  </span>
                </div>
              </div>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function getErrorTitle(message: string, t: (key: string) => string): string {
  const lower = message.toLowerCase()
  if (lower.includes('登录失败') || lower.includes('login')) {
    return t('SenseNova login failed')
  }
  if (lower.includes('账号未配置') || lower.includes('credential')) {
    return t('SenseNova credentials not configured')
  }
  return t('Unable to load SenseNova usage')
}

export function SenseNovaUsageDialog(props: SenseNovaUsageDialogProps) {
  const { t, i18n } = useTranslation()
  const [showRawJson, setShowRawJson] = useState(false)
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const pools = Array.isArray(props.response?.data?.pools)
    ? props.response.data.pools.filter(
        (pool): pool is SenseNovaUsagePool => Boolean(pool && typeof pool === 'object')
      )
    : []
  const isLoading = props.response === null
  const errorMessage =
    props.response?.success === false
      ? props.response.message?.trim() || t('Failed to fetch usage')
      : ''
  const rawJsonText = props.response
    ? JSON.stringify(props.response, null, 2)
    : ''
  const showRawPanel = Boolean(props.response && props.response.success && rawJsonText)
  const showDegraded = Boolean(props.response?.success && pools.length === 0)

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('SenseNova Usage')}
      contentHeight='auto'
      bodyClassName='flex max-h-[min(76vh,760px)] flex-col gap-4 overflow-y-auto'
      footer={
        <Button type='button' variant='outline' onClick={() => props.onOpenChange(false)}>
          {t('Close')}
        </Button>
      }
    >
      <div className='flex flex-col gap-4'>
        {isLoading && (
          <div className='text-muted-foreground flex items-center justify-center gap-2 py-12 text-sm'>
            <Loader2 className='size-4 animate-spin' />
            {t('Loading SenseNova usage...')}
          </div>
        )}
        {errorMessage && (
          <Alert variant='destructive'>
            <AlertTriangle />
            <AlertTitle>{getErrorTitle(errorMessage, t)}</AlertTitle>
            <AlertDescription>{errorMessage}</AlertDescription>
          </Alert>
        )}
        {showDegraded && (
          <Alert>
            <AlertTriangle />
            <AlertTitle>{t('Unable to identify usage data')}</AlertTitle>
            <AlertDescription>{t('SenseNova returned no pool data.')}</AlertDescription>
          </Alert>
        )}
        {props.response?.success && props.response.data?.plan?.name && (
          <div className='flex items-center gap-2 text-sm'>
            <span className='text-muted-foreground'>{t('Plan')}</span>
            <Badge variant='secondary'>{props.response.data.plan.name}</Badge>
          </div>
        )}
        {props.onRefresh && (
          <div className='flex justify-end'>
            <Button type='button' variant='outline' size='sm' onClick={props.onRefresh} disabled={Boolean(props.isRefreshing)}>
              <RefreshCw data-icon='inline-start' />
              {t('Refresh usage')}
            </Button>
          </div>
        )}
        {pools.length > 0 && (
          <div className='flex flex-col gap-3'>
            {pools.map((pool, index) => (
              <PoolCard key={`${pool.pool_type || 'pool'}-${pool.name || index}`} pool={pool} locale={locale} />
            ))}
          </div>
        )}
        {pools.some((pool) => pool.pool_type === 'dedicated') && (
          <div className='border-info/20 bg-info/5 text-info-foreground flex items-start gap-2 rounded-lg border px-3 py-2.5 text-xs'>
            <Info className='mt-0.5 size-4 shrink-0' aria-hidden='true' />
            <span>{t('Using a dedicated pool earns rewards for the general pool.')}</span>
          </div>
        )}
        {showRawPanel && (
          <Collapsible open={showRawJson} onOpenChange={setShowRawJson} className='rounded-lg border'>
            <CollapsibleTrigger
              render={
                <button type='button' className='hover:bg-muted/40 flex w-full items-center justify-between gap-2 p-3 text-left transition-colors' aria-expanded={showRawJson} />
              }
            >
              <div className='text-sm font-medium'>{t('Show raw upstream response')}</div>
              {showRawJson ? <ChevronUp className='text-muted-foreground size-4' /> : <ChevronDown className='text-muted-foreground size-4' />}
            </CollapsibleTrigger>
            <CollapsibleContent>
              <ScrollArea className='max-h-[50vh]'>
                <pre className='bg-muted/30 m-0 p-3 text-xs break-words whitespace-pre-wrap'>{rawJsonText}</pre>
              </ScrollArea>
            </CollapsibleContent>
          </Collapsible>
        )}
      </div>
    </Dialog>
  )
}
