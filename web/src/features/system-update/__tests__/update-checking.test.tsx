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
import {
  focusManager,
  onlineManager,
  QueryClientProvider,
  type QueryClient,
} from '@tanstack/react-query'
import {
  act,
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { UpdateCheckerSection } from '@/features/system-settings/maintenance/update-checker-section'
import { api } from '@/lib/api'
import { createAppQueryClient } from '@/lib/query-client'
import { ROLE } from '@/lib/roles'
import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'

import { useSystemUpdatePreferencesStore, useSystemUpdateStore } from '../store'
import { SystemUpdateAction } from '../system-update-action'
import { SYSTEM_UPDATE_INTERVAL, useSystemUpdate } from '../use-system-update'

const release = {
  tag_name: 'v1.0.0-rc.36',
  draft: false,
  prerelease: true,
  published_at: '2026-09-08T13:01:00Z',
  body: 'Release notes for administrators.',
}
let client: QueryClient

function Wrapper(props: { children: ReactNode }) {
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

function respondWithRelease(tag = release.tag_name): void {
  // /api/update/check 走统一 axios 实例（携带 Authorization + 401 刷新），
  // 这里按 URL 分流 mock：update/check 返回 release，其余（/api/status 等）
  // 返回默认状态数据。
  vi.mocked(api.get).mockImplementation(async (url) => {
    if (url === '/api/update/check') {
      return { data: { success: true, data: { ...release, tag_name: tag } } }
    }
    return { data: { success: true, data: { version: 'v1.0.0-rc.35' } } }
  })
}

/** 只统计 /api/update/check 的请求（update/check 与 /api/status 共用 api.get）。 */
function updateCheckCalls() {
  return vi
    .mocked(api.get)
    .mock.calls.filter(([url]) => url === '/api/update/check')
}

/** 让 /api/update/check 请求失败（axios reject → network）。 */
function failUpdateCheck(): void {
  vi.mocked(api.get).mockImplementation(async (url) => {
    if (url === '/api/update/check') {
      throw new Error('Request failed with status code 429')
    }
    return { data: { success: true, data: { version: 'v1.0.0-rc.35' } } }
  })
}

beforeEach(() => {
  localStorage.clear()
  useSystemUpdateStore.setState({ snapshot: null })
  useSystemUpdatePreferencesStore.setState({ ignoredVersionsByUserId: {} })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'admin', role: ROLE.ADMIN })
  focusManager.setFocused(true)
  onlineManager.setOnline(true)
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { version: 'v1.0.0-rc.35' } },
  })
  respondWithRelease()
  client = createAppQueryClient()
  client.setQueryData(STATUS_QUERY_KEY, { version: 'v1.0.0-rc.35' })
})

