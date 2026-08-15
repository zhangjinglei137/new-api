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
// opencode_go template (4 routes) serialized into the advanced_custom field.
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
    opencode_workspace_id: 'wrk_test',
    opencode_auth_cookie: 'session=abc123',
    ...overrides,
  }
}

describe('OpenCode Go channel', () => {
  test('registers selection, ordering, model discovery, and icon metadata', () => {
    const option = CHANNEL_TYPE_OPTIONS.find(
      (item) => item.value === CHANNEL_TYPE_OPENCODE_GO
    )

    assert.deepEqual(option, {
      value: CHANNEL_TYPE_OPENCODE_GO,
      label: 'OpenCode Go',
    })
    assert.equal(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_OPENCODE_GO), true)
    assert.equal(getChannelTypeIcon(CHANNEL_TYPE_OPENCODE_GO), 'OpenCode')
    assert.equal(
      getKeyPromptForType(CHANNEL_TYPE_OPENCODE_GO),
      'Enter API key for this channel'
    )
    assert.equal(
      getChannelTypeConfig(CHANNEL_TYPE_OPENCODE_GO).defaultBaseUrl,
      'https://opencode.ai/zen/go'
    )
  })

  test('opencode_go template prefill matches the OpenCode Zen gateway routes', () => {
    const template = ADVANCED_CUSTOM_TEMPLATE_OPTIONS.find(
      (option) => option.value === 'opencode_go'
    )
    assert.ok(template)
    assert.equal(template.label, 'OpenCode Go')

    const config = getAdvancedCustomTemplateConfig('opencode_go')
    const routes = config.advanced_routes ?? []
    assert.equal(routes.length, 4)
    assert.deepEqual(
      routes.map((route) => route.incoming_path),
      ['/v1/chat/completions', '/v1/messages', '/v1/responses', '/v1/models']
    )

    const responsesRoute = routes[2]
    assert.deepEqual(responsesRoute.models, ['grok-4.5'])
    assert.deepEqual(responsesRoute.auth, {
      type: 'header',
      name: 'Authorization',
      value: 'Bearer {api_key}',
    })

    const messagesRoute = routes[1]
    assert.deepEqual(messagesRoute.auth, {
      type: 'header',
      name: 'x-api-key',
      value: '{api_key}',
    })
  })

  test('validates the OpenCode Go form with the prefill base URL', () => {
    // The prefill template uses relative upstream paths, which require the
    // prefill base URL (https://opencode.ai/zen/go) to validate.
    assert.equal(
      channelFormSchema.safeParse(
        opencodeForm({ base_url: 'https://opencode.ai/zen/go' })
      ).success,
      true
    )
  })

  test('requires a Base URL for relative advanced routes', () => {
    const result = channelFormSchema.safeParse(opencodeForm({ base_url: '' }))
    assert.equal(result.success, false)
    if (!result.success) {
      assert.equal(
        result.error.issues.some(
          (issue) =>
            issue.path[0] === 'base_url' &&
            issue.message ===
              'Base URL is required when an advanced route uses an upstream path'
        ),
        true
      )
    }
  })

  test('round-trips workspace ID, auth cookie, and advanced_custom through settings JSON', () => {
    const payload = transformFormDataToCreatePayload(opencodeForm())
    const settings = JSON.parse(payload.channel.settings as string)
    assert.equal(settings.opencode_workspace_id, 'wrk_test')
    assert.equal(settings.opencode_auth_cookie, 'session=abc123')

    // advanced_custom must survive serialization: the backend requires it for
    // type 61 (IsAdvancedCustomChannelType), otherwise channel creation fails.
    assert.ok(settings.advanced_custom)
    assert.equal(
      (settings.advanced_custom as { advanced_routes?: unknown[] })
        .advanced_routes?.length,
      4
    )

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
        opencode_workspace_id: 'wrk_test',
        opencode_auth_cookie: 'session=abc123',
      }),
    })
    const defaults = transformChannelToFormDefaults(channel)
    assert.equal(defaults.opencode_workspace_id, 'wrk_test')
    assert.equal(defaults.opencode_auth_cookie, 'session=abc123')
    assert.ok(defaults.advanced_custom?.trim())
  })

  test('removes OpenCode settings when the channel type changes away', () => {
    const payload = transformFormDataToCreatePayload(opencodeForm({ type: 1 }))
    const settings = JSON.parse(payload.channel.settings as string)
    assert.equal('opencode_workspace_id' in settings, false)
    assert.equal('opencode_auth_cookie' in settings, false)
    assert.equal('advanced_custom' in settings, false)
  })

  test('rejects type 61 without an advanced_custom config', () => {
    const result = channelFormSchema.safeParse(opencodeForm({ type: 61, advanced_custom: '' }))
    assert.equal(result.success, false)
    if (!result.success) {
      assert.equal(
        result.error.issues.some(
          (issue) =>
            issue.path[0] === 'advanced_custom' &&
            issue.message === 'Advanced custom configuration is required'
        ),
        true
      )
    }
  })
})
