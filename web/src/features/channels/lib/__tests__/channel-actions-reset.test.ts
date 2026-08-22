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
import type { QueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { describe, expect, test, vi } from 'vitest'

import { resetChannelBalance } from '../../api'
import {
  channelsQueryKeys,
  handleResetChannelBalance,
} from '../channel-actions'

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

vi.mock('../../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api')>()
  return {
    ...actual,
    resetChannelBalance: vi.fn(),
  }
})

const mockedResetChannelBalance = vi.mocked(resetChannelBalance)

function createQueryClient() {
  return {
    invalidateQueries: vi.fn(),
  } as unknown as QueryClient
}

describe('handleResetChannelBalance', () => {
  test('resets and invalidates on success', async () => {
    mockedResetChannelBalance.mockResolvedValue({ success: true })
    const queryClient = createQueryClient()
    const onSuccess = vi.fn()

    await handleResetChannelBalance(42, queryClient, onSuccess)

    expect(toast.success).toHaveBeenCalledWith(
      'Balance and used quota reset successfully'
    )
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: channelsQueryKeys.lists(),
    })
    expect(onSuccess).toHaveBeenCalledTimes(1)
  })

  test('shows server message on failure', async () => {
    mockedResetChannelBalance.mockResolvedValue({
      success: false,
      message: 'boom',
    })

    await handleResetChannelBalance(42)

    expect(toast.error).toHaveBeenCalledWith('boom')
  })

  test('falls back on thrown error', async () => {
    mockedResetChannelBalance.mockRejectedValue(new Error('network error'))

    await handleResetChannelBalance(42)

    expect(toast.error).toHaveBeenCalledWith('Failed to update channel')
  })
})