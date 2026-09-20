import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'

const pushMock = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: pushMock }),
}))

const getAppsMock = vi.fn()
const logoutMock = vi.fn()

vi.mock('@/lib/api', () => ({
  api: {
    getApps: (...args: unknown[]) => getAppsMock(...args),
    logout: (...args: unknown[]) => logoutMock(...args),
  },
}))

import DashboardPage from '@/app/dashboard/page'

describe('DashboardPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows an empty state when there are no apps', async () => {
    getAppsMock.mockResolvedValue({ apps: [] })

    render(<DashboardPage />)

    expect(await screen.findByText('No apps yet')).toBeInTheDocument()
  })

  it('renders a user-facing error with a retry button when loading apps fails', async () => {
    getAppsMock.mockRejectedValueOnce(new Error('network error'))

    render(<DashboardPage />)

    expect(await screen.findByText('Failed to load your apps. Please try again.')).toBeInTheDocument()

    // The failed-load empty state must not also show "No apps yet" underneath the error.
    expect(screen.queryByText('No apps yet')).not.toBeInTheDocument()

    getAppsMock.mockResolvedValueOnce({ apps: [] })
    fireEvent.click(screen.getByText('Retry'))

    await waitFor(() => {
      expect(screen.queryByText('Failed to load your apps. Please try again.')).not.toBeInTheDocument()
    })
    expect(getAppsMock).toHaveBeenCalledTimes(2)
  })

  it('renders apps returned from the API', async () => {
    getAppsMock.mockResolvedValue({
      apps: [
        { id: '1', name: 'My Portfolio', slug: 'my-portfolio', kind: 'website', status: 'ready', created_at: '2024-01-01T00:00:00Z' },
      ],
    })

    render(<DashboardPage />)

    expect(await screen.findByText('My Portfolio')).toBeInTheDocument()
  })

  it('still redirects to /login if logout() itself fails', async () => {
    getAppsMock.mockResolvedValue({ apps: [] })
    logoutMock.mockRejectedValueOnce(new Error('network error'))

    render(<DashboardPage />)
    await screen.findByText('No apps yet')

    fireEvent.click(screen.getByText('Log out'))

    await waitFor(() => {
      expect(pushMock).toHaveBeenCalledWith('/login')
    })
  })
})
