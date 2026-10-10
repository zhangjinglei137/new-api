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
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, renderHook, screen } from '@testing-library/react'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { beforeAll, describe, expect, test } from 'vitest'

import { usageLogSchema, type UsageLog } from '../../data/schema'
import type { LogOtherData } from '../../types'
import { useCommonLogsColumns } from '../columns/common-logs-columns'
import { UsageLogsProvider } from '../usage-logs-provider'

function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <UsageLogsProvider>{children}</UsageLogsProvider>
    </QueryClientProvider>
  )
}

function makeLog(type: number, other: LogOtherData): UsageLog {
  return usageLogSchema.parse({
    id: 1,
    user_id: 1,
    created_at: 1788840000,
    type,
    content: '',
    username: 'admin',
    token_name: 'token',
    model_name: 'gpt-test',
    quota: 100,
    prompt_tokens: 1,
    completion_tokens: 1,
    use_time: 1,
    is_stream: false,
    channel: 3,
    channel_name: 'ch-3',
    token_id: 1,
    group: 'default',
    ip: '',
    other: JSON.stringify(other),
    request_id: 'req-1',
    upstream_request_id: '',
  })
}

function renderColumns(admin: boolean) {
  return renderHook(() => useCommonLogsColumns(admin, false), { wrapper })
}

function LinkCountCell({ log }: { log: UsageLog }) {
  const columns = useCommonLogsColumns(true, false)
  const table = useReactTable({
    data: [log],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0].getVisibleCells()
    .find((c) => c.column.id === 'link-count')
  if (!cell) return null
  return <>{flexRender(cell.column.columnDef.cell, cell.getContext())}</>
}

function renderCell(log: UsageLog) {
  return render(<LinkCountCell log={log} />, { wrapper })
}

describe('usage log retry count column', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      'Retry Count': 'Retry Count',
    })
  })

  test('adds the retry count column immediately after the channel column for admins', () => {
    const { result } = renderColumns(true)
    const ids = result.current.map((c) => c.id)
    expect(ids).toContain('link-count')
    expect(ids.indexOf('link-count')).toBe(ids.indexOf('channel') + 1)
  })

  test('omits the retry count column for non-admins', () => {
    const { result } = renderColumns(false)
    expect(result.current.map((c) => c.id)).not.toContain('link-count')
  })

  test.each([
    { useChannel: [3], expected: '0' },
    { useChannel: [3, 5, 7], expected: '2' },
  ])(
    'shows $expected retries for use_channel $useChannel',
    ({ useChannel, expected }) => {
      renderCell(makeLog(2, { admin_info: { use_channel: useChannel } }))
      expect(screen.getByText(expected)).toBeVisible()
    }
  )

  test('shows an empty placeholder for a relay log with no channel chain', () => {
    renderCell(makeLog(2, { admin_info: { use_channel: [] } }))
    expect(screen.getByText('-')).toBeVisible()
  })

  test('renders nothing for a non relay log type', () => {
    const { container } = renderCell(makeLog(7, {}))
    expect(container.textContent).toBe('')
  })
})
