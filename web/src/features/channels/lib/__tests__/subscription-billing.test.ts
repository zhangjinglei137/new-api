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
import { describe, expect, test } from 'vitest'

import {
  SUBSCRIPTION_DANGER_PERCENT,
  SUBSCRIPTION_DEFAULT_FIVE_HOUR_RATIO_PERCENT,
  SUBSCRIPTION_DEFAULT_MONTHLY_TOTAL_USD,
  SUBSCRIPTION_DEFAULT_WEEKLY_RATIO_PERCENT,
  SUBSCRIPTION_QUOTA_PER_USD,
  SUBSCRIPTION_WARNING_PERCENT,
  bpsToPercent,
  createSubscriptionBillingConfig,
  deriveBaselineUsd,
  deriveFiveHourUsd,
  deriveWeeklyUsd,
  formatSubscriptionUsageUpdatedAt,
  getSubscriptionPercentVariant,
  getSubscriptionWindowPercent,
  isSubscriptionBillingMode,
  normalizeSubscriptionBillingConfig,
  parseSubscriptionBillingConfig,
  percentToBps,
  quotaToUsd,
  stringifySubscriptionBillingConfig,
  toBaselineForm,
  toSubscriptionBillingPutPayload,
  usdToQuota,
  validateBaselineForm,
  validateSubscriptionBillingConfig,
  type SubscriptionBillingRawConfig,
} from '../subscription-billing'
import type { SubscriptionBillingConfig } from '../../types'

function validConfig(): SubscriptionBillingConfig {
  return {
    billing_mode: 'subscription',
    monthly_total_usd: 60,
    five_hour_ratio_percent: 20,
    weekly_ratio_percent: 50,
    model_tiers: [{ model: '*', monthly_usd: 60 }],
  }
}

describe('subscription billing unit conversions', () => {
  test('quota and USD convert with a 500000 quota per USD unit', () => {
    expect(quotaToUsd(SUBSCRIPTION_QUOTA_PER_USD * 60)).toBe(60)
    expect(usdToQuota(60)).toBe(SUBSCRIPTION_QUOTA_PER_USD * 60)
    expect(quotaToUsd(0)).toBe(0)
    expect(quotaToUsd(-5)).toBe(0)
    expect(usdToQuota(Number.NaN)).toBe(0)
    expect(usdToQuota(undefined)).toBe(0)
  })

  test('percent and basis points convert round-trip', () => {
    expect(percentToBps(20)).toBe(2000)
    expect(bpsToPercent(2000)).toBe(20)
    expect(bpsToPercent(1250)).toBe(12.5)
    expect(percentToBps(Number.NaN)).toBe(0)
    expect(bpsToPercent(Number.NaN)).toBe(0)
  })
})

describe('subscription billing derived limits', () => {
  test('derives 5-hour and weekly limits from the monthly total and ratios', () => {
    const config = validConfig()
    expect(deriveFiveHourUsd(config)).toBe(12)
    expect(deriveWeeklyUsd(config)).toBe(30)
  })

  test('returns zero derived limits for a zero monthly total', () => {
    const config = { ...validConfig(), monthly_total_usd: 0 }
    expect(deriveFiveHourUsd(config)).toBe(0)
    expect(deriveWeeklyUsd(config)).toBe(0)
  })
})

describe('subscription billing percent thresholds', () => {
  test('flags warning at and above 80% and danger at and above 95%', () => {
    expect(getSubscriptionPercentVariant(0)).toBe('info')
    expect(getSubscriptionPercentVariant(79)).toBe('info')
    expect(getSubscriptionPercentVariant(SUBSCRIPTION_WARNING_PERCENT)).toBe(
      'warning'
    )
    expect(getSubscriptionPercentVariant(94)).toBe('warning')
    expect(getSubscriptionPercentVariant(SUBSCRIPTION_DANGER_PERCENT)).toBe(
      'danger'
    )
    expect(getSubscriptionPercentVariant(Number.NaN)).toBe('info')
  })

  test('prefers display_percent over used_percent for windows', () => {
    expect(
      getSubscriptionWindowPercent({ display_percent: 42, used_percent: 10 })
    ).toBe(42)
    expect(getSubscriptionWindowPercent({ used_percent: 10 })).toBe(10)
    expect(getSubscriptionWindowPercent(null)).toBe(0)
  })
})

