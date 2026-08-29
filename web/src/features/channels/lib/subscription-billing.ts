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
import type {
  SubscriptionBillingConfig,
  SubscriptionBillingMode,
  SubscriptionBillingModelTier,
} from '../types'

import dayjs from '@/lib/dayjs'
import { formatDateTimeStr } from '@/lib/format'

// ============================================================================
// Subscription Billing Constants
// ============================================================================

/** Quota units per 1 USD. Mirrors the backend QuotaPerUnit value. */
export const SUBSCRIPTION_QUOTA_PER_USD = 500000

/** Monthly window is always 100% of the monthly total. */
export const SUBSCRIPTION_MONTHLY_RATIO_PERCENT = 100

/** Defaults applied when the backend returns no configuration. */
export const SUBSCRIPTION_DEFAULT_MONTHLY_TOTAL_USD = 60
export const SUBSCRIPTION_DEFAULT_FIVE_HOUR_RATIO_PERCENT = 20
export const SUBSCRIPTION_DEFAULT_WEEKLY_RATIO_PERCENT = 50

/** Usage summary color thresholds (aligned with the Codex usage dialog). */
export const SUBSCRIPTION_WARNING_PERCENT = 80
export const SUBSCRIPTION_DANGER_PERCENT = 95

/** Upper bound for the monthly total in USD to keep quota arithmetic safe. */
export const SUBSCRIPTION_MAX_MONTHLY_USD = 1000000

export const SUBSCRIPTION_BILLING_MODE_OPTIONS: Array<{
  value: SubscriptionBillingMode
  label: string
}> = [
  { value: 'metered', label: 'Pay as you go' },
  { value: 'subscription', label: 'Subscription' },
]

export const SUBSCRIPTION_WINDOW_KEYS = ['5h', '7d', '31d'] as const
export type SubscriptionWindowKey = (typeof SUBSCRIPTION_WINDOW_KEYS)[number]

// ============================================================================
// Unit Conversion Helpers
// ============================================================================

export function quotaToUsd(quota: number | null | undefined): number {
  const value = Number(quota)
  if (!Number.isFinite(value) || value <= 0) return 0
  return value / SUBSCRIPTION_QUOTA_PER_USD
}

export function usdToQuota(usd: number | null | undefined): number {
  const value = Number(usd)
  if (!Number.isFinite(value) || value <= 0) return 0
  return Math.round(value * SUBSCRIPTION_QUOTA_PER_USD)
}

export function percentToBps(percent: unknown): number {
  const value = Number(percent)
  if (!Number.isFinite(value)) return 0
  return Math.round(value * 100)
}

export function bpsToPercent(bps: unknown): number {
  const value = Number(bps)
  if (!Number.isFinite(value)) return 0
  return value / 100
}

// ============================================================================
// Derived Limits
// ============================================================================

export function deriveFiveHourUsd(config: SubscriptionBillingConfig): number {
  return (
    (config.monthly_total_usd * config.five_hour_ratio_percent) / 100
  )
}

export function deriveWeeklyUsd(config: SubscriptionBillingConfig): number {
  return (config.monthly_total_usd * config.weekly_ratio_percent) / 100
}

// ============================================================================
// Color Thresholds
// ============================================================================

export type SubscriptionPercentVariant = 'danger' | 'warning' | 'info'

export function getSubscriptionPercentVariant(
  percent: number | null | undefined
): SubscriptionPercentVariant {
  const value = Number(percent)
  if (!Number.isFinite(value)) return 'info'
  if (value >= SUBSCRIPTION_DANGER_PERCENT) return 'danger'
  if (value >= SUBSCRIPTION_WARNING_PERCENT) return 'warning'
  return 'info'
}

export function getSubscriptionWindowPercent(
  window: { display_percent?: number; used_percent?: number } | null | undefined
): number {
  if (!window) return 0
  const display = Number(window.display_percent)
  if (Number.isFinite(display)) return display
  const used = Number(window.used_percent)
  return Number.isFinite(used) ? used : 0
}

/**
 * Format the usage `updated_at` value for display. The backend may send a unix
 * timestamp (seconds or milliseconds) or an ISO string.
 */
