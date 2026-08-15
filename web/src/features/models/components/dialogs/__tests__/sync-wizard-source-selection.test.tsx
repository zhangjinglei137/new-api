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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { ModelsProvider } from '../../models-provider'
import { SyncWizardDialog } from '../sync-wizard-dialog'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('@/hooks/use-mobile', () => ({
  useIsMobile: () => false,
}))

vi.mock('../../../api', () => ({
  previewUpstreamDiff: vi.fn(),
  syncUpstream: vi.fn(),
}))

function renderDialog() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <ModelsProvider>
        <SyncWizardDialog open onOpenChange={vi.fn()} />
      </ModelsProvider>
    </QueryClientProvider>
  )
}

// Locate the radio control of a source option by its accessible name
// (Base UI wires aria-labelledby to the option's label element).
function getSourceRadio(labelText: string): HTMLElement {
  return screen.getByRole('radio', { name: new RegExp(labelText) })
}

describe('SyncWizardDialog source selection', () => {
  test('selecting OpenCode Go stays selected and is not reset to Official Repository', async () => {
    const user = userEvent.setup()
    renderDialog()

    const opencodeRadio = getSourceRadio('OpenCode Go')
    await user.click(opencodeRadio)

    await waitFor(() => {
      expect(opencodeRadio.getAttribute('aria-checked')).toBe('true')
    })
    expect(
      getSourceRadio('Official Repository').getAttribute('aria-checked')
    ).toBe('false')
  })

  test('selecting Official Repository still works', async () => {
    const user = userEvent.setup()
    renderDialog()

    const officialRadio = getSourceRadio('Official Repository')
    await user.click(officialRadio)

    await waitFor(() => {
      expect(officialRadio.getAttribute('aria-checked')).toBe('true')
    })
  })

  test('Configuration File stays disabled and click does not change selection', async () => {
    const user = userEvent.setup()
    renderDialog()

    const configRadio = getSourceRadio('Configuration File')
    await user.click(configRadio)

    expect(
      getSourceRadio('Official Repository').getAttribute('aria-checked')
    ).toBe('true')
    expect(configRadio.getAttribute('aria-checked')).toBe('false')
  })
})
