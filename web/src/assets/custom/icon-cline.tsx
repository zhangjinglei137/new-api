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
import type { SVGProps } from 'react'

import { cn } from '@/lib/utils'

type IconClineProps = SVGProps<SVGSVGElement> & {
  size?: number
}

export function IconCline({ size = 24, className, ...props }: IconClineProps) {
  return (
    <svg
      role='img'
      viewBox='0 0 24 24'
      width={size}
      height={size}
      className={cn(className)}
      {...props}
    >
      <title>Cline</title>
      {/* Cline 品牌 Logomark（@lobehub/icons Cline Mono 路径），缩放至 24 viewBox */}
      <path
        fill='currentColor'
        fillRule='evenodd'
        d='M17.035 3.991c2.75 0 4.98 2.24 4.98 5.003v1.667l1.45 2.896a1.01 1.01 0 0 1-.002.909l-1.448 2.864v1.668c0 2.762-2.23 5.002-4.98 5.002H7.074c-2.751 0-4.98-2.24-4.98-5.002V17.33l-1.48-2.855a1.01 1.01 0 0 1-.003-.927l1.482-2.887V8.994c0-2.763 2.23-5.003 4.98-5.003h9.962ZM8.265 9.6a2.274 2.274 0 0 0-2.274 2.274v4.042a2.274 2.274 0 0 0 4.547 0v-4.042A2.274 2.274 0 0 0 8.265 9.6Zm7.326 0a2.274 2.274 0 0 0-2.274 2.274v4.042a2.274 2.274 0 1 0 4.548 0v-4.042A2.274 2.274 0 0 0 15.59 9.6Z'
      />
      <path
        fill='currentColor'
        d='M12.054 5.558a2.779 2.779 0 1 0 0-5.558 2.779 2.779 0 0 0 0 5.558Z'
      />
    </svg>
  )
}