export function formatSubscriptionUsageUpdatedAt(
  value: number | string | null | undefined
): string {
  if (value == null || value === '') {
    return '-'
  }
  if (typeof value === 'number') {
    if (value <= 0) return '-'
    const ms = value < 1e12 ? value * 1000 : value
    return formatDateTimeStr(new Date(ms))
  }
  const parsed = dayjs(value)
  if (!parsed.isValid()) {
    return '-'
  }
  return formatDateTimeStr(parsed.toDate())
}

// ============================================================================
// Parse / Normalize / Serialize
// ============================================================================

function toFiniteNumber(value: unknown, fallback = 0): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

function normalizeBillingMode(value: unknown): SubscriptionBillingMode {
  if (value === 'subscription' || value === 1 || value === '1') {
    return 'subscription'
  }
  return 'metered'
}

/**
 * True when the (possibly backend-raw) billing mode value denotes the
 * subscription mode. The backend serializes the mode as int (0/1) while the
 * frontend uses the string enum, so both are accepted here.
 */
export function isSubscriptionBillingMode(
  value: unknown
): value is SubscriptionBillingMode {
  return normalizeBillingMode(value) === 'subscription'
}

function normalizeModelTiers(value: unknown): SubscriptionBillingModelTier[] {
  if (!Array.isArray(value)) return []
  const tiers: SubscriptionBillingModelTier[] = []
  const seenModels = new Set<string>()
  for (const item of value) {
    if (!item || typeof item !== 'object') continue
    const record = item as Record<string, unknown>
    const model = String(record.model ?? '').trim()
    if (!model || seenModels.has(model)) continue
    seenModels.add(model)
    tiers.push({
      model,
      monthly_usd: toFiniteNumber(record.monthly_usd),
    })
  }
  return tiers
}

/**
 * Raw backend payload for a subscription billing configuration. The backend may
 * return either percent or basis-point fields for the ratios, and either USD or
 * quota values for the monthly total; each pair falls back to the other.
 */
export type SubscriptionBillingRawConfig = Partial<SubscriptionBillingConfig> & {
  billing_mode?: unknown
  monthly_total_quota?: number
  five_hour_ratio_bps?: number
  weekly_ratio_bps?: number
}

export function normalizeSubscriptionBillingConfig(
  config: SubscriptionBillingRawConfig
): SubscriptionBillingConfig {
  const monthlyTotalQuota = toFiniteNumber(config.monthly_total_quota)
  let monthlyTotalUsd: number
  if (
    typeof config.monthly_total_usd === 'number' &&
    Number.isFinite(config.monthly_total_usd)
  ) {
    monthlyTotalUsd = config.monthly_total_usd
  } else if (monthlyTotalQuota > 0) {
    monthlyTotalUsd = quotaToUsd(monthlyTotalQuota)
  } else {
    monthlyTotalUsd = SUBSCRIPTION_DEFAULT_MONTHLY_TOTAL_USD
  }

  let fiveHourPercent: number
  if (
    typeof config.five_hour_ratio_percent === 'number' &&
    Number.isFinite(config.five_hour_ratio_percent)
  ) {
    fiveHourPercent = config.five_hour_ratio_percent
  } else if (
    typeof config.five_hour_ratio_bps === 'number' &&
    Number.isFinite(config.five_hour_ratio_bps)
  ) {
    fiveHourPercent = bpsToPercent(config.five_hour_ratio_bps)
  } else {
    fiveHourPercent = SUBSCRIPTION_DEFAULT_FIVE_HOUR_RATIO_PERCENT
  }
  let weeklyPercent: number
  if (
    typeof config.weekly_ratio_percent === 'number' &&
    Number.isFinite(config.weekly_ratio_percent)
  ) {
    weeklyPercent = config.weekly_ratio_percent
  } else if (
    typeof config.weekly_ratio_bps === 'number' &&
    Number.isFinite(config.weekly_ratio_bps)
  ) {
    weeklyPercent = bpsToPercent(config.weekly_ratio_bps)
  } else {
    weeklyPercent = SUBSCRIPTION_DEFAULT_WEEKLY_RATIO_PERCENT
  }

  return {
    billing_mode: normalizeBillingMode(config.billing_mode),
    monthly_total_usd:
      monthlyTotalUsd > 0
        ? monthlyTotalUsd
        : SUBSCRIPTION_DEFAULT_MONTHLY_TOTAL_USD,
    five_hour_ratio_percent: fiveHourPercent,
    weekly_ratio_percent: weeklyPercent,
    model_tiers: normalizeModelTiers(config.model_tiers),
  }
}

