import { describe, it, expect, vi, beforeEach } from 'vitest'
import axios, { AxiosInstance, AxiosResponse } from 'axios'

vi.mock('axios')

const mockGet = vi.fn()
const mockPost = vi.fn()
const mockDelete = vi.fn()

const mockAxiosInstance = {
  get: mockGet,
  post: mockPost,
  delete: mockDelete,
  interceptors: {
    request: { use: vi.fn() },
    response: { use: vi.fn() },
  },
} as unknown as AxiosInstance

vi.mocked(axios.create).mockReturnValue(mockAxiosInstance)

describe('api client', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.resetModules()
  })

  it('creates axios instance with correct config', async () => {
    await import('./api')

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
      const { api } = await import('./api')
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
      const { api } = await import('./api')
      const mockResponse = { data: { apps: [] } } as AxiosResponse
      mockGet.mockResolvedValue(mockResponse)

      const result = await api.getApps()

      expect(mockGet).toHaveBeenCalledWith('/apps')
      expect(result).toEqual(mockResponse.data)
    })
  })

  describe('createApp', () => {
    it('calls POST /apps with app data', async () => {
      const { api } = await import('./api')
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
})