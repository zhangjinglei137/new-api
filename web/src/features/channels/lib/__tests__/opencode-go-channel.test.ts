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
  CHANNEL_TYPE_OPENCODE_GO,
  CHANNEL_TYPE_OPTIONS,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import {
  ADVANCED_CUSTOM_TEMPLATE_OPTIONS,
  getAdvancedCustomTemplateConfig,
  stringifyAdvancedCustomConfig,
} from '../advanced-custom'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'
import { getChannelTypeConfig } from '../channel-type-config'
import { getChannelTypeIcon, getKeyPromptForType } from '../channel-utils'
import { channelSchema } from '../../types'

// The same prefill the drawer applies when type 61 is selected: the
// opencode_go template (7 routes) serialized into the advanced_custom field.
const OPENCODE_GO_TEMPLATE_CONFIG = stringifyAdvancedCustomConfig(
  getAdvancedCustomTemplateConfig('opencode_go')
)

function opencodeForm(overrides: Record<string, unknown> = {}) {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'OpenCode Go upstream',
    type: CHANNEL_TYPE_OPENCODE_GO,
    key: 'test-key',
    models: 'grok-4.5',
    advanced_custom: OPENCODE_GO_TEMPLATE_CONFIG,
    ...overrides,
  }
}

describe('OpenCode Go channel', () => {
  test('registers selection, ordering, model discovery, and icon metadata', () => {
    const option = CHANNEL_TYPE_OPTIONS.find(
      (item) => item.value === CHANNEL_TYPE_OPENCODE_GO
    )

    expect(option).toStrictEqual({
      value: CHANNEL_TYPE_OPENCODE_GO,
      label: 'OpenCode Go',
    })
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_OPENCODE_GO)).toBe(true)
    expect(getChannelTypeIcon(CHANNEL_TYPE_OPENCODE_GO)).toBe('OpenCode')
    expect(
      getKeyPromptForType(CHANNEL_TYPE_OPENCODE_GO)
    ).toBe('Enter API key for this channel')
    expect(
      getChannelTypeConfig(CHANNEL_TYPE_OPENCODE_GO).defaultBaseUrl
    ).toBe('https://opencode.ai/zen/go')
  })

  test('opencode_go template prefill matches the OpenCode Zen gateway routes', () => {
    const template = ADVANCED_CUSTOM_TEMPLATE_OPTIONS.find(
      (option) => option.value === 'opencode_go'
    )
    expect(template).toBeTruthy()
    if (!template) throw new Error('Expected opencode_go template option')
    expect(template.label).toBe('OpenCode Go')

    const config = getAdvancedCustomTemplateConfig('opencode_go')
    const routes = config.advanced_routes ?? []
    expect(routes.length).toBe(7)
    expect(
      routes.map((route) => route.incoming_path)
    ).toStrictEqual([
      '/v1/chat/completions',
      '/v1/chat/completions',
      '/v1/messages',
      '/v1/messages',
      '/v1/responses',
      '/v1/responses',
      '/v1/models',
    ])

    // *-free 模型走 zen 网关（完整 URL，忽略渠道 base URL），且排在各自
    // zen/go 兜底路由之前，保证首条命中优先分流。
    const freeChatRoute = routes[0]
    expect(freeChatRoute.upstream_path).toBe(
      'https://opencode.ai/zen/v1/chat/completions'
    )
    expect(freeChatRoute.models).toStrictEqual(['re:.*-free$'])

    const freeMessagesRoute = routes[2]
    expect(freeMessagesRoute.upstream_path).toBe(
      'https://opencode.ai/zen/v1/messages'
    )
    expect(freeMessagesRoute.models).toStrictEqual(['re:.*-free$'])
    expect(freeMessagesRoute.auth).toStrictEqual({
      type: 'header',
      name: 'x-api-key',
      value: '{api_key}',
    })

    const freeResponsesRoute = routes[4]
    expect(freeResponsesRoute.upstream_path).toBe(
      'https://opencode.ai/zen/v1/responses'
    )
    expect(freeResponsesRoute.models).toStrictEqual(['re:.*-free$'])
    expect(freeResponsesRoute.auth).toStrictEqual({
      type: 'header',
      name: 'Authorization',
      value: 'Bearer {api_key}',
    })

    // zen/go 兜底路由保持相对路径，镜像各自端点的 auth。
    const responsesRoute = routes[5]
    expect(responsesRoute.models).toBeUndefined()
    expect(responsesRoute.auth).toStrictEqual({
      type: 'header',
      name: 'Authorization',
      value: 'Bearer {api_key}',
    })

    const messagesRoute = routes[3]
    expect(messagesRoute.auth).toStrictEqual({
      type: 'header',
      name: 'x-api-key',
      value: '{api_key}',
    })
  })

  test('validates the OpenCode Go form with the prefill base URL', () => {
    // The prefill template uses relative upstream paths, which require the
    // prefill base URL (https://opencode.ai/zen/go) to validate.
    expect(
      channelFormSchema.safeParse(
        opencodeForm({ base_url: 'https://opencode.ai/zen/go' })
      ).success
    ).toBe(true)
  })

  test('requires a Base URL for relative advanced routes', () => {
    const result = channelFormSchema.safeParse(opencodeForm({ base_url: '' }))
    expect(result.success).toBe(false)
    if (!result.success) {
    expect(
      result.error.issues.some(
        (issue) =>
          issue.path[0] === 'base_url' &&
          issue.message ===
            'Base URL is required when an advanced route uses an upstream path'
      )
    ).toBe(true)
    }
  })

  test('round-trips advanced_custom through settings JSON', () => {
    const payload = transformFormDataToCreatePayload(opencodeForm())
    const settings = JSON.parse(payload.channel.settings as string)

    // advanced_custom must survive serialization: the backend requires it for
    // type 61 (IsAdvancedCustomChannelType), otherwise channel creation fails.
    expect(settings.advanced_custom).toBeTruthy()
    expect(
      (settings.advanced_custom as { advanced_routes?: unknown[] })
        .advanced_routes?.length
    ).toBe(7)

    // Editing a channel restores the values from the stored settings JSON.
    const channel = channelSchema.parse({
      id: 1,
      type: CHANNEL_TYPE_OPENCODE_GO,
      key: 'test-key',
      status: 1,
      name: 'OpenCode Go upstream',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      models: 'grok-4.5',
      settings: JSON.stringify({
        advanced_custom: {
          advanced_routes: [
            { incoming_path: '/v1/chat/completions', converter: 'none' },
          ],
        },
      }),
    })
    const defaults = transformChannelToFormDefaults(channel)
    expect(defaults.advanced_custom?.trim()).toBeTruthy()
  })

  test('removes OpenCode advanced settings when the channel type changes away', () => {
    const payload = transformFormDataToCreatePayload(opencodeForm({ type: 1 }))
    const settings = JSON.parse(payload.channel.settings as string)
    expect('advanced_custom' in settings).toBe(false)
  })

  test('rejects type 61 without an advanced_custom config', () => {
    const result = channelFormSchema.safeParse(opencodeForm({ type: 99, advanced_custom: '' }))
    expect(result.success).toBe(false)
    if (!result.success) {
    expect(
      result.error.issues.some(
        (issue) =>
          issue.path[0] === 'advanced_custom' &&
          issue.message === 'Advanced custom configuration is required'
      )
    ).toBe(true)
    }
  })
})