afterEach(() => {
  cleanup()
  client.clear()
  useSystemUpdateStore.setState({ snapshot: null })
  useSystemUpdatePreferencesStore.setState({ ignoredVersionsByUserId: {} })
  useAuthStore.getState().auth.reset()
  localStorage.clear()
  focusManager.setFocused(undefined)
  onlineManager.setOnline(true)
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('administrator update entry', () => {
  test.each([null, ROLE.USER])(
    'hides the entry and makes no release request for role %s',
    (role) => {
      useAuthStore
        .getState()
        .auth.setUser(role === null ? null : { id: 2, username: 'user', role })
      render(
        <>
          <SystemUpdateAction />
          <SystemUpdateAction presentation='version' />
        </>,
        { wrapper: Wrapper }
      )
      expect(screen.queryByRole('button')).not.toBeInTheDocument()
      expect(updateCheckCalls()).toHaveLength(0)
    }
  )

  test.each([ROLE.ADMIN, ROLE.SUPER_ADMIN])(
    'automatically shows the update indicator for administrator role %s without opening a dialog',
    async (role) => {
      useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role })
      render(<SystemUpdateAction />, { wrapper: Wrapper })
      const button = await screen.findByRole('button', {
        name: 'New version available: v1.0.0-rc.36',
      })
      expect(button).toHaveAttribute('aria-haspopup', 'dialog')
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      expect(updateCheckCalls()).toHaveLength(1)
      const [url] = updateCheckCalls()[0]
      expect(url).toBe('/api/update/check')
    }
  )

  test('shares results with maintenance, opens release details by keyboard and returns focus on Escape', async () => {
    const user = userEvent.setup()
    render(
      <>
        <SystemUpdateAction presentation='version' />
        <UpdateCheckerSection
          currentVersion='v1.0.0-rc.35'
          startTime={1_700_000_000}
        />
      </>,
      { wrapper: Wrapper }
    )
    const buttons = await screen.findAllByRole('button', {
      name: /New version available: v1\.0\.0-rc\.36/,
    })
    expect(buttons).toHaveLength(2)
    expect(updateCheckCalls()).toHaveLength(1)
    expect(within(buttons[0]).getByText('v1.0.0-rc.35')).toHaveClass('truncate')
    expect(within(buttons[0]).getByText('Update available')).toHaveClass(
      'hidden',
      '@min-[22rem]/system-brand:inline-flex'
    )
    expect(within(buttons[1]).getByText('Update available')).not.toHaveClass(
      'hidden'
    )

    buttons[0].focus()
    await user.keyboard('{Enter}')
    const dialog = await screen.findByRole('dialog', { name: 'System updates' })
    expect(await within(dialog).findByText(release.body)).toBeInTheDocument()
    expect(within(dialog).getByText('Pre-release')).toBeInTheDocument()
    expect(within(dialog).getByText('Published at')).toBeInTheDocument()
    expect(
      within(dialog).queryByText(
        'Checks hourly while you are online, including pre-releases.'
      )
    ).not.toBeInTheDocument()
    expect(
      within(dialog).queryByText('Last successful check')
    ).not.toBeInTheDocument()
    expect(
      within(dialog).getByRole('link', { name: 'Go to GitHub' })
    ).toHaveAttribute(
      'href',
      'https://github.com/zhangjinglei137/new-api/releases/tag/v1.0.0-rc.36'
    )
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(buttons[0]).toHaveFocus()
  })

  test('does not suggest a downgrade and updates every entry after a manual recheck', async () => {
    const user = userEvent.setup()
    client.setQueryData(STATUS_QUERY_KEY, { version: 'v1.0.0-rc.37' })
    render(
      <>
        <SystemUpdateAction />
        <SystemUpdateAction compact={false} />
      </>,
      { wrapper: Wrapper }
    )
    await waitFor(() => expect(updateCheckCalls()).toHaveLength(1))
    await user.click(
      screen.getAllByRole('button', { name: 'Check for updates' })[0]
    )
    expect(
      await screen.findByText('No newer version available.')
    ).toBeInTheDocument()
    respondWithRelease('v1.0.0-rc.38')
    await user.click(screen.getByRole('button', { name: 'Check again' }))
    expect(
      await screen.findByText('New version available: v1.0.0-rc.38', {
        selector: 'p',
      })
    ).toBeInTheDocument()
    expect(updateCheckCalls()).toHaveLength(2)
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(
      screen.getAllByRole('button', {
        name: 'New version available: v1.0.0-rc.38',
      })
    ).toHaveLength(2)

    act(() =>
      client.setQueryData(STATUS_QUERY_KEY, { version: 'v1.0.0-rc.38' })
    )
    expect(
      await screen.findAllByRole('button', { name: 'Check for updates' })
    ).toHaveLength(2)
  })

  test.each(['', 'v0.0.0'])(
    'shows an unknown current version for %s and allows ignoring the fetched release',
    async (version) => {
      const user = userEvent.setup()
      client.setQueryData(STATUS_QUERY_KEY, { version })
      render(<SystemUpdateAction presentation='version' />, {
        wrapper: Wrapper,
      })
      const trigger = screen.getByRole('button', {
        name: 'System updates, current version: Unknown version',
      })
      await user.click(trigger)
      const dialog = screen.getByRole('dialog')
      expect(within(dialog).getByText('Unknown version')).toBeInTheDocument()
      expect(
        await within(dialog).findByRole('link', { name: 'Go to GitHub' })
      ).toBeInTheDocument()
      expect(
        within(dialog).queryByText('Unable to compare versions')
      ).not.toBeInTheDocument()
      expect(
        within(dialog).queryByText('No newer version available.')
      ).not.toBeInTheDocument()
      await user.click(
        within(dialog).getByRole('button', { name: 'Ignore this version' })
      )
      expect(
        within(dialog).getByText('This version is ignored')
      ).toBeInTheDocument()
      expect(trigger).not.toHaveAttribute(
        'title',
        expect.stringContaining('New version available')
      )
    }
  )

  test('ignores and restores reminders in every entry while keeping release details open', async () => {
    const user = userEvent.setup()
    render(
      <>
        <SystemUpdateAction presentation='version' />
        <SystemUpdateAction compact={false} />
      </>,
      { wrapper: Wrapper }
    )
    const triggers = await screen.findAllByRole('button', {
      name: /New version available: v1\.0\.0-rc\.36/,
    })
    await user.click(triggers[0])
    const dialog = screen.getByRole('dialog')
    within(dialog).getByRole('button', { name: 'Ignore this version' }).focus()
    await user.keyboard('{Enter}')
    expect(dialog).toBeInTheDocument()
    expect(
      within(dialog).getByText('This version is ignored')
    ).toBeInTheDocument()
    for (const trigger of triggers) {
      expect(
        within(trigger).queryByText('Update available')
      ).not.toBeInTheDocument()
      expect(
        within(trigger).getByRole('status', { hidden: true })
      ).toBeEmptyDOMElement()
      expect(trigger).not.toHaveAttribute(
        'title',
        expect.stringContaining('New version available')
      )
    }
    expect(
      within(dialog).getByRole('link', { name: 'Go to GitHub' })
    ).toBeInTheDocument()
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(triggers[0]).toHaveFocus()
    await user.keyboard('{Enter}')
    await user.click(
      screen.getByRole('button', { name: 'Restore notifications' })
    )
    expect(
      screen.queryByText('This version is ignored')
    ).not.toBeInTheDocument()
    for (const trigger of triggers) {
      expect(within(trigger).getByText('Update available')).toBeInTheDocument()
      expect(
        within(trigger).getByRole('status', { hidden: true })
      ).toHaveTextContent('New version available: v1.0.0-rc.36')
    }
  })

  test('keeps the previous release on automatic failure and reports a failed manual check once', async () => {
    const user = userEvent.setup()
    const oldCheck = Date.now() - SYSTEM_UPDATE_INTERVAL - 1
    useSystemUpdateStore.getState().setSnapshot({
      release,
      lastCheckedAt: oldCheck,
      lastAttemptAt: oldCheck,
      error: null,
    })
    failUpdateCheck()
    const toastError = vi.spyOn(toast, 'error')
    render(<SystemUpdateAction />, { wrapper: Wrapper })
    await waitFor(() =>
      expect(useSystemUpdateStore.getState().snapshot?.error).toBe('network')
    )
    expect(toastError).not.toHaveBeenCalled()
    await user.click(
      screen.getByRole('button', {
        name: 'New version available: v1.0.0-rc.36',
      })
    )
    expect(
      screen.getByText('Showing the last successfully checked release.')
    ).toBeInTheDocument()
    expect(useSystemUpdateStore.getState().snapshot?.lastCheckedAt).toBe(
      oldCheck
    )
    await user.click(screen.getByRole('button', { name: 'Check again' }))
    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1))
    expect(
      screen.getByText('Failed to check for updates')
    ).toBeInTheDocument()
    expect(updateCheckCalls()).toHaveLength(2)
  })
})

