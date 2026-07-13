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
  serverDownloadDir: string
  qbitDownloadDir: string
  maxConcurrentDownloads: number
  updatedAt: number
}

export type DownloadSettingsInput = Omit<DownloadSettings, 'serverDownloadDir' | 'updatedAt'>

export interface MediaStorageSettings {
  defaultRoot: string
  extraRoots: string[]
  updatedAt: number
}

export interface BangumiCustomSearchSettings {
  tags: string[]
  updatedAt: number
}

export interface ViewerUser {
  id: number
  username: string
  disabled: boolean
  disabledAt: number | null
  createdAt: number
  updatedAt: number
  lastActivity: ViewerUserActivity | null
}

export interface ViewerUserPage {
  items: ViewerUser[]
  total: number
  page: number
  pageSize: number
}

export interface ViewerUserActivity {
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

export interface ViewerSiteSettings {
  siteName: string
  registrationEnabled: boolean
  inviteRequired: boolean
  hasFavicon: boolean
  faviconUpdatedAt: number | null
  updatedAt: number
}

export interface ViewerCarouselItem {
  id: number
  bangumiId: number
  title: string
  sortOrder: number
  imageUpdatedAt: number
  createdAt: number
  updatedAt: number
}

export interface ViewerCarouselInput {
  bangumiId: number
  sortOrder: number
  file?: File | null
}

export interface AppRelease {
  id: number
  version: string
  releaseNotes: string
  apkSize: number
  apkSha256: string
  publishedAt: number
}

export interface AppReleaseInput {
  version: string
  releaseNotes: string
  file: File
}

export interface AppReleaseUpdateInput {
  version: string
  releaseNotes: string
  file?: File | null
}

export interface ViewerFilterDimension {
  id: number
  name: string
  sortOrder: number
  tags: string[]
  createdAt: number
  updatedAt: number
}

export interface ViewerFilterDimensionInput {
  name: string
  sortOrder: number
  tags: string[]
}

export interface ViewerInvite {
  id: number
  code: string
  used: boolean
  usedByUserId: number | null
  usedByUsername: string
  usedAt: number | null
  createdAt: number
}

export interface ViewerInvitePage {
  items: ViewerInvite[]
  total: number
  page: number
  pageSize: number
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
  subscriptionEpisodeOffset: number
  matchedEpisodes: AnimeMatchedEpisode[]
  createdAt: number
}

export interface AnimeSettings {
  bangumiId: number
  subscriptionEpisodeOffset: number
}

export type AnimeListSort = 'created' | 'airDate'

export interface AnimeListOptions {
  sort?: AnimeListSort
  query?: string
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

export interface ManualEpisodeInput {
  magnetUrl: string
  seasonNumber: number
  episodeType: string
  episodeNumber: string
}

export interface ManualEpisodeResult {
  item: SubscriptionItem
  downloadJobId: number
}

export interface EpisodeReplacementCleanup {
  mediaJobsRemoved: number
  filesDeleted: number
}

export interface EpisodeBindingIdentity {
  seasonNumber: number
  episodeType: string
  episodeNumber: string
}

export interface EpisodeBindingMutationResult {
  bangumiId: number
  source: EpisodeBindingIdentity
  target?: EpisodeBindingIdentity
  updatedItems: number
  updatedMediaJobs: number
  updatedTitleRules: number
  deletedDownloadJobs: number
  deletedMediaJobs: number
  deletedTitleRules: number
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
  opSkip: AnimeEpisodeOPSkip
}

export interface AnimeEpisodeOPSkip {
  mediaId: number
  status: 'no_media' | 'pending' | 'detected' | 'not_found' | 'failed' | 'unsupported'
  startSeconds: number
  endSeconds: number
  errorMessage: string
  updatedAt: number | null
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
  const headers =
    options?.body instanceof FormData
      ? options.headers
      : {
          'Content-Type': 'application/json',
          ...options?.headers,
        }
  const response = await fetch(path, {
    credentials: 'same-origin',
    ...options,
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

function viewerCarouselForm(input: ViewerCarouselInput) {
  const form = new FormData()
  form.append('bangumiId', String(input.bangumiId))
  form.append('sortOrder', String(input.sortOrder))
  if (input.file) form.append('file', input.file)
  return form
}

function appReleaseForm(input: AppReleaseInput | AppReleaseUpdateInput) {
  const form = new FormData()
  form.append('version', input.version)
  form.append('releaseNotes', input.releaseNotes)
  if (input.file) form.append('file', input.file)
  return form
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
  updateDownloadSettings: (settings: DownloadSettingsInput) =>
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
  bangumiCustomSearchSettings: () =>
    request<{ settings: BangumiCustomSearchSettings }>('/api/settings/bangumi-custom-search'),
  updateBangumiCustomSearchSettings: (tags: string[]) =>
    request<{ settings: BangumiCustomSearchSettings }>('/api/settings/bangumi-custom-search', {
      method: 'PUT',
      body: JSON.stringify({ tags }),
    }),
  viewerUsers: (page: number, pageSize: number, query = '') => {
    const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
    if (query.trim()) params.set('q', query.trim())
    return request<ViewerUserPage>(`/api/viewer/users?${params}`)
  },
  viewerUserActivities: (userId: number, limit = 200) => {
    const params = new URLSearchParams({ limit: String(limit) })
    return request<{ items: ViewerUserActivity[] }>(`/api/viewer/users/${userId}/activities?${params}`)
  },
  updateViewerUser: (userId: number, update: { disabled: boolean }) =>
    request<{ user: ViewerUser }>(`/api/viewer/users/${userId}`, {
      method: 'PATCH',
      body: JSON.stringify(update),
    }),
  resetViewerUserPassword: (userId: number, password: string) =>
    request<{ user: ViewerUser }>(`/api/viewer/users/${userId}/password`, {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),
  viewerInvites: (page: number, pageSize: number) => {
    const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
    return request<ViewerInvitePage>(`/api/viewer/invites?${params}`)
  },
  generateViewerInvite: () =>
    request<{ invite: ViewerInvite }>('/api/viewer/invites', {
      method: 'POST',
      body: '{}',
    }),
  viewerSiteSettings: () => request<{ settings: ViewerSiteSettings }>('/api/viewer/site-settings'),
  updateViewerSiteSettings: (settings: Pick<ViewerSiteSettings, 'siteName' | 'registrationEnabled' | 'inviteRequired'>) =>
    request<{ settings: ViewerSiteSettings }>('/api/viewer/site-settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    }),
  uploadViewerFavicon: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return request<{ settings: ViewerSiteSettings }>('/api/viewer/site-settings/favicon', {
      method: 'PUT',
      body: form,
    })
  },
  viewerCarousels: () => request<{ items: ViewerCarouselItem[] }>('/api/viewer/carousels'),
  createViewerCarousel: (input: ViewerCarouselInput) =>
    request<{ item: ViewerCarouselItem }>('/api/viewer/carousels', {
      method: 'POST',
      body: viewerCarouselForm(input),
    }),
  updateViewerCarousel: (carouselId: number, input: ViewerCarouselInput) =>
    request<{ item: ViewerCarouselItem }>(`/api/viewer/carousels/${carouselId}`, {
      method: 'PUT',
      body: viewerCarouselForm(input),
    }),
  deleteViewerCarousel: (carouselId: number) =>
    request<void>(`/api/viewer/carousels/${carouselId}`, { method: 'DELETE' }),
  appReleases: () => request<{ items: AppRelease[] }>('/api/viewer/app-releases'),
  publishAppRelease: (input: AppReleaseInput) =>
    request<{ release: AppRelease }>('/api/viewer/app-releases', {
      method: 'POST',
      body: appReleaseForm(input),
    }),
  updateAppRelease: (releaseId: number, input: AppReleaseUpdateInput) =>
    request<{ release: AppRelease }>(`/api/viewer/app-releases/${releaseId}`, {
      method: 'PUT',
      body: appReleaseForm(input),
    }),
  deleteAppRelease: (releaseId: number) =>
    request<void>(`/api/viewer/app-releases/${releaseId}`, { method: 'DELETE' }),
  viewerFilterDimensions: () =>
    request<{ items: ViewerFilterDimension[] }>('/api/viewer/filter-dimensions'),
  createViewerFilterDimension: (input: ViewerFilterDimensionInput) =>
    request<{ item: ViewerFilterDimension }>('/api/viewer/filter-dimensions', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  updateViewerFilterDimension: (dimensionId: number, input: ViewerFilterDimensionInput) =>
    request<{ item: ViewerFilterDimension }>(`/api/viewer/filter-dimensions/${dimensionId}`, {
      method: 'PUT',
      body: JSON.stringify(input),
    }),
  deleteViewerFilterDimension: (dimensionId: number) =>
    request<void>(`/api/viewer/filter-dimensions/${dimensionId}`, { method: 'DELETE' }),
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
  testDownloadSettings: (settings: DownloadSettingsInput) =>
    request<{ result: DownloadConnectionTestResult }>('/api/settings/download/test', {
      method: 'POST',
      body: JSON.stringify(settings),
    }),
  systemLogs: (levels: LogLevel[]) => {
    const query = new URLSearchParams()
    levels.forEach((level) => query.append('level', level))
    return request<{ logs: SystemLog[] }>(`/api/system-logs?${query}`)
  },
  animeList: (page: number, pageSize: number, options: AnimeListOptions = {}) => {
    const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
    if (options.sort) params.set('sort', options.sort)
    if (options.query?.trim()) params.set('q', options.query.trim())
    return request<AnimePage>(`/api/anime?${params}`)
  },
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
  syncAnimeEpisode: (bangumiId: number, input: ManualEpisodeInput) =>
    request<{ result: ManualEpisodeResult; cleanup: EpisodeReplacementCleanup }>(`/api/anime/${bangumiId}/sync-episode`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  updateAnimeEpisodeBinding: (bangumiId: number, source: EpisodeBindingIdentity, target: EpisodeBindingIdentity) =>
    request<{ result: EpisodeBindingMutationResult }>(`/api/anime/${bangumiId}/bound-episodes`, {
      method: 'PATCH',
      body: JSON.stringify({ source, target }),
    }),
  deleteAnimeEpisodeBinding: (bangumiId: number, source: EpisodeBindingIdentity) =>
    request<{ result: EpisodeBindingMutationResult }>(`/api/anime/${bangumiId}/bound-episodes`, {
      method: 'DELETE',
      body: JSON.stringify({ source }),
    }),
  moveAnimeStorage: (bangumiId: number, storageRoot: string) =>
    request<{ result: StorageMoveResult }>(`/api/anime/${bangumiId}/storage`, {
      method: 'POST',
      body: JSON.stringify({ storageRoot }),
    }),
  updateAnimeSettings: (bangumiId: number, settings: Pick<AnimeSettings, 'subscriptionEpisodeOffset'>) =>
    request<{ settings: AnimeSettings }>(`/api/anime/${bangumiId}/settings`, {
      method: 'PATCH',
      body: JSON.stringify(settings),
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
