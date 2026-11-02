import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { UpdateCheckerSection } from '../update-checker-section'

afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals() })
it('release details keep long notes in one region and never open a release or check the network until explicitly requested', async () => {
  const fetchRelease = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ tag_name: 'v0.0.0-synthetic-next', body: 'Synthetic complete long release notes with a final suffix.', html_url: 'https://synthetic.invalid/release', published_at: '2026-01-01T00:00:00Z' }) })
  vi.stubGlobal('fetch', fetchRelease)
  const open = vi.spyOn(window, 'open').mockReturnValue(null)
  const user = userEvent.setup()
  render(<UpdateCheckerSection currentVersion='v0.0.0-synthetic-current' />)
  expect(fetchRelease).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Check for updates' }))
  const body = await screen.findByRole('region', { name: 'Release details' })
  expect(within(body).getByText('Synthetic complete long release notes with a final suffix.')).toBeVisible()
  expect(body).toHaveAttribute('tabindex', '0')
  expect(body).not.toContainElement(screen.getByRole('button', { name: 'Open release' }))
  expect(open).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Open release' }))
  expect(open).toHaveBeenCalledWith('https://synthetic.invalid/release', '_blank', 'noopener,noreferrer')
})
