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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { TFunction } from 'i18next'

import { getSyncSourceOptions } from '../constants'
import type { SyncSource } from '../types'

// Identity translation: labels/descriptions are i18n keys, so the returned
// values are compared against the keys themselves.
const t = ((key: string) => key) as TFunction

describe('getSyncSourceOptions', () => {
  test('offers the opencode-go source as a selectable option', () => {
    const options = getSyncSourceOptions(t)
    const opencodeGo = options.find((option) => option.value === 'opencode-go')

    assert.deepEqual(opencodeGo, {
      label: 'OpenCode Go',
      value: 'opencode-go' as SyncSource,
      description: 'Sync from the official opencode-go catalog.',
      disabled: false,
    })
  })

  test('keeps the official source enabled and the config source disabled', () => {
    const options = getSyncSourceOptions(t)

    const official = options.find((option) => option.value === 'official')
    assert.ok(official)
    assert.equal(official.disabled, false)

    const config = options.find((option) => option.value === 'config')
    assert.ok(config)
    assert.equal(config.disabled, true)
  })
})
