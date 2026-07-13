import { tauriGlobal } from './tauri'

function tauriCore() {
  return tauriGlobal()?.core
}

export async function enterNativeFullscreen() {
  const core = tauriCore()
  if (!core) {
    return false
  }
  try {
    await core.invoke('plugin:player|enterFullscreen', { orientation: 'sensorLandscape' })
  } catch {
    await core.invoke('plugin:player|enter_fullscreen', { args: { orientation: 'sensorLandscape' } })
  }
  return true
}

export async function exitNativeFullscreen() {
  const core = tauriCore()
  if (!core) {
    return false
  }
  try {
    await core.invoke('plugin:player|exitFullscreen')
  } catch {
    await core.invoke('plugin:player|exit_fullscreen')
  }
  return true
}

export async function setNativeKeepScreenOn(enabled: boolean) {
  const core = tauriCore()
  if (!core) {
    return false
  }
  try {
    await core.invoke('plugin:player|setKeepScreenOn', { enabled })
  } catch {
    await core.invoke('plugin:player|set_keep_screen_on', { args: { enabled } })
  }
  return true
}
