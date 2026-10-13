export type LibraryAnimeType = 'all' | 'series' | 'movie'

export const libraryTypeOptions = [
  { value: 'series', label: 'TV/连载' },
  { value: 'movie', label: '动画电影/剧场版' },
] as const

// Clicking an active type clears the selection and includes both types.
export function toggleLibraryType(current: LibraryAnimeType, selected: 'series' | 'movie'): LibraryAnimeType {
  return current === selected ? 'all' : selected
}

interface HistoryItem {
  animeType: 'series' | 'movie'
  completed: boolean
  episodeLabel: string
  progressPercent: number
  totalEpisodes: number
  latestEpisodeLabel: string
}

export function historyProgressText(item: HistoryItem): string {
  if (item.animeType === 'movie') return item.completed ? '已看完' : `看到 ${item.progressPercent}%`
  return item.completed ? `已看完 ${item.episodeLabel}` : `看到 ${item.episodeLabel} ${item.progressPercent}%`
}

export function historyTotalText(item: HistoryItem): string {
  if (item.animeType === 'movie') return '动画电影'
  const total = item.totalEpisodes > 0 ? `全 ${item.totalEpisodes} 话` : '话数未定'
  const latest = item.latestEpisodeLabel ? `更新至 ${item.latestEpisodeLabel}` : '更新话数未知'
  return `${total} / ${latest}`
}
