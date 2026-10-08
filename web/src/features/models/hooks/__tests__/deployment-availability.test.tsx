import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { getDeploymentSettings, testDeploymentConnection } from '../../api'
import { useModelDeploymentSettings } from '../use-model-deployment-settings'

vi.mock('../../api', () => ({
  getDeploymentSettings: vi.fn(),
  testDeploymentConnection: vi.fn(),
}))

afterEach(() => vi.clearAllMocks())

describe('deployment availability evidence', () => {
  it('keeps unavailable settings distinct from a confirmed disabled service', async () => {
    vi.mocked(getDeploymentSettings).mockRejectedValue(new Error('offline'))
    const { result } = renderHook(() => useModelDeploymentSettings())
    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.settingsError).toBe('Unable to load settings')
    expect(result.current.isIoNetEnabled).toBe(false)
    expect(testDeploymentConnection).not.toHaveBeenCalled()
    vi.mocked(getDeploymentSettings).mockResolvedValue({
      success: true,
      data: { enabled: false },
    })
    await act(async () => {
      await result.current.refresh()
    })
    expect(result.current.settingsError).toBeNull()
    expect(result.current.isIoNetEnabled).toBe(false)
    expect(testDeploymentConnection).not.toHaveBeenCalled()
  })

  it('does not query deployment services on the metadata tab', async () => {
    vi.mocked(getDeploymentSettings).mockResolvedValue({
      success: true,
      data: { enabled: false },
    })
    const { rerender, result } = renderHook(
      ({ active }) => useModelDeploymentSettings(active),
      { initialProps: { active: false } }
    )
    expect(getDeploymentSettings).not.toHaveBeenCalled()
    rerender({ active: true })
    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(getDeploymentSettings).toHaveBeenCalledOnce()
    expect(result.current.settingsError).toBeNull()
    expect(result.current.isIoNetEnabled).toBe(false)
    expect(testDeploymentConnection).not.toHaveBeenCalled()
  })
})
