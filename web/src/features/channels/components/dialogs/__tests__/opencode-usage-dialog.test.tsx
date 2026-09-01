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
  OpenCodeGoUsageDialog,
  type OpenCodeGoUsageResponse,
} from '../opencode-usage-dialog'

function renderDialog(response: OpenCodeGoUsageResponse | null) {
  render(
    <OpenCodeGoUsageDialog
      open
      onOpenChange={() => undefined}
      channelName='OpenCode Go'
      channelId={7}
      response={response}
    />
  )
}

const THREE_WINDOW_RESPONSE: OpenCodeGoUsageResponse = {
  success: true,
  data: {
    // Intentionally out of order: the dialog must render session → weekly → monthly.
    windows: [
      {
        period: 'monthly',
        status: 'ok',
        used_percent: 65.8,
        remaining_percent: 34.2,
        reset_in_sec: 1615573,
        reset_at: '2026-09-13T06:06:01.287Z',
      },
      {
        period: 'session',
        status: 'ok',
        used_percent: 23.6,
        remaining_percent: 76.4,
        reset_in_sec: 3811,
        reset_at: '2026-09-01T16:27:38.287Z',
      },
      {
        period: 'weekly',
        status: 'ok',
        used_percent: 81.2,
        remaining_percent: 18.8,
        reset_in_sec: 397146,
        reset_at: '2026-09-07T00:00:00.287Z',
      },
    ],
  },
}

describe('OpenCodeGoUsageDialog', () => {
  test('renders all three windows in fixed order with two-decimal used percentages', () => {
    renderDialog(THREE_WINDOW_RESPONSE)

    expect(screen.getByText('OpenCode Go Usage')).toBeInTheDocument()

    const titles = screen
      .getAllByText(/Last 5 hours|Weekly|Monthly/)
      .map((el) => el.textContent)
    expect(titles).toStrictEqual(['Last 5 hours', 'Weekly', 'Monthly'])

    // Two-decimal formatting for used percentages (big number + Used row).
    expect(screen.getAllByText('23.60%')).toHaveLength(2)
    expect(screen.getAllByText('81.20%')).toHaveLength(2)
    expect(screen.getAllByText('65.80%')).toHaveLength(2)
    // Remaining percentages are no longer surfaced.
    expect(screen.queryByText('76.40%')).not.toBeInTheDocument()
  })

  test('renders the reset time as a concrete timestamp for each window', () => {
    renderDialog(THREE_WINDOW_RESPONSE)

    // reset_at is formatted as a concrete YYYY-MM-DD HH:mm:ss timestamp in the
    // local timezone (same formatting as the dialog), not the raw ISO string
    // and not a countdown.
    const windows = THREE_WINDOW_RESPONSE.data?.windows ?? []
    for (const w of windows) {
      const expected = formatDateTimeStr(dayjs(w.reset_at).toDate())
      expect(screen.getByText(expected)).toBeInTheDocument()
    }
    expect(
      screen.queryByText('2026-09-13T06:06:01.287Z')
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Resets in:')).not.toBeInTheDocument()
  })

  test('does not show any balance', () => {
    renderDialog(THREE_WINDOW_RESPONSE)

    expect(screen.queryByText(/Balance/)).not.toBeInTheDocument()
  })

  test('shows a dash instead of 0% when used percent is missing', () => {
    renderDialog({
      success: true,
      data: {
        windows: [{ period: 'session' }],
      },
    })

    expect(screen.getByText('Last 5 hours')).toBeInTheDocument()
    expect(screen.queryByText('0.00%')).not.toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBeGreaterThan(0)
  })

  test('shows a dash instead of 0% when the backend reports -1 sentinel values', () => {
    renderDialog({
      success: true,
      data: {
        windows: [
          {
            period: 'session',
            used_percent: -1,
            remaining_percent: -1,
            reset_in_sec: 3811,
            reset_at: '2026-09-01T16:27:38.287Z',
          },
        ],
      },
    })

    expect(screen.getByText('Last 5 hours')).toBeInTheDocument()
    expect(screen.queryByText('0.00%')).not.toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBeGreaterThan(0)
  })

  test('shows the status badge and still displays real percentages when status is not ok', () => {
    renderDialog({
      success: true,
      data: {
        windows: [
          {
            period: 'session',
            status: 'rate_limited',
            used_percent: 100,
            remaining_percent: 0,
            reset_in_sec: 3811,
            reset_at: '2026-09-01T16:27:38.287Z',
          },
        ],
      },
    })

    // The non-ok status is surfaced as a neutral badge on the card title.
    expect(screen.getByText('Status: rate_limited')).toBeInTheDocument()
    // A rate-limited window still reports meaningful usage: 100% used is shown
    // as a real number (big figure + Used row), not hidden behind "-".
    expect(screen.getAllByText('100.00%')).toHaveLength(2)
    expect(screen.queryByText('-')).not.toBeInTheDocument()
  })

  test('still shows a dash when a non-ok window has the -1 sentinel', () => {
    renderDialog({
      success: true,
      data: {
        windows: [
          {
            period: 'session',
            status: 'rate_limited',
            used_percent: -1,
            remaining_percent: -1,
            reset_in_sec: 3811,
            reset_at: '2026-09-01T16:27:38.287Z',
          },
        ],
      },
    })

    expect(screen.getByText('Status: rate_limited')).toBeInTheDocument()
    expect(screen.queryByText('0.00%')).not.toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBeGreaterThan(0)
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
      message: 'workspace not configured',
    })

    expect(screen.getByText('Unable to identify usage data')).toBeInTheDocument()
    expect(screen.getByText('workspace not configured')).toBeInTheDocument()
  })

  test('omits the raw JSON panel when the query failed', () => {
    renderDialog({
      success: false,
      message: 'workspace not configured',
    })

    expect(
      screen.queryByText('Show raw upstream response')
    ).not.toBeInTheDocument()
  })

  test('shows a refresh button when a refresh handler is provided', () => {
    render(
      <OpenCodeGoUsageDialog
        open
        onOpenChange={() => undefined}
        response={{ success: true, data: { windows: [] } }}
        onRefresh={() => undefined}
      />
    )

    expect(screen.getByText('Refresh usage')).toBeInTheDocument()
  })
})
