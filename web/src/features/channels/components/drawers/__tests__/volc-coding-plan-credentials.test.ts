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
import { beforeEach, describe, expect, test, vi } from 'vitest'

import {
  clearVolcCodingPlanCredential,
  hasVolcCodingPlanCredential,
  saveVolcCodingPlanCredentials,
} from '../../../lib/volc-coding-plan-credentials'
import { updateVolcCodingPlanCredentials } from '../../../api'

vi.mock('../../../api', () => ({
  updateVolcCodingPlanCredentials: vi.fn(),
}))

const updateMock = vi.mocked(updateVolcCodingPlanCredentials)

describe('VolcEngine Coding Plan credentials', () => {
  beforeEach(() => {
    updateMock.mockReset()
    updateMock.mockResolvedValue({ success: true })
  })

  test('requires at least one non-blank credential to be considered present', () => {
    expect(
      hasVolcCodingPlanCredential({
        csrfToken: ' ',
        cookie: '',
        accessKeyId: '',
        secretAccessKey: '  ',
      })
    ).toBe(false)

    expect(
      hasVolcCodingPlanCredential({
        csrfToken: '',
        cookie: '',
        accessKeyId: 'AK-test',
        secretAccessKey: '',
      })
    ).toBe(true)
  })

  test('saves AK/SK along with CSRF and cookie, trimming each value', async () => {
    await saveVolcCodingPlanCredentials(7, {
      csrfToken: ' csrf ',
      cookie: 'cookie',
      accessKeyId: ' AK-test ',
      secretAccessKey: ' SK-test ',
    })

    expect(updateMock).toHaveBeenCalledWith(7, {
      csrf_token: 'csrf',
      cookie: 'cookie',
      access_key_id: 'AK-test',
      secret_access_key: 'SK-test',
    })
  })

  test('omits blank values when saving credentials', async () => {
    await saveVolcCodingPlanCredentials(7, {
      csrfToken: '',
      cookie: '  ',
      accessKeyId: 'AK-test',
      secretAccessKey: '',
    })

    expect(updateMock).toHaveBeenCalledWith(7, {
      csrf_token: undefined,
      cookie: undefined,
      access_key_id: 'AK-test',
      secret_access_key: undefined,
    })
  })

  test.each([
    ['csrf', { clear_csrf: true }],
    ['cookie', { clear_cookie: true }],
    ['access_key_id', { clear_access_key_id: true }],
    ['secret_access_key', { clear_secret_access_key: true }],
  ] as const)(
    'clears the %s credential through the matching clear_* field',
    async (kind, expected) => {
      await clearVolcCodingPlanCredential(7, kind)
      expect(updateMock).toHaveBeenCalledWith(7, expected)
    }
  )

  test('returns the backend response unchanged', async () => {
    updateMock.mockResolvedValueOnce({
      success: true,
      data: {
        csrf_token_configured: true,
        cookie_configured: false,
        access_key_id_configured: true,
        secret_access_key_configured: false,
      },
    })

    const result = await saveVolcCodingPlanCredentials(7, {
      csrfToken: 'csrf',
      cookie: '',
      accessKeyId: 'AK-test',
      secretAccessKey: '',
    })

    expect(result.data?.access_key_id_configured).toBe(true)
    expect(result.data?.secret_access_key_configured).toBe(false)
  })
})