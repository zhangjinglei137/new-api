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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { toast } from 'sonner'

import { api } from '@/lib/api'

import { CHANNEL_TYPE_CLINE } from '../../constants'
import { channelSchema } from '../../types'
import { ChannelRowActionsLayoutContext } from '../channel-row-actions-context'
import { BalanceCell } from '../channels-columns'
import { ChannelsProvider } from '../channels-provider'

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

function clineChannel(settings: string) {
  return channelSchema.parse({
    id: 42,
    type: CHANNEL_TYPE_CLINE,
    key: '',
    name: 'Test ClinePass',
    status: 1,
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    settings,
  })
}

function renderBalanceCell(channel: ReturnType<typeof clineChannel>) {
  return render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <ChannelRowActionsLayoutContext.Provider value='table'>
          <BalanceCell channel={channel} />
        </ChannelRowActionsLayoutContext.Provider>
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

it('shows the Coding Plan entry for a ClinePass channel and opens the usage dialog on click', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        limits: [
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
    },
  })
  const user = userEvent.setup()
  renderBalanceCell(clineChannel('{"endpoint_profile":"clinepass"}'))

  const entry = screen.getByTitle('Coding Plan')
  await user.click(entry)

  expect(get).toHaveBeenCalledWith(
    '/api/channel/42/cline/usage',
    expect.anything()
  )
  expect(
    await screen.findByText('Cline Coding Plan Usage')
  ).toBeInTheDocument()
  expect(screen.getAllByText('12.50%').length).toBeGreaterThan(0)
})

it('does not show the Coding Plan entry for a pay-as-you-go Cline channel', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, balance: 1 },
  })
  const user = userEvent.setup()
  renderBalanceCell(clineChannel('{}'))

  expect(screen.queryByTitle('Coding Plan')).not.toBeInTheDocument()

  // 按量渠道的剩余额度徽标点击后走余额刷新，而不是计划用量接口。
  const remainingBadge = screen.getAllByTitle(/^\$0/)[1]
  await user.click(remainingBadge)
  // 按量渠道点击后走余额刷新，而不是计划用量接口。
  expect(
    get.mock.calls.some(([url]) => String(url).includes('update_balance'))
  ).toBe(true)
  expect(
    get.mock.calls.some(([url]) => String(url).includes('cline/usage'))
  ).toBe(false)
})

it('toasts the backend message and keeps the dialog closed when usage fetch fails', async () => {
  const notify = vi.spyOn(toast, 'error').mockReturnValue('error')
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: false,
      error_code: 'credentials_expired',
      message: 'cline 凭证无效或已过期',
    },
  })
  const user = userEvent.setup()
  renderBalanceCell(clineChannel('{"endpoint_profile":"clinepass"}'))

  await user.click(screen.getByTitle('Coding Plan'))

  expect(notify).toHaveBeenCalledWith('cline 凭证无效或已过期')
  expect(
    screen.queryByText('Cline Coding Plan Usage')
  ).not.toBeInTheDocument()
})
