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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

vi.mock('@/lib/lobe-icon', () => ({
  getLobeIcon: () => null,
}))

const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { api } = await import('@/lib/api')
const { ChannelsProvider } = await import('../../channels-provider')
const { ChannelMutateDrawer } = await import('../channel-mutate-drawer')

type ApiMethod = (url: string, data?: unknown) => Promise<{ data: unknown }>
type MockableApi = {
  get: ApiMethod
  post: ApiMethod
}

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const originalPost = apiClient.post

function installApiFixtures(): void {
  apiClient.get = async (url) => {
    switch (url) {
      case '/api/group/':
        return { data: { success: true, data: ['default'] } }
      case '/api/channel/models':
        return { data: { success: true, data: [] } }
      case '/api/prefill_group':
        return { data: { success: true, data: [] } }
      case '/api/user/2fa/status':
        return { data: { success: true, data: { enabled: false } } }
      case '/api/user/passkey':
        return { data: { success: true, data: { enabled: false } } }
      default:
        return { data: { success: true, data: null } }
    }
  }
  apiClient.post = async () => ({ data: { success: true } })
}

function getControlByLabel(labelText: string): HTMLInputElement {
  const label = [...document.querySelectorAll<HTMLLabelElement>('label')].find(
    (candidate) => candidate.textContent?.trim() === labelText
  )
  if (!label) {
    throw new Error(`Expected label "${labelText}"`)
  }
  const control =
    label.control ??
    label
      .closest('[data-slot="form-item"]')
      ?.querySelector<HTMLElement>('[data-slot="form-control"], input')
  if (!(control instanceof HTMLInputElement)) {
    throw new Error(`Expected control for label "${labelText}"`)
  }
  return control
}

function changeInput(input: HTMLInputElement, value: string): void {
  fireEvent.input(input, { target: { value } })
}

function getAlertDialogContent(): HTMLElement {
  const dialog = document.querySelector<HTMLElement>(
    '[data-slot="alert-dialog-content"]'
  )
  if (!dialog) {
    throw new Error('Expected discard confirmation dialog')
  }
  return dialog
}

function clickFooterCancel(): void {
  const buttons = screen.getAllByRole('button')
  const cancel = buttons.find(
    (button) => button.textContent?.trim() === 'Cancel'
  )
  if (!cancel) {
    throw new Error('Expected footer Cancel button')
  }
  fireEvent.click(cancel)
}

async function renderCreateDrawer(
  onOpenChange: (open: boolean) => void
): Promise<void> {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <ChannelMutateDrawer
          open
          onOpenChange={onOpenChange}
          currentRow={null}
        />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  await waitFor(() => {
    expect(getControlByLabel('Name *')).toBeTruthy()
  })
}

afterEach(() => {
  apiClient.get = originalGet
  apiClient.post = originalPost
})

describe('channel mutate drawer discard confirmation', () => {
  test('canceling the confirm dialog keeps the drawer open with edits intact', async () => {
    const onOpenChange = vi.fn()
    installApiFixtures()
    await renderCreateDrawer(onOpenChange)

    const nameInput = getControlByLabel('Name *')
    changeInput(nameInput, 'My Channel')
    expect(onOpenChange).not.toHaveBeenCalledWith(false)

    clickFooterCancel()
    await waitFor(() => {
      expect(
        document.querySelector('[data-slot="alert-dialog-content"]')
      ).toBeTruthy()
    })

    const dialogCancel = [...getAlertDialogContent().querySelectorAll('button')]
      .find((button) => button.textContent?.trim() === 'Cancel')
    if (!dialogCancel) {
      throw new Error('Expected dialog Cancel button')
    }
    fireEvent.click(dialogCancel)

    await waitFor(() => {
      expect(
        document.querySelector('[data-slot="alert-dialog-content"]')
      ).toBeNull()
    })
    // Drawer must stay open and the edited value must be preserved.
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
    expect(getControlByLabel('Name *').value).toBe('My Channel')
  })

  test('confirming the dialog discards edits and closes the drawer', async () => {
    const onOpenChange = vi.fn()
    installApiFixtures()
    await renderCreateDrawer(onOpenChange)

    const nameInput = getControlByLabel('Name *')
    changeInput(nameInput, 'My Channel')

    clickFooterCancel()
    await waitFor(() => {
      expect(
        document.querySelector('[data-slot="alert-dialog-content"]')
      ).toBeTruthy()
    })

    const leaveButton = [...getAlertDialogContent().querySelectorAll('button')]
      .find((button) => button.textContent?.trim() === 'Leave')
    if (!leaveButton) {
      throw new Error('Expected Leave button')
    }
    fireEvent.click(leaveButton)

    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
  })
})