describe('subscription billing config normalization', () => {
  test('prefers percent fields and falls back to basis points', () => {
    const config = normalizeSubscriptionBillingConfig({
      billing_mode: 'subscription',
      monthly_total_usd: 60,
      five_hour_ratio_percent: 25,
      five_hour_ratio_bps: 2000,
      weekly_ratio_percent: 55,
      weekly_ratio_bps: 5000,
      model_tiers: [{ model: '*', monthly_usd: 60 }],
    })
    expect(config.five_hour_ratio_percent).toBe(25)
    expect(config.weekly_ratio_percent).toBe(55)
  })

  test('falls back to basis points and monthly quota when USD fields are absent', () => {
    const config = normalizeSubscriptionBillingConfig({
      billing_mode: 'subscription',
      monthly_total_quota: SUBSCRIPTION_QUOTA_PER_USD * 30,
      five_hour_ratio_bps: 2000,
      weekly_ratio_bps: 5000,
    })
    expect(config.monthly_total_usd).toBe(30)
    expect(config.five_hour_ratio_percent).toBe(20)
    expect(config.weekly_ratio_percent).toBe(50)
  })

  test('applies defaults for a config missing every value', () => {
    const config = normalizeSubscriptionBillingConfig({})
    expect(config.billing_mode).toBe('metered')
    expect(config.monthly_total_usd).toBe(SUBSCRIPTION_DEFAULT_MONTHLY_TOTAL_USD)
    expect(config.five_hour_ratio_percent).toBe(
      SUBSCRIPTION_DEFAULT_FIVE_HOUR_RATIO_PERCENT
    )
    expect(config.weekly_ratio_percent).toBe(
      SUBSCRIPTION_DEFAULT_WEEKLY_RATIO_PERCENT
    )
    expect(config.model_tiers).toStrictEqual([])
  })

  test('drops duplicate model tiers and empty model names', () => {
    const tiers = [
      { model: 'gpt-4o', monthly_usd: 20 },
      { model: ' gpt-4o ', monthly_usd: 30 },
      { model: 'claude', monthly_usd: 'bad' },
      { model: '' },
    ]
    const config = normalizeSubscriptionBillingConfig({
      model_tiers: tiers,
    } as unknown as SubscriptionBillingRawConfig)
    // Invalid numeric quotas are normalized to 0 and kept; the empty model
    // name and the duplicate (whitespace-trimmed) model are dropped.
    expect(config.model_tiers).toStrictEqual([
      { model: 'gpt-4o', monthly_usd: 20 },
      { model: 'claude', monthly_usd: 0 },
    ])
  })
})

describe('subscription billing config serialize/parse', () => {
  test('round-trips through JSON', () => {
    const config = validConfig()
    const parsed = parseSubscriptionBillingConfig(
      stringifySubscriptionBillingConfig(config)
    )
    expect(parsed).toStrictEqual(config)
  })

  test('returns null for invalid JSON and for empty input', () => {
    expect(parseSubscriptionBillingConfig('')).toBeNull()
    expect(parseSubscriptionBillingConfig('{bad json')).toBeNull()
    expect(parseSubscriptionBillingConfig('[]')).toBeNull()
  })

  test('createSubscriptionBillingConfig applies defaults', () => {
    expect(createSubscriptionBillingConfig()).toStrictEqual({
      billing_mode: 'metered',
      monthly_total_usd: SUBSCRIPTION_DEFAULT_MONTHLY_TOTAL_USD,
      five_hour_ratio_percent: SUBSCRIPTION_DEFAULT_FIVE_HOUR_RATIO_PERCENT,
      weekly_ratio_percent: SUBSCRIPTION_DEFAULT_WEEKLY_RATIO_PERCENT,
      model_tiers: [],
    })
  })
})

