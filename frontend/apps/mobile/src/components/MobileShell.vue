<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

import {
  api,
  buildAuthenticatedMediaURL,
  type ViewerAnimeCard,
  type ViewerFilterDimension,
  type ViewerFollowedAnime,
  type ViewerHome,
  type ViewerLibrary,
  type ViewerSchedule,
  type ViewerScheduleCard,
  type ViewerUser,
  type ViewerWatchHistoryItem,
} from '../api'
import { normalizeAPIBaseURL, parseAPIBaseURL } from '../config'
import appIcon from '../../../../../src-tauri/icons/icon.png'
import tauriConfig from '../../../../../src-tauri/tauri.conf.json'
import homeNavIcon from '../assets/nav-home.svg?raw'
import libraryNavIcon from '../assets/nav-library.svg?raw'
import profileNavIcon from '../assets/nav-profile.svg?raw'
import scheduleNavIcon from '../assets/nav-schedule.svg?raw'
import searchIcon from '../assets/search.svg?raw'
import MobileAnimeDetailScreen from './MobileAnimeDetailScreen.vue'

interface Props {
  user: ViewerUser
  loading: boolean
  apiBaseUrl: string
  checkingAppUpdate: boolean
  appUpdateCheckMessage: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (event: 'logout'): void
  (event: 'server-address-change', value: string): void
  (event: 'check-app-update'): void
}>()

type MainTab = 'home' | 'schedule' | 'library' | 'profile'
type RoutePage = 'search' | 'follows' | 'history' | 'settings' | 'server-address' | 'about' | 'detail' | null
type SubRoutePage = Exclude<RoutePage, 'detail' | null>
type DetailReturnPage = Exclude<RoutePage, 'detail'>
type PageTransitionName = 'page-slide-forward' | 'page-slide-back' | 'page-none'

interface MobileShellHistoryState {
  bpMobileShell: true
  historyIndex: number
  tab: MainTab
  page: RoutePage
  searchQuery: string
  searchPageQuery: string
  detailReturnPage: DetailReturnPage
  detailAnimeId: number
  detailMediaId: number
  detailPosition: number
}

const appName = 'BakaVip2'
const appVersion = tauriConfig.version
const now = new Date()
const tabs: Array<{ key: MainTab; label: string; icon: string }> = [
  { key: 'home', label: '首页', icon: homeNavIcon },
  { key: 'schedule', label: '时间表', icon: scheduleNavIcon },
  { key: 'library', label: '图书馆', icon: libraryNavIcon },
  { key: 'profile', label: '我的', icon: profileNavIcon },
]
const libraryPageSize = 30
const weekdays = [
  { value: 1, label: '周一' },
  { value: 2, label: '周二' },
  { value: 3, label: '周三' },
  { value: 4, label: '周四' },
  { value: 5, label: '周五' },
  { value: 6, label: '周六' },
  { value: 7, label: '周日' },
  { value: 8, label: '其他' },
]

const activeTab = ref<MainTab>('home')
const routePage = ref<RoutePage>(null)
const appPage = ref<HTMLElement | null>(null)
const pageTransitionName = ref<PageTransitionName>('page-none')
const pageTransitionActive = ref(false)
const pageTransitionStyle = ref<Record<string, string>>({})
const detailReturnPage = ref<DetailReturnPage>(null)
const detailAnimeId = ref(0)
const detailMediaId = ref(0)
const detailPosition = ref(0)
const relativeTimeNow = ref(Date.now())
const failedImages = ref<Set<string>>(new Set())
const serverAddressDraft = ref(props.apiBaseUrl)
const serverAddressError = ref('')
const serverAddressSaving = ref(false)

const home = ref<ViewerHome>({
  hotRecommendations: [],
  recentUpdates: [],
  carouselSlides: [],
  myFollows: [],
})
const homeLoading = ref(false)
const homeError = ref('')
const homeRefreshToken = ref(0)

const searchQuery = ref('')
const searchPageQuery = ref('')
const searchResults = ref<ViewerScheduleCard[]>([])
const searchLoading = ref(false)
const searchError = ref('')

const seasonYear = ref(now.getFullYear())
const seasonMonth = ref(Math.floor(now.getMonth() / 3) * 3 + 1)
const selectedWeekday = ref(now.getDay() === 0 ? 7 : now.getDay())
const schedule = ref<ViewerSchedule | null>(null)
const scheduleLoading = ref(false)
const scheduleError = ref('')

const libraryDimensions = ref<ViewerFilterDimension[]>([])
const selectedLibraryFilters = ref<Record<number, string[]>>({})
const library = ref<ViewerLibrary>({ items: [], total: 0 })
const libraryVisibleCount = ref(libraryPageSize)
const libraryFiltersLoading = ref(false)
const libraryFiltersError = ref('')
const libraryLoading = ref(false)
const libraryError = ref('')

const follows = ref<ViewerFollowedAnime[]>([])
const history = ref<ViewerWatchHistoryItem[]>([])
const profileLoaded = ref(false)
const profileLoading = ref(false)
const profileError = ref('')
const profileRefreshToken = ref(0)

let relativeTimer: ReturnType<typeof setInterval> | null = null
let libraryRequestID = 0
let refreshTokenSeed = 0
let shellHistoryIndex = 0
let previousScrollRestoration: ScrollRestoration | null = null
let pendingTransitionScrollTop: number | null = null
const tabScrollPositions: Record<MainTab, number> = {
  home: 0,
  schedule: 0,
  library: 0,
  profile: 0,
}

const seasonKey = computed(() => `${seasonYear.value}-${String(seasonMonth.value).padStart(2, '0')}`)
const selectedDay = computed(() => weekdays.find((day) => day.value === selectedWeekday.value) ?? weekdays[0])
const scheduleItems = computed(() =>
  (schedule.value?.items ?? []).filter((item) => normalizedWeekday(item.airWeekday) === selectedWeekday.value),
)
const homeFollows = computed(() => home.value.myFollows.filter((item) => !item.caughtUp).slice(0, 10))
const recentItems = computed(() => home.value.recentUpdates.slice(0, 9))
const hotItems = computed(() => home.value.hotRecommendations.slice(0, 9))
const selectedLibraryTagCount = computed(() =>
  Object.values(selectedLibraryFilters.value).reduce((total, tags) => total + tags.length, 0),
)
const visibleLibraryItems = computed(() => library.value.items.slice(0, libraryVisibleCount.value))
const hasMoreLibraryItems = computed(() => libraryVisibleCount.value < library.value.items.length)
const pageTitle = computed(() => {
  if (routePage.value === 'search') return '搜索结果'
  if (routePage.value === 'follows') return '我的追番'
  if (routePage.value === 'history') return '观看历史'
  if (routePage.value === 'settings') return '系统设置'
  if (routePage.value === 'server-address') return '服务器地址'
  if (routePage.value === 'about') return '关于'
  if (routePage.value === 'detail') return '番剧详情'
  return ''
})
const topbarMode = computed(() => {
  if (routePage.value === 'detail') return 'none'
  return routePage.value === null ? 'main' : 'sub'
})
const topbarKey = computed(() => {
  if (topbarMode.value === 'sub') return `sub-${routePage.value}`
  return topbarMode.value
})
const topbarTransitionName = computed(() => {
  if (pageTransitionName.value === 'page-slide-forward') return 'topbar-slide-forward'
  if (pageTransitionName.value === 'page-slide-back') return 'topbar-slide-back'
  return 'topbar-none'
})
const pageViewKey = computed(() => {
  if (routePage.value === 'detail') {
    return `detail-${detailAnimeId.value}-${detailMediaId.value || 0}`
  }
  if (routePage.value !== null) {
    return `route-${routePage.value}`
  }
  return `tab-${activeTab.value}`
})

onMounted(() => {
  initializeShellHistory()
  void loadHome()
  window.addEventListener('scroll', handleWindowScroll, { passive: true })
  window.addEventListener('popstate', handleShellPopState)
  relativeTimer = setInterval(() => {
    relativeTimeNow.value = Date.now()
  }, 60_000)
})

onBeforeUnmount(() => {
  if (relativeTimer !== null) {
    clearInterval(relativeTimer)
  }
  restoreBrowserScrollRestoration()
  window.removeEventListener('scroll', handleWindowScroll)
  window.removeEventListener('popstate', handleShellPopState)
})

function initializeShellHistory() {
  previousScrollRestoration = window.history.scrollRestoration
  window.history.scrollRestoration = 'manual'
  shellHistoryIndex = 0
  window.history.replaceState(createShellHistoryState(), '', window.location.href)
}

function restoreBrowserScrollRestoration() {
  if (previousScrollRestoration !== null) {
    window.history.scrollRestoration = previousScrollRestoration
    previousScrollRestoration = null
  }
}

function createShellHistoryState(): MobileShellHistoryState {
  return {
    bpMobileShell: true,
    historyIndex: shellHistoryIndex,
    tab: activeTab.value,
    page: routePage.value,
    searchQuery: searchQuery.value,
    searchPageQuery: searchPageQuery.value,
    detailReturnPage: detailReturnPage.value,
    detailAnimeId: detailAnimeId.value,
    detailMediaId: detailMediaId.value,
    detailPosition: detailPosition.value,
  }
}

function pushShellHistoryState() {
  shellHistoryIndex += 1
  window.history.pushState(createShellHistoryState(), '', window.location.href)
}

function replaceShellHistoryState() {
  window.history.replaceState(createShellHistoryState(), '', window.location.href)
}

function beginPageTransition(name: PageTransitionName, scrollTop: number | null) {
  pageTransitionName.value = name
  pendingTransitionScrollTop = scrollTop
  if (name === 'page-none') {
    pageTransitionActive.value = false
    pageTransitionStyle.value = {}
    return
  }

  const currentView = appPage.value?.querySelector<HTMLElement>('.page-view')
  const rect = currentView?.getBoundingClientRect()
  pageTransitionActive.value = true
  pageTransitionStyle.value = rect
    ? {
        '--transition-page-top': `${rect.top}px`,
        '--transition-page-left': `${rect.left}px`,
        '--transition-page-width': `${rect.width}px`,
        '--transition-page-min-height': `${Math.max(appPage.value?.offsetHeight ?? 0, rect.height)}px`,
      }
    : {}
}

function applyPendingTransitionScroll() {
  if (pendingTransitionScrollTop === null) {
    return
  }
  const top = pendingTransitionScrollTop
  pendingTransitionScrollTop = null
  window.scrollTo({ top, left: 0, behavior: 'auto' })
}

function schedulePendingTransitionScroll() {
  void nextTick(() => {
    applyPendingTransitionScroll()
  })
}

