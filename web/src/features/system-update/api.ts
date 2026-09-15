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

import { api } from '@/lib/http-client'

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

// 前往后端的超时窗口。后端串行尝试直连 + 镜像两个候选站（各 5s），
// 必须给满这个预算，否则镜像兜底请求会被前端提前打断。
const UPDATE_CHECK_TIMEOUT_MS = 20_000

// 后端在服务端拉起最新 release（本 fork 仓库），支持 UpdateCheckProxy
// 代理与国内镜像回退，浏览器无需直连 GitHub。详见 controller/update.go。
// 通过统一 axios 实例请求，自动携带 Authorization 凭证并复用 401 刷新。
export async function fetchLatestSystemRelease(
  signal: AbortSignal
): Promise<SystemRelease | null> {
  const controller = new AbortController()
  const cancel = () => controller.abort()
  signal.addEventListener('abort', cancel, { once: true })
  if (signal.aborted) controller.abort()
  const timeout = setTimeout(cancel, UPDATE_CHECK_TIMEOUT_MS)

  try {
    const res = await api.get('/api/update/check', {
      signal: controller.signal,
      skipErrorHandler: true,
    })
    if (!controller.signal.aborted) {
      const parsed = updateCheckResponseSchema.safeParse(res.data)
      if (parsed.success) {
        if (!parsed.data.success) {
          // 后端明确失败（上游不可达/未知错误），与网络失败同等对待。
          throw new UpdateCheckError('network')
        }
        return parsed.data.data ?? null
      }
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