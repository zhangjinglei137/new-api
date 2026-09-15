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
import { z } from 'zod'

import { systemReleaseSchema, type SystemRelease } from './releases'

export type UpdateCheckErrorCode =
  | 'network'
  | 'rate-limit'
  | 'timeout'
  | 'payload'

export class UpdateCheckError extends Error {
  constructor(public readonly code: UpdateCheckErrorCode) {
    super(code)
    this.name = 'UpdateCheckError'
  }
}

const updateCheckResponseSchema = z.object({
  success: z.boolean(),
  message: z.string().optional(),
  data: systemReleaseSchema.nullable().optional(),
})

// 后端在服务端拉起最新 release（本 fork 仓库），支持 UpdateCheckProxy
// 代理与国内镜像回退，浏览器无需直连 GitHub。详见 controller/update.go。
export async function fetchLatestSystemRelease(
  signal: AbortSignal
): Promise<SystemRelease | null> {
  const controller = new AbortController()
  const cancel = () => controller.abort()
  signal.addEventListener('abort', cancel, { once: true })
  if (signal.aborted) controller.abort()
  const timeout = setTimeout(cancel, 10_000)

  try {
    const response = await fetch('/api/update/check', {
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
      signal: controller.signal,
    })
    if (!response.ok) throw new UpdateCheckError('network')

    let json: unknown
    try {
      json = await response.json()
    } catch {
      throw new UpdateCheckError('payload')
    }
    const parsed = updateCheckResponseSchema.safeParse(json)
    if (parsed.success) {
      if (!parsed.data.success) throw new UpdateCheckError('network')
      return parsed.data.data ?? null
    }
    throw new UpdateCheckError('payload')
  } catch (error) {
    if (signal.aborted) throw error
    if (controller.signal.aborted) throw new UpdateCheckError('timeout')
    if (error instanceof UpdateCheckError) throw error
    throw new UpdateCheckError('network')
  } finally {
    clearTimeout(timeout)
    signal.removeEventListener('abort', cancel)
  }
}