import { onMounted, onUnmounted, readonly, ref } from 'vue'

// 与 index.html 中首次绘制前执行的主题初始化保持一致。
const storageKey = 'bp-viewer-theme'

export function useViewerTheme(isAppDownloadPage: boolean) {
  const isNightMode = ref(!isAppDownloadPage && document.documentElement.dataset.viewerTheme === 'dark')

  function applyTheme(dark: boolean) {
    isNightMode.value = !isAppDownloadPage && dark
    document.documentElement.dataset.viewerTheme = isNightMode.value ? 'dark' : 'light'
    const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
    if (meta) meta.content = isNightMode.value ? '#10131d' : '#ff5f9e'
  }

  function toggleNightMode() {
    if (isAppDownloadPage) return
    applyTheme(!isNightMode.value)
    try {
      localStorage.setItem(storageKey, isNightMode.value ? 'dark' : 'light')
    } catch {
      // 隐私设置或存储配额限制不应阻止本次浏览切换主题。
    }
  }

  function syncTheme(event: StorageEvent) {
    if (event.key === storageKey || event.key === null) {
      applyTheme(event.newValue === 'dark')
    }
  }

  onMounted(() => window.addEventListener('storage', syncTheme))
  onUnmounted(() => window.removeEventListener('storage', syncTheme))

  return { isNightMode: readonly(isNightMode), toggleNightMode }
}
