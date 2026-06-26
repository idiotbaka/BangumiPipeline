export interface User {
  id: number
  username: string
  isAdmin: boolean
  createdAt: number
}

export interface ScheduledTask {
  key: string
  name: string
  description: string
  enabled: boolean
  intervalMinutes: number
  lastStatus: 'idle' | 'running' | 'completed' | 'failed'
  lastError: string
  lastStartedAt: number | null
  lastFinishedAt: number | null
  nextRunAt: number | null
  createdAt: number
  updatedAt: number
}

export interface NetworkSettings {
  httpProxy: string
  httpsProxy: string
  updatedAt: number
}

export interface SubscriptionSettings {
  rssUrl: string
  updatedAt: number
}
export interface DownloadSettings {
  host: string
  port: number
  username: string
  password: string
  maxConcurrentDownloads: number
  updatedAt: number
}

export interface MediaStorageSettings {
  defaultRoot: string
  extraRoots: string[]
  updatedAt: number
}

export interface LLMSettings {
  baseUrl: string
  apiKey: string
  model: string
  updatedAt: number
}

export interface LLMConnectionTestResult {
  response: string
}

export interface StorageMoveResult {
  bangumiId: number
  storageRoot: string
  storagePath: string
  moved: boolean
}

export interface DownloadConnectionTestResult {
  version: string
}

export type DownloadStatus = 'pending' | 'downloading' | 'completed' | 'failed'
export type DownloadRetryAction = 'corrected' | 'reset' | 'deleted_reset'

export interface DownloadJob {
  id: number
  subscriptionItemId: number
  title: string
  bangumiId: number
  animeName: string
  seasonNumber: number
  episodeType: string
  episodeNumber: string
  status: DownloadStatus
  folderName: string
  qbitName: string
  progress: number
  totalSize: number
  downloadedSize: number
  downloadSpeed: number
  errorMessage: string
  startedAt: number | null
  completedAt: number | null
  failedAt: number | null
  createdAt: number
  updatedAt: number
}

export interface DownloadJobPage {
  items: DownloadJob[]
  total: number
  page: number
  pageSize: number
}

export interface DownloadRetryResult {
  job: DownloadJob
  action: DownloadRetryAction
}

export type MediaJobStatus = 'pending' | 'transcoding' | 'completed' | 'failed'

export interface MediaJob {
  id: number
  downloadJobId: number
  subscriptionItemId: number
  title: string
  bangumiId: number
  animeName: string
  seasonNumber: number
  episodeType: string
  episodeNumber: string
  status: MediaJobStatus
  sourceFile: string
  subtitleFile: string
  outputFile: string
  coverFile: string
  coverStatus: 'pending' | 'completed' | 'failed'
  coverError: string
  videoCodec: string
  audioCodec: string
  hasInternalSubtitles: boolean
  hasExternalSubtitles: boolean
  needsTranscode: boolean
  action: string
  progress: number
  processedDurationMs: number
  totalDurationMs: number
  errorMessage: string
  progressUpdatedAt: number | null
  startedAt: number | null
  completedAt: number | null
  failedAt: number | null
  createdAt: number
  updatedAt: number
}

export interface MediaJobPage {
  items: MediaJob[]
  total: number
  page: number
  pageSize: number
}

export type LogLevel = 'INFO' | 'WARNING' | 'ERROR'

export interface DashboardOverview {
  subscription: {
    pendingBindings: number
  }
  download: {
    pending: number
    downloading: number
    failed: number
  }
  media: {
    pending: number
    transcoding: number
    failed: number
  }
  storage: {
    roots: DashboardStorageRoot[]
  }
}

export interface DashboardStorageRoot {
  label: string
  path: string
  isDefault: boolean
  available: boolean
  freeBytes: number
  totalBytes: number
  usedBytes: number
  usedPercent: number
  errorMessage: string
}

export interface SystemLog {
  id: number
  level: LogLevel
  source: string
  message: string
  fields: Record<string, unknown>
  createdAt: number
}

export interface AnimeMatchedEpisode {
  seasonNumber: number
  episodeType: string
  episodeNumber: string
  status: 'matched' | 'completed'
}

export interface AnimeListItem {
  bangumiId: number
  name: string
  nameCN: string
  airDate: string
  airWeekday: number
  episodes: number
  platform: string
  imageStatus: string
  hasCover: boolean
  detailStatus: string
  storageRoot: string
  storagePath: string
  matchedEpisodes: AnimeMatchedEpisode[]
  createdAt: number
}

export interface AnimePage {
  items: AnimeListItem[]
  total: number
  page: number
  pageSize: number
}

export interface HistorySyncResult {
  bangumiId: number
  sourceTitle: string
  searchTitle: string
  fetched: number
  inserted: number
  bound: number
  skippedExisting: number
  skippedIgnored: number
  skippedUnmatched: number
}

