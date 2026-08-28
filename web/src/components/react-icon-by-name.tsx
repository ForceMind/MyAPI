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
import { Circle, type LucideProps } from 'lucide-react'

function normalizeIconName(name: string | null | undefined): string | null {
  const trimmed = name?.trim()
  if (!trimmed || !/^[A-Z][A-Za-z0-9]*$/.test(trimmed)) return null
  return trimmed
}

type ReactIconByNameProps = LucideProps & {
  name?: string | null
  title?: string
}

/**
 * Keep accepting stored react-icons names without shipping every icon pack.
 * Self-hosted installations get a lightweight neutral marker for configured
 * names; image URL icons and the built-in payment/provider icons remain intact.
 */
export function ReactIconByName({
  name,
  title,
  ...props
}: ReactIconByNameProps) {
  const iconName = normalizeIconName(name)
  if (!iconName) return null
  return <Circle aria-label={title || iconName} {...props} />
}
