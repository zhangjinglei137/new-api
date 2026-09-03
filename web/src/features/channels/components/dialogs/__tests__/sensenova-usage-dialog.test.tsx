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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import dayjs from '@/lib/dayjs'
import { formatDateTimeStr } from '@/lib/format'

import type { SenseNovaUsagePool, SenseNovaUsageWindow } from '../../../api'
import {
  SenseNovaUsageDialog,
  type SenseNovaUsageResponse,
} from '../sensenova-usage-dialog'

function renderDialog(response: SenseNovaUsageResponse | null, onRefresh?: () => void) {
  render(
    <SenseNovaUsageDialog
      open
      onOpenChange={() => undefined}
      channelName='SenseNova'
      channelId={97}
      response={response}
      onRefresh={onRefresh}
    />
  )
}

const GENERAL_POOL: SenseNovaUsagePool = {
  pool_type: 'general',
  name: 'Free Pool',
  model_ids: ['SenseNova-5', 'SenseNova-Turbo', 'SenseNova-Lite', 'SenseNova-Max'],
  window_5h: {
    limit: 1000,
    used: 250,
    remaining: 750,
    reset_at: '2026-08-14T10:00:00.000Z',
  },
  window_7d: {
    limit: 5000,
    used: 1000,
    remaining: 4000,
    reset_at: '2026-08-17T00:00:00.000Z',
  },
  grant_balance: 100,
  nearest_grant_expiry: '2026-12-31T00:00:00.000Z',
  nearest_grant_expiring_balance: 50,
}

const DEDICATED_POOL: SenseNovaUsagePool = {
  pool_type: 'dedicated',
  name: 'Dedicated Pool',
  model_ids: ['SenseNova-5'],
  window_5h: {
    limit: 200,
    used: 40,
    remaining: 160,
    reset_at: '2026-08-14T10:00:00.000Z',
  },
  window_7d: null,
  grant_balance: 0,
}

const TWO_POOL_RESPONSE: SenseNovaUsageResponse = {
  success: true,
  data: {
    plan: { id: 'pro', name: 'SenseNova Pro' },
    pools: [GENERAL_POOL, DEDICATED_POOL],
  },
}