function finishPageTransition() {
  pageTransitionActive.value = false
  pageTransitionStyle.value = {}
  pendingTransitionScrollTop = null
}

function handleShellPopState(event: PopStateEvent) {
  if (!isShellHistoryState(event.state)) {
    return
  }

  const previousPage = routePage.value
  const previousTab = activeTab.value
  const previousDetailAnimeId = detailAnimeId.value
  const previousDetailMediaId = detailMediaId.value
  const transitionName =
    event.state.historyIndex < shellHistoryIndex
      ? 'page-slide-back'
      : event.state.historyIndex > shellHistoryIndex
        ? 'page-slide-forward'
        : 'page-none'
  const transitionScrollTop = event.state.page === null ? tabScrollPositions[event.state.tab] ?? 0 : 0
  beginPageTransition(transitionName, transitionName === 'page-none' ? null : transitionScrollTop)
  shellHistoryIndex = Math.max(0, event.state.historyIndex)
  activeTab.value = event.state.tab
  routePage.value = event.state.page
  searchQuery.value = event.state.searchQuery
  searchPageQuery.value = event.state.searchPageQuery

  if (event.state.page === 'detail') {
    detailReturnPage.value = event.state.detailReturnPage
    detailAnimeId.value = event.state.detailAnimeId
    detailMediaId.value = event.state.detailMediaId
    detailPosition.value = event.state.detailPosition
  } else {
    resetDetailState()
  }

  ensureCurrentViewData()

  const isSameDetail =
    previousPage === 'detail' &&
    event.state.page === 'detail' &&
    previousDetailAnimeId === event.state.detailAnimeId &&
    previousDetailMediaId === event.state.detailMediaId

  if (transitionName !== 'page-none' && !isSameDetail) {
    schedulePendingTransitionScroll()
  } else if (event.state.page === null) {
    if (previousPage !== null || previousTab !== event.state.tab) {
      restoreTabScroll(event.state.tab)
    }
  } else if (!isSameDetail) {
    scrollToTopAfterRender()
  }
}

function isShellHistoryState(value: unknown): value is MobileShellHistoryState {
  if (!value || typeof value !== 'object') {
    return false
  }
  const state = value as Partial<MobileShellHistoryState>
  return (
    state.bpMobileShell === true &&
    typeof state.historyIndex === 'number' &&
    isMainTab(state.tab) &&
    isRoutePage(state.page)
  )
}

function isMainTab(value: unknown): value is MainTab {
  return value === 'home' || value === 'schedule' || value === 'library' || value === 'profile'
}

function isRoutePage(value: unknown): value is RoutePage {
  return (
    value === null ||
    value === 'search' ||
    value === 'follows' ||
    value === 'history' ||
    value === 'settings' ||
    value === 'server-address' ||
    value === 'about' ||
    value === 'detail'
  )
}

function navigateBackOrClose() {
  if (shellHistoryIndex > 0) {
    window.history.back()
    return
  }
  if (routePage.value === 'detail') {
    closeAnimeDetailDirect()
    return
  }
  if (routePage.value !== null) {
    closeRouteDirect()
  }
}

function resetDetailState() {
  detailReturnPage.value = null
  detailAnimeId.value = 0
  detailMediaId.value = 0
  detailPosition.value = 0
}

function ensureCurrentViewData() {
  if (routePage.value === 'follows' || routePage.value === 'history') {
    void ensureProfile()
    return
  }
  if (routePage.value === null) {
    if (activeTab.value === 'home') {
      refreshHomeIfNeeded()
    }
    if (activeTab.value === 'schedule' && schedule.value === null && !scheduleLoading.value) {
      void loadSchedule()
    }
    if (activeTab.value === 'library') {
      void ensureLibrary()
    }
    if (activeTab.value === 'profile') {
      void ensureProfile()
    }
  }
}

function showTab(tab: MainTab) {
  if (routePage.value === null && activeTab.value === tab) {
    if (tab === 'home') {
      refreshHomeIfNeeded()
    }
    if (tab === 'profile') {
      void ensureProfile()
    }
    return
  }
  saveCurrentTabScroll()
  pageTransitionName.value = 'page-none'
  activeTab.value = tab
  routePage.value = null
  resetDetailState()
  if (tab === 'schedule' && schedule.value === null && !scheduleLoading.value) {
    void loadSchedule()
  }
  if (tab === 'library') {
    void ensureLibrary()
  }
  if (tab === 'profile') {
    void ensureProfile()
  }
  if (tab === 'home') {
    refreshHomeIfNeeded()
  }
  replaceShellHistoryState()
  restoreTabScroll(tab)
}

function openRoute(page: SubRoutePage) {
  saveCurrentTabScroll()
  beginPageTransition('page-slide-forward', 0)
  routePage.value = page
  resetDetailState()
  if (page === 'follows' || page === 'history') {
    void ensureProfile()
  }
  pushShellHistoryState()
  schedulePendingTransitionScroll()
}

function openServerAddressSettings() {
  serverAddressDraft.value = props.apiBaseUrl
  serverAddressError.value = ''
  serverAddressSaving.value = false
  openRoute('server-address')
}

function saveServerAddress() {
  if (serverAddressSaving.value) {
    return
  }
  serverAddressError.value = ''
  const normalizedBaseURL = parseAPIBaseURL(serverAddressDraft.value)
  if (!normalizedBaseURL) {
    serverAddressError.value = '请输入有效的 HTTP 或 HTTPS 服务器地址'
    return
  }

  const serverChanged = normalizedBaseURL !== normalizeAPIBaseURL(props.apiBaseUrl)
  serverAddressDraft.value = normalizedBaseURL
  if (!serverChanged) {
    emit('server-address-change', normalizedBaseURL)
    closeRoute()
    return
  }

  serverAddressSaving.value = true
  if (shellHistoryIndex <= 0) {
    emit('server-address-change', normalizedBaseURL)
    return
  }

  const returnSteps = shellHistoryIndex
  window.addEventListener('popstate', () => emit('server-address-change', normalizedBaseURL), { once: true })
  window.history.go(-returnSteps)
}

function closeRoute() {
  navigateBackOrClose()
}

function closeRouteDirect() {
  if (routePage.value === 'detail') {
    closeAnimeDetailDirect()
    return
  }
  beginPageTransition('page-slide-back', tabScrollPositions[activeTab.value] ?? 0)
  routePage.value = null
  resetDetailState()
  if (activeTab.value === 'home') {
    refreshHomeIfNeeded()
  }
  if (activeTab.value === 'profile') {
    void ensureProfile()
  }
  replaceShellHistoryState()
  schedulePendingTransitionScroll()
}

function openAnimeDetail(bangumiId: number, mediaId = 0, positionSeconds = 0) {
  if (!Number.isFinite(bangumiId) || bangumiId <= 0) {
    return
  }
  markPlaybackDataNeedsRefresh()
  if (routePage.value === null) {
    saveCurrentTabScroll()
    detailReturnPage.value = null
  } else if (routePage.value !== 'detail') {
    detailReturnPage.value = routePage.value
  }
  detailAnimeId.value = bangumiId
  detailMediaId.value = mediaId > 0 ? mediaId : 0
  detailPosition.value = positionSeconds > 0 ? positionSeconds : 0
  beginPageTransition('page-slide-forward', 0)
  routePage.value = 'detail'
  pushShellHistoryState()
  schedulePendingTransitionScroll()
}

function closeAnimeDetail() {
  navigateBackOrClose()
}

function closeAnimeDetailDirect() {
  const returnPage = detailReturnPage.value
  beginPageTransition('page-slide-back', returnPage === null ? tabScrollPositions[activeTab.value] ?? 0 : 0)
  resetDetailState()
  routePage.value = returnPage
  if (returnPage === 'follows' || returnPage === 'history') {
    void ensureProfile()
  }
  if (returnPage === null) {
    if (activeTab.value === 'home') {
      refreshHomeIfNeeded()
    }
    if (activeTab.value === 'profile') {
      void ensureProfile()
    }
  } else {
    // 子页面当前没有独立滚动记忆，返回时保持在顶部。
  }
  replaceShellHistoryState()
  schedulePendingTransitionScroll()
}

function handleDetailFollowChanged() {
  markPlaybackDataNeedsRefresh()
}

function markPlaybackDataNeedsRefresh() {
  refreshTokenSeed += 1
  homeRefreshToken.value = refreshTokenSeed
  profileRefreshToken.value = refreshTokenSeed
}

function refreshHomeIfNeeded() {
  if (homeRefreshToken.value === 0 || homeLoading.value) {
    return
  }
  void loadHome()
}

async function loadHome() {
  const requestRefreshToken = homeRefreshToken.value
  homeLoading.value = true
  homeError.value = ''
  try {
    const result = await api.home()
    home.value = result.home
    if (homeRefreshToken.value === requestRefreshToken) {
      homeRefreshToken.value = 0
    }
  } catch (error) {
    homeError.value = error instanceof Error ? error.message : '首页加载失败'
  } finally {
    homeLoading.value = false
  }
}

async function submitSearch() {
  const query = searchQuery.value.trim()
  if (!query) {
    return
  }
  const shouldPushRoute = routePage.value !== 'search'
  if (routePage.value === null) {
    saveCurrentTabScroll()
  }
  searchPageQuery.value = query
  if (shouldPushRoute) {
    beginPageTransition('page-slide-forward', 0)
  } else {
    beginPageTransition('page-none', null)
  }
  routePage.value = 'search'
  resetDetailState()
  if (shouldPushRoute) {
    pushShellHistoryState()
  } else {
    replaceShellHistoryState()
  }
  if (shouldPushRoute) {
    schedulePendingTransitionScroll()
  } else {
    scrollToTopAfterRender()
  }
  await loadSearch(query)
}

async function loadSearch(query: string) {
  searchLoading.value = true
  searchError.value = ''
  try {
    const result = await api.animeLibrary(query)
    searchResults.value = result.library.items
  } catch (error) {
    searchError.value = error instanceof Error ? error.message : '搜索失败'
  } finally {
    searchLoading.value = false
  }
}

async function loadSchedule() {
  scheduleLoading.value = true
  scheduleError.value = ''
  try {
    const result = await api.animeSchedule(seasonKey.value)
    schedule.value = result.schedule
  } catch (error) {
    scheduleError.value = error instanceof Error ? error.message : '时间表加载失败'
  } finally {
    scheduleLoading.value = false
  }
}

async function ensureLibrary() {
  if (libraryDimensions.value.length === 0 && !libraryFiltersLoading.value) {
    void loadLibraryFilters()
  }
  if (library.value.items.length === 0 && !libraryLoading.value) {
    await loadLibrary()
  }
}

