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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import type { PricingModel } from '../../types'
import { ModelCardGrid } from '../model-card-grid'

// Passthrough t() with {{var}} interpolation. Real i18next is avoided because
// its default nsSeparator (":") makes t('Page ... of ...') return "" with the
// empty test resources, which would mask the pagination's own label rendering.
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

const { getPerfMetricsSummaryMock } = vi.hoisted(() => ({
  getPerfMetricsSummaryMock: vi.fn(),
}))

vi.mock('@/features/performance-metrics/api', () => ({
  getPerfMetricsSummary: getPerfMetricsSummaryMock,
}))

// Render ModelCard as a lightweight stub so the pagination contract can be
// asserted on thousands of models without rendering thousands of real cards.
// The card's own rendering is out of scope for these tests.
vi.mock('@/features/pricing/components/model-card', () => ({
  ModelCard: ({ model }: { model: { model_name?: string } }) => (
    <div data-testid='model-card'>{model.model_name}</div>
  ),
}))

const PAGE_SIZE = 1000

function makeModels(count: number): PricingModel[] {
  return Array.from({ length: count }, (_, index) => ({
    id: index + 1,
    model_name: `model-${index + 1}`,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['all'],
  }))
}

function renderGrid(models: PricingModel[]) {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <ModelCardGrid models={models} onModelClick={vi.fn()} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  getPerfMetricsSummaryMock.mockResolvedValue({
    success: true,
    data: { models: [] },
  })
})

describe('ModelCardGrid pagination', () => {
  test('renders all models without pagination controls when count is below the page size', () => {
    const models = makeModels(50)
    renderGrid(models)

    expect(screen.getAllByTestId('model-card')).toHaveLength(50)
    expect(screen.getByText('model-1')).toBeInTheDocument()
    expect(screen.getByText('model-50')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Previous page' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Next page' })
    ).not.toBeInTheDocument()
  })

  test('renders all models without pagination controls when count exactly equals the page size', () => {
    renderGrid(makeModels(PAGE_SIZE))

    expect(screen.getAllByTestId('model-card')).toHaveLength(PAGE_SIZE)
    expect(
      screen.queryByRole('button', { name: 'Next page' })
    ).not.toBeInTheDocument()
  })

  test('shows pagination controls and renders only the first page when count exceeds the page size', () => {
    renderGrid(makeModels(PAGE_SIZE + 1))

    // First page renders exactly PAGE_SIZE cards, the remainder is on page 2.
    expect(screen.getAllByTestId('model-card')).toHaveLength(PAGE_SIZE)
    expect(screen.getByText('Page 1 of 2')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Previous page' })
    ).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Next page' })
    ).toBeEnabled()
  })

  test('navigates to the last page and disables next when the tail of the catalog is shown', async () => {
    const user = userEvent.setup()
    renderGrid(makeModels(PAGE_SIZE + 1))

    await user.click(screen.getByRole('button', { name: 'Next page' }))

    expect(screen.getByText('Page 2 of 2')).toBeInTheDocument()
    expect(screen.getAllByTestId('model-card')).toHaveLength(1)
    expect(screen.getByText('model-1001')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Previous page' })
    ).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled()
  })
})