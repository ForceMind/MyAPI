import { QueryClient } from '@tanstack/react-query'
import { cleanup, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { DirectionProvider } from '@/context/direction-provider'
import { ThemeProvider } from '@/context/theme-provider'
import { About } from '@/features/about'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { getPrivacyPolicy } from '../api'
import { LegalDocument } from '../legal-document'

const originalAuth = useAuthStore.getState()
let client: QueryClient
beforeEach(() => {
  useAuthStore.getState().auth.reset('idle')
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['status'], {})
  client.setQueryData(['notice'], { success: true, data: '' })
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })
it.each(['about', 'privacy'])('separates a failed %s read from intentionally unset content and retries only the document GET', async (kind) => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { success: false, message: 'Synthetic document read rejected', data: 'UNCONFIRMED CONTENT' } })
  const element = kind === 'about' ? <About /> : <LegalDocument title='Privacy Policy' queryKey='privacy-policy' fetchDocument={getPrivacyPolicy} emptyMessage='Synthetic unset document' />
  await renderListRoute({ client, path: '/public-document', element: <DirectionProvider><ThemeProvider>{element}</ThemeProvider></DirectionProvider> })
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic document read rejected')
  expect(screen.queryByText('UNCONFIRMED CONTENT')).not.toBeInTheDocument()
  expect(screen.queryByText('No About Content Set')).not.toBeInTheDocument()
  get.mockResolvedValue({ data: { success: true, data: '' } })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText(kind === 'about' ? 'No About Content Set' : 'Synthetic unset document')).toBeVisible()
  expect(get).toHaveBeenCalledTimes(2)
})
