import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { SearchContext } from '@/context/search-context'

import { CommandMenu } from './command-menu'

const navigate = vi.fn()
const setOpen = vi.fn()
const setTheme = vi.fn()

vi.mock('@tanstack/react-router', () => ({
  useLocation: () => ({ pathname: '/dashboard' }),
  useNavigate: () => navigate,
}))

vi.mock('@/context/theme-provider', () => ({
  useTheme: () => ({ setTheme }),
}))

vi.mock('@/hooks/use-sidebar-data', () => ({
  useSidebarData: () => ({
    navGroups: [
      {
        id: 'main',
        title: 'Main',
        items: [
          { title: 'Dashboard', url: '/dashboard' },
          { title: 'Dashboard', url: '/dashboard' },
          {
            title: 'Settings',
            items: [{ title: 'Profile', url: '/settings/profile' }],
          },
        ],
      },
    ],
  }),
}))

vi.mock('./layout/lib/sidebar-view-registry', () => ({
  getNavGroupsForPath: () => null,
}))

describe('CommandMenu', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  test('keeps duplicate commands and runs the selected navigation command', () => {
    render(
      <SearchContext.Provider value={{ open: true, setOpen }}>
        <CommandMenu />
      </SearchContext.Provider>
    )

    const dashboardCommands = screen.getAllByText('Dashboard')
    expect(dashboardCommands).toHaveLength(2)

    fireEvent.click(dashboardCommands[1])
    expect(setOpen).toHaveBeenCalledWith(false)
    expect(navigate).toHaveBeenCalledWith({ to: '/dashboard' })
  })
})
