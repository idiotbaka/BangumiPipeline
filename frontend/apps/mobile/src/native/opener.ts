import { tauriGlobal } from './tauri'

export async function openExternalURL(url: string) {
  const tauri = tauriGlobal()
  if (tauri) {
    if (!tauri.opener) {
      throw new Error('系统浏览器组件尚未就绪')
    }
    await tauri.opener.openUrl(url)
    return
  }

  const opened = window.open(url, '_blank', 'noopener,noreferrer')
  if (!opened) {
    throw new Error('浏览器阻止了新窗口，请检查弹窗权限')
  }
}
