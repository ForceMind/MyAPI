/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { describe, expect, it } from 'vitest'

import {
  placeLegendForPhone,
  processChartData,
  processUserChartData,
} from '@/features/dashboard/lib/charts'
import type { QuotaDataItem } from '@/features/dashboard/types'
import type { ChartPalette } from '@/lib/chart-palette'

const PALETTE: ChartPalette = {
  series: ['#000001', '#000002', '#000003', '#000004', '#000005'],
  neutral: '#999999',
}

/** One record per model; the call count decides each model's rank. */
const MODEL_CALLS: Record<string, number> = {
  'model-a': 1,
  'model-b': 70,
  'model-c': 60,
  'model-d': 50,
  'model-e': 40,
  'model-f': 30,
  'model-g': 20,
}

function modelRows(): QuotaDataItem[] {
  return Object.entries(MODEL_CALLS).map(([model, count], index) => ({
    model_name: model,
    count,
    quota: count * 10,
    token_used: count,
    created_at: 1_790_000_000 + index,
  }))
}

function colorOf(scale: { domain: string[]; range: string[] }) {
  return (series: string) => scale.range[scale.domain.indexOf(series)]
}

describe('dashboard chart colours', () => {
  it('gives the five busiest models the token colours in rank order and every other series the neutral colour', () => {
    const chart = processChartData(
      modelRows(),
      'day',
      undefined,
      undefined,
      0,
      PALETTE
    )
    const color = colorOf(chart.spec_pie.color)

    expect(
      ['model-b', 'model-c', 'model-d', 'model-e', 'model-f'].map(color)
    ).toEqual(PALETTE.series)
    expect(color('model-g')).toBe(PALETTE.neutral)
    expect(color('model-a')).toBe(PALETTE.neutral)
    expect(color('Other')).toBe(PALETTE.neutral)
    // Every chart of the page uses the same colour for the same model.
    expect(chart.spec_line.color).toEqual(chart.spec_pie.color)
    expect(chart.spec_model_line.color).toEqual(chart.spec_pie.color)
  })

  it('ranks users by consumption for the token colours', () => {
    const rows: QuotaDataItem[] = [
      'user-a',
      'user-b',
      'user-c',
      'user-d',
      'user-e',
      'user-f',
    ].map((username, index) => ({
      username,
      quota: (index + 1) * 100,
      created_at: 1_790_000_000,
    }))

    const chart = processUserChartData(rows, 'day', undefined, 10, 0, PALETTE)

    expect(chart.spec_user_rank.color.specified).toEqual({
      'user-f': PALETTE.series[0],
      'user-e': PALETTE.series[1],
      'user-d': PALETTE.series[2],
      'user-c': PALETTE.series[3],
      'user-b': PALETTE.series[4],
      'user-a': PALETTE.neutral,
    })
  })

  it('moves a visible legend under the chart only on a phone', () => {
    const spec = { type: 'pie', legends: { visible: true, orient: 'left' } }

    expect(placeLegendForPhone(spec, true).legends).toEqual({
      visible: true,
      orient: 'bottom',
    })
    expect(placeLegendForPhone(spec, false)).toBe(spec)
    const hidden = { type: 'bar', legends: { visible: false } }
    expect(placeLegendForPhone(hidden, true)).toBe(hidden)
  })
})
