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

export interface ViewerAnimeCard {
  bangumiId: number
  name: string
  nameCN: string
  title: string
  airDate: string
  hasCover: boolean
  imageStatus: string
  ratingScore: number | null
  latestEpisode: string
  latestEpisodeLabel: string
  latestEpisodeTitle: string
  updatedAt: number | null
}

export interface ViewerHome {
  hotRecommendations: ViewerAnimeCard[]
  recentUpdates: ViewerAnimeCard[]
  carouselSlides: unknown[]
  myFollows: ViewerFollowedAnime[]
}

export interface ViewerScheduleCard {
  bangumiId: number
  title: string
  airDate: string
  airWeekday: number
  totalEpisodes: number
  hasCover: boolean
  imageStatus: string
  latestEpisode: string
  latestEpisodeLabel: string
  latestEpisodeUpdatedAt: number | null
}

export interface ViewerSchedule {
  seasonKey: string
  seasonLabel: string
  items: ViewerScheduleCard[]
}

export interface ViewerFilterDimension {
  id: number
  name: string
  sortOrder: number
  tags: string[]
  createdAt: number
  updatedAt: number
}

export interface ViewerLibrary {
  items: ViewerScheduleCard[]
  total: number
}

export interface ViewerAnimeTag {
  name: string
  count: number
}

export interface ViewerAnimeActor {
  actorId: number
  name: string
  summary: string
  career: string[]
  hasImage: boolean
  imageStatus: string
}

export interface ViewerAnimeCharacter {
  characterId: number
  name: string
  summary: string
  relation: string
  hasImage: boolean
  imageStatus: string
  actors: ViewerAnimeActor[]
}

export interface ViewerOPSkipSegment {
  startSeconds: number
  endSeconds: number
  promptStartSeconds: number
  promptEndSeconds: number
  seekToSeconds: number
}

export interface ViewerDetailEpisode {
  key: string
  episodeId: number
  mediaId: number
  label: string
  title: string
  originalTitle: string
  summary: string
  airDate: string
  duration: string
  sortNumber: number
  type: number
  hasMedia: boolean
  hasCover: boolean
  opSkip: ViewerOPSkipSegment | null
}

export interface ViewerAnimeDetail {
  bangumiId: number
  title: string
  originalTitle: string
  airDate: string
  airWeekday: number
  platform: string
  summary: string
  totalEpisodes: number
  hasCover: boolean
  ratingScore: number | null
  infobox: Array<Record<string, unknown>>
  metaTags: string[]
  tags: ViewerAnimeTag[]
  characters: ViewerAnimeCharacter[]
  episodes: ViewerDetailEpisode[]
}

export interface ViewerWatchProgress {
  mediaId: number
  bangumiId: number
  positionSeconds: number
  durationSeconds: number
  completed: boolean
  updatedAt: number
}

export interface ViewerWatchHistoryItem {
  bangumiId: number
  mediaId: number
  animeTitle: string
  episodeLabel: string
  episodeTitle: string
  latestEpisodeLabel: string
  totalEpisodes: number
  positionSeconds: number
  durationSeconds: number
  progressPercent: number
  completed: boolean
  hasCover: boolean
  lastWatchedAt: number
}

export interface ViewerFollowedAnime {
  bangumiId: number
  animeTitle: string
  totalEpisodes: number
  mediaId: number
  episodeLabel: string
  episodeTitle: string
  hasCover: boolean
  hasWatchProgress: boolean
  watchedEpisodeLabel: string
  positionSeconds: number
  durationSeconds: number
  progressPercent: number
  watchCompleted: boolean
  latestEpisodeLabel: string
  caughtUp: boolean
  lastWatchedAt: number
  followedAt: number
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
  home: () => request<{ home: ViewerHome }>('/api/home'),
  animeSchedule: (season: string) =>
    request<{ schedule: ViewerSchedule }>(`/api/anime-schedule?season=${encodeURIComponent(season)}`),
  libraryFilters: () => request<{ items: ViewerFilterDimension[] }>('/api/library/filters'),
  animeLibrary: (query: string, filters: Record<number, string[]> = {}) => {
    const params = new URLSearchParams()
    if (query.trim()) {
      params.set('q', query.trim())
    }
    for (const [dimensionID, tags] of Object.entries(filters)) {
      for (const tag of tags) {
        params.append('filter', `${dimensionID}:${tag}`)
      }
    }
    return request<{ library: ViewerLibrary }>(`/api/library?${params}`)
  },
  followedAnime: () => request<{ items: ViewerFollowedAnime[] }>('/api/follows'),
  watchHistory: () => request<{ items: ViewerWatchHistoryItem[] }>('/api/watch-history'),
  animeDetail: (bangumiId: number) =>
    request<{ anime: ViewerAnimeDetail; watchProgress: ViewerWatchProgress | null; followed: boolean }>(
      `/api/anime/${bangumiId}/detail`,
    ),
  updateAnimeFollow: (bangumiId: number, followed: boolean) =>
    request<{ followed: boolean }>(`/api/anime/${bangumiId}/follow`, {
      method: 'PUT',
      body: JSON.stringify({ followed }),
    }),
  updateWatchProgress: (
    bangumiId: number,
    mediaId: number,
    positionSeconds: number,
    durationSeconds: number,
  ) =>
    request<{ progress: ViewerWatchProgress | null }>(`/api/anime/${bangumiId}/media/${mediaId}/progress`, {
      method: 'PUT',
      body: JSON.stringify({ positionSeconds, durationSeconds }),
    }),
}