export function createSubscriptionBillingConfig(): SubscriptionBillingConfig {
  return {
    billing_mode: 'metered',
    monthly_total_usd: SUBSCRIPTION_DEFAULT_MONTHLY_TOTAL_USD,
    five_hour_ratio_percent: SUBSCRIPTION_DEFAULT_FIVE_HOUR_RATIO_PERCENT,
    weekly_ratio_percent: SUBSCRIPTION_DEFAULT_WEEKLY_RATIO_PERCENT,
    model_tiers: [],
  }
}

export function parseSubscriptionBillingConfig(
  value: string | null | undefined
): SubscriptionBillingConfig | null {
  if (!value?.trim()) return null
  try {
    const parsed = JSON.parse(value)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return null
    }
    return normalizeSubscriptionBillingConfig(
      parsed as SubscriptionBillingRawConfig
    )
  } catch {
    return null
  }
}

export function stringifySubscriptionBillingConfig(
  config: SubscriptionBillingConfig
): string {
  return JSON.stringify(normalizeSubscriptionBillingConfig(config), null, 2)
}

/**
 * Backend PUT payload for saving the subscription billing config. The backend
 * persists the billing mode as an int enum (0=metered, 1=subscription) and
 * accepts USD/percent inputs which it converts to quota/bps internally.
 */
export type SubscriptionBillingPutPayload = {
  billing_mode: number
  monthly_total_usd: number
  five_hour_ratio_percent: number
  weekly_ratio_percent: number
  model_tiers: Array<{ model: string; monthly_usd: number }>
}

export function toSubscriptionBillingPutPayload(
  config: SubscriptionBillingConfig
): SubscriptionBillingPutPayload {
  return {
    billing_mode: config.billing_mode === 'subscription' ? 1 : 0,
    monthly_total_usd: config.monthly_total_usd,
    five_hour_ratio_percent: config.five_hour_ratio_percent,
    weekly_ratio_percent: config.weekly_ratio_percent,
    model_tiers: config.model_tiers.map((tier) => ({
      model: tier.model,
      monthly_usd: tier.monthly_usd,
    })),
  }
}

// ============================================================================
// Subscription Usage Baseline
// ============================================================================

/**
 * Manual usage baseline form for a subscription channel: 5h/7d/31d 三个窗口各自
 * 独立的已用百分比（允许 >100 表示超限）与起始时间（unix 秒）。保存后各窗口
 * 增量只从各自起点累计，不再重读更早历史日志。
 */
export interface SubscriptionBaselineForm {
  used_percent_5h: number
  used_percent_7d: number
  used_percent_31d: number
  baseline_at_5h: number
  baseline_at_7d: number
  baseline_at_31d: number
}

/** 三窗口的通用 key（与后端窗口名一致）。 */
export const SUBSCRIPTION_BASELINE_WINDOWS = ['5h', '7d', '31d'] as const
export type SubscriptionBaselineWindowKey =
  (typeof SUBSCRIPTION_BASELINE_WINDOWS)[number]

/**
 * Validate the baseline form before saving. Returns an i18n message key on
 * failure or null when valid. Percentages may exceed 100 (over-limit); only
 * negatives / non-finite values and future start times are rejected.
 */
export function validateBaselineForm(
  form: SubscriptionBaselineForm
): string | null {
  const now = Math.floor(Date.now() / 1000)
  for (const window of SUBSCRIPTION_BASELINE_WINDOWS) {
    const percentKey = `used_percent_${window}` as const
    const atKey = `baseline_at_${window}` as const
    const percent = form[percentKey]
    const at = form[atKey]
    if (!Number.isFinite(percent) || percent < 0) {
      return 'Monthly used percentage must be a non-negative number'
    }
    if (!Number.isFinite(at) || at < 0) {
      return 'Billing cycle start must be a valid time'
    }
    if (at > now) {
      return 'Billing cycle start must not be in the future'
    }
  }
  return null
}

