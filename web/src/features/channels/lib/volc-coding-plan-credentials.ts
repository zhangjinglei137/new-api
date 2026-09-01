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
import { updateVolcCodingPlanCredentials } from '../api'

export type VolcCodingPlanCredentialKey =
  | 'csrf'
  | 'cookie'
  | 'access_key_id'
  | 'secret_access_key'

export type VolcCodingPlanCredentialInput = {
  csrfToken: string
  cookie: string
  accessKeyId: string
  secretAccessKey: string
}

export function hasVolcCodingPlanCredential(
  input: VolcCodingPlanCredentialInput
): boolean {
  return [
    input.csrfToken,
    input.cookie,
    input.accessKeyId,
    input.secretAccessKey,
  ].some((value) => value.trim() !== '')
}

function buildVolcCodingPlanSavePayload(input: VolcCodingPlanCredentialInput) {
  return {
    csrf_token: input.csrfToken.trim() || undefined,
    cookie: input.cookie.trim() || undefined,
    access_key_id: input.accessKeyId.trim() || undefined,
    secret_access_key: input.secretAccessKey.trim() || undefined,
  }
}

function buildVolcCodingPlanClearPayload(kind: VolcCodingPlanCredentialKey) {
  switch (kind) {
    case 'csrf':
      return { clear_csrf: true }
    case 'cookie':
      return { clear_cookie: true }
    case 'access_key_id':
      return { clear_access_key_id: true }
    case 'secret_access_key':
      return { clear_secret_access_key: true }
  }
}

export async function saveVolcCodingPlanCredentials(
  channelId: number,
  input: VolcCodingPlanCredentialInput
) {
  return updateVolcCodingPlanCredentials(
    channelId,
    buildVolcCodingPlanSavePayload(input)
  )
}

export async function clearVolcCodingPlanCredential(
  channelId: number,
  kind: VolcCodingPlanCredentialKey
) {
  return updateVolcCodingPlanCredentials(
    channelId,
    buildVolcCodingPlanClearPayload(kind)
  )
}