export interface HistorySyncInput {
  rssUrl?: string
  excludeTitle?: string
  includeTitle?: string
}

export type SubscriptionMatchStatus = 'matched' | 'unmatched'
export type SubscriptionBindingStatus = 'pending' | 'bound' | 'ignored'

export interface AnimeSearchItem {
  bangumiId: number
  name: string
  nameCN: string
}

export interface SubscriptionItem {
  id: number
  guid: string
  title: string
  description: string
  link: string
  enclosureUrl: string
  torrentUrl: string
  contentLength: number
  pubDate: string
  publishedAt: number | null
  matchStatus: SubscriptionMatchStatus
  bangumiId: number | null
  matchedName: string
  parsedName: string
  seasonNumber: number | null
  episodeType: string
  episodeNumber: string
  matchScore: number
  matchReason: string
  bindingStatus: SubscriptionBindingStatus
  boundBangumiId: number | null
  boundAnimeName: string
  boundSeasonNumber: number | null
  boundEpisodeType: string
  boundEpisodeNumber: string
  bindingNote: string
  boundAt: number | null
  ignoredAt: number | null
  createdAt: number
  updatedAt: number
}

export interface SubscriptionItemPage {
  items: SubscriptionItem[]
  total: number
  page: number
  pageSize: number
}

export interface AnimeActor {
  actorId: number
  name: string
  summary: string
  career: string[]
  hasImage: boolean
  imageStatus: string
}

export interface AnimeCharacter {
  characterId: number
  name: string
  summary: string
  relation: string
  hasImage: boolean
  imageStatus: string
  actors: AnimeActor[]
}

export interface AnimeEpisode {
  episodeId: number
  epNumber: number
  sortNumber: number
  type: number
  disc: number
  airdate: string
  name: string
  nameCN: string
  duration: string
  durationSeconds: number
  description: string
  commentCount: number
}

