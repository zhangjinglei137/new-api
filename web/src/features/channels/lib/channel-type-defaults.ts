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
import {
  CHANNEL_TYPE_OPENCODE_GO,
  CHANNEL_TYPE_SENSENOVA,
} from '../constants'
import {
  getAdvancedCustomTemplateConfig,
  stringifyAdvancedCustomConfig,
} from './advanced-custom'

// ============================================================================
// Channel Type Defaults
//
// When creating a channel of a given type, the mutate drawer may pre-fill
// default values that only make sense for that type. Keeping them in a single
// table (instead of per-type `if` blocks) makes the mapping easy to review and
// extend. Values are only applied when the corresponding field is empty.
// ============================================================================

export type ChannelTypeDefaults = {
  base_url?: string
  other?: string
  advanced_custom?: string
  header_override?: string
}

/**
 * Default header override for OpenCode Go channels: passthrough the client
 * headers that the upstream gateway relies on (plus a fixed Host).
 */
const OPENCODE_GO_DEFAULT_HEADER_OVERRIDE = JSON.stringify(
  {
    '*': true,
    Authorization: 'Bearer {api_key}',
    'x-api-key': '{api_key}',
    'Content-Type': '{client_header:Content-Type}',
    'User-Agent': '{client_header:User-Agent}',
    'x-opencode-client': '{client_header:x-opencode-client}',
    'x-opencode-project': '{client_header:x-opencode-project}',
    'x-opencode-request': '{client_header:x-opencode-request}',
    'x-opencode-session': '{client_header:x-opencode-session}',
    Accept: '{client_header:Accept}',
    Host: 'opencode.ai',
    'Accept-Encoding': '{client_header:Accept-Encoding}',
  },
  null,
  2
)

export const CHANNEL_TYPE_DEFAULTS: Record<number, ChannelTypeDefaults> = {
  // VolcEngine (45): default mainland region endpoint.
  45: {
    base_url: 'https://ark.cn-beijing.volces.com',
  },
  // Xunfei/Spark (18): default API version.
  18: {
    other: 'v2.1',
  },
  // OpenCode Go (99): default gateway endpoint, route template and headers.
  [CHANNEL_TYPE_OPENCODE_GO]: {
    base_url: 'https://opencode.ai/zen/go',
    advanced_custom: stringifyAdvancedCustomConfig(
      getAdvancedCustomTemplateConfig('opencode_go')
    ),
    header_override: OPENCODE_GO_DEFAULT_HEADER_OVERRIDE,
  },
  // SenseNova (97): default endpoint.
  [CHANNEL_TYPE_SENSENOVA]: {
    base_url: 'https://token.sensenova.cn',
  },
}