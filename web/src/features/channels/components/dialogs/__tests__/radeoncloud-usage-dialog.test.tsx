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
import { describe, expect, test } from 'vitest'

import dayjs from '@/lib/dayjs'
import { formatDateTimeStr } from '@/lib/format'

import {
  RadeonCloudUsageDialog,
  type RadeonCloudUsageResponse,
} from '../radeoncloud-usage-dialog'

function renderDialog(response: RadeonCloudUsageResponse | null) {
  render(
    <RadeonCloudUsageDialog
      open
      onOpenChange={() => undefined}
      channelName='AMD Radeon Cloud'
      channelId={95}
      response={response}
    />
  )
}

const USAGE_RESPONSE: RadeonCloudUsageResponse = {
  success: true,
  data: {
    rpm_limit: 20,
    daily_limit_points: 1000000,
    daily_used_points: 186.52,
    daily_remaining_points: 999813.48,
    daily_used_percent: 0.0002,
    daily_reset_at: '2026-09-03T06:37:37.282Z',
    daily_reset_in_sec: 12345,
    today_requests: 17,
    today_tokens: 615,
    last_24h_requests: 42,
    last_24h_tokens: 1234,
    last_24h_last_request_at: '2026-09-02 06:40:06.206049',
    period_started_at: '2026-09-02T06:37:37.282Z',
  },
}

describe('RadeonCloudUsageDialog', () => {
  test('renders the daily free-allowance usage in the expected visual order', () => {
    renderDialog(USAGE_RESPONSE)

    expect(screen.getByText('AMD Radeon Cloud Usage')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Free to use. Points track API usage against your daily limit—they are not charges.'
      )
    ).toBeInTheDocument()

    // 1. Today remaining points is the main metric.
    expect(screen.getByText('Today Remaining')).toBeInTheDocument()
    expect(screen.getByText('999,813.48 pts')).toBeInTheDocument()

    // 2. Daily usage progress shows the fraction as a two-decimal percentage.
    expect(screen.getByText('0.02%')).toBeInTheDocument()

    // 3. Today used / daily allowance.
    expect(
      screen.getByText('186.52 / 1,000,000 pts')
    ).toBeInTheDocument()

    // 4. Today requests and tokens.
    expect(screen.getByText('17 requests · 615 tokens')).toBeInTheDocument()

    // 5-8. RPM, reset time, last request and last-24h stats.
    expect(screen.getByText('RPM Limit')).toBeInTheDocument()
    expect(screen.getByText('20')).toBeInTheDocument()
    expect(screen.getByText('Last 24h')).toBeInTheDocument()
    expect(screen.getByText('42 requests · 1234 tokens')).toBeInTheDocument()
  })

  test('renders the reset time and last request time as concrete timestamps', () => {
    renderDialog(USAGE_RESPONSE)

    const resetAt = USAGE_RESPONSE.data?.daily_reset_at ?? ''
    expect(
      screen.getByText(formatDateTimeStr(dayjs(resetAt).toDate()))
    ).toBeInTheDocument()
    expect(
      screen.queryByText('2026-09-03T06:37:37.282Z')
    ).not.toBeInTheDocument()

    const lastRequestAt =
      USAGE_RESPONSE.data?.last_24h_last_request_at ?? ''
    expect(
      screen.getByText(formatDateTimeStr(dayjs(lastRequestAt).toDate()))
    ).toBeInTheDocument()
  })

  test('formats points with thousand separators and at most two decimals', () => {
    renderDialog({
      success: true,
      data: {
        daily_limit_points: 1000000000,
        daily_used_points: 98765.4321,
        daily_remaining_points: 1234567890.5,
        today_requests: 1,
        today_tokens: 1,
      },
    })

    expect(screen.getByText('1,234,567,890.5 pts')).toBeInTheDocument()
    expect(
      screen.getByText('98,765.43 / 1,000,000,000 pts')
    ).toBeInTheDocument()
  })

  test('derives the percentage from used/allowance when daily_used_percent is missing', () => {
    renderDialog({
      success: true,
      data: {
        daily_limit_points: 1000000,
        daily_used_points: 250000,
        today_requests: 1,
      },
    })

    expect(screen.getByText('25.00%')).toBeInTheDocument()
  })

  test('shows a dash instead of 0% when the percentage cannot be derived', () => {
    renderDialog({
      success: true,
      data: {
        daily_remaining_points: 500000,
        today_requests: 5,
      },
    })

    expect(screen.queryByText('0.00%')).not.toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBeGreaterThan(0)
  })

  test('shows a degraded message plus raw panel when data is empty', () => {
    renderDialog({ success: true, data: {} })

    expect(screen.getByText('Unable to identify usage data')).toBeInTheDocument()
    expect(
      screen.getByText(
        'The upstream response did not include recognizable usage data.'
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

  test('passes through the backend message when credentials are expired', () => {
    renderDialog({
      success: false,
      error_code: 'credentials_expired',
      message: 'AMD Radeon Cloud API key 已过期，请更新凭证',
    })

    expect(
      screen.getByText('Usage credentials not configured')
    ).toBeInTheDocument()
    expect(
      screen.getByText('AMD Radeon Cloud API key 已过期，请更新凭证')
    ).toBeInTheDocument()
    // The free-to-use notice is hidden when the query failed.
    expect(
      screen.queryByText(/Free to use/)
    ).not.toBeInTheDocument()
  })

  test('maps fetch_failed to a friendly error title', () => {
    renderDialog({
      success: false,
      error_code: 'fetch_failed',
      message: 'upstream exploded',
    })

    expect(screen.getByText('Failed to fetch usage')).toBeInTheDocument()
    expect(screen.getByText('upstream exploded')).toBeInTheDocument()
  })

  test('maps usage_schema_unknown to the unable-to-identify message', () => {
    renderDialog({
      success: false,
      error_code: 'usage_schema_unknown',
      message: 'unexpected shape',
    })

    expect(screen.getByText('Unable to identify usage data')).toBeInTheDocument()
    expect(screen.getByText('unexpected shape')).toBeInTheDocument()
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
      <RadeonCloudUsageDialog
        open
        onOpenChange={() => undefined}
        response={USAGE_RESPONSE}
        onRefresh={() => undefined}
      />
    )

    expect(screen.getByText('Refresh usage')).toBeInTheDocument()
  })

  test('expands and collapses the raw upstream response panel', () => {
    renderDialog(USAGE_RESPONSE)

    expect(
      screen.queryByText(/daily_remaining_points/)
    ).not.toBeInTheDocument()

    fireEvent.click(
      screen.getByRole('button', { name: /Show raw upstream response/ })
    )
    expect(screen.getByText(/daily_remaining_points/)).toBeInTheDocument()
  })

  test('renders exactly one progress indicator for the daily usage bar', () => {
    renderDialog(USAGE_RESPONSE)

    // Regression: passing <ProgressIndicator> as a child of <Progress> used to
    // render a second, un-tracked color block beside the tracked bar, making
    // the bar wrap into two rows and (in Radeon Cloud) cover the percentage
    // label. The indicator color is now threaded through `indicatorClassName`,
    // so exactly one tracked indicator must exist.
    const indicators = document.querySelectorAll(
      '[data-slot="progress-indicator"]'
    )
    expect(indicators).toHaveLength(1)
    // USAGE_RESPONSE maps to a "success" variant (< 80%), so the single
    // indicator carries the success color class.
    expect(indicators[0]).toHaveClass('bg-success')
  })
})
