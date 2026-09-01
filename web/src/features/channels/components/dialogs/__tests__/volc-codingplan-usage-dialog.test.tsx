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

describe('VolcCodingPlanUsageDialog', () => {
  test('shows remaining and used percent when usage data is present', () => {
    renderDialog({
      success: true,
      data: {
        status: 'ok',
        period: 'monthly',
        remaining_percent: 62.5,
        used_percent: 37.5,
        reset_at: '2026-09-01T00:00:00Z',
        reset_in_sec: 3600,
      },
    })

    expect(
      screen.getByText('VolcEngine Coding Plan Usage')
    ).toBeInTheDocument()
    expect(screen.getByText('62.5%')).toBeInTheDocument()
    expect(screen.getByText('37.5%')).toBeInTheDocument()
    // The reset time is formatted instead of shown as the raw ISO string.
    expect(screen.queryByText('2026-09-01T00:00:00Z')).not.toBeInTheDocument()
    // Raw JSON panel is available once usage data exists.
    expect(
      screen.getByText('Show raw upstream response')
    ).toBeInTheDocument()
  })

  test('shows a dash instead of 0% when remaining percent is missing', () => {
    renderDialog({
      success: true,
      data: {
        status: 'ok',
        period: 'monthly',
      },
    })

    expect(screen.queryByText('0%')).not.toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBeGreaterThan(0)
    // Raw JSON panel is available while usage data exists.
    expect(
      screen.getByText('Show raw upstream response')
    ).toBeInTheDocument()
  })

  test('omits the raw JSON panel when the response carries no usage data', () => {
    renderDialog({ success: true })

    expect(
      screen.queryByText('Show raw upstream response')
    ).not.toBeInTheDocument()
  })

  test('clamps out-of-range remaining percent into [0, 100]', () => {
    renderDialog({
      success: true,
      data: {
        status: 'ok',
        period: 'monthly',
        remaining_percent: 250,
        used_percent: -10,
      },
    })

    expect(screen.getByText('100%')).toBeInTheDocument()
    expect(screen.queryByText('250%')).not.toBeInTheDocument()
    expect(screen.queryByText('-10%')).not.toBeInTheDocument()
  })

  test('guides the admin to configure credentials when none are set', () => {
    renderDialog({
      success: false,
      error_code: 'credentials_not_configured',
      message: 'credentials missing',
    })

    expect(
      screen.getByText('Usage credentials not configured')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'Configure the CSRF token and session cookie in the channel settings before querying usage.'
      )
    ).toBeInTheDocument()
  })

  test('guides the admin to refresh the session when credentials expired', () => {
    renderDialog({
      success: false,
      error_code: 'credentials_expired',
      message: 'session expired upstream',
    })

    expect(
      screen.getByText('Session expired, please update Cookie and CSRF token')
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

  test('shows a refresh button when a refresh handler is provided', () => {
    render(
      <VolcCodingPlanUsageDialog
        open
        onOpenChange={() => undefined}
        response={{ success: true, data: { status: 'ok', period: 'monthly' } }}
        onRefresh={() => undefined}
      />
    )

    expect(screen.getByText('Refresh usage')).toBeInTheDocument()
  })
})