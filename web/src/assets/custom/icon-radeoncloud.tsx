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

type IconRadeonCloudProps = SVGProps<SVGSVGElement> & {
  size?: number
}

export function IconRadeonCloud({
  size = 24,
  className,
  ...props
}: IconRadeonCloudProps) {
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
      <title>AMD Radeon Cloud</title>
      <path d='M4 3h16v1H4Z M4 4h17v1H4Z M5 5h16v1H5Z M6 6h15v1H6Z M7 7h14v1H7Z M16 8h5v9H16Z M6 10h2v1H6Z M5 11h3v1H5Z M4 12h4v1H4Z M3 13h5v3H3Z M3 16h11v1H3Z M3 17h10v1H3Z M17 17h4v1H17Z M3 18h9v1H3Z M18 18h3v1H18Z M3 19h8v1H3Z M19 19h2v1H19Z M3 20h7v1H3Z' />
    </svg>
  )
}