describe('SenseNovaUsageDialog', () => {
  test('renders a card per pool with name and a type badge distinguishing dedicated from general', () => {
    renderDialog(TWO_POOL_RESPONSE)

    expect(screen.getByText('Free Pool')).toBeInTheDocument()
    expect(screen.getByText('Dedicated Pool')).toBeInTheDocument()

    const generalBadge = screen.getByText('General pool')
    const dedicatedBadge = screen.getByText('Dedicated pool')
    // general pool uses the outline badge, dedicated uses the secondary badge
    expect(generalBadge.className).toContain('border-border')
    expect(dedicatedBadge.className).toContain('bg-secondary')

    // weekly balance comes from the 7-day window remaining amount; "4,000"
    // appears both as the weekly balance and inside the weekly window card
    expect(screen.getAllByText('4,000').length).toBe(2)
  })

  test('formats the 5h and weekly windows with separators, one-decimal percent and reset time, and no monthly window', () => {
    renderDialog({ success: true, data: { pools: [GENERAL_POOL] } })

    expect(screen.getByText('5h window')).toBeInTheDocument()
    expect(screen.getByText('Weekly window')).toBeInTheDocument()

    // remaining / limit with thousand separators; the amount is rendered as
    // the big number ("750") followed by a lighter "/ 1,000" suffix span
    expect(screen.getByText('750')).toBeInTheDocument()
    expect(screen.getByText('/ 1,000')).toBeInTheDocument()
    expect(screen.getByText('/ 5,000')).toBeInTheDocument()

    // one-decimal remaining percentage
    expect(screen.getByText('75.0%')).toBeInTheDocument()
    expect(screen.getByText('80.0%')).toBeInTheDocument()

    // concrete reset timestamps, not raw ISO strings
    const fiveHourReset = formatDateTimeStr(
      dayjs('2026-08-14T10:00:00.000Z').toDate()
    )
    const weeklyReset = formatDateTimeStr(
      dayjs('2026-08-17T00:00:00.000Z').toDate()
    )
    expect(screen.getByText(`Resets at: ${fiveHourReset}`)).toBeInTheDocument()
    expect(screen.getByText(`Resets at: ${weeklyReset}`)).toBeInTheDocument()

    // the dialog exposes no monthly window
    expect(screen.queryByText('Monthly')).not.toBeInTheDocument()
  })

  test('renders the activity credits section only when grant balance is positive', () => {
    renderDialog(TWO_POOL_RESPONSE)

    // general pool has a grant balance of 100 and an expiring balance of 50
    expect(screen.getByText('Activity credits')).toBeInTheDocument()
    expect(screen.getByText(/Total balance:/)).toBeInTheDocument()
    expect(screen.getByText('100')).toBeInTheDocument()
    const expectedExpiry = formatDateTimeStr(
      dayjs('2026-12-31T00:00:00.000Z').toDate()
    )
    expect(
      screen.getByText(`Next expiry: ${expectedExpiry} · 50`)
    ).toBeInTheDocument()

    // only the general pool (grant_balance 100) shows an activity credits
    // section; the dedicated pool with grant_balance 0 does not
    expect(screen.getAllByText('Activity credits').length).toBe(1)
  })

  test('omits the activity credits section when grant balance is zero or missing', () => {
    renderDialog({
      success: true,
      data: {
        pools: [
          {
            pool_type: 'general',
            name: 'No Grant Pool',
            window_5h: { limit: 10, used: 2, remaining: 8 },
            window_7d: { limit: 10, used: 2, remaining: 8 },
            grant_balance: 0,
            nearest_grant_expiry: null,
          },
          {
            pool_type: 'general',
            name: 'Missing Grant Pool',
            window_5h: { limit: 10, used: 2, remaining: 8 },
            window_7d: { limit: 10, used: 2, remaining: 8 },
          },
        ],
      },
    })

    expect(screen.queryByText('Activity credits')).not.toBeInTheDocument()
    expect(screen.queryByText(/Total balance:/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Next expiry:/)).not.toBeInTheDocument()
  })

  test('flags the next expiry with a warning style when it expires within 7 days', () => {
    renderDialog({
      success: true,
      data: {
        pools: [
          {
            pool_type: 'general',
            name: 'Expiring Pool',
            window_5h: { limit: 10, used: 2, remaining: 8 },
            window_7d: null,
            grant_balance: 30,
            nearest_grant_expiry: dayjs().add(3, 'day').toISOString(),
            nearest_grant_expiring_balance: 10,
          },
        ],
      },
    })

    const expiryRow = screen.getByText(/Next expiry:/)
    expect(expiryRow.className).toContain('warning')
  })

  test('does not flag a far-future expiry with the warning style', () => {
    renderDialog({
      success: true,
      data: {
        pools: [
          {
            pool_type: 'general',
            name: 'Safe Pool',
            window_5h: { limit: 10, used: 2, remaining: 8 },
            window_7d: null,
            grant_balance: 30,
            nearest_grant_expiry: '2027-12-31T00:00:00.000Z',
            nearest_grant_expiring_balance: 10,
          },
        ],
      },
    })

    const expiryRow = screen.getByText(/Next expiry:/)
    expect(expiryRow.className).not.toContain('warning')
  })

  test('shows the dedicated pool reward banner when a dedicated pool exists', () => {
    renderDialog(TWO_POOL_RESPONSE)

    expect(
      screen.getByText('Using a dedicated pool earns rewards for the general pool.')
    ).toBeInTheDocument()
  })

  test('omits the dedicated pool reward banner when only general pools exist', () => {
    renderDialog({
      success: true,
      data: {
        pools: [
          {
            pool_type: 'general',
            name: 'Only General Pool',
            window_5h: { limit: 10, used: 2, remaining: 8 },
            window_7d: null,
          },
        ],
      },
    })

    expect(
      screen.queryByText('Using a dedicated pool earns rewards for the general pool.')
    ).not.toBeInTheDocument()
  })

  test('branches the failure title on login_failed and passes the backend message through', () => {
    // login_failed -> login-failed title; the backend message is passed
    // through verbatim (asserted by substring, not the full message).
    renderDialog({
      success: false,
      error_code: 'login_failed',
      message: 'SenseNova 登录失败，请检查账号',
    })
    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('SenseNova login failed')).toBeInTheDocument()
    expect(screen.getByText(/请检查账号/)).toBeInTheDocument()
  })

  test('maps credentials_not_configured and credentials_expired to the credential title', () => {
    renderDialog({
      success: false,
      error_code: 'credentials_not_configured',
      message: 'SenseNova 账号未配置',
    })
    expect(screen.getByText('SenseNova credentials not configured')).toBeInTheDocument()

    renderDialog({
      success: false,
      error_code: 'credentials_expired',
      message: 'SenseNova 凭证已失效',
    })
    expect(
      screen.getAllByText('SenseNova credentials not configured')
    ).toHaveLength(2)
  })

  test('falls back to the generic copy for unknown error codes', () => {
    renderDialog({ success: false, error_code: 'none', message: 'upstream exploded' })
    expect(screen.getByText('Unable to load SenseNova usage')).toBeInTheDocument()
  })

  test('renders the failure alert with the destructive style', () => {
    renderDialog({
      success: false,
      error_code: 'login_failed',
      message: 'SenseNova 登录失败',
    })

    const alert = screen.getByRole('alert')
    expect(alert.className).toContain('destructive')
    // the raw JSON panel must not appear for a failed query
    expect(screen.queryByText('Show raw upstream response')).not.toBeInTheDocument()
  })

  test('shows a degraded alert when the query succeeded but no pools are present', () => {
    renderDialog({ success: true, data: { pools: [] } })

    expect(screen.getByText('Unable to identify usage data')).toBeInTheDocument()
    expect(
      screen.getByText('SenseNova returned no pool data.')
    ).toBeInTheDocument()
  })

  test('shows the loading state while the response is still null', () => {
    renderDialog(null)

    expect(screen.getByText('Loading SenseNova usage...')).toBeInTheDocument()
    expect(screen.queryByText('Free Pool')).not.toBeInTheDocument()
  })

  test('expands the hidden model list and switches to the hide label', () => {
    renderDialog({ success: true, data: { pools: [GENERAL_POOL] } })

    // 4 models in the pool, 3 previewed, 1 collapsed behind the toggle
    const toggle = screen.getByText('+1 more models')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('SenseNova-Max')).not.toBeInTheDocument()

    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('SenseNova-Max')).toBeInTheDocument()
    expect(screen.getByText('Hide models')).toBeInTheDocument()
  })

  test('triggers onRefresh from the refresh button', () => {
    const onRefresh = vi.fn()
    renderDialog(TWO_POOL_RESPONSE, onRefresh)

    fireEvent.click(screen.getByText('Refresh usage'))
    expect(onRefresh).toHaveBeenCalledTimes(1)
  })

  test('expands the raw upstream response panel on click', () => {
    renderDialog(TWO_POOL_RESPONSE)

    const trigger = screen.getByText('Show raw upstream response')
    expect(screen.queryByText(/"pool_type"/)).not.toBeInTheDocument()

    fireEvent.click(trigger)
    expect(screen.getByText(/"pool_type"/)).toBeInTheDocument()
  })

  test('renders "-" for missing or invalid window fields instead of crashing', () => {
    // Deliberately invalid values that bypass the number types at runtime.
    const brokenWindow = {
      limit: 'abc',
      used: 'xyz',
      remaining: 'oops',
      reset_at: 'garbage',
    } as unknown as SenseNovaUsageWindow
    renderDialog({
      success: true,
      data: {
        pools: [
          {
            pool_type: 'general',
            name: 'Broken Pool',
            window_5h: brokenWindow,
            window_7d: {},
          },
        ],
      },
    })

    expect(screen.getByText('Broken Pool')).toBeInTheDocument()
    expect(screen.getAllByText('Usage percentage unavailable').length).toBe(2)
    expect(screen.getAllByText('Resets at: -').length).toBe(2)
    expect(screen.getAllByText('-').length).toBeGreaterThan(0)
  })
})