export interface AnimeDetail {
  bangumiId: number
  url: string
  name: string
  nameCN: string
  airDate: string
  airWeekday: number
  detailDate: string
  platform: string
  summary: string
  eps: number
  totalEpisodes: number
  volumes: number
  series: boolean
  locked: boolean
  nsfw: boolean
  hasCover: boolean
  imageStatus: string
  detailStatus: string
  characterStatus: string
  episodesStatus: string
  storageRoot: string
  storagePath: string
  infobox: Array<{ key?: string; value?: unknown }>
  rating: Record<string, unknown>
  collection: Record<string, unknown>
  metaTags: string[]
  tags: Array<{ name: string; count: number }>
  aliases: string[]
  characters: AnimeCharacter[]
  episodes: AnimeEpisode[]
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
  setupStatus: () => request<{ initialized: boolean }>('/api/setup/status'),
  setup: (username: string, password: string) =>
    request<{ user: User }>('/api/setup', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  login: (username: string, password: string) =>
    request<{ user: User }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  me: () => request<{ user: User }>('/api/auth/me'),
  logout: () => request<void>('/api/auth/logout', { method: 'POST', body: '{}' }),
  dashboardOverview: () => request<{ overview: DashboardOverview }>('/api/dashboard/overview'),
  scheduledTasks: () => request<{ tasks: ScheduledTask[] }>('/api/scheduled-tasks'),
  updateScheduledTask: (key: string, update: { enabled?: boolean; intervalMinutes?: number }) =>
    request<{ task: ScheduledTask }>(`/api/scheduled-tasks/${encodeURIComponent(key)}`, {
      method: 'PATCH',
      body: JSON.stringify(update),
    }),
  runScheduledTask: (key: string) =>
    request<{ task: ScheduledTask }>(`/api/scheduled-tasks/${encodeURIComponent(key)}/run`, {
      method: 'POST',
      body: '{}',
    }),
  networkSettings: () => request<{ settings: NetworkSettings }>('/api/settings/network'),
  updateNetworkSettings: (httpProxy: string, httpsProxy: string) =>
    request<{ settings: NetworkSettings }>('/api/settings/network', {
      method: 'PUT',
      body: JSON.stringify({ httpProxy, httpsProxy }),
    }),
  subscriptionSettings: () => request<{ settings: SubscriptionSettings }>('/api/settings/subscription'),
  updateSubscriptionSettings: (rssUrl: string) =>
    request<{ settings: SubscriptionSettings }>('/api/settings/subscription', {
      method: 'PUT',
      body: JSON.stringify({ rssUrl }),
    }),
  downloadSettings: () => request<{ settings: DownloadSettings }>('/api/settings/download'),
  updateDownloadSettings: (settings: Omit<DownloadSettings, 'updatedAt'>) =>
    request<{ settings: DownloadSettings }>('/api/settings/download', {
      method: 'PUT',
      body: JSON.stringify(settings),
    }),
  mediaStorageSettings: () => request<{ settings: MediaStorageSettings }>('/api/settings/media-storage'),
  updateMediaStorageSettings: (extraRoots: string[]) =>
    request<{ settings: MediaStorageSettings }>('/api/settings/media-storage', {
      method: 'PUT',
      body: JSON.stringify({ extraRoots }),
    }),
  llmSettings: () => request<{ settings: LLMSettings }>('/api/settings/llm'),
  updateLLMSettings: (settings: Omit<LLMSettings, 'updatedAt'>) =>
    request<{ settings: LLMSettings }>('/api/settings/llm', {
      method: 'PUT',
      body: JSON.stringify(settings),
    }),
  testLLMSettings: (settings: Omit<LLMSettings, 'updatedAt'>) =>
    request<{ result: LLMConnectionTestResult }>('/api/settings/llm/test', {
      method: 'POST',
      body: JSON.stringify(settings),
    }),
  testDownloadSettings: (settings: Omit<DownloadSettings, 'updatedAt'>) =>
    request<{ result: DownloadConnectionTestResult }>('/api/settings/download/test', {
      method: 'POST',
      body: JSON.stringify(settings),
    }),
  systemLogs: (levels: LogLevel[]) => {
    const query = new URLSearchParams()
    levels.forEach((level) => query.append('level', level))
    return request<{ logs: SystemLog[] }>(`/api/system-logs?${query}`)
  },
  animeList: (page: number, pageSize: number) =>
    request<AnimePage>(`/api/anime?page=${page}&pageSize=${pageSize}`),
  createAnime: (bangumiId: number) =>
    request<{ anime: AnimeDetail }>('/api/anime', {
      method: 'POST',
      body: JSON.stringify({ bangumiId }),
    }),
  animeDetail: (bangumiId: number) =>
    request<{ anime: AnimeDetail }>(`/api/anime/${bangumiId}`),
  refreshAnime: (bangumiId: number) =>
    request<{ anime: AnimeDetail }>(`/api/anime/${bangumiId}/refresh`, {
      method: 'POST',
      body: '{}',
    }),
  syncAnimeHistory: (bangumiId: number, input: HistorySyncInput = {}) =>
    request<{ result: HistorySyncResult }>(`/api/anime/${bangumiId}/sync-history`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  moveAnimeStorage: (bangumiId: number, storageRoot: string) =>
    request<{ result: StorageMoveResult }>(`/api/anime/${bangumiId}/storage`, {
      method: 'POST',
      body: JSON.stringify({ storageRoot }),
    }),
  deleteAnime: (bangumiId: number) =>
    request<void>(`/api/anime/${bangumiId}`, { method: 'DELETE' }),
  animeSearch: (query: string, limit = 100) => {
    const params = new URLSearchParams({ q: query, limit: String(limit) })
    return request<{ items: AnimeSearchItem[] }>(`/api/anime/search?${params}`)
  },
  subscriptionItems: (page: number, pageSize: number, bindingStatus?: SubscriptionBindingStatus) => {
    const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
    if (bindingStatus) params.set('bindingStatus', bindingStatus)
    return request<SubscriptionItemPage>(`/api/subscription/items?${params}`)
  },
  downloadJobs: (page: number, pageSize: number, status?: DownloadStatus) => {
    const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
    if (status) params.set('status', status)
    return request<DownloadJobPage>(`/api/download/jobs?${params}`)
  },
  retryDownloadJob: (jobId: number) =>
    request<{ result: DownloadRetryResult }>(`/api/download/jobs/${jobId}/retry`, {
      method: 'POST',
      body: '{}',
    }),
  mediaJobs: (page: number, pageSize: number, status?: MediaJobStatus) => {
    const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
    if (status) params.set('status', status)
    return request<MediaJobPage>(`/api/media/jobs?${params}`)
  },
  retryMediaJob: (jobId: number) =>
    request<{ job: MediaJob }>(`/api/media/jobs/${jobId}/retry`, {
      method: 'POST',
      body: '{}',
    }),
  confirmSubscriptionBinding: (itemId: number) =>
    request<{ item: SubscriptionItem }>(`/api/subscription/items/${itemId}/confirm`, {
      method: 'POST',
      body: '{}',
    }),
  bindSubscriptionItem: (itemId: number, input: { bangumiId: number; seasonNumber: number; episodeType: string; episodeNumber: string }) =>
    request<{ item: SubscriptionItem }>(`/api/subscription/items/${itemId}/binding`, {
      method: 'PUT',
      body: JSON.stringify(input),
    }),
  ignoreSubscriptionItem: (itemId: number) =>
    request<{ item: SubscriptionItem }>(`/api/subscription/items/${itemId}/ignore`, {
      method: 'POST',
      body: '{}',
    }),
}

export function systemLogStreamURL(levels: LogLevel[], afterId: number): string {
  const query = new URLSearchParams({ afterId: String(afterId) })
  levels.forEach((level) => query.append('level', level))
  return `/api/system-logs/stream?${query}`
}
