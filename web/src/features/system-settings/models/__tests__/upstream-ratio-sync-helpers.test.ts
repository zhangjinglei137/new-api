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
import { describe, expect, it } from 'vitest'

import {
  MODELS_DEV_PRESET_ID,
  MODELS_DEV_PRESET_NAME,
  OFFICIAL_CHANNEL_ID,
  OFFICIAL_CHANNEL_NAME,
  OPENCODE_GO_PRESET_ID,
  OPENCODE_GO_PRESET_NAME,
} from '../constants'
import { getUpstreamDisplayName } from '../upstream-ratio-sync-helpers'

const t = (key: string) => key

describe('getUpstreamDisplayName', () => {
  it('keeps the local OpenCode Go preset display after the upstream merge', () => {
    // The upstream rewrite of this helper must not drop the local
    // opencode-go preset: both the bare name and the "name(id)" source
    // form (as sent by the ratio-sync controller) must resolve to the
    // OpenCode Go label instead of the raw source string.
    expect(getUpstreamDisplayName(OPENCODE_GO_PRESET_NAME, t)).toBe(
      'OpenCode Go pricing preset'
    )
    expect(
      getUpstreamDisplayName(
        `${OPENCODE_GO_PRESET_NAME}(${OPENCODE_GO_PRESET_ID})`,
        t
      )
    ).toBe('OpenCode Go pricing preset')
  })

  it('still maps the official and models.dev presets', () => {
    expect(getUpstreamDisplayName(OFFICIAL_CHANNEL_NAME, t)).toBe(
      'Official pricing preset'
    )
    expect(
      getUpstreamDisplayName(
        `${OFFICIAL_CHANNEL_NAME}(${OFFICIAL_CHANNEL_ID})`,
        t
      )
    ).toBe('Official pricing preset')
    expect(getUpstreamDisplayName(MODELS_DEV_PRESET_NAME, t)).toBe(
      'models.dev pricing preset'
    )
    expect(
      getUpstreamDisplayName(
        `${MODELS_DEV_PRESET_NAME}(${MODELS_DEV_PRESET_ID})`,
        t
      )
    ).toBe('models.dev pricing preset')
  })

  it('falls back to the raw source string for unknown presets', () => {
    expect(getUpstreamDisplayName('custom-source', t)).toBe('custom-source')
  })
})
