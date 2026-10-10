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
import { describe, expect, it } from 'vitest'

import { ModelBadge } from '../model-badge'

describe('model badge inline layout contract', () => {
  it('keeps the provider icon and model name in one non-wrapping row and truncates a long name instead of overflowing', () => {
    const { container } = render(
      // Simulate a narrow desktop table cell.
      <div style={{ width: 80 }}>
        <ModelBadge modelName='codex-auto-review' actualModel='gpt-5-codex' />
      </div>
    )

    const badge = container.querySelector('[data-slot=status-badge]')
    expect(badge).not.toBeNull()
    expect(badge).toHaveClass('max-w-full')
    expect(badge).not.toHaveClass('max-w-none')

    const name = screen.getByText('codex-auto-review')
    expect(name).toHaveClass('truncate')

    const icon = screen.getByLabelText('OpenAI')
    // The icon and the name must stay in the same flex row so the icon is
    // never pushed onto a second line in narrow columns.
    expect(name.parentElement).toBe(icon.parentElement)
    expect(name.parentElement).toHaveClass('flex')
    expect(name.parentElement).not.toHaveClass('flex-wrap')
  })

  it('still allows the name to wrap in mobile wrapText mode', () => {
    render(
      <ModelBadge
        modelName='codex-auto-review'
        wrapText
        onInspect={() => {}}
      />
    )
    expect(screen.getByText('codex-auto-review')).toHaveClass('line-clamp-2')
  })
})
