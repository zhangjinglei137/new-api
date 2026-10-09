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

import {
  ClineUsageDialog,
  type ClineUsageResponse,
} from '../cline-usage-dialog'

function renderDialog(response: ClineUsageResponse | null) {
  render(
    <ClineUsageDialog
      open
      onOpenChange={() => undefined}
      channelName='ClinePass'
      channelId={94}
      response={response}
    />
  )
}

const THREE_LIMIT_RESPONSE: ClineUsageResponse = {
  success: true,
  data: {
    // Intentionally out of order: the dialog must render five_hour → weekly → monthly.
    limits: [
      {
        type: 'monthly',
        percentUsed: 100,
        resetsAt: '2026-11-08T03:44:01Z',
      },
      {
        type: 'five_hour',
        percentUsed: 12.5,
        resetsAt: '2026-10-09T08:44:01Z',
      },
      {
        type: 'weekly',
        percentUsed: 42.5,
        resetsAt: '2026-10-16T03:44:01Z',
      },
    ],
  },
}

describe('ClineUsageDialog', () => {
  test('renders limits in fixed order with two-decimal percentages', () => {
    renderDialog(THREE_LIMIT_RESPONSE)

    expect(screen.getByText('Cline Coding Plan Usage')).toBeInTheDocument()

    const titles = screen
      .getAllByText(/Last 5 hours|Weekly|Monthly/)
      .map((el) => el.textContent)
    expect(titles).toStrictEqual(['Last 5 hours', 'Weekly', 'Monthly'])

    // Two-decimal formatting for used percentages (big number + Used row).
    expect(screen.getAllByText('12.50%')).toHaveLength(2)
    expect(screen.getAllByText('42.50%')).toHaveLength(2)
    expect(screen.getAllByText('100.00%')).toHaveLength(2)

    // Reset times are formatted instead of shown as the raw ISO strings.
    expect(screen.queryByText('2026-10-09T08:44:01Z')).not.toBeInTheDocument()

    // Raw JSON panel is available once usage data exists.
    expect(
      screen.getByText('Show raw upstream response')
    ).toBeInTheDocument()
  })

  test('renders a single card when only one limit is present', () => {
    renderDialog({
      success: true,
      data: {
        limits: [
          {
            type: 'five_hour',
            percentUsed: 75,
            resetsAt: '2026-10-09T08:44:01Z',
          },
        ],
      },
    })

    expect(screen.getByText('Last 5 hours')).toBeInTheDocument()
    expect(screen.getAllByText('75.00%')).toHaveLength(2)
    expect(screen.queryByText('Weekly')).not.toBeInTheDocument()
  })

  test('renders a valid 0% when the limit is unused and a dash for empty reset time', () => {
    renderDialog({
      success: true,
      data: {
        limits: [{ type: 'five_hour', percentUsed: 0, resetsAt: '' }],
      },
    })

    expect(screen.getByText('Last 5 hours')).toBeInTheDocument()
    expect(screen.getAllByText('0.00%').length).toBeGreaterThan(0)
    expect(screen.getAllByText('-').length).toBeGreaterThan(0)
  })

  test('shows a degraded message plus raw panel when limits are empty', () => {
    renderDialog({
      success: true,
      data: { limits: [] },
    })

    expect(
      screen.getByText('Unable to identify usage data')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'The upstream response did not include recognizable usage windows.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByText('Show raw upstream response')
    ).toBeInTheDocument()
  })

  test('shows a degraded message plus raw panel when data is missing', () => {
    renderDialog({ success: true })

    expect(
      screen.getByText('Unable to identify usage data')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Show raw upstream response')
    ).toBeInTheDocument()
  })

  test('maps the credentials_expired failure to the credentials copy', () => {
    renderDialog({
      success: false,
      error_code: 'credentials_expired',
      message: 'cline 凭证无效或已过期',
    })

    expect(
      screen.getByText('Usage credentials not configured')
    ).toBeInTheDocument()
    // Payload contract: backend message passes through unchanged.
    expect(screen.getByText(/cline 凭证无效或已过期/)).toBeInTheDocument()
  })

  test('falls back to the upstream message for fetch_failed', () => {
    renderDialog({
      success: false,
      error_code: 'fetch_failed',
      message: 'upstream exploded',
    })

    expect(
      screen.getByText('Unable to identify usage data')
    ).toBeInTheDocument()
    expect(screen.getByText('upstream exploded')).toBeInTheDocument()
  })

  test('omits the raw JSON panel when the query failed', () => {
    renderDialog({
      success: false,
      error_code: 'usage_schema_unknown',
      message: 'schema changed',
    })

    expect(
      screen.queryByText('Show raw upstream response')
    ).not.toBeInTheDocument()
  })
})