/**
 * Window-specific limit in USD for a given subscription config.
 * 5h = monthly × five_hour_ratio%, 7d = monthly × weekly_ratio%, 31d = monthly.
 */
export function deriveWindowLimitUsd(
  window: SubscriptionBaselineWindowKey,
  config: SubscriptionBillingConfig
): number {
  if (window === '5h') return deriveFiveHourUsd(config)
  if (window === '7d') return deriveWeeklyUsd(config)
  return config.monthly_total_usd
}

/**
 * Used percentage → equivalent USD for a specific window's limit.
 * Used as an inline hint under each percentage input.
 */
export function deriveBaselineUsd(
  window: SubscriptionBaselineWindowKey,
  percent: number,
  config: SubscriptionBillingConfig
): number {
  const value = Number(percent)
  if (!Number.isFinite(value) || value <= 0) return 0
  const limit = Number(deriveWindowLimitUsd(window, config))
  if (!Number.isFinite(limit) || limit <= 0) return 0
  return (limit * value) / 100
}

/**
 * Normalize a backend baseline payload into a form value, tolerating missing
 * fields: each window's used_percent defaults to 0, baseline_at defaults to now.
 */
export function toBaselineForm(
  value:
    | {
        used_percent_5h?: number
        used_percent_7d?: number
        used_percent_31d?: number
        baseline_set_at_5h?: number
        baseline_set_at_7d?: number
        baseline_set_at_31d?: number
      }
    | null
    | undefined
): SubscriptionBaselineForm {
  const now = Math.floor(Date.now() / 1000)
  const normalized: SubscriptionBaselineForm = {
    used_percent_5h: 0,
    used_percent_7d: 0,
    used_percent_31d: 0,
    baseline_at_5h: now,
    baseline_at_7d: now,
    baseline_at_31d: now,
  }
  if (!value || typeof value !== 'object') {
    return normalized
  }
  for (const window of SUBSCRIPTION_BASELINE_WINDOWS) {
    const percentKey = `used_percent_${window}` as const
    const atKey = `baseline_set_at_${window}` as const
    const percent = Number(value[percentKey])
    const at = Number(value[atKey])
    const formPercentKey = `used_percent_${window}` as const
    const formAtKey = `baseline_at_${window}` as const
    if (Number.isFinite(percent) && percent >= 0) {
      normalized[formPercentKey] = percent
    }
    if (Number.isFinite(at) && at > 0) {
      normalized[formAtKey] = at
    }
  }
  return normalized
}

// ============================================================================
// Validation
// ============================================================================

/**
 * Validate a config before saving. Returns an i18n message key on failure or
 * null when the config is valid.
 */
export function validateSubscriptionBillingConfig(
  config: SubscriptionBillingConfig | null
): string | null {
  if (!config) {
    return 'Billing mode is required'
  }
  if (
    config.billing_mode !== 'metered' &&
    config.billing_mode !== 'subscription'
  ) {
    return 'Billing mode is required'
  }
  if (!(config.monthly_total_usd > 0)) {
    return 'Monthly total must be greater than 0'
  }
  if (config.monthly_total_usd > SUBSCRIPTION_MAX_MONTHLY_USD) {
    return 'Monthly total is too large'
  }
  if (
    !(config.five_hour_ratio_percent >= 0) ||
    !(config.five_hour_ratio_percent <= 100)
  ) {
    return 'Five-hour ratio must be between 0 and 100'
  }
  if (
    !(config.weekly_ratio_percent >= 0) ||
    !(config.weekly_ratio_percent <= 100)
  ) {
    return 'Weekly ratio must be between 0 and 100'
  }
  if (
    config.five_hour_ratio_percent + config.weekly_ratio_percent >
    SUBSCRIPTION_MONTHLY_RATIO_PERCENT
  ) {
    return 'Five-hour and weekly ratios cannot exceed 100% in total'
  }

  const seenModels = new Set<string>()
  for (const tier of config.model_tiers) {
    if (!tier.model.trim()) {
      return 'Model name is required'
    }
    if (seenModels.has(tier.model.trim())) {
      return 'Model names must be unique'
    }
    seenModels.add(tier.model.trim())
    if (!(tier.monthly_usd >= 0)) {
      return 'Model monthly quota must be greater than or equal to 0'
    }
  }

  return null
}
