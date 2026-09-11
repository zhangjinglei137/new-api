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
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { defineConfig } from 'vitest/config'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    // CI 批量运行（143 个文件并发）在 jsdom 环境下单个重量级集成测试
    // 会超过 vitest 默认的 5000ms 超时（如 model 抽屉类用例）；放宽到
    // 30s 使超时反映真实故障而非 CI 负载，语义不变。
    testTimeout: 30000,
    hookTimeout: 30000,
    server: {
      deps: { inline: [/@lobehub\//, /antd-style/] },
    },
    setupFiles: ['./src/test-setup.ts'],
    clearMocks: true,
    restoreMocks: true,
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
  },
})
