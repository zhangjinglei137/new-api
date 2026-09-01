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
import { describe, expect, test } from 'vitest'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'
import { channelSchema } from '../../types'

const VOLC_CODING_PLAN_TYPE = 45

function volcCodingPlanForm(overrides: Record<string, unknown> = {}) {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'VolcEngine Coding Plan',
    type: VOLC_CODING_PLAN_TYPE,
    base_url: 'https://ark.cn-beijing.volces.com',
    key: 'test-key',
    endpoint_profile: 'coding' as const,
    ...overrides,
  }
}

describe('VolcEngine Coding Plan AK/SK credentials', () => {
  test('serializes trimmed AK/SK into the settings JSON on create', () => {
    const payload = transformFormDataToCreatePayload(
      volcCodingPlanForm({
        volc_coding_plan_access_key_id: ' AK-test ',
        volc_coding_plan_secret_access_key: ' SK-test ',
      })
    )
    const settings = JSON.parse(payload.channel.settings as string)
    expect(settings.volc_coding_plan_access_key_id).toBe('AK-test')
    expect(settings.volc_coding_plan_secret_access_key).toBe('SK-test')
  })

  test('omits empty AK/SK so the backend keeps the stored value', () => {
    const payload = transformFormDataToCreatePayload(
      volcCodingPlanForm({
        volc_coding_plan_access_key_id: '',
        volc_coding_plan_secret_access_key: '  ',
      })
    )
    const settings = JSON.parse(payload.channel.settings as string)
    expect('volc_coding_plan_access_key_id' in settings).toBe(false)
    expect('volc_coding_plan_secret_access_key' in settings).toBe(false)
  })

  test('does not leak AK/SK into settings for other channel types', () => {
    const payload = transformFormDataToCreatePayload(
      volcCodingPlanForm({
        type: 1,
        volc_coding_plan_access_key_id: 'AK-test',
        volc_coding_plan_secret_access_key: 'SK-test',
      })
    )
    const settings = JSON.parse(payload.channel.settings as string)
    expect('volc_coding_plan_access_key_id' in settings).toBe(false)
    expect('volc_coding_plan_secret_access_key' in settings).toBe(false)
  })

  test('does not populate AK/SK from the backend when editing', () => {
    const channel = channelSchema.parse({
      id: 1,
      type: VOLC_CODING_PLAN_TYPE,
      key: 'test-key',
      status: 1,
      name: 'VolcEngine Coding Plan',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      models: 'deepseek-v3.2',
      settings: JSON.stringify({
        endpoint_profile: 'coding',
        volc_coding_plan_access_key_id: 'AK-test',
        volc_coding_plan_secret_access_key: 'SK-test',
      }),
    })
    const defaults = transformChannelToFormDefaults(channel)
    expect(defaults.volc_coding_plan_access_key_id).toBe('')
    expect(defaults.volc_coding_plan_secret_access_key).toBe('')
  })
})