describe('version label presentation', () => {
  test('keeps the current version visible while checking and after finding no newer version', async () => {
    let finishRequest:
      | ((response: { data: { success: boolean; data?: typeof release } }) => void)
      | undefined
    vi.mocked(api.get).mockImplementation(
      (url) =>
        new Promise((resolve) => {
          if (url === '/api/update/check') {
            finishRequest = resolve as (
              response: { data: { success: boolean; data?: typeof release } }
            ) => void
          } else {
            resolve({ data: { success: true, data: { version: 'v1.0.0-rc.35' } } })
          }
        })
    )
    client.setQueryData(STATUS_QUERY_KEY, { version: release.tag_name })
    render(<SystemUpdateAction presentation='version' />, { wrapper: Wrapper })
    const trigger = screen.getByRole('button', {
      name: 'System updates, current version: v1.0.0-rc.36',
    })
    expect(trigger).toHaveAttribute('aria-busy', 'true')
    expect(within(trigger).getByText(release.tag_name)).toBeInTheDocument()
    expect(
      within(trigger).queryByText('Check for updates')
    ).not.toBeInTheDocument()
    await act(async () => {
      finishRequest?.({ data: { success: true, data: release } })
    })
    await waitFor(() => expect(trigger).toHaveAttribute('aria-busy', 'false'))
    expect(within(trigger).getByText(release.tag_name)).toBeInTheDocument()
    expect(
      within(trigger).queryByText('Update available')
    ).not.toBeInTheDocument()
  })

  test('shows an unknown-version label when the server has not supplied a version', async () => {
    client.setQueryData(STATUS_QUERY_KEY, { version: '' })
    render(<SystemUpdateAction presentation='version' />, { wrapper: Wrapper })
    const trigger = screen.getByRole('button', {
      name: 'System updates, current version: Unknown version',
    })
    expect(within(trigger).getByText('Unknown version')).toBeInTheDocument()
    await waitFor(() => expect(trigger).toHaveAttribute('aria-busy', 'false'))
    expect(
      within(trigger).queryByText('Update available')
    ).not.toBeInTheDocument()
  })

  test('retains the version after a failed check and exposes the error in its tooltip and details', async () => {
    const user = userEvent.setup()
    failUpdateCheck()
    render(<SystemUpdateAction presentation='version' />, { wrapper: Wrapper })
    const trigger = await screen.findByRole('button', {
      name: /System updates, current version: v1\.0\.0-rc\.35.*Failed to check for updates/s,
    })
    expect(within(trigger).getByText('v1.0.0-rc.35')).toBeInTheDocument()
    expect(trigger).toHaveAttribute(
      'title',
      expect.stringContaining('Failed to check for updates')
    )
    await user.click(trigger)
    expect(
      screen.getByText('Failed to check for updates')
    ).toBeInTheDocument()
  })

  test('keeps a long version accessible in the tooltip while constraining its visible label', async () => {
    const version = 'v1.0.0+long-build-metadata-for-a-custom-deployment'
    client.setQueryData(STATUS_QUERY_KEY, { version })
    render(<SystemUpdateAction presentation='version' />, { wrapper: Wrapper })
    const trigger = screen.getByRole('button', {
      name: `System updates, current version: ${version}`,
    })
    expect(trigger).toHaveAttribute('title', expect.stringContaining(version))
    expect(within(trigger).getByText(version)).toHaveClass(
      'truncate',
      'max-w-32',
      'hidden',
      '@min-[22rem]/system-brand:inline'
    )
    await waitFor(() => expect(trigger).toHaveAttribute('aria-busy', 'false'))
  })
})

