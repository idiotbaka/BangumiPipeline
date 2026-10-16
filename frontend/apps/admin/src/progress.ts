import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import type { ProgressPage, TranslationKind } from './api'

export const translationKinds: Array<{ value: TranslationKind; label: string }> = [
  { value: 'anime_title', label: '番剧标题' },
  { value: 'anime_summary', label: '番剧简介' },
  { value: 'episode_title', label: '分集标题' },
  { value: 'episode_summary', label: '分集简介' },
  { value: 'character_summary', label: '角色简介' },
  { value: 'actor_short_summary', label: '声优简介' },
]

export function formatProgressTime(timestamp: number | null) {
  return timestamp ? new Date(timestamp * 1000).toLocaleString() : '—'
}

export function useProgressDetails<T>(getID: () => number, fetchDetails: (id: number) => Promise<{ items: T[] }>) {
  const items = shallowRef<T[]>([])
  const loading = ref(false)
  const error = ref('')
  let generation = 0
  async function load() {
    const current = ++generation
    loading.value = true
    error.value = ''
    items.value = []
    try {
      const result = await fetchDetails(getID())
      if (current === generation) items.value = result.items
    } catch (cause) {
      if (current === generation) error.value = cause instanceof Error ? cause.message : '加载明细失败'
    } finally {
      if (current === generation) loading.value = false
    }
  }
  watch(getID, load, { immediate: true })
  onBeforeUnmount(() => { generation++ })
  return { items, loading, error, load }
}

export function useProgressList<T, S extends string>(
  fetchPage: (page: number, pageSize: number, query: string, status: S) => Promise<ProgressPage<T>>,
  initialStatus: S,
) {
  const items = shallowRef<T[]>([])
  const total = ref(0)
  const page = ref(1)
  const pageSize = 50
  const query = ref('')
  const status = shallowRef(initialStatus)
  const loading = ref(false)
  const error = ref('')
  const revision = ref(0)
  let generation = 0
  let appliedQuery = ''

  async function load() {
    const current = ++generation
    const selectedQuery = appliedQuery
    const selectedStatus = status.value
    loading.value = true
    error.value = ''
    // Clear stale rows when the selected search/filter changes or a refresh fails.
    items.value = []
    total.value = 0
    try {
      let result = await fetchPage(page.value, pageSize, selectedQuery, selectedStatus)
      if (current !== generation) return
      const lastPage = Math.max(1, Math.ceil(result.total / pageSize))
      if (page.value > lastPage) {
        page.value = lastPage
        result = await fetchPage(page.value, pageSize, selectedQuery, selectedStatus)
        if (current !== generation) return
      }
      items.value = result.items
      total.value = result.total
    } catch (cause) {
      if (current === generation) error.value = cause instanceof Error ? cause.message : '加载进度失败'
    } finally {
      if (current === generation) loading.value = false
    }
  }

  function search() {
    appliedQuery = query.value.trim()
    page.value = 1
    void load()
  }

  function changePage(value: number) {
    page.value = value
    void load()
  }

  function refresh() {
    revision.value++
    void load()
  }

  onMounted(load)
  onBeforeUnmount(() => { generation++ })
  return { items, total, page, pageSize, query, status, loading, error, revision, load, search, changePage, refresh }
}
