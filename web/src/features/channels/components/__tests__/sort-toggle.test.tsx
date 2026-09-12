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
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { ChannelsPrimaryButtons } from '../channels-primary-buttons'
import { ChannelsProvider } from '../channels-provider'

vi.mock('@tanstack/react-query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-query')>()
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: vi.fn() }),
  }
})

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (selector: (state: unknown) => unknown) =>
    selector({
      auth: {
        user: { id: 1, username: 'admin', role: 100 },
      },
    }),
}))

function renderButtons() {
  return render(
    <ChannelsProvider>
      <ChannelsPrimaryButtons />
    </ChannelsProvider>
  )
}

describe('ChannelsPrimaryButtons sort toggle', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  test('renders a Sort by Sort switch in the desktop toolbar', () => {
    renderButtons()

    expect(
      screen.getByRole('switch', { name: 'Sort by Sort' })
    ).toBeInTheDocument()
  })

  test('toggles sortSort and persists it to localStorage', async () => {
    const user = userEvent.setup()
    renderButtons()

    const toggle = screen.getByRole('switch', { name: 'Sort by Sort' })
    expect(toggle).toHaveAttribute('aria-checked', 'false')

    await user.click(toggle)

    expect(toggle).toHaveAttribute('aria-checked', 'true')
    expect(localStorage.getItem('channels-sort-sort')).toBe('true')

    await user.click(toggle)

    expect(toggle).toHaveAttribute('aria-checked', 'false')
    expect(localStorage.getItem('channels-sort-sort')).toBe('false')
  })

  test('restores sortSort from localStorage on mount', () => {
    localStorage.setItem('channels-sort-sort', 'true')

    renderButtons()

    expect(
      screen.getByRole('switch', { name: 'Sort by Sort' })
    ).toHaveAttribute('aria-checked', 'true')
  })
})