async function loadLibraryFilters() {
  libraryFiltersLoading.value = true
  libraryFiltersError.value = ''
  try {
    const result = await api.libraryFilters()
    libraryDimensions.value = result.items
  } catch (error) {
    libraryFiltersError.value = error instanceof Error ? error.message : '筛选标签加载失败'
  } finally {
    libraryFiltersLoading.value = false
  }
}

async function loadLibrary() {
  const currentRequest = ++libraryRequestID
  libraryLoading.value = true
  libraryError.value = ''
  try {
    const result = await api.animeLibrary('', selectedLibraryFilters.value)
    if (currentRequest === libraryRequestID) {
      library.value = result.library
      libraryVisibleCount.value = libraryPageSize
    }
  } catch (error) {
    if (currentRequest === libraryRequestID) {
      libraryError.value = error instanceof Error ? error.message : '番剧图书馆加载失败'
    }
  } finally {
    if (currentRequest === libraryRequestID) {
      libraryLoading.value = false
    }
  }
}

function isLibraryTagSelected(dimensionID: number, tag: string) {
  return selectedLibraryFilters.value[dimensionID]?.includes(tag) ?? false
}

function toggleLibraryTag(dimensionID: number, tag: string) {
  const next = { ...selectedLibraryFilters.value }
  const tags = [...(next[dimensionID] ?? [])]
  const index = tags.indexOf(tag)
  if (index >= 0) {
    tags.splice(index, 1)
  } else {
    tags.push(tag)
  }
  if (tags.length) {
    next[dimensionID] = tags
  } else {
    delete next[dimensionID]
  }
  selectedLibraryFilters.value = next
  void loadLibrary()
}

function resetLibraryFilters() {
  if (selectedLibraryTagCount.value === 0 || libraryLoading.value) {
    return
  }
  selectedLibraryFilters.value = {}
  void loadLibrary()
}

function loadMoreLibraryItems() {
  if (!hasMoreLibraryItems.value || libraryLoading.value) {
    return
  }
  libraryVisibleCount.value = Math.min(libraryVisibleCount.value + libraryPageSize, library.value.items.length)
}

function saveCurrentTabScroll() {
  if (routePage.value !== null) {
    return
  }
  tabScrollPositions[activeTab.value] = window.scrollY
}

function restoreTabScroll(tab: MainTab) {
  void nextTick(() => {
    window.requestAnimationFrame(() => {
      window.scrollTo({ top: tabScrollPositions[tab] ?? 0, left: 0, behavior: 'auto' })
    })
  })
}

function scrollToTopAfterRender() {
  void nextTick(() => {
    window.requestAnimationFrame(() => {
      window.scrollTo({ top: 0, left: 0, behavior: 'auto' })
    })
  })
}

function handleWindowScroll() {
  if (routePage.value === null) {
    tabScrollPositions[activeTab.value] = window.scrollY
  }
  if (activeTab.value !== 'library' || routePage.value !== null || libraryLoading.value || !hasMoreLibraryItems.value) {
    return
  }
  const scrollElement = document.documentElement
  const distanceToBottom = scrollElement.scrollHeight - (window.scrollY + window.innerHeight)
  if (distanceToBottom < 360) {
    loadMoreLibraryItems()
  }
}

function changeSeason(direction: number) {
  const next = new Date(seasonYear.value, seasonMonth.value - 1 + direction * 3, 1)
  seasonYear.value = next.getFullYear()
  seasonMonth.value = next.getMonth() + 1
  void loadSchedule()
}

async function ensureProfile() {
  if (profileLoading.value) {
    return
  }
  if (profileLoaded.value && profileRefreshToken.value === 0 && !profileError.value) {
    return
  }
  await loadProfile()
}

async function loadProfile() {
  const requestRefreshToken = profileRefreshToken.value
  profileLoading.value = true
  profileError.value = ''
  try {
    const [followResult, historyResult] = await Promise.all([api.followedAnime(), api.watchHistory()])
    follows.value = followResult.items
    history.value = historyResult.items
    profileLoaded.value = true
    if (profileRefreshToken.value === requestRefreshToken) {
      profileRefreshToken.value = 0
    }
  } catch (error) {
    profileError.value = error instanceof Error ? error.message : '个人数据加载失败'
  } finally {
    profileLoading.value = false
  }
}

function normalizedWeekday(value: number) {
  return value >= 1 && value <= 7 ? value : 8
}

function animeCoverURL(bangumiId: number) {
  return buildAuthenticatedMediaURL(`/api/anime/${bangumiId}/cover`)
}

function followCoverURL(item: ViewerFollowedAnime) {
  return buildAuthenticatedMediaURL(`/api/anime/${item.bangumiId}/media/${item.mediaId}/cover`)
}

function mediaCoverURL(item: ViewerWatchHistoryItem) {
  return buildAuthenticatedMediaURL(`/api/anime/${item.bangumiId}/media/${item.mediaId}/cover`)
}

function imageAvailable(key: string, hasImage: boolean) {
  return hasImage && !failedImages.value.has(key)
}

function markImageFailed(key: string) {
  const next = new Set(failedImages.value)
  next.add(key)
  failedImages.value = next
}

function ratingText(score: number | null) {
  return score === null ? '--' : score.toFixed(1)
}

function updateText(item: ViewerAnimeCard | ViewerScheduleCard) {
  return item.latestEpisodeLabel ? `更新至 ${item.latestEpisodeLabel}` : '暂未放流'
}

