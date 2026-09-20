import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { WebPreview, WebPreviewBody } from '../web-preview'

describe('WebPreviewBody sandbox', () => {
  test('allows scripts and forms without same-origin, popups, downloads, or presentation', () => {
    render(
      <WebPreview defaultUrl='https://preview.example'>
        <WebPreviewBody />
      </WebPreview>
    )

    const frame = screen.getByTitle('Preview')
    expect(frame).toHaveAttribute('sandbox', 'allow-scripts allow-forms')
    expect(frame.getAttribute('sandbox')).not.toContain('allow-same-origin')
    expect(frame.getAttribute('sandbox')).not.toContain('allow-popups')
    expect(frame.getAttribute('sandbox')).not.toContain('allow-downloads')
    expect(frame.getAttribute('sandbox')).not.toContain('allow-presentation')
  })
})
