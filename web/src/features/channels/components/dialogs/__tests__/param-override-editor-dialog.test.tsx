import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { ParamOverrideEditorDialog } from '../param-override-editor-dialog'

vi.mock('@/components/ui/scroll-area', () => ({
  ScrollArea: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

describe('ParamOverrideEditorDialog', () => {
  test('renders the sync endpoint editor for a sync_fields operation', async () => {
    render(
      <ParamOverrideEditorDialog
        open
        onOpenChange={vi.fn()}
        onSave={vi.fn()}
        value={JSON.stringify({
          operations: [
            {
              mode: 'sync_fields',
              from: 'header:session_id',
              to: 'json:prompt_cache_key',
            },
          ],
        })}
      />
    )

    expect(await screen.findByText('Sync Endpoints')).toBeInTheDocument()
    expect(screen.getByDisplayValue('session_id')).toBeInTheDocument()
    expect(screen.getByDisplayValue('prompt_cache_key')).toBeInTheDocument()
  })
})
