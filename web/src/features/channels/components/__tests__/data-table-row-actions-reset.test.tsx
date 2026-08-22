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
import type { Row } from '@tanstack/react-table'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { CHANNEL_TYPE_CODEX } from '../../constants'
import { DataTableRowActions } from '../data-table-row-actions'
import { handleResetChannelBalance } from '../../lib'
import type { Channel } from '../../types'

let mockUser: { id: number; username: string; role: number } = {
  id: 1,
  username: 'admin',
  role: 100,
}

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (selector: (state: unknown) => unknown) =>
    selector({
      auth: {
        user: mockUser,
      },
    }),
}))

vi.mock('@tanstack/react-query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-query')>()
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: vi.fn() }),
  }
})

vi.mock('../channels-provider', () => ({
  useChannels: () => ({
    setOpen: vi.fn(),
    setCurrentRow: vi.fn(),
    upstream: {
      openModal: vi.fn(),
      detectChannelUpdates: vi.fn(),
    },
  }),
}))

vi.mock('../../lib', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib')>()
  return {
    ...actual,
    handleResetChannelBalance: vi.fn(),
  }
})

const mockedHandleResetChannelBalance = vi.mocked(handleResetChannelBalance)

function createRow(type: number) {
  const channel = {
    id: 7,
    name: 'Test Channel',
    type,
    status: 1,
    settings: '{}',
  } as Channel
  return { original: channel } as Row<Channel>
}

function openMenu() {
  fireEvent.click(screen.getByRole('button', { name: 'Open menu' }))
}

describe('DataTableRowActions reset balance', () => {
  beforeEach(() => {
    mockUser = { id: 1, username: 'admin', role: 100 }
  })

  test('shows reset item for non-codex channel', () => {
    render(<DataTableRowActions row={createRow(1)} />)

    openMenu()

    expect(screen.getByText('Reset balance and used quota')).toBeInTheDocument()
  })

  test('hides reset item for codex channel', () => {
    render(<DataTableRowActions row={createRow(CHANNEL_TYPE_CODEX)} />)

    openMenu()

    expect(
      screen.queryByText('Reset balance and used quota')
    ).not.toBeInTheDocument()
  })

  test('hides reset item for user without operate permission', () => {
    mockUser = { id: 2, username: 'operator', role: 10 }

    render(<DataTableRowActions row={createRow(1)} />)

    openMenu()

    expect(
      screen.queryByText('Reset balance and used quota')
    ).not.toBeInTheDocument()
  })

  test('opens confirm dialog and confirms reset', () => {
    render(<DataTableRowActions row={createRow(1)} />)

    openMenu()
    fireEvent.click(screen.getByText('Reset balance and used quota'))

    expect(
      screen.getByText(
        'This will set the balance and used quota of "Test Channel" to 0. This action cannot be undone.'
      )
    ).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Reset' }))

    expect(mockedHandleResetChannelBalance).toHaveBeenCalledWith(
      7,
      expect.anything()
    )
  })
})