describe('version notification preferences', () => {
  test('keeps the ignored release after reload even when the update cache is cleared', async () => {
    const first = renderHook(useSystemUpdate, { wrapper: Wrapper })
    await waitFor(() => expect(first.result.current.shouldNotify).toBe(true))
    act(() => first.result.current.setIgnored(true))
    expect(first.result.current.hasUpdate).toBe(true)
    expect(first.result.current.shouldNotify).toBe(false)
    const saved = localStorage.getItem('system-update-preferences:v1')
    expect(saved).not.toBeNull()
    first.unmount()
    client.clear()
    useSystemUpdateStore.setState({ snapshot: null })
    useSystemUpdateStore.persist.clearStorage()
    useSystemUpdatePreferencesStore.setState({ ignoredVersionsByUserId: {} })
    localStorage.setItem('system-update-preferences:v1', String(saved))
    await useSystemUpdatePreferencesStore.persist.rehydrate()
    client = createAppQueryClient()
    client.setQueryData(STATUS_QUERY_KEY, { version: 'v1.0.0-rc.35' })
    const second = renderHook(useSystemUpdate, { wrapper: Wrapper })
    await waitFor(() => expect(second.result.current.hasUpdate).toBe(true))
    expect(second.result.current.isIgnored).toBe(true)
    expect(second.result.current.shouldNotify).toBe(false)
    expect(updateCheckCalls()).toHaveLength(2)
  })

  test('keeps preferences across logout and isolates ignore and restore actions by administrator', async () => {
    const hook = renderHook(useSystemUpdate, { wrapper: Wrapper })
    await waitFor(() => expect(hook.result.current.shouldNotify).toBe(true))
    act(() => hook.result.current.setIgnored(true))
    act(() => useAuthStore.getState().auth.reset())
    expect(hook.result.current.shouldNotify).toBe(false)
    act(() =>
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, username: 'other-admin', role: ROLE.ADMIN })
    )
    expect(hook.result.current.shouldNotify).toBe(true)
    act(() => hook.result.current.setIgnored(true))
    act(() =>
      useAuthStore
        .getState()
        .auth.setUser({ id: 1, username: 'admin', role: ROLE.ADMIN })
    )
    expect(hook.result.current.isIgnored).toBe(true)
    act(() => hook.result.current.setIgnored(false))
    expect(hook.result.current.shouldNotify).toBe(true)
    act(() =>
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, username: 'other-admin', role: ROLE.ADMIN })
    )
    expect(hook.result.current.isIgnored).toBe(true)
    expect(hook.result.current.shouldNotify).toBe(false)
    expect(updateCheckCalls()).toHaveLength(1)
  })

  test('retains ignored tags after manual checks and failures while notifying for a different release', async () => {
    const hook = renderHook(useSystemUpdate, { wrapper: Wrapper })
    await waitFor(() => expect(hook.result.current.shouldNotify).toBe(true))
    act(() => hook.result.current.setIgnored(true))
    await act(async () => hook.result.current.checkNow())
    expect(hook.result.current.isIgnored).toBe(true)
    expect(hook.result.current.shouldNotify).toBe(false)
    failUpdateCheck()
    await act(async () => hook.result.current.checkNow())
    await waitFor(() =>
      expect(hook.result.current.snapshot?.error).toBe('network')
    )
    expect(hook.result.current.release?.tag_name).toBe(release.tag_name)
    expect(hook.result.current.isIgnored).toBe(true)
    expect(hook.result.current.shouldNotify).toBe(false)
    respondWithRelease('v1.0.0-rc.37')
    await act(async () => hook.result.current.checkNow())
    await waitFor(() =>
      expect(hook.result.current.release?.tag_name).toBe('v1.0.0-rc.37')
    )
    expect(hook.result.current.shouldNotify).toBe(true)
    expect(hook.result.current.isIgnored).toBe(false)
    respondWithRelease()
    await act(async () => hook.result.current.checkNow())
    await waitFor(() =>
      expect(hook.result.current.release?.tag_name).toBe(release.tag_name)
    )
    expect(hook.result.current.isIgnored).toBe(true)
    expect(hook.result.current.shouldNotify).toBe(false)
  })

  test('updates every entry when another tab ignores a version or clears preferences', async () => {
    const first = renderHook(useSystemUpdate, { wrapper: Wrapper })
    const second = renderHook(useSystemUpdate, { wrapper: Wrapper })
    await waitFor(() => expect(first.result.current.shouldNotify).toBe(true))
    const saved = JSON.stringify({
      state: { ignoredVersionsByUserId: { 1: [release.tag_name] } },
      version: 0,
    })
    act(() => {
      localStorage.setItem('system-update-preferences:v1', saved)
      window.dispatchEvent(
        new StorageEvent('storage', {
          key: 'system-update-preferences:v1',
          newValue: saved,
        })
      )
    })
    expect(first.result.current.shouldNotify).toBe(false)
    expect(second.result.current.shouldNotify).toBe(false)
    act(() => {
      localStorage.removeItem('system-update-preferences:v1')
      window.dispatchEvent(
        new StorageEvent('storage', {
          key: 'system-update-preferences:v1',
          newValue: null,
        })
      )
    })
    expect(first.result.current.shouldNotify).toBe(true)
    expect(second.result.current.shouldNotify).toBe(true)
    act(() => first.result.current.setIgnored(true))
    act(() => {
      localStorage.clear()
      window.dispatchEvent(new StorageEvent('storage', { key: null }))
    })
    expect(first.result.current.shouldNotify).toBe(true)
    expect(second.result.current.shouldNotify).toBe(true)
    expect(updateCheckCalls()).toHaveLength(1)
  })

  test.each(['corrupt', 'invalid', 'blocked'])(
    'keeps ignore and restore usable when preference storage is %s',
    async (state) => {
      if (state === 'blocked') {
        vi.spyOn(localStorage, 'getItem').mockImplementation(() => {
          throw new Error('storage blocked')
        })
        vi.spyOn(localStorage, 'setItem').mockImplementation(() => {
          throw new Error('storage blocked')
        })
      } else {
        localStorage.setItem(
          'system-update-preferences:v1',
          state === 'corrupt'
            ? '{bad json'
            : JSON.stringify({
                state: { ignoredVersionsByUserId: { 1: release.tag_name } },
                version: 0,
              })
        )
      }
      const hook = renderHook(useSystemUpdate, { wrapper: Wrapper })
      await waitFor(() => expect(hook.result.current.shouldNotify).toBe(true))
      act(() => hook.result.current.setIgnored(true))
      expect(hook.result.current.shouldNotify).toBe(false)
      act(() => hook.result.current.setIgnored(false))
      expect(hook.result.current.shouldNotify).toBe(true)
    }
  )
})

