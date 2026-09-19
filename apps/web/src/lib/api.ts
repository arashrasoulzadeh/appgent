import axios, { AxiosInstance, AxiosError, InternalAxiosRequestConfig } from 'axios'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL || 'http://localhost:8080/api/v1'

class ApiClient {
  private client: AxiosInstance

  constructor() {
    this.client = axios.create({
      baseURL: API_BASE_URL,
      withCredentials: true,
      headers: {
        'Content-Type': 'application/json',
      },
    })

    this.client.interceptors.request.use(
      (config: InternalAxiosRequestConfig) => config,
      (error: AxiosError) => Promise.reject(error)
    )

    this.client.interceptors.response.use(
      response => response,
      (error: AxiosError) => {
        if (error.response?.status === 401) {
          if (typeof window !== 'undefined') {
            // eslint-disable-next-line @next/next/no-location-assign-relative-destination
            window.location.href = '/login'
          }
        }
        return Promise.reject(error)
      }
    )
  }

  async login(email: string, password: string) {
    const response = await this.client.post<{ user: { id: string; email: string } }>(
      '/auth/login',
      { email, password }
    )
    return response.data
  }

  async logout() {
    await this.client.post('/auth/logout')
  }

  async me() {
    const response = await this.client.get<{ user: { id: string; email: string } }>('/auth/me')
    return response.data
  }

  async getApps() {
    const response = await this.client.get<{ apps: App[] }>('/apps')
    return response.data
  }

  async createApp(data: CreateAppRequest) {
    const response = await this.client.post<{ app: App; run: Run }>('/apps', data)
    return response.data
  }

  async getApp(appId: string) {
    const response = await this.client.get<{ app: App; latest_run: Run }>(`/apps/${appId}`)
    return response.data
  }

  async deleteApp(appId: string) {
    await this.client.delete(`/apps/${appId}`)
  }

  async regenerateApp(appId: string, prompt?: string) {
    const response = await this.client.post<{ run: Run }>(`/apps/${appId}/regenerate`, { prompt })
    return response.data
  }

  async getRuns(appId: string) {
    const response = await this.client.get<{ runs: Run[] }>(`/apps/${appId}/runs`)
    return response.data
  }

  async getRun(appId: string, runId: string) {
    const response = await this.client.get<{ run: Run; steps: AgentStep[] }>(
      `/apps/${appId}/runs/${runId}`
    )
    return response.data
  }

  async getRunEvents(appId: string, runId: string, onEvent: (event: SSEEvent) => void) {
    const response = await fetch(`${API_BASE_URL}/apps/${appId}/runs/${runId}/events`, {
      credentials: 'include',
    })

    if (!response.ok) throw new Error('Failed to connect to SSE')

    const reader = response.body?.getReader()
    if (!reader) throw new Error('No reader')

    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (line.startsWith('event:')) {
          const eventType = line.slice(6).trim()
          const dataLine = lines[lines.indexOf(line) + 1]
          if (dataLine?.startsWith('data:')) {
            try {
              const data = JSON.parse(dataLine.slice(5).trim())
              onEvent({ type: eventType as SSEEvent['type'], data })
            } catch {
              // Ignore parse errors
            }
          }
        }
      }
    }
  }

  async deployApp(appId: string) {
    const response = await this.client.post<{ deployment: Deployment }>(`/apps/${appId}/deploy`)
    return response.data
  }

  async getDeployments(appId: string) {
    const response = await this.client.get<{ deployments: Deployment[] }>(`/apps/${appId}/deployments`)
    return response.data
  }

  async getPreviewUrl(appId: string, runId: string) {
    const response = await this.client.get<{ preview_url: string; expires_at: string }>(
      `/apps/${appId}/runs/${runId}/preview`
    )
    return response.data
  }
}

export const api = new ApiClient()

export interface App {
  id: string
  name: string
  slug: string
  kind: 'website' | 'pwa'
  status: 'draft' | 'generating' | 'ready' | 'needs_review' | 'failed'
  created_at: string
}

export interface Run {
  id: string
  version: number
  status: 'queued' | 'running' | 'succeeded' | 'needs_review' | 'failed'
  user_prompt: string
  created_at: string
}

export interface AgentStep {
  agent_type: 'plan' | 'design' | 'code' | 'qa'
  attempt: number
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  model_used: string
  tokens_used?: number
  started_at?: string
  finished_at?: string
  output_summary?: string
}

export interface Deployment {
  id: string
  url: string
  status: 'deploying' | 'live' | 'failed' | 'retired'
  deployed_at?: string
}

export interface CreateAppRequest {
  name: string
  kind: 'website' | 'pwa'
  prompt: string
}

export interface SSEEvent {
  type: 'step' | 'run'
  data: {
    agent_type?: string
    status?: string
  }
}