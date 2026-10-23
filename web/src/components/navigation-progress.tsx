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
import { useRouterState } from '@tanstack/react-router'
import { useEffect, useRef } from 'react'
import LoadingBar, { type LoadingBarRef } from 'react-top-loading-bar'

export function NavigationProgress(props: { scope?: 'page' | 'workspace' }) {
  const ref = useRef<LoadingBarRef>(null)
  const started = useRef(false)
  const pending = useRouterState({
    select: (state) => state.status === 'pending',
  })
  const authenticated = useRouterState({
    select: (state) =>
      state.matches.some((match) => match.routeId === '/_authenticated'),
  })
  const hidden = props.scope !== 'workspace' && authenticated

  useEffect(() => {
    if (hidden) {
      started.current = false
      return
    }
    if (pending) {
      ref.current?.continuousStart()
      started.current = true
    } else if (started.current) {
      ref.current?.complete()
      started.current = false
    }
  }, [pending, hidden])

  if (hidden) return null

  return (
    <LoadingBar
      color='var(--primary)'
      ref={ref}
      shadow={false}
      height={2}
      containerClassName='myapi-navigation-progress'
      containerStyle={
        props.scope === 'workspace' ? { position: 'absolute' } : undefined
      }
    />
  )
}
