interface TauriCore {
  invoke<T>(command: string, args?: Record<string, unknown>): Promise<T>
}

interface TauriOpener {
  openUrl(url: string, openWith?: string): Promise<void>
}

interface TauriGlobal {
  core?: TauriCore
  opener?: TauriOpener
}

declare global {
  interface Window {
    __TAURI__?: TauriGlobal
  }
}

export function tauriGlobal() {
  return window.__TAURI__
}
