import { normalizeAPIBaseURL } from './config'

export interface ViewerUser {
  id: number
  username: string
  createdAt: number
}

export interface SiteSettings {
  siteName: string
  registrationEnabled: boolean
  inviteRequired: boolean
  hasFavicon: boolean
  faviconUpdatedAt: number | null
  updatedAt: number
}

interface AuthResponse {
  user: ViewerUser
  token: string
  expiresAt: number
}

interface ErrorPayload {
  error?: {
    code?: string
    message?: string
  }
}

export class APIError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly code?: string,
  ) {
    super(message)
  }
}

const tokenStorageKey = 'bp.mobile.viewerToken'
const tokenExpiresStorageKey = 'bp.mobile.viewerTokenExpiresAt'

let apiBaseUrl = 'https://baka.vip/'
let authToken = localStorage.getItem(tokenStorageKey) || ''

export function configureAPI(nextBaseUrl: string) {
  apiBaseUrl = normalizeAPIBaseURL(nextBaseUrl)
}

export function currentAPIBaseURL() {
  return apiBaseUrl
}

export function currentAuthToken() {
  return authToken
}

export function setAuthSession(token: string, expiresAt: number) {
  authToken = token
  localStorage.setItem(tokenStorageKey, token)
  localStorage.setItem(tokenExpiresStorageKey, String(expiresAt))
}

export function clearAuthSession() {
  authToken = ''
  localStorage.removeItem(tokenStorageKey)
  localStorage.removeItem(tokenExpiresStorageKey)
}

export function buildAPIURL(path: string, query?: Record<string, string | number | boolean | undefined>) {
  const url = new URL(path.replace(/^\/+/, ''), apiBaseUrl)
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== undefined) {
      url.searchParams.set(key, String(value))
    }
  }
  return url.toString()
}

export function buildAuthenticatedMediaURL(path: string) {
  return buildAPIURL(path, authToken ? { viewer_token: authToken } : undefined)
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers)
  if (authToken) {
    headers.set('Authorization', `Bearer ${authToken}`)
  }
  if (options?.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(buildAPIURL(path), {
    ...options,
    credentials: 'omit',
    headers,
  })

  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as ErrorPayload
    throw new APIError(payload.error?.message ?? `请求失败（${response.status}）`, response.status, payload.error?.code)
  }
  if (response.status === 204) {
    return undefined as T
  }
  return response.json() as Promise<T>
}

async function applyAuthResponse(promise: Promise<AuthResponse>) {
  const result = await promise
  setAuthSession(result.token, result.expiresAt)
  return result
}

export const api = {
  siteSettings: () => request<{ settings: SiteSettings }>('/api/site-settings'),
  me: () => request<{ user: ViewerUser }>('/api/auth/me'),
  login: (username: string, password: string) =>
    applyAuthResponse(
      request<AuthResponse>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      }),
    ),
  register: (username: string, password: string, inviteCode = '') =>
    applyAuthResponse(
      request<AuthResponse>('/api/auth/register', {
        method: 'POST',
        body: JSON.stringify({ username, password, inviteCode }),
      }),
    ),
  logout: async () => {
    try {
      await request<void>('/api/auth/logout', { method: 'POST', body: '{}' })
    } finally {
      clearAuthSession()
    }
  },
}
