import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { CreemProductDialog } from '../creem-product-dialog'

afterEach(() => cleanup())
it('the payment product editor keeps its complete existing fields and exact decimal currency payload in one task region', async () => {
  const product = { name: 'Synthetic product with a complete long display name and final suffix', productId: 'synthetic-long-product-id', price: 12.34, quota: 500000, currency: 'EUR' as const }
  const save = vi.fn()
  const user = userEvent.setup()
  render(<CreemProductDialog open onOpenChange={() => {}} onSave={save} editData={product} />)
  const body = screen.getByRole('region', { name: 'Payment configuration details' })
  expect(screen.getByDisplayValue(product.name)).toBeVisible()
  expect(screen.getByDisplayValue(product.productId)).toBeVisible()
  const submit = screen.getByRole('button', { name: 'Update' })
  expect(submit).toHaveClass('min-h-11')
  expect(body).not.toContainElement(submit)
  await user.click(submit)
  expect(save).toHaveBeenCalledWith(product)
})
