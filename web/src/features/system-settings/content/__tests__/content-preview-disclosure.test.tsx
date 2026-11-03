import { QueryClient } from '@tanstack/react-query'
import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { AnnouncementsSection } from '../announcements-section'
import { FAQSection } from '../faq-section'

let client: QueryClient
beforeEach(() => { client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } }) })
afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks() })
function ContentFixture(props: { kind: 'faq' | 'announcements'; raw: string }) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return <><div ref={setActions} /><SettingsPageProvider actionsContainer={actions} suppressSectionHeader={false}>{props.kind === 'faq' ? <FAQSection enabled data={props.raw} /> : <AnnouncementsSection enabled data={props.raw} />}</SettingsPageProvider></>
}
it('keeps the full zero-id FAQ draft intact when its readonly answer preview is expanded and collapsed before the explicit page save', async () => {
  const answer = 'Synthetic complete answer '.repeat(12) + 'Original answer ending.'
  const entry = { id: 0, question: 'Saved question', answer, extension: { enabled: false, count: 0 } }
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  const get = vi.spyOn(api, 'get').mockRejectedValue(new Error('Unexpected synthetic content read'))
  await renderListRoute({ client, path: '/system-settings/content/$section', entry: '/system-settings/content/faq', element: <ContentFixture kind='faq' raw={JSON.stringify([entry])} /> })
  const user = userEvent.setup()
  expect(screen.getByText(answer)).not.toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Edit' }))
  const dialog = await screen.findByRole('dialog')
  fireEvent.change(within(dialog).getByLabelText('Question'), { target: { value: 'Unsaved local question' } })
  await user.click(within(dialog).getByRole('button', { name: 'Update' }))
  const trigger = screen.getByRole('button', { name: 'View full text' })
  trigger.focus()
  await user.keyboard('{Enter}')
  expect(screen.getByText(answer)).toBeVisible()
  await user.keyboard('{Enter}')
  expect(screen.getByText(answer)).not.toBeVisible()
  expect(screen.getByText('Unsaved local question')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Save Settings' })).toBeEnabled()
  expect(put).not.toHaveBeenCalled()
  expect(get).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Save Settings' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  const body = put.mock.calls[0]?.[1] as { key: string; value: string }
  expect(body.key).toBe('console_setting.faq')
  expect(JSON.parse(body.value)).toEqual([{ ...entry, question: 'Unsaved local question' }])
})
it('summarizes announcement body and notes while selection and the original edit action remain available without writes', async () => {
  const content = 'Synthetic announcement body '.repeat(10) + 'Original body ending.'
  const extra = 'Complete saved additional note.'
  const entry = { id: 0, content, extra, publishDate: '2026-01-01T00:00:00Z', type: 'default' }
  const put = vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected synthetic content write'))
  const get = vi.spyOn(api, 'get').mockRejectedValue(new Error('Unexpected synthetic content read'))
  await renderListRoute({ client, path: '/system-settings/content/$section', entry: '/system-settings/content/announcements', element: <ContentFixture kind='announcements' raw={JSON.stringify([entry])} /> })
  expect(screen.getByText(content)).not.toBeVisible()
  expect(screen.getByText(extra)).not.toBeVisible()
  expect(screen.getByText('Additional notes: 1')).toBeVisible()
  const user = userEvent.setup()
  const selection = screen.getAllByRole('checkbox')[1]
  if (!selection) throw new Error('Expected the original announcement selection')
  await user.click(selection)
  const trigger = screen.getByRole('button', { name: 'View full text' })
  trigger.focus()
  await user.keyboard('{Enter}')
  expect(screen.getByText(content)).toBeVisible()
  expect(screen.getByText(extra)).toBeVisible()
  await user.keyboard('{Enter}')
  expect(screen.getByText(content)).not.toBeVisible()
  expect(selection).toHaveAttribute('aria-checked', 'true')
  expect(screen.getByRole('button', { name: 'Edit' })).toBeVisible()
  expect(put).not.toHaveBeenCalled()
  expect(get).not.toHaveBeenCalled()
})
