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
import type { TFunction } from 'i18next'
import { describe, expect, test, vi } from 'vitest'

import {
  getModelsSectionNavItems,
  MODELS_DEFAULT_SECTION,
  MODELS_SECTION_IDS,
} from '../../section-registry'

// Identity translation: navigation titles are i18n keys, so components
// under test receive the keys themselves (see Task 4 for title assertions).
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const t = ((key: string) => key) as TFunction

describe('MODELS_SECTION_IDS', () => {
  test('orders model page sections as metadata, vendors, endpoints, deployments', () => {
    expect(MODELS_SECTION_IDS).toEqual([
      'metadata',
      'vendors',
      'endpoints',
      'deployments',
    ])
  })
})

describe('models navigation URLs', () => {
  test('maps every section to its /models/<section> URL in tab order', () => {
    // 每个 tab 对应一个独立 URL（path 风格），点击 tab 即切换 URL，这是
    // tab 顺序与 URL 切换契约的单一事实来源。
    const navItems = getModelsSectionNavItems(t)

    expect(navItems.map((item) => item.url)).toEqual([
      '/models/metadata',
      '/models/vendors',
      '/models/endpoints',
      '/models/deployments',
    ])
  })

  test('falls back to the metadata section when the URL has no section', () => {
    expect(MODELS_DEFAULT_SECTION).toBe('metadata')
  })
})
