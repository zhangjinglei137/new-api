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

type IconCommandcodeProps = SVGProps<SVGSVGElement> & {
  size?: number
}

export function IconCommandcode({
  size = 24,
  className,
  ...props
}: IconCommandcodeProps) {
  return (
    <svg
      role='img'
      viewBox='0 0 24 24'
      width={size}
      height={size}
      fill='currentColor'
      className={cn(className)}
      {...props}
    >
      <title>commandcode</title>
      <path
        fillRule='evenodd'
        d='M5.5 2A3.5 3.5 0 0 0 2 5.5v1A3.5 3.5 0 0 0 5.5 10H7v4H5.5A3.5 3.5 0 0 0 2 17.5v1A3.5 3.5 0 0 0 5.5 22H7a3.5 3.5 0 0 0 3.5-3.5V17h3v1.5A3.5 3.5 0 0 0 17 22h1.5a3.5 3.5 0 0 0 3.5-3.5v-1a3.5 3.5 0 0 0-3.5-3.5H17v-4h1.5A3.5 3.5 0 0 0 22 6.5v-1A3.5 3.5 0 0 0 18.5 2H17a3.5 3.5 0 0 0-3.5 3.5V7h-3V5.5A3.5 3.5 0 0 0 7 2H5.5zm0 2H7a1.5 1.5 0 0 1 1.5 1.5V7h-3A1.5 1.5 0 0 1 4 5.5v-1A1.5 1.5 0 0 1 5.5 4zM16.5 4H18a1.5 1.5 0 0 1 1.5 1.5v1A1.5 1.5 0 0 1 18 8h-3V5.5A1.5 1.5 0 0 1 16.5 4zM10.5 9h3v6h-3V9zm-2 0v6h-3A1.5 1.5 0 0 1 4 13.5v-1A1.5 1.5 0 0 1 5.5 11h3zm6 0h3a1.5 1.5 0 0 1 1.5 1.5v1A1.5 1.5 0 0 1 18 13h-3V9zm-6 8h3v1.5A1.5 1.5 0 0 1 10 20H8.5A1.5 1.5 0 0 1 7 18.5V17zm5 0h3v1.5A1.5 1.5 0 0 1 16.5 20H15a1.5 1.5 0 0 1-1.5-1.5V17z'
      />
    </svg>
  )
}
