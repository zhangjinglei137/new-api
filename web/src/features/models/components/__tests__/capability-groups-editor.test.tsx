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

import {
  CapabilityGroupsEditor,
  parseCapabilitiesToGroups,
  serializeGroupsToCapabilities,
} from '../capability-groups-editor'

// Realistic upstream (opencode-go) capabilities: reasoning_options is an
// OBJECT array `[{"type":"effort","values":["high","max"]}]`, which used to
// crash chip rendering with React error #31 (object rendered as child).
const LEGACY_SAMPLE = JSON.stringify({
  modalities: { input: ['text', 'image'], output: ['text'] },
  reasoning_options: [{ type: 'effort', values: ['high', 'max'] }],
  limits: { context: 128000, output: 8192 },
})

const GROUPS_SAMPLE = JSON.stringify([
  {
    name: 'chat',
    modalities: { input: ['text'], output: ['text'] },
    reasoning_options: [{ type: 'effort', values: ['high'] }],
  },
  { name: 'image', modalities: { input: ['image'] } },
])

describe('parseCapabilitiesToGroups', () => {
  test('maps object-shaped reasoning_options to their type strings in the legacy format', () => {
    const groups = parseCapabilitiesToGroups(LEGACY_SAMPLE)

    expect(groups).toHaveLength(1)
    expect(groups[0].name).toBe('chat')
    expect(groups[0].input).toEqual(['text', 'image'])
    expect(groups[0].output).toEqual(['text'])
    expect(groups[0].limits).toEqual({ context: 128000, output: 8192 })
    // Object entries are mapped to their `type` so they stay visible/editable.
    expect(groups[0].reasoning_options).toEqual(['effort'])
  })

  test('normalizes object-shaped reasoning_options in the groups format', () => {
    const groups = parseCapabilitiesToGroups(GROUPS_SAMPLE)

    expect(groups).toHaveLength(2)
    expect(groups[0].name).toBe('chat')
    expect(groups[0].input).toEqual(['text'])
    expect(groups[0].reasoning_options).toEqual(['effort'])
    expect(groups[1].name).toBe('image')
    expect(groups[1].input).toEqual(['image'])
  })

  test('keeps string entries and maps objects when both are mixed', () => {
    const groups = parseCapabilitiesToGroups(
      JSON.stringify({
        reasoning_options: ['low', { type: 'effort' }, 'high'],
      })
    )

    expect(groups[0].reasoning_options).toEqual(['low', 'effort', 'high'])
  })

  test('reads the flat shape written by earlier editor versions', () => {
    const groups = parseCapabilitiesToGroups(
      JSON.stringify([
        { name: 'chat', input: ['text'], output: ['text'] },
      ])
    )

    expect(groups[0].input).toEqual(['text'])
    expect(groups[0].output).toEqual(['text'])
  })
})

describe('serializeGroupsToCapabilities', () => {
  test('writes nested modalities and object reasoning_options per backend contract', () => {
    const serialized = serializeGroupsToCapabilities([
      { name: 'chat', input: ['text'], reasoning_options: ['low', 'effort'] },
    ])

    const parsed = JSON.parse(serialized) as Array<Record<string, unknown>>
    expect(parsed).toHaveLength(1)
    expect(parsed[0].name).toBe('chat')
    // Backend contract: modalities is a nested object.
    expect(parsed[0].modalities).toEqual({ input: ['text'] })
    // Backend contract: reasoning_options is an object array.
    expect(parsed[0].reasoning_options).toEqual([
      { type: 'low' },
      { type: 'effort' },
    ])
  })

  test('omits empty modality arrays and empty reasoning options', () => {
    const serialized = serializeGroupsToCapabilities([
      { name: 'chat', input: [], output: [], reasoning_options: [] },
      { name: 'empty-ish' },
    ])

    const parsed = JSON.parse(serialized) as Array<Record<string, unknown>>
    expect(parsed[0]).toEqual({ name: 'chat' })
    expect(parsed[1]).toEqual({ name: 'empty-ish' })
  })

  test('round-trips a parsed legacy sample into valid JSON without objects', () => {
    const groups = parseCapabilitiesToGroups(LEGACY_SAMPLE)
    const serialized = serializeGroupsToCapabilities(groups)
    const parsed = JSON.parse(serialized) as Array<Record<string, unknown>>

    expect(parsed[0].name).toBe('chat')
    expect(parsed[0].modalities).toEqual({
      input: ['text', 'image'],
      output: ['text'],
    })
    expect(parsed[0].reasoning_options).toEqual([{ type: 'effort' }])
    expect(parsed[0].limits).toEqual({ context: 128000, output: 8192 })
  })
})

describe('CapabilityGroupsEditor rendering', () => {
  test('renders upstream capabilities with object reasoning_options without throwing', () => {
    expect(() =>
      render(
        <CapabilityGroupsEditor value={LEGACY_SAMPLE} onChange={vi.fn()} />
      )
    ).not.toThrow()

    // The legacy shape is shown as a "chat" group (readable, editable).
    expect(screen.getByDisplayValue('chat')).toBeInTheDocument()
  })
})