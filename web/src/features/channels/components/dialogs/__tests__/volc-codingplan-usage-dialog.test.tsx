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
  VolcCodingPlanUsageDialog,
  type VolcCodingPlanUsageResponse,
} from '../volc-codingplan-usage-dialog'

function renderDialog(response: VolcCodingPlanUsageResponse | null) {
  render(
    <VolcCodingPlanUsageDialog
      open
      onOpenChange={() => undefined}
      channelName='Volc Coding Plan'
      channelId={7}
      response={response}
    />
  )
}

const THREE_WINDOW_RESPONSE: VolcCodingPlanUsageResponse = {
  success: true,
  data: {
    status: 'Running',
    // Intentionally out of order: the dialog must render session → weekly → monthly.
    windows: [
      {
        period: 'monthly',
        used_percent: 75,
        remaining_percent: 25,
        reset_at: '2026-10-01T00:00:00Z',
        reset_in_sec: 99999,
      },
      {
        period: 'session',
        used_percent: 12.5,
        remaining_percent: 87.5,
        reset_at: '2026-09-01T00:00:00Z',
        reset_in_sec: 12345,
      },
      {
        period: 'weekly',
        used_percent: 50,
        remaining_percent: 50,
        reset_at: '2026-09-07T00:00:00Z',
        reset_in_sec: 45678,
      },
    ],
  },
}

describe('VolcCodingPlanUsageDialog', () => {
  test('renders all three windows in fixed order with two-decimal percentages', () => {
    renderDialog(THREE_WINDOW_RESPONSE)

    expect(
      screen.getByText('VolcEngine Coding Plan Usage')
    ).toBeInTheDocument()

    const titles = screen
      .getAllByText(/Last 5 hours|Weekly|Monthly/)
      .map((el) => el.textContent)
    expect(titles).toStrictEqual(['Last 5 hours', 'Weekly', 'Monthly'])

    // Two-decimal formatting for used percentages (big number + Used row).
    expect(screen.getAllByText('12.50%')).toHaveLength(2)
    expect(screen.getAllByText('50.00%')).toHaveLength(2)
    expect(screen.getAllByText('75.00%')).toHaveLength(2)
    // Remaining percentages are no longer surfaced.
    expect(screen.queryByText('87.50%')).not.toBeInTheDocument()
    expect(screen.queryByText('25.00%')).not.toBeInTheDocument()

    // Reset times are formatted instead of shown as the raw ISO strings.
    expect(screen.queryByText('2026-10-01T00:00:00Z')).not.toBeInTheDocument()
    expect(screen.queryByText('2026-09-01T00:00:00Z')).not.toBeInTheDocument()

    // Raw JSON panel is available once usage data exists.
    expect(
      screen.getByText('Show raw upstream response')
    ).toBeInTheDocument()
  })

  test('renders a single card when only one window is present', () => {
    renderDialog({
      success: true,
      data: {
        status: 'ok',
        windows: [
          {
            period: 'monthly',
            used_percent: 75,
            remaining_percent: 25,
          },
        ],
      },
    })

    expect(screen.getByText('Monthly')).toBeInTheDocument()
    expect(screen.getAllByText('75.00%')).toHaveLength(2)
    expect(screen.queryByText('Last 5 hours')).not.toBeInTheDocument()
    expect(screen.queryByText('Weekly')).not.toBeInTheDocument()
  })

  test('shows a dash instead of 0% when window percentages are missing', () => {
    renderDialog({
      success: true,
      data: {
        status: 'ok',
        windows: [{ period: 'session' }],
      },
    })

    expect(screen.getByText('Last 5 hours')).toBeInTheDocument()
    expect(screen.queryByText('0.00%')).not.toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBeGreaterThan(0)
  })

  test('clamps out-of-range used percentages into [0, 100]', () => {
    renderDialog({
      success: true,
      data: {
        status: 'ok',
        windows: [
          {
            period: 'session',
            used_percent: 250,
          },
          {
            period: 'weekly',
            used_percent: -10,
          },
        ],
      },
    })

    expect(screen.getAllByText('100.00%')).toHaveLength(2)
    expect(screen.queryByText('250.00%')).not.toBeInTheDocument()
    // Negative used percent clamps to 0 and stays visible.
    expect(screen.getAllByText('0.00%')).toHaveLength(2)
  })

  test('shows a degraded message plus raw panel when windows are empty', () => {
    renderDialog({
      success: true,
      data: {
        status: 'ok',
        windows: [],
      },
    })

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

  test('shows a degraded message plus raw panel when data is missing', () => {
    renderDialog({ success: true })

    expect(screen.getByText('Unable to identify usage data')).toBeInTheDocument()
    expect(
      screen.getByText('Show raw upstream response')
    ).toBeInTheDocument()
  })

  test('passes through the backend message when credentials are not configured', () => {
    renderDialog({
      success: false,
      error_code: 'credentials_not_configured',
      message: 'OpenAPI Access Key / Secret Access Key 未配置，请在渠道设置中更新凭证',
    })

    expect(
      screen.getByText('Usage credentials not configured')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'OpenAPI Access Key / Secret Access Key 未配置，请在渠道设置中更新凭证'
      )
    ).toBeInTheDocument()
  })

  test('passes through the backend message when credentials expired', () => {
    renderDialog({
      success: false,
      error_code: 'credentials_expired',
      message:
        '火山 OpenAPI Access Key / Secret Access Key 无效或已被禁用，请在渠道设置中更新凭证',
    })

    expect(
      screen.getByText('Usage credentials not configured')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        '火山 OpenAPI Access Key / Secret Access Key 无效或已被禁用，请在渠道设置中更新凭证'
      )
    ).toBeInTheDocument()
  })

  test('falls back to the upstream message for unknown failures', () => {
    renderDialog({
      success: false,
      error_code: 'fetch_failed',
      message: 'upstream exploded',
    })

    expect(screen.getByText('Unable to identify usage data')).toBeInTheDocument()
    expect(screen.getByText('upstream exploded')).toBeInTheDocument()
  })

  test('omits the raw JSON panel when the query failed', () => {
    renderDialog({
      success: false,
      error_code: 'fetch_failed',
      message: 'upstream exploded',
    })

    expect(
      screen.queryByText('Show raw upstream response')
    ).not.toBeInTheDocument()
  })

  test('shows a refresh button when a refresh handler is provided', () => {
    render(
      <VolcCodingPlanUsageDialog
        open
        onOpenChange={() => undefined}
        response={THREE_WINDOW_RESPONSE}
        onRefresh={() => undefined}
      />
    )

    expect(screen.getByText('Refresh usage')).toBeInTheDocument()
  })
})