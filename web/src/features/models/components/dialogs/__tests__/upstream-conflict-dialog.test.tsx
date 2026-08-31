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
import i18next from 'i18next'
import { useEffect } from 'react'
import { afterEach, beforeAll, beforeEach, describe, expect, test, vi } from 'vitest'

import { ModelsProvider, useModels } from '../../models-provider'
import { UpstreamConflictDialog } from '../upstream-conflict-dialog'
import type { SyncDiffData } from '../../../types'

vi.mock('../../../api', () => ({
  applyUpstreamOverwrite: vi.fn(),
}))

// Two models, three fields total (1 model × 2 fields + 1 model × 1 field).
const CONFLICTS: SyncDiffData['conflicts'] = [
  {
    model_name: 'model-a',
    fields: [
      { field: 'description', local: 'old', upstream: 'new' },
      { field: 'icon', local: null, upstream: 'SiOpenAI' },
    ],
  },
  {
    model_name: 'model-b',
    fields: [{ field: 'tags', local: [], upstream: ['fast'] }],
  },
]

const ZH_TRANSLATIONS = {
  '{{count}} model with conflicts': '{{count}} 个模型有冲突',
  '{{count}} field showing • {{selected}} selected':
    '{{count}} 个字段显示 • {{selected}} 个已选择',
  'Showing {{start}}-{{end}} of {{count}} fields':
    '显示第 {{start}}-{{end}} 条,共 {{count}} 个字段',
}

function SeedConflicts({ conflicts }: { conflicts: SyncDiffData['conflicts'] }) {
  const { setUpstreamConflicts } = useModels()
  useEffect(() => {
    setUpstreamConflicts(conflicts ?? [])
  }, [conflicts, setUpstreamConflicts])
  return null
}

function renderDialog(conflicts: SyncDiffData['conflicts'] = CONFLICTS) {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <ModelsProvider>
        <SeedConflicts conflicts={conflicts} />
        <UpstreamConflictDialog open onOpenChange={vi.fn()} />
      </ModelsProvider>
    </QueryClientProvider>
  )
}

beforeAll(async () => {
  i18next.addResourceBundle('zh', 'translation', ZH_TRANSLATIONS)
})

beforeEach(async () => {
  await i18next.changeLanguage('zh')
})

afterEach(async () => {
  await i18next.changeLanguage('en')
})

describe('UpstreamConflictDialog counters', () => {
  test('renders localized plural counters without English plural mixing', async () => {
    renderDialog()

    await waitFor(() => {
      expect(screen.getByText('2 个模型有冲突')).toBeInTheDocument()
    })

    // Fields counter with the selection count (0 initially).
    expect(screen.getByText('3 个字段显示 • 0 个已选择')).toBeInTheDocument()

    // Footer range counter.
    expect(screen.getByText('显示第 1-3 条,共 3 个字段')).toBeInTheDocument()

    // No leftover English plural artifact (e.g. "模型s" / "字段s").
    expect(screen.queryByText(/模型s|字段s/)).not.toBeInTheDocument()
  })

  test('shows the singular form for a single conflicting model', async () => {
    renderDialog([
      {
        model_name: 'model-a',
        fields: [{ field: 'description', local: 'old', upstream: 'new' }],
      },
    ])

    await waitFor(() => {
      expect(screen.getByText('1 个模型有冲突')).toBeInTheDocument()
    })
    expect(screen.getByText('1 个字段显示 • 0 个已选择')).toBeInTheDocument()
  })
})