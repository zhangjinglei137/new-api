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
import { describe, expect, test, vi } from 'vitest'

import { SortCell, useChannelsColumns } from '../channels-columns'
import type { TagRow } from '../../lib'
import type { Channel } from '../../types'

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
    sensitiveVisible: false,
    upstream: {
      openModal: vi.fn(),
      detectChannelUpdates: vi.fn(),
    },
  }),
}))

function createChannel(overrides: Partial<Channel> = {}): Channel {
  return {
    id: 7,
    name: 'Test Channel',
    type: 1,
    status: 1,
    settings: '{}',
    sort: 5,
    ...overrides,
  } as Channel
}

function createTagRow(): TagRow {
  return {
    ...createChannel({ sort: 5 }),
    tag: 'test-tag',
    children: [],
  } as TagRow
}

function collectColumns() {
  let columns: ReturnType<typeof useChannelsColumns>
  function CaptureColumns() {
    columns = useChannelsColumns()
    return null
  }
  render(<CaptureColumns />)
  return () => columns
}

describe('SortCell', () => {
  test('renders an inline sort editor for a regular channel row', () => {
    render(<SortCell channel={createChannel({ sort: 5 })} />)

    expect(screen.getByRole('button', { name: 'Increment' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '5' })).toBeInTheDocument()
  })

  test('renders nothing for a tag aggregate row', () => {
    const { container } = render(<SortCell channel={createTagRow()} />)

    expect(
      screen.queryByRole('button', { name: 'Increment' })
    ).not.toBeInTheDocument()
    expect(container).toBeEmptyDOMElement()
  })
})

describe('sort column definition', () => {
  test('places the sort column right before the priority column', () => {
    const getColumns = collectColumns()
    const columns = getColumns()

    const priorityIndex = columns.findIndex(
      (column) => 'accessorKey' in column && column.accessorKey === 'priority'
    )
    const sortIndex = columns.findIndex(
      (column) => 'accessorKey' in column && column.accessorKey === 'sort'
    )

    expect(sortIndex).toBeGreaterThanOrEqual(0)
    expect(priorityIndex).toBe(sortIndex + 1)
  })

  test('configures sortable header, mobile hidden and size', () => {
    const getColumns = collectColumns()
    const sortColumn = getColumns().find(
      (column) => 'accessorKey' in column && column.accessorKey === 'sort'
    ) as
      | {
          accessorKey?: string
          header?: unknown
          size?: unknown
          meta?: { mobileHidden?: unknown }
        }
      | undefined

    expect(sortColumn?.accessorKey).toBe('sort')
    expect(sortColumn?.header).toBe('Sort Number')
    expect(sortColumn?.size).toBe(100)
    expect(sortColumn?.meta?.mobileHidden).toBe(true)
  })
})
