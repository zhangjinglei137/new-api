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
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { VendorManagementDialog } from '../vendor-management-dialog'
import { getVendors, searchVendors } from '../../../api'

// Passthrough t() with {{var}} interpolation. Real i18next is avoided because
// its default nsSeparator (":") makes t('Total:') return "" with the empty
// test resources, which would mask the pagination's own label rendering.
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) =>
      typeof options === 'object' && options
        ? key.replaceAll(/\{\{\s*(\w+)\s*\}\}/g, (_, name: string) =>
            String(options[name] ?? '')
          )
        : key,
  }),
}))

vi.mock('../../../api', () => ({
  getVendors: vi.fn(),
  searchVendors: vi.fn(),
  deleteVendor: vi.fn(),
}))

const PAGE_SIZE = 10
const TOTAL = 25 // 3 pages at 10 per page

function makeVendors(count: number) {
  return Array.from({ length: count }, (_, index) => ({
    id: index + 1,
    name: `Vendor ${index + 1}`,
    status: 1,
    created_time: 0,
    updated_time: 0,
  }))
}

function vendorPageResponse(page: number) {
  return {
    success: true,
    data: {
      items: makeVendors(PAGE_SIZE),
      total: TOTAL,
      page,
      page_size: PAGE_SIZE,
    },
  }
}

function renderDialog() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <VendorManagementDialog open onOpenChange={vi.fn()} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.mocked(getVendors).mockResolvedValue(vendorPageResponse(1))
  vi.mocked(searchVendors).mockResolvedValue(vendorPageResponse(1))
})

describe('VendorManagementDialog pagination', () => {
  test('renders the pagination bar with total, page buttons, and page-size select', async () => {
    renderDialog()

    await screen.findByText('Vendor 1')

    // Total count (dialog badge + pagination's own Total:).
    expect(screen.getByText('25 vendors')).toBeInTheDocument()
    expect(screen.getByText('Total:')).toBeInTheDocument()

    // Page buttons for the 3 pages, plus previous/next navigation. The page
    // buttons concatenate the sr-only label with the visible number, so the
    // accessible name is "Go to page 11" / "Go to page 22" / "Go to page 33".
    expect(
      screen.getByRole('button', { name: /Go to page 1/ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Go to page 2/ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Go to page 3/ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Go to next page' })
    ).toBeInTheDocument()

    // Rows-per-page select.
    expect(screen.getByRole('combobox')).toBeInTheDocument()
  })

  test('clicking a page number refetches with the new page param', async () => {
    const user = userEvent.setup()
    renderDialog()

    await screen.findByText('Vendor 1')
    expect(vi.mocked(getVendors)).toHaveBeenLastCalledWith({
      p: 1,
      page_size: PAGE_SIZE,
    })

    await user.click(screen.getByRole('button', { name: /Go to page 3/ }))

    await waitFor(() => {
      expect(vi.mocked(getVendors)).toHaveBeenLastCalledWith({
        p: 3,
        page_size: PAGE_SIZE,
      })
    })
  })

  test('next/previous navigation refetches with the page param', async () => {
    const user = userEvent.setup()
    renderDialog()

    await screen.findByText('Vendor 1')

    await user.click(screen.getByRole('button', { name: 'Go to next page' }))
    await waitFor(() => {
      expect(vi.mocked(getVendors)).toHaveBeenLastCalledWith({
        p: 2,
        page_size: PAGE_SIZE,
      })
    })

    await user.click(screen.getByRole('button', { name: 'Go to previous page' }))
    await waitFor(() => {
      expect(vi.mocked(getVendors)).toHaveBeenLastCalledWith({
        p: 1,
        page_size: PAGE_SIZE,
      })
    })
  })

  test('changing the page size resets to the first page', async () => {
    const user = userEvent.setup()
    renderDialog()

    await screen.findByText('Vendor 1')

    // Move to page 2 first.
    await user.click(screen.getByRole('button', { name: /Go to page 2/ }))
    await waitFor(() => {
      expect(vi.mocked(getVendors)).toHaveBeenLastCalledWith({
        p: 2,
        page_size: PAGE_SIZE,
      })
    })

    // Change page size to 20: must reset to page 1.
    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: '20' }))

    await waitFor(() => {
      expect(vi.mocked(getVendors)).toHaveBeenLastCalledWith({
        p: 1,
        page_size: 20,
      })
    })
  })
})

describe('VendorManagementDialog pagination layout contract', () => {
  test('pagination sits in block flow so it is not collapsed or clipped', async () => {
    renderDialog()

    await screen.findByText('Vendor 1')

    // The pagination root keeps its container-query + clip classes...
    const pagination = [...document.querySelectorAll('div')].find(
      (element) => element.className.includes('@container/pagination')
    )
    expect(pagination).toBeDefined()
    expect(pagination?.className).toContain('overflow-clip')

    // ...but it must NOT be a flex item: a flex item with
    // `container-type: inline-size` collapses to zero width (contents cannot
    // influence its size) and the whole bar is clipped by its own
    // `overflow-clip`. Block-level flow (direct child of the dialog body)
    // makes it fill the body width so every control stays visible.
    const parent = pagination?.parentElement
    expect(parent?.className).toContain('space-y-3')
    expect(parent?.className).not.toContain('flex')
  })
})