describe('subscription billing config validation', () => {
  test('accepts a valid subscription config', () => {
    expect(validateSubscriptionBillingConfig(validConfig())).toBeNull()
  })

  test('rejects invalid or missing billing mode', () => {
    expect(
      validateSubscriptionBillingConfig({
        ...validConfig(),
        billing_mode: 'other' as never,
      })
    ).toBe('Billing mode is required')
    expect(validateSubscriptionBillingConfig(null)).toBe(
      'Billing mode is required'
    )
  })

  test('rejects non-positive or oversized monthly totals', () => {
    expect(
      validateSubscriptionBillingConfig({ ...validConfig(), monthly_total_usd: 0 })
    ).toBe('Monthly total must be greater than 0')
    expect(
      validateSubscriptionBillingConfig({ ...validConfig(), monthly_total_usd: -1 })
    ).toBe('Monthly total must be greater than 0')
    expect(
      validateSubscriptionBillingConfig({ ...validConfig(), monthly_total_usd: 1000001 })
    ).toBe('Monthly total is too large')
  })

  test('rejects out-of-range ratios and totals above 100%', () => {
    expect(
      validateSubscriptionBillingConfig({ ...validConfig(), five_hour_ratio_percent: 101 })
    ).toBe('Five-hour ratio must be between 0 and 100')
    expect(
      validateSubscriptionBillingConfig({ ...validConfig(), weekly_ratio_percent: -5 })
    ).toBe('Weekly ratio must be between 0 and 100')
    expect(
      validateSubscriptionBillingConfig({
        ...validConfig(),
        five_hour_ratio_percent: 60,
        weekly_ratio_percent: 50,
      })
    ).toBe('Five-hour and weekly ratios cannot exceed 100% in total')
  })

  test('rejects empty, duplicate, or negative model tiers', () => {
    expect(
      validateSubscriptionBillingConfig({
        ...validConfig(),
        model_tiers: [{ model: '  ', monthly_usd: 10 }],
      })
    ).toBe('Model name is required')
    expect(
      validateSubscriptionBillingConfig({
        ...validConfig(),
        model_tiers: [
          { model: 'gpt-4o', monthly_usd: 10 },
          { model: 'gpt-4o', monthly_usd: 20 },
        ],
      })
    ).toBe('Model names must be unique')
    expect(
      validateSubscriptionBillingConfig({
        ...validConfig(),
        model_tiers: [{ model: 'gpt-4o', monthly_usd: -1 }],
      })
    ).toBe('Model monthly quota must be greater than or equal to 0')
  })
})

describe('subscription usage updated_at formatting', () => {
  test('formats unix seconds and milliseconds timestamps', () => {
    const date = new Date('2026-01-02T03:04:05Z')
    const seconds = Math.floor(date.getTime() / 1000)
    const formatted = formatSubscriptionUsageUpdatedAt(seconds)
    expect(formatted).not.toBe('-')
    expect(formatted).toBe(formatSubscriptionUsageUpdatedAt(date.getTime()))
  })

  test('falls back for empty, invalid, and non-positive values', () => {
    expect(formatSubscriptionUsageUpdatedAt(undefined)).toBe('-')
    expect(formatSubscriptionUsageUpdatedAt('')).toBe('-')
    expect(formatSubscriptionUsageUpdatedAt(0)).toBe('-')
    expect(formatSubscriptionUsageUpdatedAt('not-a-date')).toBe('-')
  })
})

