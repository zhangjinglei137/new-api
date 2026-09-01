import { describe, expect, test } from 'vitest'

import {
  CHANNEL_TYPE_SENSENOVA,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import { getChannelTypeIcon } from '../channel-utils'

// Channel type for VolcEngine (45) — no dedicated exported constant in
// constants.ts, only the numeric type id.
const CHANNEL_TYPE_VOLCENGINE = 45

describe('VolcEngine channel', () => {
  test('registers model discovery for the Coding Plan endpoint', () => {
    // The upstream model discovery mechanism (GET {baseURL}/v1/models, for
    // Coding Plan /api/coding/v1/models) must stay enabled, otherwise the
    // "Fetch from Upstream" button and upstream model detection settings are
    // hidden.
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_VOLCENGINE)).toBe(true)
    expect(getChannelTypeIcon(CHANNEL_TYPE_VOLCENGINE)).toBe('Volcengine')
  })
})

describe('SenseNova channel', () => {
  test('registers model discovery via the OpenAI-compatible endpoint', () => {
    // SenseNova exposes GET https://token.sensenova.cn/v1/models (OpenAI
    // compatible, Bearer auth). Without this flag the "Fetch from Upstream"
    // button stays hidden.
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_SENSENOVA)).toBe(true)
    expect(getChannelTypeIcon(CHANNEL_TYPE_SENSENOVA)).toBe('SenseNova')
  })
})
