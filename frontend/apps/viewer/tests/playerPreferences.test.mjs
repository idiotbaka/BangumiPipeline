import assert from 'node:assert/strict'
import test from 'node:test'

import {
  defaultPlayerPreferences,
  loadPlayerPreferences,
  playerPreferencesStorageKey,
  savePlayerPreferences,
} from '../src/playerPreferences.ts'

function createMemoryStorage(initialValue = null) {
  const values = new Map()
  if (initialValue !== null) values.set(playerPreferencesStorageKey, initialValue)
  return {
    getItem(key) {
      return values.get(key) ?? null
    },
    setItem(key, value) {
      values.set(key, value)
    },
  }
}

test('uses pause playback and a visible OP skip button by default', () => {
  assert.deepEqual(loadPlayerPreferences(createMemoryStorage()), defaultPlayerPreferences)
})

test('loads and normalizes stored player preferences', () => {
  const storage = createMemoryStorage(JSON.stringify({
    playbackMode: 'auto-next',
    showOPSkipButton: false,
    volume: 1.5,
    muted: true,
  }))

  assert.deepEqual(loadPlayerPreferences(storage), {
    playbackMode: 'auto-next',
    showOPSkipButton: false,
    volume: 1,
    muted: true,
  })
})

test('persists playback, OP skip, and volume preferences together', () => {
  const storage = createMemoryStorage()
  savePlayerPreferences({
    playbackMode: 'auto-next',
    showOPSkipButton: false,
    volume: 0.45,
    muted: false,
  }, storage)

  assert.deepEqual(loadPlayerPreferences(storage), {
    playbackMode: 'auto-next',
    showOPSkipButton: false,
    volume: 0.45,
    muted: false,
  })
})
