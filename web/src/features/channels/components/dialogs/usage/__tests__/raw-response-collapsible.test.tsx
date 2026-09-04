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
import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, test } from 'vitest'

import { RawResponseCollapsible } from '../raw-response-collapsible'

function ControlledCollapsible({ text }: { text: string }) {
  const [open, setOpen] = useState(false)
  return (
    <RawResponseCollapsible
      open={open}
      onOpenChange={setOpen}
      text={text}
    />
  )
}

function renderPanel(text: string) {
  return render(<ControlledCollapsible text={text} />)
}

describe('RawResponseCollapsible', () => {
  test('keeps a huge raw JSON response inside a scrollable container instead of overflowing the dialog', () => {
    // A realistic large upstream response: ~40KB of nested JSON, including
    // long unbroken lines that must wrap rather than widen the container.
    const pools = Array.from({ length: 60 }, (_, i) => ({
      pool_type: i % 2 === 0 ? 'general' : 'dedicated',
      name: `Pool-${i}-with-a-very-long-name-that-would-previously-blow-the-dialog-width-open-beyond-any-reasonable-bound-${i}`,
      model_ids: Array.from({ length: 20 }, (_, m) => `model-${i}-${m}`),
      window_5h: { limit: 1000 + i, used: 100, remaining: 900 + i },
      window_7d: { limit: 5000, used: 1000, remaining: 4000 },
    }))
    const bigJson = JSON.stringify({ success: true, data: { pools } }, null, 2)
    expect(bigJson.length).toBeGreaterThan(20_000)

    renderPanel(bigJson)
    fireEvent.click(screen.getByText('Show raw upstream response'))

    const pre = screen.getByText(/"pool_type"/).closest('pre')
    expect(pre).not.toBeNull()

    // The raw text is fully rendered, not truncated.
    expect(pre?.textContent).toContain('Pool-59-with-a-very-long-name')

    // The scroll container constrains the panel height and scrolls inside
    // itself, so a large response can never stretch past the dialog bounds.
    const scrollContainer = pre?.parentElement
    expect(scrollContainer?.className).toContain('overflow-y-auto')
    expect(scrollContainer?.className).toContain('max-h-')

    // Long lines wrap inside the <pre> instead of widening the container.
    expect(pre?.className).toContain('whitespace-pre-wrap')
    expect(pre?.className).toContain('break-words')
  })

  test('keeps the panel collapsed until the trigger is clicked', () => {
    renderPanel('{"hidden":true}')
    expect(screen.queryByText(/"hidden"/)).not.toBeInTheDocument()

    fireEvent.click(screen.getByText('Show raw upstream response'))
    expect(screen.getByText(/"hidden"/)).toBeInTheDocument()
  })
})
