/*
Copyright (C) 2026 ForceMind for MyAPI distribution changes.

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

*/
import { useId, type SVGProps } from 'react'

type IconMyapiProps = SVGProps<SVGSVGElement> & {
  size?: number
}

/** Compact MyAPI marker used for the built-in MyAPI channel type. */
export function IconMyapi({ size = 20, ...props }: IconMyapiProps) {
  const gradientId = useId()

  return (
    <svg
      xmlns='http://www.w3.org/2000/svg'
      viewBox='0 0 24 24'
      width={size}
      height={size}
      role='img'
      aria-label='MyAPI'
      {...props}
    >
      <defs>
        <linearGradient
          id={gradientId}
          x1='3'
          y1='3'
          x2='21'
          y2='21'
          gradientUnits='userSpaceOnUse'
        >
          <stop stopColor='#67EDB1' />
          <stop offset='.5' stopColor='#2FD3E1' />
          <stop offset='1' stopColor='#2E68EA' />
        </linearGradient>
      </defs>
      <rect
        x='1.5'
        y='1.5'
        width='21'
        height='21'
        rx='6'
        fill={`url(#${gradientId})`}
      />
      <path
        d='m6.5 16.5 2.25-8 3.25 5 3.25-5 2.25 8'
        fill='none'
        stroke='white'
        strokeLinecap='round'
        strokeLinejoin='round'
        strokeWidth='1.9'
      />
    </svg>
  )
}
