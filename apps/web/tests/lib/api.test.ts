import { describe, it, expect, vi, beforeEach } from 'vitest'
import axios, { AxiosInstance, AxiosResponse, AxiosError } from 'axios'

vi.mock('axios')

const mockGet = vi.fn()
const mockPost = vi.fn()
const mockDelete = vi.fn()

type InterceptorPair = {
  onFulfilled?: (value: unknown) => unknown
  onRejected?: (error: AxiosError) => unknown
}

let responseInterceptor: InterceptorPair = {}

const mockAxiosInstance = {
  get: mockGet,
  post: mockPost,
  delete: mockDelete,
  interceptors: {
    request: {
      use: vi.fn(),
    },
    response: {
      use: vi.fn((onFulfilled, onRejected) => {
        responseInterceptor = { onFulfilled, onRejected }
      }),
    },
  },
} as unknown as AxiosInstance

vi.mocked(axios.create).mockReturnValue(mockAxiosInstance)

describe('api client', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.resetModules()
    responseInterceptor = {}
  })

  it('creates axios instance with correct config', async () => {
    await import('@/lib/api')

    expect(axios.create).toHaveBeenCalledWith({
      baseURL: 'http://localhost:8080/api/v1',
      withCredentials: true,
      headers: { 'Content-Type': 'application/json' },
    })
    expect(mockAxiosInstance.interceptors.request.use).toHaveBeenCalled()
    expect(mockAxiosInstance.interceptors.response.use).toHaveBeenCalled()
  })

  describe('login', () => {
    it('calls POST /auth/login with credentials', async () => {
      const { api } = await import('@/lib/api')
      const mockResponse = { data: { user: { id: '1', email: 'admin' } } } as AxiosResponse
      mockPost.mockResolvedValue(mockResponse)

      const result = await api.login('admin', 'admin')

      expect(mockPost).toHaveBeenCalledWith('/auth/login', {
        email: 'admin',
        password: 'admin',
      })
      expect(result).toEqual(mockResponse.data)
    })
  })

  describe('getApps', () => {
    it('calls GET /apps', async () => {
      const { api } = await import('@/lib/api')
      const mockResponse = { data: { apps: [] } } as AxiosResponse
      mockGet.mockResolvedValue(mockResponse)

      const result = await api.getApps()

      expect(mockGet).toHaveBeenCalledWith('/apps')
      expect(result).toEqual(mockResponse.data)
    })
  })

  describe('getApp', () => {
    it('returns latest_run as null when the app has no runs yet', async () => {
      const { api } = await import('@/lib/api')
      const mockResponse = {
        data: { app: { id: '1', name: 'App', slug: 'app', kind: 'website', status: 'draft', created_at: '2024-01-01' }, latest_run: null },
      } as AxiosResponse
      mockGet.mockResolvedValue(mockResponse)

      const result = await api.getApp('1')

      expect(mockGet).toHaveBeenCalledWith('/apps/1')
      expect(result.latest_run).toBeNull()
    })
  })

  describe('createApp', () => {
    it('calls POST /apps with app data', async () => {
      const { api } = await import('@/lib/api')
      const mockResponse = { data: { app: { id: '1' }, run: { id: '1' } } } as AxiosResponse
      mockPost.mockResolvedValue(mockResponse)

      const result = await api.createApp({
        name: 'Test App',
        kind: 'website',
        prompt: 'A test app',
      })

      expect(mockPost).toHaveBeenCalledWith('/apps', {
        name: 'Test App',
        kind: 'website',
        prompt: 'A test app',
      })
      expect(result).toEqual(mockResponse.data)
    })
  })

  describe('response interceptor (401 handling)', () => {
    const originalLocation = window.location

    beforeEach(() => {
      // @ts-expect-error - reassigning window.location for test purposes
      delete window.location
      // @ts-expect-error - partial mock is fine for this test
      window.location = { href: '' }
    })

    it('redirects to /login on a 401 from a protected endpoint', async () => {
      await import('@/lib/api')

      const error = {
        response: { status: 401 },
        config: { url: '/apps' },
      } as unknown as AxiosError

      await expect(responseInterceptor.onRejected!(error)).rejects.toBe(error)
      expect(window.location.href).toBe('/login')

      // @ts-expect-error restoring for other tests
      window.location = originalLocation
    })

    it('does not redirect on a 401 from the login endpoint itself', async () => {
      await import('@/lib/api')

      const error = {
        response: { status: 401 },
        config: { url: '/auth/login' },
      } as unknown as AxiosError

      await expect(responseInterceptor.onRejected!(error)).rejects.toBe(error)
      expect(window.location.href).toBe('')

      // @ts-expect-error restoring for other tests
      window.location = originalLocation
    })

    it('passes through non-401 errors without redirecting', async () => {
      await import('@/lib/api')

      const error = {
        response: { status: 500 },
        config: { url: '/apps' },
      } as unknown as AxiosError

      await expect(responseInterceptor.onRejected!(error)).rejects.toBe(error)
      expect(window.location.href).toBe('')

      // @ts-expect-error restoring for other tests
      window.location = originalLocation
    })
  })
})
