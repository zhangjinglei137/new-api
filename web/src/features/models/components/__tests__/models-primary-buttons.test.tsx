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
import { describe, expect, test, vi } from 'vitest'

import { ModelsPrimaryButtons } from '../models-primary-buttons'
import { ModelsProvider } from '../models-provider'

// Identity translation: button labels are i18n keys, so the components under
// test receive the keys themselves (upstream text handling lands in Task 4).
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

describe('ModelsPrimaryButtons', () => {
  test('shows a persistent Sync metadata button in the primary actions', () => {
    render(
      <ModelsProvider>
        <ModelsPrimaryButtons />
      </ModelsProvider>
    )

    // Sync metadata 是常驻入口，不再藏进「更多」菜单
    expect(
      screen.getByRole('button', { name: 'Sync metadata' })
    ).toBeInTheDocument()
  })

  test('keeps Manage Vendors / Manage Endpoints / Sync Upstream out of the more menu', async () => {
    const user = userEvent.setup()
    render(
      <ModelsProvider>
        <ModelsPrimaryButtons />
      </ModelsProvider>
    )

    await user.click(screen.getByRole('button', { name: 'Open menu' }))

    // 旧入口已迁移到 tab 页或常驻按钮，菜单中不应再出现
    expect(screen.queryByText('Manage Vendors')).not.toBeInTheDocument()
    expect(screen.queryByText('Manage Endpoints')).not.toBeInTheDocument()
    expect(screen.queryByText('Sync Upstream')).not.toBeInTheDocument()
  })
})
