export type PlaybackMode = 'pause' | 'auto-next'

export interface PlayerPreferences {
  playbackMode: PlaybackMode
  showOPSkipButton: boolean
  volume: number
  muted: boolean
}

type PlayerPreferenceStorage = Pick<Storage, 'getItem' | 'setItem'>

export const playerPreferencesStorageKey = 'bangumi-pipeline.viewer.player-preferences'

export const defaultPlayerPreferences: PlayerPreferences = {
  playbackMode: 'pause',
  showOPSkipButton: true,
  volume: 1,
  muted: false,
}

export function loadPlayerPreferences(storage = browserStorage()): PlayerPreferences {
  if (!storage) return { ...defaultPlayerPreferences }
  try {
    const rawValue = storage.getItem(playerPreferencesStorageKey)
    if (!rawValue) return { ...defaultPlayerPreferences }
    return normalizePlayerPreferences(JSON.parse(rawValue))
  } catch {
    return { ...defaultPlayerPreferences }
  }
}

export function savePlayerPreferences(preferences: PlayerPreferences, storage = browserStorage()) {
  if (!storage) return
  try {
    storage.setItem(playerPreferencesStorageKey, JSON.stringify(normalizePlayerPreferences(preferences)))
  } catch {
    // 本地存储不可用时不影响播放器的当前会话。
  }
}

export function normalizePlayerPreferences(value: unknown): PlayerPreferences {
  if (!value || typeof value !== 'object') return { ...defaultPlayerPreferences }
  const stored = value as Partial<PlayerPreferences>
  const volume = typeof stored.volume === 'number' && Number.isFinite(stored.volume)
    ? Math.round(Math.max(0, Math.min(1, stored.volume)) * 100) / 100
    : defaultPlayerPreferences.volume

  return {
    playbackMode: stored.playbackMode === 'auto-next' ? 'auto-next' : 'pause',
    showOPSkipButton: typeof stored.showOPSkipButton === 'boolean'
      ? stored.showOPSkipButton
      : defaultPlayerPreferences.showOPSkipButton,
    volume,
    muted: volume === 0 || (typeof stored.muted === 'boolean' ? stored.muted : false),
  }
}

function browserStorage(): PlayerPreferenceStorage | null {
  if (typeof window === 'undefined') return null
  try {
    return window.localStorage
  } catch {
    return null
  }
}
