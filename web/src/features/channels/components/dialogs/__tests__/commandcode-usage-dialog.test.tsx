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
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import dayjs from '@/lib/dayjs'
import { formatDateTimeStr } from '@/lib/format'

import {
  CommandCodeUsageDialog,
  type CommandCodeUsageResponse,
} from '../commandcode-usage-dialog'

function renderDialog(response: CommandCodeUsageResponse | null) {
  render(
    <CommandCodeUsageDialog
      open
      onOpenChange={() => undefined}
      channelName='Command Code'
      channelId={98}
      response={response}
    />
  )
}

const FOUR_WINDOW_RESPONSE: CommandCodeUsageResponse = {
  success: true,
  data: {
    // Intentionally out of order: the dialog must render session → weekly →
    // monthly → topup.
    windows: [
      {
        period: 'monthly',
        status: 'ok',
        used_percent: 12.22,
        remaining_percent: 87.78,
        used: 1.2216,
        limit: 10,
        reset_at: '2026-09-15T04:42:16.000Z',
        metered: true,
      },
      {
        period: 'topup',
        status: 'ok',
        used_percent: -1,
        remaining_percent: -1,
        used: 1.2216,
        limit: 3,
        reset_at: '',
        metered: false,
      },
      {
        period: 'session',
        status: 'ok',
        used_percent: 40.72,
        remaining_percent: 59.28,
        used: 1.2216,
        limit: 3,
        reset_at: '2026-08-14T10:00:00.000Z',
        metered: true,
      },
      {
        period: 'weekly',
        status: 'ok',
        used_percent: 20.36,
        remaining_percent: 79.64,
        used: 1.2216,
        limit: 6,
        reset_at: '2026-08-17T00:00:00.000Z',
        metered: true,
      },
    ],
  },
}

describe('CommandCodeUsageDialog', () => {
  test('renders all four windows in fixed order with two-decimal used percentages', () => {
    renderDialog(FOUR_WINDOW_RESPONSE)

    expect(screen.getByText('Command Code Usage')).toBeInTheDocument()

    const titles = screen
      .getAllByText(/Last 5 hours|Weekly|Monthly|Top-up credits/)
      .map((el) => el.textContent)
    expect(titles).toStrictEqual([
      'Last 5 hours',
      'Weekly',
      'Monthly',
      'Top-up credits',
    ])

    // Two-decimal formatting for used percentages (big number per window).
    expect(screen.getAllByText('40.72%')).toHaveLength(1)
    expect(screen.getAllByText('20.36%')).toHaveLength(1)
    expect(screen.getAllByText('12.22%')).toHaveLength(1)
    // Remaining percentages are no longer surfaced.
    expect(screen.queryByText('59.28%')).not.toBeInTheDocument()
    expect(screen.queryByText('79.64%')).not.toBeInTheDocument()
    expect(screen.queryByText('87.78%')).not.toBeInTheDocument()
  })

  test('renders the reset time as a concrete timestamp for metered windows', () => {
    renderDialog(FOUR_WINDOW_RESPONSE)

    const windows = FOUR_WINDOW_RESPONSE.data?.windows ?? []
    for (const w of windows) {
      if (w.period === 'topup') {
        continue
      }
      const expected = formatDateTimeStr(dayjs(w.reset_at).toDate())
      expect(screen.getByText(expected)).toBeInTheDocument()
    }
    // Raw ISO strings are not shown, and the topup window has no reset time.
    expect(screen.queryByText('2026-08-14T10:00:00.000Z')).not.toBeInTheDocument()
    expect(screen.queryByText('Resets in:')).not.toBeInTheDocument()
  })

  test('shows the used amount against the limit for metered windows', () => {
    renderDialog(FOUR_WINDOW_RESPONSE)

    // Used row renders "$used / $limit" (default currency is USD in tests;
    // trailing zeros are trimmed by the currency formatter).
    expect(screen.getByText('$1.22 / $3')).toBeInTheDocument()
    expect(screen.getByText('$1.22 / $6')).toBeInTheDocument()
    expect(screen.getByText('$1.22 / $10')).toBeInTheDocument()
  })

  test('shows the remaining balance as the big number for the non-metered topup window', () => {
    renderDialog(FOUR_WINDOW_RESPONSE)

    // topup is metered:false with used_percent -1 → remaining USD amount,
    // not a percentage and no progress bar.
    expect(screen.getByText('Top-up credits')).toBeInTheDocument()
    expect(screen.getByText('$1.78')).toBeInTheDocument()
    expect(screen.queryByText('-1.00%')).not.toBeInTheDocument()
    expect(
      screen.queryByText('Top-up credits: -1.00%')
    ).not.toBeInTheDocument()
  })

  test('treats -1 sentinel percentages as missing even when metered', () => {
    renderDialog({
      success: true,
      data: {
        windows: [
          {
            period: 'session',
            status: 'ok',
            used_percent: -1,
            remaining_percent: -1,
            used: -1,
            limit: -1,
            reset_at: '',
            metered: true,
          },
        ],
      },
    })

    expect(screen.getByText('Last 5 hours')).toBeInTheDocument()
    expect(screen.queryByText('0.00%')).not.toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBeGreaterThan(0)
  })

  test('renders only the windows present in the response', () => {
    renderDialog({
      success: true,
      data: {
        windows: [
          {
            period: 'monthly',
            status: 'ok',
            used_percent: 50,
            remaining_percent: 50,
            used: 5,
            limit: 10,
            reset_at: '2026-09-01T00:00:00Z',
            metered: true,
          },
        ],
      },
    })

    expect(screen.getByText('Monthly')).toBeInTheDocument()
    expect(screen.getAllByText('50.00%')).toHaveLength(1)
    expect(screen.queryByText('Last 5 hours')).not.toBeInTheDocument()
    expect(screen.queryByText('Weekly')).not.toBeInTheDocument()
    expect(screen.queryByText('Top-up credits')).not.toBeInTheDocument()
  })

  test('shows a degraded message plus raw panel when windows are empty', () => {
    renderDialog({ success: true, data: { windows: [] } })

    expect(screen.getByText('Unable to identify usage data')).toBeInTheDocument()
    expect(
      screen.getByText(
        'The upstream response did not include recognizable usage windows.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByText('Show raw upstream response')
    ).toBeInTheDocument()
  })

  test('shows the upstream error message on failure', () => {
    renderDialog({
      success: false,
      message: 'commandcode cookie 未配置',
    })

    expect(screen.getByText('Unable to identify usage data')).toBeInTheDocument()
    expect(
      screen.getByText('commandcode cookie 未配置')
    ).toBeInTheDocument()
  })

  test('omits the raw JSON panel when the query failed', () => {
    renderDialog({
      success: false,
      message: 'commandcode cookie 未配置',
    })

    expect(
      screen.queryByText('Show raw upstream response')
    ).not.toBeInTheDocument()
  })

  test('shows a refresh button when a refresh handler is provided', () => {
    render(
      <CommandCodeUsageDialog
        open
        onOpenChange={() => undefined}
        response={{ success: true, data: { windows: [] } }}
        onRefresh={() => undefined}
      />
    )

    expect(screen.getByText('Refresh usage')).toBeInTheDocument()
  })
})
