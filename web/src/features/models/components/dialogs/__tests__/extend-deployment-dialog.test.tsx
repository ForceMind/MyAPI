import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { ExtendDeploymentDialog } from '../extend-deployment-dialog'

describe('ExtendDeploymentDialog', () => {
  test('keeps submission disabled when no deployment is selected', () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <ExtendDeploymentDialog
          open
          onOpenChange={vi.fn()}
          deploymentId={null}
        />
      </QueryClientProvider>
    )

    expect(screen.getByRole('button', { name: 'Extend' })).toBeDisabled()
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})
