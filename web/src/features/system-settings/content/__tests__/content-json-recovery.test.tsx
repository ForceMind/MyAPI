import { QueryClient } from '@tanstack/react-query'
import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { FAQSection } from '../faq-section'

let client: QueryClient
beforeEach(() => { client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } }) })
afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks() })

function FAQFixture(props: { raw: string }) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return <><div ref={setActions} /><SettingsPageProvider actionsContainer={actions} suppressSectionHeader={false}><FAQSection enabled data={props.raw} /></SettingsPageProvider></>
}
async function renderFAQ(raw: string) {
  await renderListRoute({ client, path: '/system-settings/content/$section', entry: '/system-settings/content/faq', element: <FAQFixture raw={raw} /> })
}

it('keeps an explicit zero FAQ identity and unknown false or zero fields when only its question is edited and then the page is saved', async () => {
  const entry = { id: 0, question: 'Original question', answer: 'Saved answer', extra: { enabled: false, count: 0 }, future: false }
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  await renderFAQ(JSON.stringify([entry]))
  await screen.findByText(entry.question)
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Edit' }))
  const dialog = await screen.findByRole('dialog')
  fireEvent.change(within(dialog).getByLabelText('Question'), { target: { value: 'Updated question' } })
  await user.click(within(dialog).getByRole('button', { name: 'Update' }))
  await screen.findByText('Updated question')
  expect(put).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Save Settings' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  const request = put.mock.calls[0]?.[1] as { key: string; value: string }
  expect(request.key).toBe('console_setting.faq')
  expect(JSON.parse(request.value)).toEqual([{ ...entry, question: 'Updated question' }])
})

it.each([
  ['broken syntax', '[{"broken":'],
  ['duplicate identities', '[{"id":0,"question":"First","answer":"A"},{"id":0,"question":"Second","answer":"B"}]'],
  ['non-numeric identity', '[{"id":false,"question":"First","answer":"A","extension":0}]'],
])('keeps the original FAQ JSON and blocks visual overwriting when it has %s', async (_name, raw) => {
  const put = vi.spyOn(api, 'put')
  await renderFAQ(raw)
  expect(await screen.findByRole('alert')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Add FAQ' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Save Settings' })).toBeDisabled()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Repair JSON' }))
  expect(await screen.findByRole('textbox', { name: 'Original JSON' })).toHaveValue(raw)
  expect(put).not.toHaveBeenCalled()
})
