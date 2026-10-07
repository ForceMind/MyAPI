// Browser-computed sRGB text contrast for isolated UI qualification.
import assert from 'node:assert/strict'

export async function assertTextContrast(locator, name, theme) {
  const colors = await locator.evaluate(element => {
    // Canvas converts the browser's resolved OKLCH/RGB colors to sRGB. Composite
    // ancestor backgrounds in order so transparent surfaces are not read as black.
    const canvas = document.createElement('canvas')
    canvas.width = 1; canvas.height = 1
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    ctx.fillStyle = '#fff'; ctx.fillRect(0, 0, 1, 1)
    const ancestors = []
    for (let parent = element; parent; parent = parent.parentElement) ancestors.unshift(parent)
    for (const parent of ancestors) {
      ctx.fillStyle = getComputedStyle(parent).backgroundColor
      ctx.fillRect(0, 0, 1, 1)
    }
    const background = [...ctx.getImageData(0, 0, 1, 1).data].slice(0, 3)
    ctx.fillStyle = getComputedStyle(element).color; ctx.fillRect(0, 0, 1, 1)
    const foreground = [...ctx.getImageData(0, 0, 1, 1).data].slice(0, 3)
    return { background, foreground }
  })
  const luminance = color => color.map(value => value / 255).map(value => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4).reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0)
  const values = [luminance(colors.background), luminance(colors.foreground)].sort((a, b) => a - b)
  const ratio = (values[1] + 0.05) / (values[0] + 0.05)
  assert(ratio >= 4.5, `${theme} ${name}: text contrast ${ratio.toFixed(2)} must be at least 4.5:1`)
  return { name, theme, ...colors, ratio, minimum: 4.5 }
}