function formatUpdatedAt(value: number | null) {
  if (!value) {
    return '更新时间未知'
  }
  const elapsedSeconds = Math.max(Math.floor(relativeTimeNow.value / 1000) - value, 0)
  const minutes = Math.max(Math.floor(elapsedSeconds / 60), 1)
  if (minutes < 60) return `${minutes}分钟前更新`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前更新`
  return `${Math.floor(hours / 24)}天前更新`
}

function formatAirDate(value: string) {
  return value ? value.replaceAll('-', '.') : '日期未定'
}

function scheduleProgress(item: ViewerScheduleCard) {
  if (item.latestEpisodeLabel) {
    return `更新至 ${item.latestEpisodeLabel}`
  }
  if (!item.airDate) {
    return '开播时间未定'
  }
  const premiere = new Date(`${item.airDate}T00:00:00`)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  return !Number.isNaN(premiere.getTime()) && premiere > today ? '尚未开播' : '尚未放流'
}

function schedulePageProgress(item: ViewerScheduleCard) {
  if (!item.latestEpisodeLabel) {
    return scheduleProgress(item)
  }
  const updateAge = formatScheduleUpdateAge(item.latestEpisodeUpdatedAt)
  return updateAge ? `${updateAge}更新至${item.latestEpisodeLabel}` : `更新至 ${item.latestEpisodeLabel}`
}

function formatScheduleUpdateAge(value: number | null) {
  if (!value) return ''
  const elapsedSeconds = Math.max(Math.floor(relativeTimeNow.value / 1000) - value, 0)
  const hourSeconds = 60 * 60
  const daySeconds = 24 * hourSeconds
  if (elapsedSeconds < hourSeconds) return '刚刚'
  if (elapsedSeconds < daySeconds) return `${Math.floor(elapsedSeconds / hourSeconds)} 小时前`
  if (elapsedSeconds > 30 * daySeconds) return ''
  const days = Math.floor(elapsedSeconds / daySeconds)
  return days === 1 ? '昨天' : `${days} 天前`
}

function totalEpisodesText(value: number) {
  return value > 0 ? `全 ${value} 话` : '话数未定'
}

function followOverlayText(item: ViewerFollowedAnime) {
  if (!item.hasWatchProgress) return '尚未开始观看'
  if (item.watchCompleted && !item.caughtUp) return `已看完 ${item.watchedEpisodeLabel}，有新内容`
  if (item.watchCompleted) return `已追完 ${item.watchedEpisodeLabel}`
  return `看到 ${item.watchedEpisodeLabel || item.episodeLabel} ${item.progressPercent}%`
}

function progressWidth(value: number) {
  return `${Math.min(Math.max(value, 0), 100)}%`
}

function followUpdateText(item: ViewerFollowedAnime) {
  const total = item.totalEpisodes > 0 ? `全 ${item.totalEpisodes} 话` : '话数未定'
  const latest = item.latestEpisodeLabel ? `更新至 ${item.latestEpisodeLabel}` : '尚无成品'
  return `${total} / ${latest}`
}

function followListOverlayText(item: ViewerFollowedAnime) {
  if (item.hasWatchProgress && item.watchedEpisodeLabel) {
    return `看到 ${item.watchedEpisodeLabel}`
  }
  if (item.episodeLabel) {
    return `看到 ${item.episodeLabel}`
  }
  return '等待放流'
}

function followSubtitle(item: ViewerFollowedAnime) {
  if (item.episodeTitle) {
    return item.episodeTitle
  }
  return item.episodeLabel || '等待放流'
}

function historyListOverlayText(item: ViewerWatchHistoryItem) {
  return item.episodeLabel ? `看到 ${item.episodeLabel}` : '继续观看'
}

function historyUpdateText(item: ViewerWatchHistoryItem) {
  const total = item.totalEpisodes > 0 ? `全 ${item.totalEpisodes} 话` : '话数未定'
  const latest = item.latestEpisodeLabel ? `更新至 ${item.latestEpisodeLabel}` : '尚无成品'
  return `${total} / ${latest}`
}
</script>

<template>
  <main class="mobile-shell" :class="{ 'route-mode': routePage !== null, 'detail-mode': routePage === 'detail' }">
    <Transition :name="topbarTransitionName">
      <header v-if="topbarMode === 'main'" :key="topbarKey" class="app-topbar">
        <div class="brand-mini">
          <img :src="appIcon" alt="" />
          <span class="brand-name">{{ appName }}</span>
        </div>
        <form v-if="activeTab === 'home'" class="top-search" role="search" @submit.prevent="submitSearch">
          <input v-model="searchQuery" type="search" placeholder="搜索番剧" />
          <button class="search-icon-button" type="submit" :disabled="searchLoading" aria-label="搜索番剧">
            <i aria-hidden="true" v-html="searchIcon" />
          </button>
        </form>
      </header>

      <header v-else-if="topbarMode === 'sub'" :key="topbarKey" class="sub-topbar">
        <button type="button" aria-label="返回" @click="closeRoute">‹</button>
        <div>
          <span>{{ appName }}</span>
          <p class="page-title">{{ pageTitle }}</p>
        </div>
      </header>
    </Transition>

    <section
      ref="appPage"
      class="app-page"
      :class="{ 'transition-active': pageTransitionActive }"
      :style="pageTransitionStyle"
    >
      <Transition :name="pageTransitionName" @after-enter="finishPageTransition" @after-leave="finishPageTransition">
        <div :key="pageViewKey" class="page-view">
          <MobileAnimeDetailScreen
            v-if="routePage === 'detail'"
            :bangumi-id="detailAnimeId"
            :initial-media-id="detailMediaId"
            :initial-position="detailPosition"
            @back="closeAnimeDetail"
            @follow-changed="handleDetailFollowChanged"
          />

      <div v-else-if="routePage === 'search'" class="page-stack search-page">
        <form class="search-page-form" role="search" @submit.prevent="submitSearch">
          <input v-model="searchQuery" type="search" placeholder="搜索番剧" />
          <button type="submit" :disabled="searchLoading">搜索</button>
        </form>
        <div v-if="searchLoading" class="state-card">正在搜索...</div>
        <div v-else-if="searchError" class="state-card error">{{ searchError }}</div>
        <div v-else-if="searchResults.length === 0" class="state-card">
          没有找到“{{ searchPageQuery }}”
        </div>
        <div v-else class="result-list">
          <article
            v-for="item in searchResults"
            :key="item.bangumiId"
            class="list-row poster-row"
            role="button"
            tabindex="0"
            @click="openAnimeDetail(item.bangumiId)"
            @keydown.enter.prevent="openAnimeDetail(item.bangumiId)"
            @keydown.space.prevent="openAnimeDetail(item.bangumiId)"
          >
            <img
              v-if="imageAvailable(`search-${item.bangumiId}`, item.hasCover)"
              :src="animeCoverURL(item.bangumiId)"
              :alt="item.title"
              @error="markImageFailed(`search-${item.bangumiId}`)"
            />
            <div v-else class="poster-fallback small">{{ item.title.slice(0, 2) }}</div>
            <div>
              <p class="item-title">{{ item.title }}</p>
              <p>{{ formatAirDate(item.airDate) }} / {{ scheduleProgress(item) }}</p>
              <small>{{ totalEpisodesText(item.totalEpisodes) }}</small>
            </div>
          </article>
        </div>
      </div>

      <div v-else-if="routePage === 'settings'" class="page-stack settings-page">
        <section class="menu-list settings-list">
          <button type="button" @click="openServerAddressSettings">
            <span class="settings-option-copy">
              <strong>服务器地址</strong>
              <small>{{ props.apiBaseUrl }}</small>
            </span>
            <span class="chevron" aria-hidden="true">&gt;</span>
          </button>
          <button type="button" @click="openRoute('about')">
            <span class="settings-option-copy">
              <strong>关于 APP</strong>
              <small>{{ appName }} · v{{ appVersion }}</small>
            </span>
            <span class="chevron" aria-hidden="true">&gt;</span>
          </button>
        </section>
      </div>

      <div v-else-if="routePage === 'server-address'" class="page-stack settings-page">
        <section class="settings-editor-card">
          <div class="settings-editor-head">
            <span class="settings-kicker">SERVER</span>
            <h2>服务器地址</h2>
            <p>设置 APP 用于登录、加载番剧和播放视频的观看端地址。</p>
          </div>

          <form class="server-address-form" @submit.prevent="saveServerAddress">
            <label class="server-address-field">
              <span>地址</span>
              <input
                v-model.trim="serverAddressDraft"
                autocomplete="url"
                inputmode="url"
                placeholder="https://example.com/"
                required
                type="text"
                :disabled="serverAddressSaving"
              />
              <small>未填写协议时将自动使用 HTTPS。</small>
            </label>
            <p v-if="serverAddressError" class="settings-error" role="alert">{{ serverAddressError }}</p>
            <button class="save-settings-button" :disabled="serverAddressSaving" type="submit">
              {{ serverAddressSaving ? '正在应用' : '保存并应用' }}
            </button>
          </form>
        </section>
        <p class="settings-note">更换服务器后，当前登录状态会被安全清除，需要使用新服务器的账号重新登录。</p>
      </div>

      <div v-else-if="routePage === 'about'" class="page-stack about-page">
        <section class="about-hero-card">
          <div class="about-decoration about-decoration-one" aria-hidden="true" />
          <div class="about-decoration about-decoration-two" aria-hidden="true" />
          <div class="about-logo-shell">
            <img :src="appIcon" :alt="`${appName} Logo`" />
          </div>
          <span class="about-kicker">MOBILE VIEWER</span>
          <h1>{{ appName }}</h1>
          <p>随时随地，继续你的番剧时光。</p>
          <span class="about-version">Version {{ appVersion }}</span>
        </section>

        <section class="about-info-card">
          <div>
            <span>开发者</span>
            <strong>idiotbaka</strong>
          </div>
          <div>
            <span>APP 版本</span>
            <strong>v{{ appVersion }}</strong>
          </div>
        </section>

        <section class="about-update-card">
          <button
            type="button"
            :disabled="props.checkingAppUpdate"
            @click="emit('check-app-update')"
          >
            <span class="about-update-icon" aria-hidden="true">
              <svg viewBox="0 0 24 24">
                <path d="M20 12a8 8 0 1 1-2.35-5.65M20 4v5h-5" />
              </svg>
            </span>
            <span class="about-update-copy">
              <strong>检查新版本</strong>
              <small>
                {{ props.checkingAppUpdate ? '正在检查更新...' : (props.appUpdateCheckMessage || '查看是否有可用的新版本') }}
              </small>
            </span>
            <span class="chevron" aria-hidden="true">&gt;</span>
          </button>
        </section>
      </div>

      <div v-else-if="routePage === 'follows'" class="page-stack">
        <div v-if="profileLoading" class="state-card">正在加载追番...</div>
        <div v-else-if="profileError" class="state-card error">{{ profileError }}</div>
        <div v-else-if="follows.length === 0" class="state-card">还没有追番</div>
        <div v-else class="result-list">
          <article
            v-for="item in follows"
            :key="item.bangumiId"
            class="list-row media-row"
            role="button"
            tabindex="0"
            @click="openAnimeDetail(item.bangumiId, item.mediaId, item.watchCompleted ? 0 : item.positionSeconds)"
            @keydown.enter.prevent="openAnimeDetail(item.bangumiId, item.mediaId, item.watchCompleted ? 0 : item.positionSeconds)"
            @keydown.space.prevent="openAnimeDetail(item.bangumiId, item.mediaId, item.watchCompleted ? 0 : item.positionSeconds)"
          >
            <div class="list-cover">
              <img
                v-if="item.mediaId > 0 && imageAvailable(`follow-list-${item.bangumiId}`, item.hasCover)"
                :src="followCoverURL(item)"
                :alt="`${item.animeTitle} ${item.episodeLabel}`"
                @error="markImageFailed(`follow-list-${item.bangumiId}`)"
              />
              <div v-else class="media-fallback">{{ item.episodeLabel || item.animeTitle.slice(0, 2) }}</div>
              <span>{{ followListOverlayText(item) }}</span>
              <div class="list-progress">
                <i :style="{ width: progressWidth(item.hasWatchProgress ? (item.watchCompleted ? 100 : item.progressPercent) : 0) }" />
              </div>
            </div>
            <div>
              <p class="item-title">{{ item.animeTitle }}</p>
              <p>{{ followSubtitle(item) }}</p>
              <small>{{ followUpdateText(item) }}</small>
            </div>
          </article>
        </div>
      </div>

      <div v-else-if="routePage === 'history'" class="page-stack">
        <div v-if="profileLoading" class="state-card">正在加载历史...</div>
        <div v-else-if="profileError" class="state-card error">{{ profileError }}</div>
        <div v-else-if="history.length === 0" class="state-card">还没有观看历史</div>
        <div v-else class="result-list">
          <article
            v-for="item in history"
            :key="`${item.bangumiId}-${item.mediaId}`"
            class="list-row media-row"
            role="button"
            tabindex="0"
            @click="openAnimeDetail(item.bangumiId, item.mediaId, item.completed ? 0 : item.positionSeconds)"
            @keydown.enter.prevent="openAnimeDetail(item.bangumiId, item.mediaId, item.completed ? 0 : item.positionSeconds)"
            @keydown.space.prevent="openAnimeDetail(item.bangumiId, item.mediaId, item.completed ? 0 : item.positionSeconds)"
          >
            <div class="list-cover">
              <img
                v-if="imageAvailable(`history-${item.mediaId}`, item.hasCover)"
                :src="mediaCoverURL(item)"
                :alt="`${item.animeTitle} ${item.episodeLabel}`"
                @error="markImageFailed(`history-${item.mediaId}`)"
              />
              <div v-else class="media-fallback">{{ item.episodeLabel }}</div>
              <span>{{ historyListOverlayText(item) }}</span>
              <div class="list-progress">
                <i :style="{ width: progressWidth(item.completed ? 100 : item.progressPercent) }" />
              </div>
            </div>
            <div>
              <p class="item-title">{{ item.animeTitle }}</p>
              <p>{{ item.episodeTitle || item.episodeLabel }}</p>
              <small>{{ historyUpdateText(item) }}</small>
            </div>
          </article>
        </div>
      </div>

      <div v-else-if="activeTab === 'home'" class="page-stack home-page">
        <section class="content-section follow-section">
          <div class="section-head">
            <p class="section-title">我的追番</p>
            <button class="section-link" type="button" @click="openRoute('follows')">全部 &gt;</button>
          </div>
          <div v-if="homeLoading" class="follow-rail skeleton-follow-rail" aria-label="正在加载追番">
            <article v-for="index in 2" :key="index" class="continue-card skeleton-continue-card">
              <div class="episode-cover skeleton-block" />
              <div class="skeleton-line skeleton-block" />
              <div class="skeleton-line short skeleton-block" />
            </article>
          </div>
          <div v-else-if="homeFollows.length === 0" class="state-card">暂无待观看的追番</div>
          <div v-else class="follow-rail">
            <article
              v-for="item in homeFollows"
              :key="item.bangumiId"
              class="continue-card"
              role="button"
              tabindex="0"
              @click="openAnimeDetail(item.bangumiId, item.mediaId, item.watchCompleted ? 0 : item.positionSeconds)"
              @keydown.enter.prevent="openAnimeDetail(item.bangumiId, item.mediaId, item.watchCompleted ? 0 : item.positionSeconds)"
              @keydown.space.prevent="openAnimeDetail(item.bangumiId, item.mediaId, item.watchCompleted ? 0 : item.positionSeconds)"
            >
              <div class="episode-cover">
                <img
                  v-if="item.mediaId > 0 && imageAvailable(`home-follow-${item.bangumiId}`, item.hasCover)"
                  :src="followCoverURL(item)"
                  :alt="`${item.animeTitle} ${item.episodeLabel}`"
                  @error="markImageFailed(`home-follow-${item.bangumiId}`)"
                />
                <div v-else class="media-fallback">{{ item.episodeLabel || item.animeTitle.slice(0, 2) }}</div>
                <div class="progress-overlay">
                  <span>{{ followOverlayText(item) }}</span>
                  <div><i :style="{ width: `${item.hasWatchProgress ? item.progressPercent : 0}%` }" /></div>
                </div>
              </div>
              <p class="item-title">{{ item.animeTitle }}</p>
              <p>{{ followSubtitle(item) }}</p>
              <small>{{ followUpdateText(item) }}</small>
            </article>
          </div>
        </section>

        <section class="content-section">
          <div class="section-head">
            <p class="section-title">最近更新</p>
          </div>
          <div v-if="homeLoading" class="poster-grid" aria-label="正在加载最近更新">
            <article v-for="index in 6" :key="index" class="poster-card skeleton-card">
              <div class="poster-cover skeleton-block" />
              <div class="skeleton-line skeleton-block" />
              <div class="skeleton-line short skeleton-block" />
            </article>
          </div>
          <div v-else-if="homeError" class="state-card error">{{ homeError }}</div>
          <div v-else class="poster-grid">
            <article
              v-for="item in recentItems"
              :key="item.bangumiId"
              class="poster-card"
              role="button"
              tabindex="0"
              @click="openAnimeDetail(item.bangumiId)"
              @keydown.enter.prevent="openAnimeDetail(item.bangumiId)"
              @keydown.space.prevent="openAnimeDetail(item.bangumiId)"
            >
              <div class="poster-cover">
                <img
                  v-if="imageAvailable(`recent-${item.bangumiId}`, item.hasCover)"
                  :src="animeCoverURL(item.bangumiId)"
                  :alt="item.title"
                  @error="markImageFailed(`recent-${item.bangumiId}`)"
                />
                <div v-else class="poster-fallback">{{ item.title.slice(0, 2) }}</div>
                <span class="time-pill">{{ formatUpdatedAt(item.updatedAt) }}</span>
              </div>
              <p class="item-title">{{ item.title }}</p>
              <p>{{ updateText(item) }}</p>
            </article>
          </div>
        </section>

        <section class="content-section">
          <div class="section-head">
            <p class="section-title">热播推荐</p>
          </div>
          <div v-if="homeLoading" class="poster-grid" aria-label="正在加载热播推荐">
            <article v-for="index in 6" :key="index" class="poster-card skeleton-card">
              <div class="poster-cover skeleton-block" />
              <div class="skeleton-line skeleton-block" />
              <div class="skeleton-line short skeleton-block" />
            </article>
          </div>
          <div v-else class="poster-grid">
            <article
              v-for="item in hotItems"
              :key="item.bangumiId"
              class="poster-card"
              role="button"
              tabindex="0"
              @click="openAnimeDetail(item.bangumiId)"
              @keydown.enter.prevent="openAnimeDetail(item.bangumiId)"
              @keydown.space.prevent="openAnimeDetail(item.bangumiId)"
            >
              <div class="poster-cover">
                <img
                  v-if="imageAvailable(`hot-${item.bangumiId}`, item.hasCover)"
                  :src="animeCoverURL(item.bangumiId)"
                  :alt="item.title"
                  @error="markImageFailed(`hot-${item.bangumiId}`)"
                />
                <div v-else class="poster-fallback">{{ item.title.slice(0, 2) }}</div>
                <span class="score-overlay">{{ ratingText(item.ratingScore) }}</span>
              </div>
              <p class="item-title">{{ item.title }}</p>
              <p>{{ updateText(item) }}</p>
            </article>
          </div>
        </section>
      </div>

      <div v-else-if="activeTab === 'schedule'" class="page-stack schedule-page">
        <section class="schedule-sticky">
          <div class="schedule-toolbar">
            <div>
              <p class="toolbar-title">{{ schedule?.seasonLabel || `${seasonYear}年${seasonMonth}月` }}</p>
              <p class="toolbar-subtitle">{{ selectedDay.label }} · {{ scheduleItems.length }} 部</p>
            </div>
            <div class="season-actions">
              <button type="button" :disabled="scheduleLoading" @click="changeSeason(-1)">上一季</button>
              <button type="button" :disabled="scheduleLoading" @click="changeSeason(1)">下一季</button>
            </div>
          </div>

          <div class="weekday-scroll" role="tablist" aria-label="按星期筛选">
            <button
              v-for="day in weekdays"
              :key="day.value"
              :class="{ active: selectedWeekday === day.value }"
              type="button"
              role="tab"
              :aria-selected="selectedWeekday === day.value"
              @click="selectedWeekday = day.value"
            >
              {{ day.label }}
            </button>
          </div>
        </section>

        <div v-if="scheduleLoading" class="result-list schedule-list" aria-label="正在加载时间表">
          <article v-for="index in 6" :key="index" class="list-row poster-row skeleton-list-row">
            <div class="poster-fallback small skeleton-block" />
            <div>
              <div class="skeleton-line skeleton-block" />
              <div class="skeleton-line short skeleton-block" />
              <div class="skeleton-line tiny skeleton-block" />
            </div>
          </article>
        </div>
        <div v-else-if="scheduleError" class="state-card error">{{ scheduleError }}</div>
        <div v-else-if="scheduleItems.length === 0" class="state-card">这一天暂时没有番剧</div>
        <div v-else class="result-list schedule-list">
          <article
            v-for="item in scheduleItems"
            :key="item.bangumiId"
            class="list-row poster-row"
            role="button"
            tabindex="0"
            @click="openAnimeDetail(item.bangumiId)"
            @keydown.enter.prevent="openAnimeDetail(item.bangumiId)"
            @keydown.space.prevent="openAnimeDetail(item.bangumiId)"
          >
            <img
              v-if="imageAvailable(`schedule-${item.bangumiId}`, item.hasCover)"
              :src="animeCoverURL(item.bangumiId)"
              :alt="item.title"
              @error="markImageFailed(`schedule-${item.bangumiId}`)"
            />
            <div v-else class="poster-fallback small">{{ item.title.slice(0, 2) }}</div>
            <div>
              <p class="item-title">{{ item.title }}</p>
              <p>{{ formatAirDate(item.airDate) }} / {{ totalEpisodesText(item.totalEpisodes) }}</p>
              <small>{{ schedulePageProgress(item) }}</small>
            </div>
          </article>
        </div>
      </div>

      <div v-else-if="activeTab === 'library'" class="page-stack library-page">
        <section class="library-filter-card">
          <div class="library-filter-head">
            <div>
              <p class="toolbar-title">番剧图书馆</p>
              <p class="toolbar-subtitle">共 {{ library.total }} 部 · 已选 {{ selectedLibraryTagCount }} 个标签</p>
            </div>
            <button type="button" :disabled="selectedLibraryTagCount === 0 || libraryLoading" @click="resetLibraryFilters">
              清除
            </button>
          </div>

          <div v-if="libraryFiltersLoading" class="library-filter-list skeleton-filter-list" aria-label="正在加载筛选标签">
            <section v-for="index in 3" :key="index" class="library-filter-row">
              <div class="skeleton-filter-title skeleton-block" />
              <div class="tag-rail">
                <div v-for="tagIndex in 4" :key="tagIndex" class="skeleton-tag skeleton-block" />
              </div>
            </section>
          </div>
          <div v-else-if="libraryFiltersError" class="state-card error compact-state">
            {{ libraryFiltersError }}
            <button type="button" @click="loadLibraryFilters">重试</button>
          </div>
          <div v-else-if="libraryDimensions.length === 0" class="state-card compact-state">暂无筛选标签</div>
          <div v-else class="library-filter-list">
            <section v-for="dimension in libraryDimensions" :key="dimension.id" class="library-filter-row">
              <p class="filter-title">{{ dimension.name }}</p>
              <div class="tag-rail">
                <button
                  v-for="tag in dimension.tags"
                  :key="tag"
                  type="button"
                  :class="{ active: isLibraryTagSelected(dimension.id, tag) }"
                  :disabled="libraryLoading"
                  @click="toggleLibraryTag(dimension.id, tag)"
                >
                  {{ tag }}
                </button>
              </div>
            </section>
          </div>
        </section>

        <section class="library-results">
          <div v-if="libraryLoading" class="poster-grid">
            <article v-for="index in 9" :key="index" class="library-card skeleton-card">
              <div class="library-cover skeleton-block" />
              <div class="skeleton-line skeleton-block" />
              <div class="skeleton-line short skeleton-block" />
            </article>
          </div>

          <div v-else-if="libraryError" class="state-card error">
            {{ libraryError }}
            <button type="button" @click="loadLibrary">重新加载</button>
          </div>

          <div v-else-if="library.items.length === 0" class="state-card">没有找到符合条件的番剧</div>

          <template v-else>
            <div class="poster-grid library-grid">
              <article
                v-for="item in visibleLibraryItems"
                :key="item.bangumiId"
                class="library-card"
                role="button"
                tabindex="0"
                @click="openAnimeDetail(item.bangumiId)"
                @keydown.enter.prevent="openAnimeDetail(item.bangumiId)"
                @keydown.space.prevent="openAnimeDetail(item.bangumiId)"
              >
                <div class="library-cover">
                  <img
                    v-if="imageAvailable(`library-${item.bangumiId}`, item.hasCover)"
                    :src="animeCoverURL(item.bangumiId)"
                    :alt="item.title"
                    loading="lazy"
                    @error="markImageFailed(`library-${item.bangumiId}`)"
                  />
                  <div v-else class="poster-fallback">{{ item.title.slice(0, 2) }}</div>
                  <span class="episode-total">{{ totalEpisodesText(item.totalEpisodes) }}</span>
                  <span class="library-progress">{{ scheduleProgress(item) }}</span>
                </div>
                <p class="item-title">{{ item.title }}</p>
                <p>{{ formatAirDate(item.airDate) }}</p>
              </article>
            </div>

            <button v-if="hasMoreLibraryItems" class="load-more-button" type="button" @click="loadMoreLibraryItems">
              加载更多
            </button>
            <div v-else class="list-end">已显示全部 {{ library.total }} 部</div>
          </template>
        </section>
      </div>

          <div v-else class="page-stack profile-page">
            <section class="profile-head">
              <img :src="appIcon" alt="" />
              <div>
                <p class="profile-name">{{ props.user.username }}</p>
                <p class="profile-subtitle">{{ appName }}</p>
              </div>
            </section>

            <div v-if="profileError" class="state-card error">{{ profileError }}</div>

            <section class="menu-list">
              <button type="button" @click="openRoute('settings')">
                <span>系统设置</span>
                <span class="chevron" aria-hidden="true">&gt;</span>
              </button>
              <button type="button" @click="openRoute('follows')">
                <span>我的追番</span>
                <span class="menu-tail">
                  <small>{{ follows.length || home.myFollows.length }} 部</small>
                  <span class="chevron" aria-hidden="true">&gt;</span>
                </span>
              </button>
              <button type="button" @click="openRoute('history')">
                <span>观看历史</span>
                <span class="menu-tail">
                  <small>{{ history.length }} 条</small>
                  <span class="chevron" aria-hidden="true">&gt;</span>
                </span>
              </button>
              <button class="logout-menu" :disabled="loading" type="button" @click="emit('logout')">
                <span>{{ loading ? '退出中' : '退出登录' }}</span>
              </button>
            </section>
          </div>
        </div>
      </Transition>
    </section>

    <nav v-if="routePage === null" class="bottom-nav" aria-label="底部导航">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        :class="{ active: activeTab === tab.key }"
        type="button"
        @click="showTab(tab.key)"
      >
        <i aria-hidden="true" v-html="tab.icon" />
        <span>{{ tab.label }}</span>
      </button>
    </nav>
  </main>
</template>

<style scoped>
.mobile-shell {
  --shell-topbar-height: calc(62px + env(safe-area-inset-top));
  --mobile-topbar-height: var(--shell-topbar-height);
  --page-slide-duration: 260ms;
  --page-slide-easing: cubic-bezier(0.2, 0.82, 0.2, 1);
  --page-slide-fade-duration: 220ms;
  --page-slide-forward-enter-x: 100%;
  --page-slide-forward-leave-x: -100%;
  --page-slide-back-enter-x: -100%;
  --page-slide-back-leave-x: 100%;
  min-height: 100vh;
  min-height: 100dvh;
  overflow-x: hidden;
  padding-top: var(--mobile-topbar-height);
  color: var(--ink-900);
  background:
    linear-gradient(180deg, rgba(255, 244, 248, 0.88), rgba(247, 250, 255, 0.96) 35%, #f6f7fb 100%),
    #f6f7fb;
}

.app-topbar,
.sub-topbar {
  position: fixed;
  top: 0;
  right: 0;
  left: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  min-height: var(--shell-topbar-height);
  padding: max(12px, env(safe-area-inset-top)) 14px 10px;
  background: rgba(255, 255, 255, 0.92);
  border-bottom: 1px solid rgba(32, 40, 62, 0.06);
  box-shadow: 0 6px 18px rgba(32, 40, 62, 0.04);
  backdrop-filter: blur(14px);
}

.topbar-slide-forward-enter-active,
.topbar-slide-forward-leave-active,
.topbar-slide-back-enter-active,
.topbar-slide-back-leave-active {
  transition:
    transform var(--page-slide-duration) var(--page-slide-easing),
    opacity var(--page-slide-fade-duration) var(--ease-out);
  will-change: transform, opacity;
}

.topbar-slide-forward-enter-active,
.topbar-slide-back-enter-active {
  z-index: 22;
}

.topbar-slide-forward-leave-active,
.topbar-slide-back-leave-active {
  z-index: 21;
  pointer-events: none;
}

.topbar-slide-forward-enter-from {
  opacity: 0.72;
  transform: translate3d(var(--page-slide-forward-enter-x), 0, 0);
}

.topbar-slide-forward-leave-to {
  opacity: 0.5;
  transform: translate3d(var(--page-slide-forward-leave-x), 0, 0);
}

.topbar-slide-back-enter-from {
  opacity: 0.72;
  transform: translate3d(var(--page-slide-back-enter-x), 0, 0);
}

.topbar-slide-back-leave-to {
  opacity: 0.5;
  transform: translate3d(var(--page-slide-back-leave-x), 0, 0);
}

.brand-mini {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}

.brand-mini img {
  width: 30px;
  height: 30px;
  border-radius: 8px;
}

.brand-name {
  overflow: hidden;
  font-size: 17px;
  line-height: 1.1;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.top-search,
.search-page-form {
  flex: 1;
  min-width: 0;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 42px;
  align-items: center;
  height: 38px;
  padding: 3px;
  background: #f3f5fa;
  border: 1px solid rgba(32, 40, 62, 0.08);
  border-radius: 8px;
}

.search-page-form {
  grid-template-columns: minmax(0, 1fr) 58px;
  height: 40px;
}

.search-page .search-page-form {
  position: fixed;
  top: calc(var(--mobile-topbar-height) + 10px);
  left: 50%;
  z-index: 18;
  width: min(calc(100% - 28px), 492px);
  transform: translateX(-50%);
  box-shadow: 0 10px 24px rgba(32, 40, 62, 0.08);
}

.top-search input,
.search-page-form input {
  min-width: 0;
  height: 100%;
  padding: 0 10px;
  font-size: 13px;
}

.top-search button,
.search-page-form button,
.season-actions button,
.section-head button {
  height: 32px;
  padding: 0 10px;
  color: #ffffff;
  font-size: 12px;
  background: var(--pink-600);
  border-radius: 7px;
  transition: transform 120ms var(--ease-soft), filter 120ms var(--ease-soft);
}

.top-search .search-icon-button {
  width: 34px;
  padding: 0;
  display: grid;
  place-items: center;
  color: var(--pink-600);
  background: transparent;
  transform: translateX(2px);
}

.search-icon-button i {
  width: 18px;
  height: 18px;
  display: grid;
  place-items: center;
}

.search-icon-button :deep(svg) {
  width: 100%;
  height: 100%;
  display: block;
}

.search-icon-button :deep(path) {
  fill: currentColor;
}

.search-page-form button {
  min-width: 54px;
  padding: 0 12px;
  white-space: nowrap;
}

.top-search button:active:not(:disabled),
.search-page-form button:active:not(:disabled),
.season-actions button:active:not(:disabled),
.section-head button:active:not(:disabled),
.bottom-nav button:active,
.menu-list button:active,
.sub-topbar button:active {
  transform: scale(0.96);
}

.sub-topbar button {
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  color: var(--ink-900);
  font-size: 30px;
  line-height: 1;
  background: #f3f5fa;
  border-radius: 8px;
}

.sub-topbar div {
  min-width: 0;
}

.sub-topbar span {
  display: block;
  color: var(--ink-400);
  font-size: 11px;
}

.page-title {
  margin-top: 1px;
  font-size: 20px;
  line-height: 1.15;
}

.app-page {
  --transition-page-top: 0px;
  --transition-page-left: 0px;
  --transition-page-width: 100vw;
  --transition-page-min-height: 0px;
  position: relative;
  width: min(100%, 520px);
  min-height: calc(100dvh - 132px);
  overflow-x: hidden;
  margin: 0 auto;
  padding: 12px 14px calc(86px + env(safe-area-inset-bottom));
}

.page-view {
  min-width: 0;
}

.app-page.transition-active {
  min-height: max(calc(100dvh - 132px), var(--transition-page-min-height));
}

.page-slide-forward-enter-active,
.page-slide-forward-leave-active,
.page-slide-back-enter-active,
.page-slide-back-leave-active {
  transition:
    transform var(--page-slide-duration) var(--page-slide-easing),
    opacity var(--page-slide-fade-duration) var(--ease-out);
  will-change: transform, opacity;
}

.page-slide-forward-leave-active,
.page-slide-back-leave-active {
  position: fixed;
  top: var(--transition-page-top);
  left: var(--transition-page-left);
  z-index: 1;
  width: var(--transition-page-width);
  pointer-events: none;
}

.page-slide-forward-enter-active,
.page-slide-back-enter-active {
  position: relative;
  z-index: 2;
}

/*
 * Entering and leaving surfaces must travel complementary full widths.
 * Short parallax distances make the transparent page layers overlap and
 * leave the previous view visible until Vue removes its transition node.
 */
.page-slide-forward-enter-from {
  opacity: 0.72;
  transform: translate3d(var(--page-slide-forward-enter-x), 0, 0);
}

.page-slide-forward-leave-to {
  opacity: 0.5;
  transform: translate3d(var(--page-slide-forward-leave-x), 0, 0);
}

.page-slide-back-enter-from {
  opacity: 0.72;
  transform: translate3d(var(--page-slide-back-enter-x), 0, 0);
}

.page-slide-back-leave-to {
  opacity: 0.5;
  transform: translate3d(var(--page-slide-back-leave-x), 0, 0);
}

.route-mode .app-page {
  padding-bottom: 24px;
}

.detail-mode {
  --mobile-topbar-height: 0px;
  overflow-x: visible;
  padding-top: 0;
  background: #f6f7fb;
}

.detail-mode .app-page {
  width: 100%;
  min-height: 100dvh;
  overflow-x: visible;
  padding: 0;
}

@supports (overflow: clip) {
  .detail-mode,
  .detail-mode .app-page {
    overflow-x: clip;
  }
}

.page-stack {
  display: grid;
  gap: 18px;
}

.home-page {
  gap: 22px;
}

.search-page {
  padding-top: 58px;
}

.schedule-page {
  padding-top: 158px;
}

.library-page {
  gap: 14px;
}

.content-section,
.schedule-toolbar,
.library-filter-card,
.profile-head,
.menu-list,
.settings-editor-card,
.about-hero-card,
.about-info-card,
.about-update-card,
.state-card,
.list-row {
  background: #ffffff;
  border: 1px solid rgba(32, 40, 62, 0.07);
  border-radius: 8px;
  box-shadow: 0 10px 24px rgba(32, 40, 62, 0.06);
}

.content-section {
  min-width: 0;
  padding: 14px;
}

.follow-section {
  padding-right: 0;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  padding-right: 14px;
}

.section-title {
  font-size: 19px;
  line-height: 1.2;
}

.section-head button {
  color: var(--pink-600);
  background: var(--pink-50);
  border: 1px solid var(--line-soft);
}

.section-head .section-link {
  height: auto;
  padding: 4px 0;
  color: var(--pink-600);
  font-size: 13px;
  background: transparent;
  border: 0;
  border-radius: 0;
}

.follow-rail {
  display: grid;
  grid-auto-flow: column;
  grid-auto-columns: min(72vw, 278px);
  gap: 12px;
  overflow-x: auto;
  padding: 1px 14px 4px 0;
  scroll-padding-left: 0;
  scroll-snap-type: x proximity;
}

.follow-rail::-webkit-scrollbar,
.weekday-scroll::-webkit-scrollbar {
  display: none;
}

.continue-card {
  min-width: 0;
  outline: 0;
  scroll-snap-align: start;
}

.continue-card:active,
.poster-card:active,
.list-row:active,
.library-card:active {
  transform: scale(0.99);
}

.continue-card:focus-visible,
.poster-card:focus-visible,
.list-row:focus-visible,
.library-card:focus-visible {
  outline: 2px solid rgba(238, 63, 134, 0.34);
  outline-offset: 3px;
}

.episode-cover {
  position: relative;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  background: #eef2f7;
  border-radius: 8px;
}

.episode-cover img,
.media-row img,
.media-fallback {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.progress-overlay {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  padding: 24px 10px 9px;
  color: #ffffff;
  background: linear-gradient(to top, rgba(20, 26, 43, 0.88), rgba(20, 26, 43, 0));
  text-shadow: 0 1px 4px rgba(0, 0, 0, 0.52);
}

.progress-overlay span {
  display: block;
  overflow: hidden;
  font-size: 12px;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.progress-overlay div {
  height: 4px;
  margin-top: 7px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.36);
  border-radius: 999px;
}

.progress-overlay i {
  display: block;
  height: 100%;
  background: linear-gradient(90deg, var(--pink-500), var(--cyan-400));
}

.item-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.continue-card .item-title {
  margin-top: 8px;
  font-size: 15px;
}

.continue-card p,
.poster-card p,
.list-row p,
.list-row small {
  display: block;
  overflow: hidden;
  color: var(--ink-600);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.continue-card p {
  margin-top: 2px;
  font-size: 12px;
}

.continue-card small {
  display: block;
  margin-top: 3px;
  overflow: hidden;
  color: var(--ink-400);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.poster-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 13px 10px;
}

.poster-card {
  min-width: 0;
}

.poster-cover {
  position: relative;
  aspect-ratio: 2 / 3;
  overflow: hidden;
  background: #eef2f7;
  border-radius: 8px;
}

.poster-card img,
.poster-fallback {
  width: 100%;
  aspect-ratio: 2 / 3;
  object-fit: cover;
  background: #eef2f7;
  border-radius: 8px;
}

.poster-cover img,
.poster-cover .poster-fallback {
  height: 100%;
  border-radius: 0;
}

.time-pill {
  position: absolute;
  right: 5px;
  bottom: 5px;
  max-width: calc(100% - 10px);
  padding: 4px 6px;
  overflow: hidden;
  color: #ffffff;
  font-size: 10px;
  line-height: 1.1;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: rgba(32, 40, 62, 0.78);
  border-radius: 4px;
}

.score-overlay {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  display: flex;
  justify-content: flex-end;
  padding: 24px 6px 6px;
  color: #ffffff;
  font-size: 16px;
  line-height: 1;
  text-align: right;
  background: linear-gradient(to top, rgba(32, 40, 62, 0.82), rgba(32, 40, 62, 0));
  text-shadow: 0 1px 4px rgba(0, 0, 0, 0.55);
}

.poster-card .item-title {
  margin-top: 7px;
  font-size: 13px;
  line-height: 1.25;
}

.poster-card p {
  margin-top: 3px;
  font-size: 11px;
}

.poster-fallback,
.media-fallback {
  display: grid;
  place-items: center;
  padding: 8px;
  color: var(--pink-600);
  font-size: 12px;
  text-align: center;
  background: linear-gradient(145deg, var(--pink-50), var(--cyan-50));
}

.poster-fallback.small {
  width: 62px;
  height: 88px;
}

.state-card {
  min-height: 104px;
  display: grid;
  place-items: center;
  padding: 18px;
  color: var(--ink-400);
  font-size: 14px;
  text-align: center;
  box-shadow: none;
}

.state-card.error {
  color: var(--pink-600);
}

.state-card button {
  min-height: 32px;
  margin-top: 8px;
  padding: 0 12px;
  color: var(--pink-600);
  font-size: 12px;
  background: var(--pink-50);
  border: 1px solid var(--line-soft);
  border-radius: 7px;
}

.follow-section .state-card {
  border-color: transparent;
}

.schedule-sticky {
  position: fixed;
  top: calc(var(--mobile-topbar-height) + 10px);
  left: 50%;
  z-index: 18;
  width: min(calc(100% - 28px), 492px);
  display: grid;
  gap: 9px;
  transform: translateX(-50%);
  background: transparent;
}

.schedule-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px;
  background: #ffffff;
  border: 1px solid rgba(32, 40, 62, 0.07);
  border-radius: 8px;
  box-shadow: 0 10px 24px rgba(32, 40, 62, 0.06);
}

.toolbar-title {
  font-size: 22px;
  line-height: 1.2;
}

.toolbar-subtitle {
  margin-top: 3px;
  color: var(--ink-400);
  font-size: 13px;
}

.season-actions {
  display: flex;
  gap: 8px;
}

.season-actions button {
  color: var(--ink-700);
  background: #f3f5fa;
}

.weekday-scroll {
  display: flex;
  gap: 8px;
  max-width: 100%;
  overflow-x: auto;
  padding: 10px;
  background: #ffffff;
  border: 1px solid rgba(32, 40, 62, 0.07);
  border-radius: 8px;
  box-shadow: 0 10px 24px rgba(32, 40, 62, 0.06);
}

.weekday-scroll button {
  flex: 0 0 auto;
  min-width: 54px;
  height: 38px;
  padding: 0 12px;
  color: var(--ink-600);
  font-size: 13px;
  background: #ffffff;
  border: 1px solid rgba(32, 40, 62, 0.08);
  border-radius: 8px;
}

.weekday-scroll button.active {
  color: #ffffff;
  background: var(--ink-900);
}

.library-filter-card {
  padding: 14px;
}

.library-filter-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.library-filter-head button {
  min-width: 52px;
  height: 32px;
  padding: 0;
  color: var(--pink-600);
  font-size: 12px;
  background: transparent;
  border: 0;
  border-radius: 0;
}

.library-filter-head button:disabled {
  color: var(--ink-300);
  background: transparent;
}

.compact-state {
  min-height: 74px;
  margin-top: 12px;
}

.library-filter-list {
  display: grid;
  gap: 13px;
  margin-top: 14px;
}

.library-filter-row {
  min-width: 0;
}

.filter-title {
  margin-bottom: 8px;
  color: var(--ink-700);
  font-size: 13px;
}

.tag-rail {
  display: flex;
  gap: 8px;
  max-width: 100%;
  overflow-x: auto;
  padding-bottom: 2px;
}

.tag-rail::-webkit-scrollbar {
  display: none;
}

.tag-rail button {
  flex: 0 0 auto;
  min-height: 34px;
  padding: 0 12px;
  color: var(--ink-600);
  font-size: 12px;
  background: #f5f7fb;
  border: 1px solid rgba(32, 40, 62, 0.08);
  border-radius: 8px;
}

.tag-rail button.active {
  color: #ffffff;
  background: var(--ink-900);
  border-color: var(--ink-900);
}

.library-results {
  min-width: 0;
}

.library-card {
  min-width: 0;
}

.library-cover {
  position: relative;
  aspect-ratio: 2 / 3;
  overflow: hidden;
  background: #eef2f7;
  border-radius: 8px;
}

.library-cover img,
.library-cover .poster-fallback {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: 0;
}

.episode-total {
  position: absolute;
  top: 6px;
  right: 6px;
  max-width: calc(100% - 12px);
  padding: 3px 6px;
  overflow: hidden;
  color: var(--ink-700);
  font-size: 10px;
  line-height: 1.1;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: rgba(255, 255, 255, 0.88);
  border-radius: 4px;
}

.library-progress {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  padding: 22px 6px 6px;
  overflow: hidden;
  color: #ffffff;
  font-size: 10px;
  line-height: 1.1;
  text-align: right;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: linear-gradient(to top, rgba(32, 40, 62, 0.82), rgba(32, 40, 62, 0));
  text-shadow: 0 1px 4px rgba(0, 0, 0, 0.55);
}

.library-card .item-title {
  margin-top: 7px;
  overflow: hidden;
  font-size: 13px;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.library-card p {
  margin-top: 3px;
  overflow: hidden;
  color: var(--ink-600);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.load-more-button {
  width: 100%;
  min-height: 44px;
  margin-top: 14px;
  color: var(--pink-600);
  font-size: 13px;
  background: #ffffff;
  border: 1px solid rgba(32, 40, 62, 0.07);
  border-radius: 8px;
  box-shadow: 0 10px 24px rgba(32, 40, 62, 0.05);
}

.list-end {
  padding: 14px 0 4px;
  color: var(--ink-400);
  font-size: 12px;
  text-align: center;
}

.skeleton-block {
  position: relative;
  overflow: hidden;
  background: #f0f2f7;
}

.skeleton-block::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(100deg, transparent 18%, rgba(255, 255, 255, 0.82) 45%, transparent 72%);
  animation: mobile-skeleton 1.1s ease-in-out infinite;
}

.skeleton-line {
  width: 86%;
  height: 12px;
  margin-top: 9px;
  border-radius: 999px;
}

.skeleton-line.short {
  width: 58%;
  height: 9px;
  margin-top: 6px;
}

.skeleton-line.tiny {
  width: 46%;
  height: 9px;
  margin-top: 7px;
}

.skeleton-card .poster-cover,
.skeleton-card .library-cover,
.skeleton-continue-card .episode-cover,
.skeleton-list-row .poster-fallback {
  background: #f0f2f7;
}

.skeleton-follow-rail {
  min-height: 166px;
}

.skeleton-continue-card {
  pointer-events: none;
}

.skeleton-list-row {
  pointer-events: none;
}

.skeleton-list-row .skeleton-line {
  margin-top: 0;
}

.skeleton-list-row .skeleton-line.short,
.skeleton-list-row .skeleton-line.tiny {
  margin-top: 8px;
}

.skeleton-filter-list {
  gap: 14px;
}

.skeleton-filter-title {
  width: 68px;
  height: 13px;
  margin-bottom: 10px;
  border-radius: 999px;
}

.skeleton-tag {
  flex: 0 0 auto;
  width: 68px;
  height: 34px;
  border-radius: 8px;
}

.result-list {
  display: grid;
  gap: 10px;
}

.list-row {
  display: grid;
  gap: 12px;
  align-items: center;
  min-width: 0;
  padding: 10px;
  box-shadow: none;
}

.poster-row {
  grid-template-columns: 62px minmax(0, 1fr);
}

.poster-row img {
  width: 62px;
  height: 88px;
  object-fit: cover;
  border-radius: 8px;
}

.media-row {
  grid-template-columns: 132px minmax(0, 1fr);
}

.list-cover {
  position: relative;
  width: 132px;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  background: #eef2f7;
  border-radius: 8px;
}

.list-cover img,
.list-cover .media-fallback {
  width: 100%;
  height: 100%;
  border-radius: 0;
}

.list-cover span {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  z-index: 1;
  padding: 20px 8px 15px;
  overflow: hidden;
  color: #ffffff;
  font-size: 12px;
  line-height: 1.1;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: linear-gradient(to top, rgba(20, 26, 43, 0.86), rgba(20, 26, 43, 0));
  text-shadow: 0 1px 4px rgba(0, 0, 0, 0.5);
}

.list-progress {
  position: absolute;
  right: 8px;
  bottom: 7px;
  left: 8px;
  z-index: 2;
  height: 3px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.34);
  border-radius: 999px;
}

.list-progress i {
  display: block;
  height: 100%;
  background: linear-gradient(90deg, var(--pink-500), var(--cyan-400));
}

.list-row .item-title {
  font-size: 15px;
  line-height: 1.35;
}

.list-row p {
  margin-top: 4px;
  font-size: 12px;
}

.list-row small {
  margin-top: 3px;
  color: var(--ink-400);
  font-size: 12px;
}

.profile-page {
  gap: 14px;
}

.settings-page {
  gap: 12px;
}

.menu-list.settings-list button {
  min-height: 68px;
}

.menu-list .settings-option-copy {
  min-width: 0;
  display: grid;
  gap: 5px;
}

.menu-list .settings-option-copy strong {
  font-size: 15px;
  font-weight: 600;
}

.menu-list .settings-option-copy small {
  max-width: min(72vw, 390px);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.settings-editor-card {
  padding: 18px;
}

.settings-editor-head {
  display: grid;
  gap: 5px;
}

.settings-kicker {
  color: var(--pink-600);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.14em;
}

.settings-editor-head h2 {
  font-size: 22px;
  line-height: 1.2;
}

.settings-editor-head p,
.settings-note {
  color: var(--ink-400);
  font-size: 13px;
  line-height: 1.65;
}

.server-address-form {
  display: grid;
  gap: 14px;
  margin-top: 20px;
}

.server-address-field {
  display: grid;
  gap: 8px;
}

.server-address-field > span {
  font-size: 13px;
  font-weight: 600;
}

.server-address-field input {
  width: 100%;
  height: 46px;
  padding: 0 12px;
  color: var(--ink-900);
  font-size: 15px;
  background: #f5f7fb;
  border: 1px solid rgba(32, 40, 62, 0.1);
  border-radius: 8px;
  outline: 0;
  transition: border-color 120ms var(--ease-soft), box-shadow 120ms var(--ease-soft);
}

.server-address-field input:focus {
  border-color: var(--pink-500);
  box-shadow: 0 0 0 3px rgba(244, 114, 153, 0.12);
}

.server-address-field input:disabled {
  opacity: 0.68;
}

.server-address-field small {
  color: var(--ink-400);
  font-size: 12px;
}

.settings-error {
  color: var(--pink-600);
  font-size: 13px;
  line-height: 1.5;
}

.save-settings-button {
  width: 100%;
  height: 46px;
  color: #ffffff;
  font-size: 15px;
  font-weight: 600;
  background: var(--pink-600);
  border-radius: 8px;
  box-shadow: 0 10px 22px rgba(229, 82, 130, 0.2);
  transition: transform 120ms var(--ease-soft), filter 120ms var(--ease-soft);
}

.save-settings-button:active {
  transform: scale(0.98);
  filter: brightness(0.98);
}

.save-settings-button:disabled {
  cursor: default;
  opacity: 0.68;
  box-shadow: none;
}

.settings-note {
  padding: 0 4px;
}

.about-page {
  gap: 14px;
}

.about-hero-card {
  position: relative;
  display: grid;
  align-content: center;
  justify-items: center;
  min-height: 350px;
  padding: 42px 24px 34px;
  overflow: hidden;
  text-align: center;
  background:
    radial-gradient(circle at 15% 12%, rgba(255, 181, 207, 0.36), transparent 34%),
    radial-gradient(circle at 88% 82%, rgba(152, 225, 239, 0.34), transparent 34%),
    linear-gradient(145deg, #ffffff, #fff8fb 52%, #f4fbff);
}

.about-decoration {
  position: absolute;
  border: 1px solid rgba(229, 82, 130, 0.13);
  transform: rotate(24deg);
  pointer-events: none;
}

.about-decoration-one {
  top: -46px;
  right: -36px;
  width: 142px;
  height: 142px;
  border-radius: 34px;
}

.about-decoration-two {
  bottom: -58px;
  left: -42px;
  width: 174px;
  height: 174px;
  border-color: rgba(61, 184, 207, 0.14);
  border-radius: 50%;
}

.about-logo-shell {
  position: relative;
  z-index: 1;
  width: 112px;
  height: 112px;
  display: grid;
  place-items: center;
  padding: 8px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(255, 255, 255, 0.96);
  border-radius: 26px;
  box-shadow:
    0 18px 34px rgba(229, 82, 130, 0.18),
    0 0 0 7px rgba(255, 255, 255, 0.58);
}

.about-logo-shell img {
  width: 100%;
  height: 100%;
  border-radius: 20px;
}

.about-kicker {
  position: relative;
  z-index: 1;
  margin-top: 24px;
  color: var(--pink-600);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.15em;
}

.about-hero-card h1 {
  position: relative;
  z-index: 1;
  margin-top: 7px;
  font-size: 30px;
  line-height: 1.15;
}

.about-hero-card > p {
  position: relative;
  z-index: 1;
  margin-top: 8px;
  color: var(--ink-400);
  font-size: 13px;
  line-height: 1.6;
}

.about-version {
  position: relative;
  z-index: 1;
  margin-top: 18px;
  padding: 7px 12px;
  color: var(--pink-600);
  font-size: 12px;
  font-weight: 600;
  background: rgba(255, 255, 255, 0.78);
  border: 1px solid rgba(229, 82, 130, 0.12);
  border-radius: 999px;
}

.about-info-card {
  overflow: hidden;
}

.about-info-card > div {
  min-height: 58px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 0 16px;
  border-bottom: 1px solid rgba(32, 40, 62, 0.06);
}

.about-info-card > div:last-child {
  border-bottom: 0;
}

.about-info-card span {
  color: var(--ink-400);
  font-size: 13px;
}

.about-info-card strong {
  font-size: 14px;
  font-weight: 600;
}

.about-update-card {
  overflow: hidden;
}

.about-update-card button {
  width: 100%;
  min-height: 68px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 15px;
  text-align: left;
  transition: background 120ms var(--ease-soft);
}

.about-update-card button:active:not(:disabled) {
  background: #f8f9fc;
}

.about-update-card button:disabled {
  cursor: wait;
  opacity: 0.72;
}

.about-update-icon {
  flex: 0 0 auto;
  width: 38px;
  height: 38px;
  display: grid;
  place-items: center;
  color: var(--pink-600);
  border-radius: 10px;
  background: var(--pink-50);
}

.about-update-icon svg {
  width: 20px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.about-update-copy {
  min-width: 0;
  display: grid;
  gap: 3px;
}

.about-update-copy strong {
  font-size: 14px;
  font-weight: 600;
}

.about-update-copy small {
  color: var(--ink-400);
  font-size: 12px;
  line-height: 1.45;
}

.about-update-card .chevron {
  margin-left: auto;
}

.profile-head {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px;
}

.profile-head img {
  width: 64px;
  height: 64px;
  border-radius: 8px;
}

.profile-name {
  overflow: hidden;
  font-size: 23px;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-subtitle {
  margin-top: 3px;
  color: var(--ink-400);
  font-size: 13px;
}

.menu-list {
  overflow: hidden;
}

.menu-list button {
  width: 100%;
  min-height: 58px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 14px;
  text-align: left;
  border-bottom: 1px solid rgba(32, 40, 62, 0.06);
  transition: transform 120ms var(--ease-soft), background 120ms var(--ease-soft);
}

.menu-list button:last-child {
  border-bottom: 0;
}

.menu-list button:active {
  background: #f8f9fc;
}

.menu-list span {
  font-size: 15px;
}

.menu-list small {
  color: var(--ink-400);
  font-size: 12px;
}

.menu-tail {
  display: flex;
  align-items: center;
  gap: 8px;
}

.chevron {
  color: var(--ink-300);
  font-size: 17px;
  line-height: 1;
}

.logout-menu span {
  color: var(--pink-600);
}

.logout-menu {
  justify-content: flex-start;
}

.bottom-nav {
  position: fixed;
  right: auto;
  bottom: 10px;
  left: 50%;
  z-index: 30;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 6px;
  width: min(calc(100% - 24px), 496px);
  margin: 0 auto;
  padding: 7px 7px max(7px, env(safe-area-inset-bottom));
  transform: translateX(-50%);
  background: rgba(255, 255, 255, 0.94);
  border: 1px solid rgba(32, 40, 62, 0.08);
  border-radius: 8px;
  box-shadow: 0 10px 30px rgba(32, 40, 62, 0.12);
  backdrop-filter: blur(14px);
}

.bottom-nav button {
  min-height: 50px;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 3px;
  color: var(--ink-400);
  border-radius: 7px;
  transition: color 120ms var(--ease-soft), background 120ms var(--ease-soft), transform 120ms var(--ease-soft);
}

.bottom-nav button.active {
  color: var(--pink-600);
  background: var(--pink-50);
}

.bottom-nav i {
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
}

.bottom-nav i :deep(svg) {
  width: 100%;
  height: 100%;
  display: block;
}

.bottom-nav i :deep(path) {
  fill: currentColor;
}

.bottom-nav span {
  font-size: 12px;
}

@media (max-width: 360px) {
  .app-page {
    padding-right: 12px;
    padding-left: 12px;
  }

  .top-search {
    grid-template-columns: minmax(0, 1fr) 34px;
  }

  .brand-name {
    max-width: 104px;
  }

  .poster-grid {
    gap: 11px 8px;
  }

  .media-row {
    grid-template-columns: 112px minmax(0, 1fr);
  }

  .list-cover {
    width: 112px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .page-slide-forward-enter-active,
  .page-slide-forward-leave-active,
  .page-slide-back-enter-active,
  .page-slide-back-leave-active {
    position: static;
    transition: none;
  }

  .page-slide-forward-enter-from,
  .page-slide-forward-leave-to,
  .page-slide-back-enter-from,
  .page-slide-back-leave-to {
    opacity: 1;
    transform: none;
  }
}

@keyframes mobile-skeleton {
  from {
    transform: translateX(-100%);
  }
  to {
    transform: translateX(100%);
  }
}
</style>
