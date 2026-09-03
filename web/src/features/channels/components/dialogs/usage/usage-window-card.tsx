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

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import dayjs from '@/lib/dayjs'
import { formatDateTimeStr } from '@/lib/format'
import { cn } from '@/lib/utils'

/** The window fields every usage dialog reads. Provider types are compatible. */
export type UsageWindowData = {
  used_percent?: number | null
  remaining_percent?: number | null
  reset_at?: string | null
  status?: string
  limit?: number | null
  used?: number | null
  remaining?: number | null
  metered?: boolean
}

export type UsageWindowVariant = 'percent' | 'currency' | 'points'

// ---------------------------------------------------------------------------
// Shared formatting + variant helpers
// ---------------------------------------------------------------------------

/**
 * Clamp a percentage to [0, 100]. Returns null when the value is missing,
 * invalid, or the -1 sentinel the backend uses for "no meaningful value", so
 * the UI can show "-" instead of pretending the failure is 0%.
 */
export function clampPercent(value: unknown): number | null {
  const v = Number(value)
  if (!Number.isFinite(v) || v === -1) {
    return null
  }
  return Math.max(0, Math.min(100, v))
}

/** Always render percentages with two decimal places. */
export function formatPercent(value: number): string {
  return value.toFixed(2)
}

export function formatResetsAt(value: unknown): string {
  if (typeof value !== 'string' || value.trim() === '') {
    return '-'
  }
  const d = dayjs(value)
  return d.isValid() ? formatDateTimeStr(d.toDate()) : '-'
}

export function isValidAmount(value: unknown): value is number {
  const v = Number(value)
  return Number.isFinite(v) && v >= 0
}

/** Remaining amount for a non-metered window (limit - used), or window.remaining. */
export function getRemainingAmount(window?: UsageWindowData | null): number | null {
  if (!window) {
    return null
  }
  if (isValidAmount(window.remaining)) {
    return Number(window.remaining)
  }
  if (isValidAmount(window.used) && isValidAmount(window.limit)) {
    return Math.max(0, Number(window.limit) - Number(window.used))
  }
  return null
}

/** A higher used percentage is more alarming. */
export function getUsedVariant(percent: number): 'success' | 'warning' | 'danger' {
  if (percent >= 80) {
    return 'danger'
  }
  if (percent >= 60) {
    return 'warning'
  }
  return 'success'
}

export const usedVariantTextClass: Record<
  'success' | 'warning' | 'danger',
  string
> = {
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-destructive',
}

export const usedVariantProgressClass: Record<
  'success' | 'warning' | 'danger',
  string
> = {
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-destructive',
}

/**
 * Pick windows from a response in a fixed order (WINDOW_ORDER), keeping only
 * the periods actually present. Unknown periods are ignored.
 */
export function pickOrderedWindows<W extends { period?: string }>(
  order: readonly string[],
  windows: W[] | null | undefined
): W[] {
  return order
    .map((period) => windows?.find((w) => w?.period === period))
    .filter((w): w is W => Boolean(w))
}

// ---------------------------------------------------------------------------
// UsageWindowCard
// ---------------------------------------------------------------------------

export type UsageWindowCardProps = {
  title: string
  window: UsageWindowData | null | undefined
  variant?: UsageWindowVariant
  /** Formats amounts for currency/points variants (and percent fallbacks). */
  formatValue?: (value: number) => string
  /** Render a status badge on the title when the window status is non-ok. */
  showStatusBadge?: boolean
  /** Override the "Used:" row, e.g. Command Code's "$used / $limit". */
  usedRow?: string
}

export function UsageWindowCard(props: UsageWindowCardProps) {
  const { t } = useTranslation()
  const window = props.window
  const variant = props.variant ?? 'percent'
  const isPercent = variant === 'percent'
  const usedPercent = isPercent ? clampPercent(window?.used_percent) : null
  const usedVariant =
    usedPercent === null ? null : getUsedVariant(usedPercent)
  const remaining = getRemainingAmount(window)
  const resetsAt = formatResetsAt(window?.reset_at)
  const status = typeof window?.status === 'string' ? window.status.trim() : ''
  const isHealthy = !status || status === 'ok'

  let bigNumber = '-'
  let usedRow: string

  if (isPercent) {
    if (usedPercent !== null) {
      bigNumber = `${formatPercent(usedPercent)}%`
    } else if (remaining !== null && props.formatValue) {
      // Metered windows fall back to the remaining amount (Command Code).
      bigNumber = props.formatValue(remaining)
    }
    usedRow =
      props.usedRow !== undefined
        ? props.usedRow
        : usedPercent !== null
          ? `${formatPercent(usedPercent)}%`
          : '-'
  } else {
    // currency / points
    if (remaining !== null && props.formatValue) {
      bigNumber = props.formatValue(remaining)
    }
    const used = isValidAmount(window?.used) ? Number(window.used) : null
    usedRow = used !== null && props.formatValue ? props.formatValue(used) : '-'
  }

  return (
    <Card size='sm' className='bg-muted/30 gap-0 py-0'>
      <CardHeader className='p-4 pb-2'>
        <CardTitle className='text-muted-foreground flex items-center justify-between gap-2 text-xs font-medium'>
          <span>{props.title}</span>
          {props.showStatusBadge && !isHealthy ? (
            <span className='bg-muted text-muted-foreground rounded border px-1.5 py-0.5 text-[10px] font-normal normal-case'>
              {t('Status: ')}
              {window?.status}
            </span>
          ) : null}
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
            <div className='text-muted-foreground text-[11px]'>{t('Used:')}</div>
            <div className='tabular-nums'>{usedRow}</div>
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