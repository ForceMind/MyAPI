import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { RiskAcknowledgementDialog } from './risk-acknowledgement-dialog'

describe('RiskAcknowledgementDialog', () => {
  test('keeps duplicate acknowledgement entries visible and gates confirmation', async () => {
    const onConfirm = vi.fn()
    render(
      <RiskAcknowledgementDialog
        open
        onOpenChange={vi.fn()}
        title='Acknowledge risk'
        items={['Repeated notice', 'Repeated notice']}
        checklist={['I understand', 'I understand']}
        requiredText='DELETE'
        onConfirm={onConfirm}
      />
    )

    expect(screen.getAllByText('Repeated notice')).toHaveLength(2)
    const confirmButton = screen.getByRole('button', { name: 'Confirm' })
    expect(confirmButton).toBeDisabled()

    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes).toHaveLength(2)
    fireEvent.click(checkboxes[0])
    expect(confirmButton).toBeDisabled()
    fireEvent.click(checkboxes[1])
    fireEvent.change(screen.getByRole('textbox'), {
      target: { value: 'DELETE' },
    })

    expect(confirmButton).toBeEnabled()
    fireEvent.click(confirmButton)
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })
})
