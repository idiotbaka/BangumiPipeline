import { onBeforeUnmount, ref, shallowRef } from 'vue'

export function useDashboardSection<T>(fetchData: () => Promise<T>) {
  const data = shallowRef<T | null>(null)
  const loading = ref(false)
  const error = ref('')
  const loadedAt = ref(0)
  let generation = 0
  async function load() {
    const current = ++generation
    loading.value = true
    error.value = ''
    try {
      const result = await fetchData()
      if (current === generation) {
        data.value = result
        loadedAt.value = Date.now()
      }
    } catch (cause) {
      if (current === generation) error.value = cause instanceof Error ? cause.message : '加载失败'
    } finally {
      if (current === generation) loading.value = false
    }
  }
  onBeforeUnmount(() => { generation++ })
  return { data, loading, error, loadedAt, load }
}
