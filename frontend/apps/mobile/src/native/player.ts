import { tauriGlobal } from './tauri'
import { isTVApp } from '../platform'

function tauriCore() {
  return tauriGlobal()?.core
}

export async function enterNativeFullscreen() {
  if (isTVApp) {
    return true
  }
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
  if (isTVApp) {
    return true
  }
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
