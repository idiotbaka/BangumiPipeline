import { tauriGlobal } from './tauri'

export async function copyText(text: string) {
  const core = tauriGlobal()?.core
  if (core) {
    await core.invoke('plugin:player|copyText', { text })
    return
  }
  await navigator.clipboard.writeText(text)
}
