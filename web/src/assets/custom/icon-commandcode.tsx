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
      className={cn(className)}
      {...props}
    >
      <title>Command Code</title>
      {/* 官方 Logomark：commandcode.ai symbol.svg，缩放至 24 viewBox */}
      <path
        fill='currentColor'
        fillRule='evenodd'
        d='m 12.146 0.975 h -0.444 c -2.786 0 -4.777 0.002 -6.29 0.206 -1.485 0.2 -2.363 0.577 -3.008 1.223 s -1.023 1.523 -1.223 3.008 c -0.203 1.513 -0.206 3.504 -0.206 6.29 v 0.444 c 0 2.786 0.002 4.777 0.206 6.29 0.2 1.485 0.577 2.363 1.223 3.008 0.645 0.645 1.523 1.023 3.008 1.222 1.513 0.203 3.504 0.205 6.29 0.205 h 0.444 c 2.786 0 4.777 -0.002 6.29 -0.205 1.485 -0.2 2.363 -0.577 3.008 -1.222 0.645 -0.646 1.023 -1.523 1.222 -3.008 0.203 -1.513 0.205 -3.504 0.205 -6.29 v -0.444 c 0 -2.786 -0.002 -4.777 -0.205 -6.29 -0.2 -1.485 -0.577 -2.363 -1.222 -3.008 -0.646 -0.645 -1.523 -1.023 -3.008 -1.223 -1.513 -0.203 -3.504 -0.206 -6.29 -0.206 z m -10.432 0.739 c -1.714 1.714 -1.714 4.472 -1.714 9.988 v 0.444 c 0 5.516 0 8.274 1.714 9.988 1.714 1.714 4.472 1.714 9.988 1.714 h 0.444 c 5.516 0 8.274 0 9.988 -1.714 s 1.714 -4.472 1.714 -9.988 v -0.444 c 0 -5.516 0 -8.274 -1.714 -9.988 -1.714 -1.714 -4.472 -1.714 -9.988 -1.714 h -0.444 c -5.516 0 -8.274 0 -9.988 1.714 z M 16.408 4.586 c -1.573 0 -2.853 1.28 -2.853 2.854 v 1.223 h -3.261 v -1.223 c 0 -1.574 -1.28 -2.854 -2.853 -2.854 -1.574 0 -2.854 1.28 -2.854 2.854 s 1.28 2.853 2.854 2.853 h 1.223 v 3.261 h -1.223 c -1.574 0 -2.854 1.28 -2.854 2.854 0 1.574 1.28 2.853 2.854 2.853 1.573 0 2.853 -1.28 2.853 -2.853 v -1.223 h 3.261 v 1.223 c 0 1.574 1.28 2.853 2.853 2.853 1.574 0 2.853 -1.28 2.853 -2.853 0 -1.574 -1.28 -2.854 -2.853 -2.854 h -1.223 v -3.261 h 1.223 c 1.574 0 2.853 -1.28 2.853 -2.853 s -1.28 -2.854 -2.853 -2.854 z m -1.223 4.076 v -1.223 c 0 -0.677 0.546 -1.223 1.223 -1.223 0.677 0 1.223 0.546 1.223 1.223 0 0.677 -0.546 1.223 -1.223 1.223 z m -7.745 0 c -0.677 0 -1.223 -0.546 -1.223 -1.223 0 -0.677 0.546 -1.223 1.223 -1.223 0.677 0 1.223 0.546 1.223 1.223 v 1.223 z m 2.853 4.892 v -3.261 h 3.261 v 3.261 z m 6.115 4.076 c -0.677 0 -1.223 -0.546 -1.223 -1.223 v -1.223 h 1.223 c 0.677 0 1.223 0.546 1.223 1.223 0 0.677 -0.546 1.223 -1.223 1.223 z m -8.968 0 c -0.677 0 -1.223 -0.546 -1.223 -1.223 0 -0.677 0.546 -1.223 1.223 -1.223 h 1.223 v 1.223 c 0 0.677 -0.546 1.223 -1.223 1.223 z'
      />
    </svg>
  )
}