describe('subscription billing mode normalization', () => {
  test('treats backend int and string subscription markers as subscription', () => {
    expect(
      normalizeSubscriptionBillingConfig({
        billing_mode: 1,
        monthly_total_quota: 30000000,
      } as SubscriptionBillingRawConfig).billing_mode
    ).toBe('subscription')
    expect(
      normalizeSubscriptionBillingConfig({
        billing_mode: 'subscription',
      } as SubscriptionBillingRawConfig).billing_mode
    ).toBe('subscription')
  })

  test('treats backend int 0 and string metered as metered', () => {
    expect(
      normalizeSubscriptionBillingConfig({
        billing_mode: 0,
        monthly_total_quota: 30000000,
      } as SubscriptionBillingRawConfig).billing_mode
    ).toBe('metered')
    expect(
      normalizeSubscriptionBillingConfig({
        billing_mode: 'metered',
      } as SubscriptionBillingRawConfig).billing_mode
    ).toBe('metered')
    expect(
      normalizeSubscriptionBillingConfig({
        billing_mode: undefined,
      } as SubscriptionBillingRawConfig).billing_mode
    ).toBe('metered')
  })

  test('isSubscriptionBillingMode accepts both int and string forms', () => {
    expect(isSubscriptionBillingMode('subscription')).toBe(true)
    expect(isSubscriptionBillingMode(1)).toBe(true)
    expect(isSubscriptionBillingMode('metered')).toBe(false)
    expect(isSubscriptionBillingMode(0)).toBe(false)
    expect(isSubscriptionBillingMode(undefined)).toBe(false)
  })
})

describe('subscription billing PUT payload', () => {
  test('serializes billing mode and USD fields for the backend', () => {
    const payload = toSubscriptionBillingPutPayload(validConfig())
    expect(payload).toEqual({
      billing_mode: 1,
      monthly_total_usd: 60,
      five_hour_ratio_percent: 20,
      weekly_ratio_percent: 50,
      model_tiers: [{ model: '*', monthly_usd: 60 }],
    })
  })

  test('maps metered mode to int 0', () => {
    const payload = toSubscriptionBillingPutPayload({
      ...validConfig(),
      billing_mode: 'metered',
    })
    expect(payload.billing_mode).toBe(0)
  })
})

describe('subscription usage baseline form', () => {
  const now = Math.floor(Date.now() / 1000)

  test('accepts a valid baseline, including over-limit percentages', () => {
    expect(
      validateBaselineForm({ used_percent: 30, baseline_at: now })
    ).toBeNull()
    expect(
      validateBaselineForm({ used_percent: 200, baseline_at: now })
    ).toBeNull()
    expect(
      validateBaselineForm({ used_percent: 0, baseline_at: now })
    ).toBeNull()
  })

  test('rejects negative, non-finite and future-start baselines', () => {
    expect(
      validateBaselineForm({ used_percent: -1, baseline_at: now })
    ).toBe('Monthly used percentage must be a non-negative number')
    expect(
      validateBaselineForm({ used_percent: Number.NaN, baseline_at: now })
    ).toBe('Monthly used percentage must be a non-negative number')
    expect(
      validateBaselineForm({ used_percent: 30, baseline_at: now + 3600 })
    ).toBe('Billing cycle start must not be in the future')
    expect(
      validateBaselineForm({ used_percent: 30, baseline_at: -1 })
    ).toBe('Billing cycle start must be a valid time')
  })

  test('derives the equivalent USD of a monthly percentage', () => {
    expect(deriveBaselineUsd(30, 60)).toBe(18)
    expect(deriveBaselineUsd(0, 60)).toBe(0)
    expect(deriveBaselineUsd(150, 60)).toBe(90)
    expect(deriveBaselineUsd(Number.NaN, 60)).toBe(0)
    expect(deriveBaselineUsd(30, 0)).toBe(0)
  })

  test('normalizes a backend baseline into a form, tolerating missing fields', () => {
    const form = toBaselineForm({ used_percent: 30, baseline_set_at: now - 100 })
    expect(form).toStrictEqual({ used_percent: 30, baseline_at: now - 100 })

    const defaults = toBaselineForm(undefined)
    expect(defaults.used_percent).toBe(0)
    expect(defaults.baseline_at).toBeGreaterThan(0)

    const missingPercent = toBaselineForm({ baseline_set_at: now })
    expect(missingPercent.used_percent).toBe(0)
  })
})