describe('update cache and scheduling', () => {
  test('refreshes the server version during a later automatic check and clears an installed update', async () => {
    vi.useFakeTimers()
    const hook = renderHook(useSystemUpdate, { wrapper: Wrapper })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(hook.result.current.hasUpdate).toBe(true)
    vi.mocked(api.get).mockResolvedValue({
      data: { success: true, data: { version: release.tag_name } },
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(SYSTEM_UPDATE_INTERVAL + 1)
    })
    expect(hook.result.current.currentVersion).toBe(release.tag_name)
    expect(hook.result.current.hasUpdate).toBe(false)
  })

  test('restores a successful result from storage without requesting it again on reload', async () => {
    const first = renderHook(useSystemUpdate, { wrapper: Wrapper })
    await waitFor(() => expect(first.result.current.hasUpdate).toBe(true))
    first.unmount()
    client.clear()
    const saved = localStorage.getItem('system-update:v2')
    expect(saved).not.toBeNull()
    useSystemUpdateStore.setState({ snapshot: null })
    localStorage.setItem('system-update:v2', String(saved))
    await useSystemUpdateStore.persist.rehydrate()
    client = createAppQueryClient()
    client.setQueryData(STATUS_QUERY_KEY, { version: 'v1.0.0-rc.35' })
    const second = renderHook(useSystemUpdate, { wrapper: Wrapper })
    expect(second.result.current.hasUpdate).toBe(true)
    expect(updateCheckCalls()).toHaveLength(1)
  })

  test.each(['corrupt', 'blocked', 'future'])(
    'keeps checking when persisted storage is %s',
    async (state) => {
      if (state === 'corrupt') {
        localStorage.setItem('system-update:v2', '{bad json')
        await useSystemUpdateStore.persist.rehydrate()
      } else if (state === 'future') {
        localStorage.setItem(
          'system-update:v2',
          JSON.stringify({
            state: {
              snapshot: {
                release,
                lastCheckedAt: 0,
                lastAttemptAt: Date.now() + 86_400_000,
                error: null,
              },
            },
            version: 0,
          })
        )
        await useSystemUpdateStore.persist.rehydrate()
      } else {
        vi.spyOn(localStorage, 'getItem').mockImplementation(() => {
          throw new Error('storage blocked')
        })
        vi.spyOn(localStorage, 'setItem').mockImplementation(() => {
          throw new Error('storage blocked')
        })
      }
      const hook = renderHook(useSystemUpdate, { wrapper: Wrapper })
      await waitFor(() => expect(hook.result.current.hasUpdate).toBe(true))
      expect(updateCheckCalls()).toHaveLength(1)
    }
  )

  test('waits one hour across multiple consumers and stops polling after logout', async () => {
    vi.useFakeTimers()
    const first = renderHook(useSystemUpdate, { wrapper: Wrapper })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(first.result.current.hasUpdate).toBe(true)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(SYSTEM_UPDATE_INTERVAL / 2)
    })
    renderHook(useSystemUpdate, { wrapper: Wrapper })
    expect(updateCheckCalls()).toHaveLength(1)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(SYSTEM_UPDATE_INTERVAL / 2)
    })
    expect(updateCheckCalls()).toHaveLength(2)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(SYSTEM_UPDATE_INTERVAL / 2)
    })
    expect(updateCheckCalls()).toHaveLength(2)
    act(() => useAuthStore.getState().auth.reset())
    await act(async () => {
      await vi.advanceTimersByTimeAsync(SYSTEM_UPDATE_INTERVAL)
    })
    expect(updateCheckCalls()).toHaveLength(2)
  })

  test('waits while hidden or offline and checks stale results after visibility or connectivity returns', async () => {
    vi.useFakeTimers()
    focusManager.setFocused(false)
    renderHook(useSystemUpdate, { wrapper: Wrapper })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(updateCheckCalls()).toHaveLength(0)

    await act(async () => {
      focusManager.setFocused(true)
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(updateCheckCalls()).toHaveLength(1)
    act(() => focusManager.setFocused(false))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(SYSTEM_UPDATE_INTERVAL)
    })
    expect(updateCheckCalls()).toHaveLength(1)
    await act(async () => {
      focusManager.setFocused(true)
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(updateCheckCalls()).toHaveLength(2)

    act(() => onlineManager.setOnline(false))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(SYSTEM_UPDATE_INTERVAL)
    })
    expect(updateCheckCalls()).toHaveLength(2)
    await act(async () => {
      onlineManager.setOnline(true)
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(updateCheckCalls()).toHaveLength(3)
  })

  test('aborts an in-flight request when the administrator logs out', async () => {
    let requestSignal: AbortSignal | null | undefined
    vi.mocked(api.get).mockImplementation((url, config) => {
      if (url !== '/api/update/check') {
        return Promise.resolve({
          data: { success: true, data: { version: 'v1.0.0-rc.35' } },
        })
      }
      requestSignal = config?.signal as AbortSignal | undefined
      return new Promise((_resolve, reject) => {
        requestSignal?.addEventListener('abort', () =>
          reject(new DOMException('Aborted', 'AbortError'))
        )
      })
    })
    const view = render(<SystemUpdateAction />, { wrapper: Wrapper })
    await waitFor(() => expect(updateCheckCalls()).toHaveLength(1))
    act(() => useAuthStore.getState().auth.reset())
    expect(view.queryByRole('button')).not.toBeInTheDocument()
    await waitFor(() => expect(requestSignal?.aborted).toBe(true))
    expect(useSystemUpdateStore.getState().snapshot).toBeNull()
  })

  test('times out after twenty seconds without an immediate retry on focus or remount', async () => {
    vi.useFakeTimers()
    vi.mocked(api.get).mockImplementation((url, config) => {
      if (url !== '/api/update/check') {
        return Promise.resolve({
          data: { success: true, data: { version: 'v1.0.0-rc.35' } },
        })
      }
      const signal = config?.signal as AbortSignal | undefined
      return new Promise((_resolve, reject) => {
        signal?.addEventListener('abort', () =>
          reject(new DOMException('Aborted', 'AbortError'))
        )
      })
    })
    const hook = renderHook(useSystemUpdate, { wrapper: Wrapper })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(20_001)
    })
    expect(hook.result.current.snapshot?.error).toBe('timeout')
    expect(hook.result.current.checking).toBe(false)
    hook.unmount()
    renderHook(useSystemUpdate, { wrapper: Wrapper })
    await act(async () => {
      focusManager.setFocused(false)
      await vi.advanceTimersByTimeAsync(1)
      focusManager.setFocused(true)
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(updateCheckCalls()).toHaveLength(1)
  })

test.each([
  ['reject', 'network'],
  [{ success: false }, 'network'],
  [{}, 'payload'],
  [{ success: true, data: 'not-a-release' }, 'payload'],
])('records API response %j as %s', async (body, error) => {
  vi.mocked(api.get).mockImplementation(async (url) => {
    if (url !== '/api/update/check') {
      return { data: { success: true, data: { version: 'v1.0.0-rc.35' } } }
    }
    if (body === 'reject') throw new Error('network')
    return { data: body }
  })
  const hook = renderHook(useSystemUpdate, { wrapper: Wrapper })
  await waitFor(() => expect(hook.result.current.snapshot?.error).toBe(error))
  expect(hook.result.current.hasUpdate).toBe(false)
  expect(updateCheckCalls()).toHaveLength(1)
})
})
