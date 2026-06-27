export interface ViewerUser {
  id: number
  username: string
  createdAt: number
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

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'same-origin',
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
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

export const api = {
  register: (username: string, password: string) =>
    request<{ user: ViewerUser }>('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  login: (username: string, password: string) =>
    request<{ user: ViewerUser }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  me: () => request<{ user: ViewerUser }>('/api/auth/me'),
  logout: () => request<void>('/api/auth/logout', { method: 'POST', body: '{}' }),
}
