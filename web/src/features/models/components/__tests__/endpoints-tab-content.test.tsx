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
import {
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { EndpointsTabContent } from '../endpoints-tab-content'
import { getEndpointDefinitions, updateEndpointDefinitions } from '../../api'

vi.mock('../../api', () => ({
  getEndpointDefinitions: vi.fn(),
  updateEndpointDefinitions: vi.fn(),
}))

const ENDPOINT_TYPES = [
  'openai',
  'openai-response',
  'openai-response-compact',
  'openai-alpha-search',
  'anthropic',
  'gemini',
  'jina-rerank',
  'image-generation',
  'embeddings',
  'openai-video',
]

const ENDPOINTS = ENDPOINT_TYPES.map((type, index) => ({
  type,
  display_name: type,
  path: index === 0 ? '/v1/chat/completions' : '/v1/messages',
  method: 'POST',
  npm: index % 2 === 0 ? '@ai-sdk/openai' : '@ai-sdk/anthropic',
}))

// This value exists only in npm_options, never in a row's npm value, so its
// presence in the dropdown proves suggestions come from the backend response.
const NPM_OPTIONS = [
  '@ai-sdk/anthropic',
  '@ai-sdk/openai',
  '@ai-sdk/openai-compatible',
]

function renderTab() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <EndpointsTabContent />
    </QueryClientProvider>
  )
}

function getNpmComboboxForType(type: string): HTMLInputElement {
  const typeCell = screen.getByText(type)
  const row = typeCell.closest('tr')
  expect(row).not.toBeNull()
  return within(row as HTMLElement).getByRole('combobox') as HTMLInputElement
}

beforeEach(() => {
  vi.mocked(getEndpointDefinitions).mockResolvedValue({
    success: true,
    data: { endpoints: ENDPOINTS, npm_options: NPM_OPTIONS },
  })
})

describe('EndpointsTabContent npm combobox', () => {
  test('renders an npm ComboboxInput per row once definitions load', async () => {
    renderTab()

    await screen.findByText('openai')

    // Every row has an input combobox (the npm field).
    const comboboxes = screen.getAllByRole('combobox')
    expect(comboboxes).toHaveLength(ENDPOINT_TYPES.length)
    // The openai row's npm value is displayed from the row data.
    expect(getNpmComboboxForType('openai')).toHaveValue('@ai-sdk/openai')
  })

  test('dropdown options include values from the backend npm_options', async () => {
    const user = userEvent.setup()
    renderTab()

    await screen.findByText('openai')

    const npmInput = getNpmComboboxForType('openai')
    await user.click(npmInput)

    // @ai-sdk/openai-compatible is only present in npm_options, proving the
    // suggestions come from the backend response (plus row values).
    const backendOption = await screen.findByRole('option', {
      name: '@ai-sdk/openai-compatible',
    })
    expect(backendOption).toBeInTheDocument()

    // A value already used by a row is offered too (merged from row data).
    const rowValueOption = screen.getByRole('option', {
      name: '@ai-sdk/anthropic',
    })
    expect(rowValueOption).toBeInTheDocument()
  })

  test('selecting an npm suggestion writes it to the corresponding row', async () => {
    const user = userEvent.setup()
    renderTab()

    await screen.findByText('openai')

    const npmInput = getNpmComboboxForType('openai')
    await user.click(npmInput)

    await user.click(
      await screen.findByRole('option', {
        name: '@ai-sdk/openai-compatible',
      })
    )

    await waitFor(() => {
      expect(npmInput).toHaveValue('@ai-sdk/openai-compatible')
    })
  })

  test('types a custom npm value not present in the suggestions', async () => {
    const user = userEvent.setup()
    renderTab()

    await screen.findByText('openai')

    const npmInput = getNpmComboboxForType('openai')
    await user.clear(npmInput)
    await user.type(npmInput, '@my-scope/my-provider')

    await waitFor(() => {
      expect(npmInput).toHaveValue('@my-scope/my-provider')
    })
  })
})

describe('EndpointsTabContent layout contract', () => {
  test('keeps a wide table and reserves enough width for the npm column', async () => {
    renderTab()

    await screen.findByText('openai')

    // Stable structure classes: table min width + npm column width. No fragile
    // snapshot of rendered markup.
    const table = document.querySelector('table')
    expect(table?.className).toContain('min-w-[900px]')

    const npmHeader = screen.getByRole('columnheader', { name: 'NPM' })
    expect(npmHeader.className).toContain('w-[26%]')
  })

  test('npm cells override the table overflow so the dropdown is not clipped', async () => {
    renderTab()

    await screen.findByText('openai')

    // StaticDataTable cells default to `overflow-hidden`, which would clip the
    // ComboboxInput dropdown to the cell height (it would appear hidden
    // behind the next row). The npm cell must opt out via `overflow-visible`.
    const npmInput = getNpmComboboxForType('openai')
    const cell = npmInput.closest('td')
    expect(cell?.className).toContain('overflow-visible')
    expect(cell?.className).not.toContain('overflow-hidden')

    // Opening the dropdown must render the option list (not clipped away).
    const user = userEvent.setup()
    await user.click(npmInput)
    const listbox = await screen.findByRole('listbox')
    expect(listbox).toBeInTheDocument()
  })
})

describe('EndpointsTabContent save', () => {
  test('top toolbar Save button submits the current rows', async () => {
    const user = userEvent.setup()
    vi.mocked(updateEndpointDefinitions).mockResolvedValue({
      success: true,
      message: '',
    })
    renderTab()

    await screen.findByText('openai')

    const saveButton = screen.getByRole('button', { name: 'Save' })
    expect(saveButton).toBeInTheDocument()
    expect(saveButton).toBeEnabled()

    await user.click(saveButton)

    await waitFor(() => {
      expect(vi.mocked(updateEndpointDefinitions)).toHaveBeenCalledWith({
        endpoints: ENDPOINTS,
      })
    })
  })
})
