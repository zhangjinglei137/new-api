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

describe('OpenCodeGoUsageDialog', () => {
  test('shows remaining, used, reset countdown, and balance when data is present', () => {
    renderDialog({
      success: true,
      data: {
        usage_percent: 30,
        remaining_percent: 70,
        balance: 12.5,
        reset_in_sec: 7260,
        monthly_cap_usd: 100,
      },
    })

    expect(screen.getByText('OpenCode Go Usage')).toBeInTheDocument()
    expect(screen.getByText('70%')).toBeInTheDocument()
    expect(screen.getByText('30%')).toBeInTheDocument()
    expect(screen.getByText('2h 1m')).toBeInTheDocument()
    expect(screen.getByText(/12\.5/)).toBeInTheDocument()
  })

  test('shows a dash instead of 0% when remaining percent is missing', () => {
    renderDialog({
      success: true,
      data: {},
    })

    expect(screen.queryByText('0%')).not.toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBeGreaterThan(0)
  })

  test('shows the upstream error message on failure', () => {
    renderDialog({
      success: false,
      message: 'workspace not configured',
    })

    expect(screen.getByText('Unable to identify usage data')).toBeInTheDocument()
    expect(screen.getByText('workspace not configured')).toBeInTheDocument()
  })

  test('shows a refresh button when a refresh handler is provided', () => {
    render(
      <OpenCodeGoUsageDialog
        open
        onOpenChange={() => undefined}
        response={{ success: true, data: {} }}
        onRefresh={() => undefined}
      />
    )

    expect(screen.getByText('Refresh usage')).toBeInTheDocument()
  })
})