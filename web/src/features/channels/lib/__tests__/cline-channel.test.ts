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
  CHANNEL_PROVIDER_PRESENTATION,
  CHANNEL_TYPE_CLINE,
  CHANNEL_TYPE_OPTIONS,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import {
  CHANNEL_TYPE_DEFAULTS,
  channelTypeDefaultsToApply,
} from '../channel-type-defaults'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'
import { getChannelTypeIcon } from '../channel-utils'
import { channelSchema } from '../../types'

const CLINE_DEFAULT_BASE_URL = 'https://api.cline.bot/api'

function clineForm(overrides: Record<string, unknown> = {}) {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'Cline upstream',
    type: CHANNEL_TYPE_CLINE,
    key: 'test-key',
    models: 'deepseek-chat',
    ...overrides,
  }
}

describe('Cline channel', () => {
  test('registers selection, ordering, model discovery, and icon metadata', () => {
    const option = CHANNEL_TYPE_OPTIONS.find(
      (item) => item.value === CHANNEL_TYPE_CLINE
    )

    expect(option).toStrictEqual({
      value: CHANNEL_TYPE_CLINE,
      label: 'Cline',
    })
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_CLINE)).toBe(true)
    expect(getChannelTypeIcon(CHANNEL_TYPE_CLINE)).toBe('Cline')
    expect(
      CHANNEL_PROVIDER_PRESENTATION[CHANNEL_TYPE_CLINE]?.descriptionKey
    ).toBe('Connect to Cline model services')
  })

  test('declares the default base URL and request-body param override', () => {
    const defaults = CHANNEL_TYPE_DEFAULTS[CHANNEL_TYPE_CLINE]

    expect(defaults.base_url).toBe(CLINE_DEFAULT_BASE_URL)
    expect(JSON.parse(defaults.param_override ?? '')).toStrictEqual({
      providerOptions: { gateway: { only: ['deepseek'] } },
      provider: { only: ['deepseek'] },
    })
  })

  test('prefills base_url and param_override only while the fields are empty', () => {
    // 新建渠道、字段均为空：两项默认值都应预填。
    const patch = channelTypeDefaultsToApply(CHANNEL_TYPE_CLINE, {
      base_url: '',
      param_override: '',
    })
    expect(patch.base_url).toBe(CLINE_DEFAULT_BASE_URL)
    expect(patch.param_override).toBe(
      CHANNEL_TYPE_DEFAULTS[CHANNEL_TYPE_CLINE].param_override
    )

    // 用户已填写（或可清空后不再回填）：不覆盖现有值。
    expect(
      channelTypeDefaultsToApply(CHANNEL_TYPE_CLINE, {
        base_url: 'https://proxy.example.com',
        param_override: '',
      }).base_url
    ).toBeUndefined()
    expect(
      channelTypeDefaultsToApply(CHANNEL_TYPE_CLINE, {
        base_url: '',
        param_override: '{"temperature":0.7}',
      }).param_override
    ).toBeUndefined()

    // 无默认值的类型不产生任何预填。
    expect(
      channelTypeDefaultsToApply(1, { base_url: '', param_override: '' })
    ).toStrictEqual({})
  })

  test('accepts the clinepass endpoint profile in the form schema', () => {
    expect(
      channelFormSchema.safeParse(
        clineForm({ endpoint_profile: 'clinepass' })
      ).success
    ).toBe(true)
    expect(
      channelFormSchema.safeParse(clineForm({ endpoint_profile: '' })).success
    ).toBe(true)

    const result = channelFormSchema.safeParse(
      clineForm({ endpoint_profile: 'bogus' })
    )
    expect(result.success).toBe(false)
  })

  test('round-trips the access mode through the settings JSON without touching base_url', () => {
    // ClinePass（计划计费）写入 settings.endpoint_profile。
    const payload = transformFormDataToCreatePayload(
      clineForm({
        endpoint_profile: 'clinepass',
        base_url: CLINE_DEFAULT_BASE_URL,
      })
    )
    const settings = JSON.parse(payload.channel.settings as string)
    expect(settings.endpoint_profile).toBe('clinepass')
    expect(payload.channel.base_url).toBe(CLINE_DEFAULT_BASE_URL)

    // 按量计费（''）不落 settings.endpoint_profile。
    const paygPayload = transformFormDataToCreatePayload(clineForm())
    expect(
      'endpoint_profile' in JSON.parse(paygPayload.channel.settings as string)
    ).toBe(false)

    // 编辑回显：从 settings JSON 读回 clinepass。
    const channel = channelSchema.parse({
      id: 1,
      type: CHANNEL_TYPE_CLINE,
      key: 'test-key',
      status: 1,
      name: 'Cline upstream',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      models: 'deepseek-chat',
      base_url: CLINE_DEFAULT_BASE_URL,
      settings: JSON.stringify({ endpoint_profile: 'clinepass' }),
    })
    expect(transformChannelToFormDefaults(channel).endpoint_profile).toBe(
      'clinepass'
    )

    // 保存更新时同样携带，且自定义 base_url 不被计费方式切换改写。
    const updatePayload = transformFormDataToUpdatePayload(
      clineForm({
        endpoint_profile: 'clinepass',
        base_url: 'https://proxy.example.com',
      }),
      1
    )
    expect(
      JSON.parse(updatePayload.settings as string).endpoint_profile
    ).toBe('clinepass')
    expect(updatePayload.base_url).toBe('https://proxy.example.com')
  })

  test('drops the endpoint profile from settings when the type changes away', () => {
    const payload = transformFormDataToCreatePayload(
      clineForm({ type: 1, endpoint_profile: 'clinepass' })
    )
    const settings = JSON.parse(payload.channel.settings as string)
    expect('endpoint_profile' in settings).toBe(false)
  })
})
