import { readonly, ref } from 'vue'

import { tauriGlobal } from './native/tauri'
import { isTVApp } from './platform'

export type MobileTheme = 'light' | 'dark'

// 与 index.html 的首次绘制初始化保持一致；不随账号或服务器切换清除。
const storageKey = 'bp.mobile.theme'

export function useMobileTheme() {
  const theme = ref<MobileTheme>(document.documentElement.dataset.mobileTheme === 'dark' ? 'dark' : 'light')
  const saving = ref(false)
  const message = ref('')
  let nativeSync = Promise.resolve()

  function syncNativeTheme(value: MobileTheme) {
    // 串行更新，避免快速切换时较早的原生调用覆盖新选择。
    const update = nativeSync.then(async () => {
      const core = tauriGlobal()?.core
      if (core && !isTVApp) await core.invoke('plugin:player|setTheme', { dark: value === 'dark' })
    })
    nativeSync = update.catch(() => undefined)
    return update
  }

  async function initializeNativeTheme() {
    try {
      await syncNativeTheme(theme.value)
    } catch {
      message.value = '页面主题已应用，系统栏同步失败，可重新选择主题重试。'
    }
  }

  async function setTheme(value: MobileTheme) {
    if (saving.value) return
    saving.value = true
    message.value = ''
    theme.value = value
    document.documentElement.dataset.mobileTheme = value
    const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
    if (meta) meta.content = value === 'dark' ? '#11151f' : '#ff5f9e'
    try {
      localStorage.setItem(storageKey, value)
    } catch {
      message.value = '主题已应用，但无法保存；重新打开 APP 后可能恢复默认。'
    }
    try {
      await syncNativeTheme(value)
    } catch {
      message.value ||= '页面主题已应用，系统栏同步失败，可重新选择主题重试。'
    } finally {
      saving.value = false
    }
  }

  return { theme: readonly(theme), saving: readonly(saving), message: readonly(message), setTheme, initializeNativeTheme }
}
