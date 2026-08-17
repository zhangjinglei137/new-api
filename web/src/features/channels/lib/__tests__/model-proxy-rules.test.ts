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
  channelFormSchema,
  parseModelProxyRules,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'
import { channelSchema } from '../../types'

describe('Model proxy rules', () => {
  test('parseModelProxyRules splits comma-separated models and drops empty rules', () => {
    expect(
      parseModelProxyRules([
        { models: ' gpt-5.6-luna, regex:^gpt- ', proxy: ' socks5://127.0.0.1:1080 ' },
        { models: 'claude-3.7', proxy: '' },
        { models: '   ', proxy: 'http://127.0.0.1:8080' },
      ])
    ).toStrictEqual([
      {
        models: ['gpt-5.6-luna', 'regex:^gpt-'],
        proxy: 'socks5://127.0.0.1:1080',
      },
      { models: ['claude-3.7'], proxy: '' },
    ])
  })

  test('parseModelProxyRules returns empty array for missing value', () => {
    expect(parseModelProxyRules(undefined)).toStrictEqual([])
    expect(parseModelProxyRules([])).toStrictEqual([])
  })

  test('form accepts valid rules with exact and regex models', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'proxy channel',
      models: 'gpt-5.6-luna',
      model_proxy_rules: [
        { models: 'gpt-5.6-luna', proxy: 'socks5://127.0.0.1:1080' },
        { models: 'regex:^gpt-', proxy: '' },
      ],
    })
    expect(result.success).toBe(true)
  })

  test('form rejects a rule with empty models', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'proxy channel',
      models: 'gpt-5.6-luna',
      model_proxy_rules: [{ models: '  ', proxy: 'socks5://127.0.0.1:1080' }],
    })
    expect(result.success).toBe(false)
  })

  test('form rejects a rule with an invalid regex', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'proxy channel',
      models: 'gpt-5.6-luna',
      model_proxy_rules: [{ models: 'regex:(', proxy: '' }],
    })
    expect(result.success).toBe(false)
  })

  test('form rejects a rule with an invalid proxy address', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'proxy channel',
      models: 'gpt-5.6-luna',
      model_proxy_rules: [{ models: 'gpt-5.6-luna', proxy: 'ftp://host' }],
    })
    expect(result.success).toBe(false)
  })

  test('create payload serializes normalized rules into settings', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'proxy channel',
      key: 'test-key',
      model_proxy_rules: [
        { models: ' gpt-5.6-luna , regex:^gpt- ', proxy: ' socks5://127.0.0.1:1080 ' },
        { models: 'claude-3.7', proxy: '' },
        { models: '', proxy: '' },
      ],
    })
    const settings = JSON.parse(payload.channel.settings ?? '{}')
    expect(settings.model_proxy_rules).toStrictEqual([
      {
        models: ['gpt-5.6-luna', 'regex:^gpt-'],
        proxy: 'socks5://127.0.0.1:1080',
      },
      { models: ['claude-3.7'], proxy: '' },
    ])
  })

  test('create payload omits model_proxy_rules when no rules are configured', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'plain channel',
      key: 'test-key',
      model_proxy_rules: [{ models: '', proxy: '' }],
    })
    const settings = JSON.parse(payload.channel.settings ?? '{}')
    expect('model_proxy_rules' in settings).toBe(false)
  })

  test('form defaults round-trip rules loaded from channel settings', () => {
    const channel = channelSchema.parse({
      id: 1,
      name: 'proxy channel',
      type: 1,
      key: 'test-key',
      status: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      models: 'gpt-5.6-luna',
      settings: JSON.stringify({
        model_proxy_rules: [
          { models: ['gpt-5.6-luna', 'regex:^gpt-'], proxy: 'socks5://127.0.0.1:1080' },
        ],
      }),
    })
    const defaults = transformChannelToFormDefaults(channel)
    expect(defaults.model_proxy_rules).toStrictEqual([
      { models: 'gpt-5.6-luna,regex:^gpt-', proxy: 'socks5://127.0.0.1:1080' },
    ])
  })
})

