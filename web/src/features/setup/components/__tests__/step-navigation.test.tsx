import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { StepNavigation } from '../step-navigation'

describe('setup navigation', () => {
  it('keeps Next keyboard reachable on the first step without a misleading Back action', async () => {
    const onNext = vi.fn()
    const user = userEvent.setup()
    render(
      <StepNavigation
        currentStep={0}
        totalSteps={4}
        onBack={vi.fn()}
        onNext={onNext}
        onSubmit={vi.fn()}
      />
    )
    expect(
      screen.queryByRole('button', { name: 'Back' })
    ).not.toBeInTheDocument()
    await user.tab()
    expect(screen.getByRole('button', { name: 'Next' })).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(onNext).toHaveBeenCalledOnce()
    expect(screen.getByText('Setup progress: 0/4')).toHaveAttribute(
      'aria-live',
      'polite'
    )
  })

  it('blocks repeated initialization and backwards navigation while submission is pending', async () => {
    const onBack = vi.fn()
    const onSubmit = vi.fn()
    const user = userEvent.setup()
    render(
      <StepNavigation
        currentStep={3}
        totalSteps={4}
        onBack={onBack}
        onNext={vi.fn()}
        onSubmit={onSubmit}
        isSubmitting
      />
    )
    const submit = screen.getByRole('button', { name: 'Initializing…' })
    expect(submit).toBeDisabled()
    expect(submit).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByRole('button', { name: 'Back' })).toBeDisabled()
    await user.click(submit)
    await user.click(screen.getByRole('button', { name: 'Back' }))
    expect(onSubmit).not.toHaveBeenCalled()
    expect(onBack).not.toHaveBeenCalled()
  })

  it('restores the final action after a failed submission and lets long labels wrap', () => {
    const props = {
      currentStep: 3,
      totalSteps: 4,
      onBack: vi.fn(),
      onNext: vi.fn(),
      onSubmit: vi.fn(),
    }
    const view = render(<StepNavigation {...props} isSubmitting />)
    view.rerender(<StepNavigation {...props} isSubmitting={false} />)
    const submit = screen.getByRole('button', { name: 'Initialize system' })
    expect(submit).toBeEnabled()
    expect(submit).toHaveClass('whitespace-normal', 'h-auto', 'max-w-full')
    expect(screen.getByRole('button', { name: 'Back' })).toBeEnabled()
  })
})
