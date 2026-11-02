import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { DEFAULT_CURRENCY_CONFIG, useSystemConfigStore } from '@/stores/system-config-store'

import type { PricingModel } from '../../types'
import { ModelCatalogPrice } from '../model-catalog-price'

const initial = useSystemConfigStore.getState()
const model: PricingModel = { id: 1, model_name: 'synthetic', quota_type: 0, model_ratio: 1, completion_ratio: 2, cache_ratio: 0, enable_groups: ['default', 'free'], group_ratio: { default: 2, free: 0 } }
beforeEach(() => { useSystemConfigStore.setState({ currency: { ...DEFAULT_CURRENCY_CONFIG } }) })
afterEach(() => { useSystemConfigStore.setState(initial) })

describe('catalog price presentation', () => {
  it('preserves selected group prices and an explicitly free cache price', () => {
    const { container } = render(<ModelCatalogPrice model={model} selectedGroup='default' />)
    expect(container).toHaveTextContent(/Input\s+\$4/)
    expect(container).toHaveTextContent(/Output\s+\$8/)
    expect(container).toHaveTextContent(/Cached\s+\$0/)
    expect(container).toHaveTextContent('Prices shown per 1M tokens')
  })

  it('preserves K units and recharge conversion without changing the model', () => {
    const { container } = render(<ModelCatalogPrice model={model} selectedGroup='default' tokenUnit='K' showRechargePrice priceRate={0.5} usdExchangeRate={1} />)
    expect(container).toHaveTextContent(/Input\s+\$0.002/)
    expect(container).toHaveTextContent(/Output\s+\$0.004/)
    expect(container).toHaveTextContent('Prices shown per 1K tokens')
  })

  it('keeps a zero group ratio and distinguishes missing cache pricing from explicit zero', () => {
    const { container, rerender } = render(<ModelCatalogPrice model={model} selectedGroup='free' />)
    expect(container).toHaveTextContent(/Input\s+\$0/)
    expect(container).toHaveTextContent(/Output\s+\$0/)
    rerender(<ModelCatalogPrice model={{ ...model, cache_ratio: null }} selectedGroup='default' />)
    expect(screen.queryByText('Cached')).not.toBeInTheDocument()
    expect(container).toHaveTextContent(/Input\s+\$4/)
  })

  it('keeps per-request prices independent of token units', () => {
    const { container } = render(<ModelCatalogPrice model={{ ...model, quota_type: 1, model_price: 0.25 }} selectedGroup='default' tokenUnit='K' />)
    expect(container).toHaveTextContent('$0.5')
    expect(container).toHaveTextContent('Per Request')
    expect(container).not.toHaveTextContent('1K tokens')
  })

  it('shows raw unsupported expressions rather than a fabricated numeric price', () => {
    const expression = 'v1:custom_unparsed(p,c)'
    const { container } = render(<ModelCatalogPrice model={{ ...model, billing_mode: 'tiered_expr', billing_expr: expression }} />)
    expect(container).toHaveTextContent('Special billing expression')
    expect(container).toHaveTextContent('Unable to parse structured pricing')
    expect(container).toHaveTextContent(expression)
    expect(container).not.toHaveTextContent('$0')
  })
})
