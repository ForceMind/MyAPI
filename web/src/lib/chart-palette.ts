/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useEffect, useState } from 'react'

/** Series colours from the design tokens, as plain `#rrggbb` values. */
export type ChartPalette = {
  /** `--chart-1` … `--chart-5`, in order. */
  series: string[]
  /** `--line`: every series after the fifth, and "Other". */
  neutral: string
}

const SERIES_TOKENS = [
  '--chart-1',
  '--chart-2',
  '--chart-3',
  '--chart-4',
  '--chart-5',
] as const
const NEUTRAL_TOKEN = '--line'

/**
 * Reads the chart colours of the current theme from the document. Canvas
 * charts take plain colours, while the tokens are `oklch()`, so each one is
 * painted on a 1×1 canvas and read back. Returns null where the document has
 * no theme styles (tests) or no canvas.
 */
function readChartPalette(): ChartPalette | null {
  if (typeof document === 'undefined') return null
  const styles = getComputedStyle(document.documentElement)
  const values = [...SERIES_TOKENS, NEUTRAL_TOKEN].map((token) =>
    styles.getPropertyValue(token).trim()
  )
  if (values.some((value) => value === '')) return null
  const canvas = document.createElement('canvas')
  canvas.width = 1
  canvas.height = 1
  const context = canvas.getContext('2d', { willReadFrequently: true })
  if (!context) return null
  const colors = values.map((value) => {
    context.clearRect(0, 0, 1, 1)
    context.fillStyle = '#000000'
    context.fillStyle = value
    context.fillRect(0, 0, 1, 1)
    const [red, green, blue] = context.getImageData(0, 0, 1, 1).data
    return `#${[red, green, blue]
      .map((channel) => channel.toString(16).padStart(2, '0'))
      .join('')}`
  })
  const neutral = colors.pop()
  if (!neutral) return null
  return { series: colors, neutral }
}

/**
 * The chart colours for the current theme; null until read. `themeKey`
 * names the theme (light/dark and preset) so a switch reads them again.
 */
export function useChartPalette(themeKey: string): ChartPalette | null {
  const [palette, setPalette] = useState<ChartPalette | null>(null)

  useEffect(() => {
    // Read on the next frame: the theme provider applies the theme to the
    // document in its own effect, which runs after this one.
    const frame = requestAnimationFrame(() => setPalette(readChartPalette()))
    return () => cancelAnimationFrame(frame)
  }, [themeKey])

  return palette
}
