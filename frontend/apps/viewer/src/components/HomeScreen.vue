<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'

import { api, type ViewerAnimeCard, type ViewerFollowedAnime, type ViewerHome, type ViewerUser, type ViewerWatchHistoryItem } from '../api'
import defaultAvatar from '../assets/avatar.png'
import type { ViewerTheme } from '../theme'
import AnimeDetailScreen from './AnimeDetailScreen.vue'
import FollowCard from './FollowCard.vue'
import FollowScreen from './FollowScreen.vue'
import HistoryScreen from './HistoryScreen.vue'
import LibraryScreen from './LibraryScreen.vue'
import ParticleField from './ParticleField.vue'
import ScheduleScreen from './ScheduleScreen.vue'
import SettingsScreen from './SettingsScreen.vue'

interface Props {
  user: ViewerUser
  siteName: string
  loading: boolean
  isNightMode: boolean
}

defineProps<Props>()
const emit = defineEmits<{
  (e: 'logout'): void
  (e: 'select-theme', theme: ViewerTheme): void
}>()

type MainView = 'home' | 'schedule' | 'library' | 'history' | 'follows' | 'settings'

const hotPageSize = 8
const maxHotPages = 4
const hotSkeletonCount = 8
const recentSkeletonCount = 24 // 最近更新：8 列 × 3 排
const maxRecentCount = 24 // 最近更新最多显示 24 个（3 排）
const followPageSize = 6
const heroIntervalMs = 5500

const searchQuery = ref('')
const libraryQuery = ref('')
const librarySearchKey = ref(0)
const activeView = ref<MainView>('home')
const detailAnimeId = ref<number | null>(null)
const detailMediaId = ref(0)
const detailPosition = ref(0)
const homeLoading = ref(false)
const homeInitialized = ref(false)
const homeError = ref('')
const hotPage = ref(0)
const followPage = ref(0)
const heroIndex = ref(0)
const relativeTimeNow = ref(Date.now())
const failedCovers = ref<Set<number>>(new Set())
const home = ref<ViewerHome>({
  hotRecommendations: [],
  recentUpdates: [],
  carouselSlides: [],
  myFollows: [],
})

let heroTimer: ReturnType<typeof setInterval> | null = null
let relativeTimeTimer: ReturnType<typeof setInterval> | null = null

const heroSlides = computed(() => home.value.carouselSlides)
const currentHero = computed(() => heroSlides.value[heroIndex.value] ?? null)

const hotPages = computed(() => {
  const items = home.value.hotRecommendations.slice(0, hotPageSize * maxHotPages)
  const pages: ViewerAnimeCard[][] = []
  for (let index = 0; index < items.length; index += hotPageSize) {
    pages.push(items.slice(index, index + hotPageSize))
  }
  return pages
})

const currentHotItems = computed(() => hotPages.value[hotPage.value] ?? [])
const canTurnHot = computed(() => hotPages.value.length > 1)
const recentItems = computed(() => home.value.recentUpdates.slice(0, maxRecentCount))
const pageIndicator = computed(() => {
  const total = Math.max(hotPages.value.length, 1)
  return `${Math.min(hotPage.value + 1, total)} / ${total}`
})

onMounted(() => {
  syncDetailFromLocation()
  window.addEventListener('popstate', syncDetailFromLocation)
  void loadHome()
  relativeTimeTimer = setInterval(() => {
    relativeTimeNow.value = Date.now()
  }, 60_000)
})
const unfinishedFollows = computed(() => home.value.myFollows.filter((item) => !item.caughtUp))
const followPages = computed(() => {
  const pages: ViewerFollowedAnime[][] = []
  for (let index = 0; index < unfinishedFollows.value.length; index += followPageSize) {
    pages.push(unfinishedFollows.value.slice(index, index + followPageSize))
  }
  return pages
})
const currentFollowItems = computed(() => followPages.value[followPage.value] ?? [])
const canTurnFollow = computed(() => followPages.value.length > 1)
const followPageIndicator = computed(() => {
  const total = Math.max(followPages.value.length, 1)
  return `${Math.min(followPage.value + 1, total)} / ${total}`
})

onUnmounted(() => {
  window.removeEventListener('popstate', syncDetailFromLocation)
  stopHeroAutoplay()
  if (relativeTimeTimer !== null) {
    clearInterval(relativeTimeTimer)
  }
})

// 列表变化时重置索引并重启轮播
watch(heroSlides, (slides) => {
  heroIndex.value = 0
  if (slides.length > 1) {
    startHeroAutoplay()
  } else {
    stopHeroAutoplay()
  }
})

watch(unfinishedFollows, () => {
  followPage.value = 0
})

function startHeroAutoplay() {
  stopHeroAutoplay()
  if (heroSlides.value.length <= 1) {
    return
  }
  heroTimer = setInterval(() => {
    heroIndex.value = (heroIndex.value + 1) % heroSlides.value.length
  }, heroIntervalMs)
}

function stopHeroAutoplay() {
  if (heroTimer !== null) {
    clearInterval(heroTimer)
    heroTimer = null
  }
}

function selectHero(index: number) {
  if (!heroSlides.value.length) {
    return
  }
  heroIndex.value = (index + heroSlides.value.length) % heroSlides.value.length
  startHeroAutoplay() // 手动交互后重新计时
}

function turnHero(direction: number) {
  if (!heroSlides.value.length) {
    return
  }
  const total = heroSlides.value.length
  heroIndex.value = (heroIndex.value + direction + total) % total
  startHeroAutoplay()
}

async function loadHome() {
  if (homeLoading.value) {
    return
  }
  homeLoading.value = true
  homeError.value = ''
  try {
    const result = await api.home()
    home.value = result.home
    hotPage.value = 0
  } catch (error) {
    homeError.value = error instanceof Error ? error.message : '首页数据加载失败'
  } finally {
    homeLoading.value = false
    homeInitialized.value = true
  }
}

function turnHotPage(direction: number) {
  if (!canTurnHot.value) {
    return
  }
  const total = hotPages.value.length
  hotPage.value = (hotPage.value + direction + total) % total
}

function coverURL(item: ViewerAnimeCard) {
  return `/api/anime/${item.bangumiId}/cover`
}

function turnFollowPage(direction: number) {
  if (!canTurnFollow.value) return
  const total = followPages.value.length
  followPage.value = (followPage.value + direction + total) % total
}

function openCurrentHero() {
  if (currentHero.value) openAnime(currentHero.value.bangumiId)
}

function carouselImageURL(id: number, updatedAt: number) {
  return `/api/carousels/${id}/image?v=${updatedAt}`
}

function hasCover(item: ViewerAnimeCard) {
  return item.hasCover && !failedCovers.value.has(item.bangumiId)
}

function markCoverFailed(item: ViewerAnimeCard) {
  const next = new Set(failedCovers.value)
  next.add(item.bangumiId)
  failedCovers.value = next
}

function ratingText(score: number | null) {
  return score === null ? '--' : score.toFixed(1)
}

function updateText(item: ViewerAnimeCard) {
  return item.latestEpisodeLabel ? `更新至 ${item.latestEpisodeLabel}` : '更新至 ?'
}

function formatUpdatedAt(value: number | null) {
  if (!value) {
    return '更新时间未知'
  }
  const elapsedSeconds = Math.max(Math.floor(relativeTimeNow.value / 1000) - value, 0)
  const minutes = Math.max(Math.floor(elapsedSeconds / 60), 1)
  if (minutes < 60) {
    return `${minutes}分钟前更新`
  }
  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    return `${hours}小时前更新`
  }
  return `${Math.floor(hours / 24)}天前更新`
}

function formatAirDate(value: string) {
  return value ? value.split('-').join('.') : 'ON AIR'
}

function formatPremiereDate(value: string) {
  return value ? `于 ${formatAirDate(value)} 首播` : '首播日期未定'
}

// 卡片交错入场延迟
function stagger(index: number, base = 0.04, step = 0.05) {
  return `${base + index * step}s`
}

function submitGlobalSearch() {
  libraryQuery.value = searchQuery.value.trim()
  librarySearchKey.value++
  showView('library')
}

function showView(view: MainView) {
  const hadDetail = detailAnimeId.value !== null
  activeView.value = view
  if (hadDetail) {
    detailAnimeId.value = null
    detailMediaId.value = 0
    detailPosition.value = 0
  }
  if (view === 'settings') {
    if (window.location.pathname !== '/settings') {
      window.history.pushState({ bpView: 'settings' }, '', '/settings')
    }
  } else if (window.location.pathname === '/settings') {
    window.history.pushState({ bpView: view }, '', '/')
  } else if (hadDetail) {
    window.history.replaceState({}, '', '/')
  }
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

function openAnime(bangumiId: number, mediaId = 0, positionSeconds = 0) {
  if (bangumiId < 1) return
  detailAnimeId.value = bangumiId
  detailMediaId.value = mediaId > 0 ? mediaId : 0
  detailPosition.value = positionSeconds > 0 ? positionSeconds : 0
  const params = new URLSearchParams()
  if (detailMediaId.value > 0) params.set('media', String(detailMediaId.value))
  if (detailPosition.value > 0) params.set('t', String(Math.floor(detailPosition.value)))
  const query = params.toString()
  window.history.pushState({ bpAnimeDetail: true }, '', `/anime/${bangumiId}${query ? `?${query}` : ''}`)
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

function openHistoryItem(item: ViewerWatchHistoryItem) {
  openAnime(item.bangumiId, item.mediaId, item.completed ? 0 : item.positionSeconds)
}

function openFollowedAnime(item: ViewerFollowedAnime) {
  openAnime(item.bangumiId, item.mediaId, item.positionSeconds)
}

function closeAnimeDetail() {
  if (window.history.state?.bpAnimeDetail) {
    window.history.back()
    return
  }
  detailAnimeId.value = null
  detailMediaId.value = 0
  detailPosition.value = 0
  activeView.value = 'home'
  window.history.replaceState({}, '', '/')
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

function syncDetailFromLocation() {
  const pathname = window.location.pathname.replace(/\/+$/, '') || '/'
  if (pathname === '/settings') {
    activeView.value = 'settings'
    detailAnimeId.value = null
    detailMediaId.value = 0
    detailPosition.value = 0
    window.scrollTo({ top: 0 })
    return
  }
  const historyView = window.history.state?.bpView
  if (pathname === '/' && isMainView(historyView) && historyView !== 'settings') {
    activeView.value = historyView
  } else if (activeView.value === 'settings') {
    activeView.value = 'home'
  }
  const match = pathname.match(/^\/anime\/(\d+)$/)
  detailAnimeId.value = match ? Number(match[1]) : null
  const params = new URLSearchParams(window.location.search)
  const mediaID = Number(params.get('media'))
  const position = Number(params.get('t'))
  detailMediaId.value = Number.isInteger(mediaID) && mediaID > 0 ? mediaID : 0
  detailPosition.value = Number.isFinite(position) && position > 0 ? position : 0
  window.scrollTo({ top: 0 })
}

function isMainView(value: unknown): value is MainView {
  return value === 'home'
    || value === 'schedule'
    || value === 'library'
    || value === 'history'
    || value === 'follows'
    || value === 'settings'
}
</script>

<template>
  <main class="home-shell">
    <header class="topbar">
      <div class="brand-row">
        <div class="brand-text">
          <p>VIEWER PORTAL</p>
          <strong>{{ siteName }}</strong>
        </div>
      </div>

      <nav class="main-nav" aria-label="主导航">
        <button
          class="nav-item"
          :class="{ active: activeView === 'home' }"
          type="button"
          @click="showView('home')"
        >
          首页
        </button>
        <button
          class="nav-item"
          :class="{ active: activeView === 'schedule' }"
          type="button"
          @click="showView('schedule')"
        >
          番剧时间表
        </button>
        <button
          class="nav-item"
          :class="{ active: activeView === 'library' }"
          type="button"
          @click="showView('library')"
        >
          番剧图书馆
        </button>
      </nav>

      <form class="search-box" role="search" @submit.prevent="submitGlobalSearch">
        <span class="search-symbol" aria-hidden="true" />
        <input v-model="searchQuery" type="search" placeholder="搜索番剧" />
      </form>

      <a
        class="app-download-link"
        href="/app/download"
        target="_blank"
        rel="noopener noreferrer"
        aria-label="在新标签页打开 APP 下载页面"
      >
        <svg class="app-download-icon" viewBox="0 0 1024 1024" aria-hidden="true">
          <path d="M753.265 105.112c12.57 12.546 12.696 32.81 0.377 45.512l-0.377 0.383-73.131 72.992L816 224c70.692 0 128 57.308 128 128v448c0 70.692-57.308 128-128 128H208c-70.692 0-128-57.308-128-128V352c0-70.692 57.308-128 128-128l136.078-0.001-73.13-72.992c-12.698-12.674-12.698-33.222 0-45.895 12.697-12.674 33.284-12.674 45.982 0l119.113 118.887h152.126l119.114-118.887c12.697-12.674 33.284-12.674 45.982 0zM457 440c-28.079 0-51 22.938-51 51v170c0 9.107 2.556 18.277 7 26 15.025 24.487 46.501 32.241 71 18l138-84c7.244-4.512 13.094-10.313 17-17 15.213-24.307 7.75-55.875-16-71l-139-85c-7.994-5.355-17.305-8-27-8z" fill="currentColor" />
        </svg>
        <span>APP下载</span>
      </a>

      <div class="theme-area">
        <button
          class="theme-button"
          type="button"
          aria-haspopup="menu"
          :aria-label="`网站主题，当前为${isNightMode ? '深色模式' : '浅色模式'}`"
          title="选择网站主题"
        >
          <svg class="theme-button-icon" viewBox="0 0 1024 1024" aria-hidden="true">
            <path d="M612.92544 200.33536a46.08 46.08 0 0 1 70.36928-22.05696l205.86496 148.52096a46.08 46.08 0 0 1 17.8176 48.27136l-34.89792 143.4624a46.08 46.08 0 0 1-55.68512 33.8944l-0.2048-0.06144-84.93056-21.07392 47.65696 282.43968a46.08 46.08 0 0 1-34.54976 52.4288l-3.21536 0.67584a46.08 46.08 0 0 1-7.68 0.65536H385.55648a25.6 25.6 0 0 1 0-51.2h341.85216l-47.9232-284.16a46.08 46.08 0 0 1-0.04096-15.0528l0.75776-3.70688a46.08 46.08 0 0 1 55.82848-33.62816l87.49056 21.66784 32.8704-135.10656-197.50912-142.52032-0.1024 0.34816c-32.4608 88.064-83.41504 136.0896-152.10496 138.4448l-5.3248 0.08192c-69.77536 0-118.29248-46.05952-144.34304-131.66592l-1.76128-5.98016-203.03872 141.9264 27.89376 134.10304 84.33664-21.46304a46.08 46.08 0 0 1 12.45184-1.41312l4.17792 0.28672a46.08 46.08 0 0 1 40.52992 51.03616L285.40928 844.8a25.6 25.6 0 1 1-50.85184-5.8368l35.30752-307.95776-82.30912 20.992a46.08 46.08 0 0 1-55.0912-30.1056l-0.94208-3.1744-0.45056-1.98656-29.696-142.66368a46.08 46.08 0 0 1 18.7392-47.16544l211.78368-148.00896a46.03904 46.03904 0 0 1 70.88128 25.94816c19.57888 73.64608 51.93728 106.61888 98.59072 106.61888 48.00512 0 84.86912-34.85696 111.55456-111.12448z" fill="#ee3f86" />
          </svg>
          <span>网站主题</span>
          <i class="theme-arrow" aria-hidden="true" />
        </button>

        <div class="theme-menu" role="menu" aria-label="选择网站主题">
          <button
            type="button"
            role="menuitemradio"
            :aria-checked="!isNightMode"
            :class="{ active: !isNightMode }"
            @click="emit('select-theme', 'light')"
          >
            <svg class="theme-option-icon" viewBox="0 0 1024 1024" aria-hidden="true">
              <path d="M512 853.333333a34.133333 34.133333 0 0 1 34.133333 34.133334v102.4a34.133333 34.133333 0 0 1-68.266666 0v-102.4a34.133333 34.133333 0 0 1 34.133333-34.133334z m289.621333-99.976533l72.430934 72.3968a34.133333 34.133333 0 1 1-48.298667 48.298667l-72.362667-72.430934a34.133333 34.133333 0 0 1 48.2304-48.264533z m-530.978133 0a34.133333 34.133333 0 0 1 0 48.264533l-72.3968 72.430934a34.133333 34.133333 0 0 1-48.298667-48.298667l72.430934-72.362667a34.133333 34.133333 0 0 1 48.264533 0zM512 256c46.421333 0 89.258667 11.537067 128.477867 34.269867a255.556267 255.556267 0 0 1 93.184 93.2864A249.787733 249.787733 0 0 1 767.931733 512a250.368 250.368 0 0 1-34.338133 128.580267 257.092267 257.092267 0 0 1-93.149867 93.149866A249.9584 249.9584 0 0 1 512 768a250.197333 250.197333 0 0 1-128.512-34.269867 257.4336 257.4336 0 0 1-93.149867-93.184A250.504533 250.504533 0 0 1 256 512c0-46.523733 11.502933-89.326933 34.338133-128.443733a255.5904 255.5904 0 0 1 93.184-93.2864A251.6992 251.6992 0 0 1 512 256z m0 68.266667c-51.8144 0-96.017067 18.295467-132.744533 55.057066C342.596267 415.914667 324.266667 460.219733 324.266667 512s18.3296 96.0512 54.954666 132.778667c36.795733 36.6592 80.964267 54.954667 132.778667 54.954666 51.8144 0 96.017067-18.295467 132.744533-54.920533C681.403733 608.0512 699.733333 563.780267 699.733333 512s-18.363733-96.0512-54.954666-132.676267c-34.0992-34.133333-74.581333-52.5312-132.778667-55.057066zM989.866667 477.866667a34.133333 34.133333 0 0 1 0 68.266666h-102.4a34.133333 34.133333 0 0 1 0-68.266666h102.4zM136.533333 477.866667a34.133333 34.133333 0 0 1 0 68.266666H34.133333a34.133333 34.133333 0 0 1 0-68.266666h102.4z m61.713067-327.918934l72.362667 72.430934a34.133333 34.133333 0 0 1-48.2304 48.264533L149.947733 198.2464a34.133333 34.133333 0 0 1 48.298667-48.298667z m675.805867 0a34.133333 34.133333 0 0 1 0 48.298667l-72.430934 72.362667a34.133333 34.133333 0 0 1-48.264533-48.2304l72.3968-72.430934a34.133333 34.133333 0 0 1 48.298667 0zM512 0a34.133333 34.133333 0 0 1 34.133333 34.133333v102.4a34.133333 34.133333 0 0 1-68.266666 0V34.133333a34.133333 34.133333 0 0 1 34.133333-34.133333z" fill="#ee3f86" />
            </svg>
            <span>浅色模式</span>
          </button>
          <button
            type="button"
            role="menuitemradio"
            :aria-checked="isNightMode"
            :class="{ active: isNightMode }"
            @click="emit('select-theme', 'dark')"
          >
            <svg class="theme-option-icon" viewBox="0 0 1024 1024" fill="currentColor" aria-hidden="true">
              <path d="M339.456 891.904s-0.512 0-1.024-0.512l-13.312-4.608c-33.792-11.776-34.304-12.288-55.808-22.016-14.336-6.656-55.296-34.304-77.824-50.176-1.024-0.512-2.048-1.536-3.072-2.048-8.704-7.168-16.896-14.848-25.088-23.04-43.52-44.032-74.752-95.744-94.208-154.112 140.8 77.824 315.392 52.736 429.568-62.976s138.752-292.352 61.952-434.688c57.856 19.456 108.544 51.2 152.064 95.232 21.504 22.016 39.936 46.08 55.808 71.68l4.608 7.68c16.384 32.256 16.384 32.768 25.088 54.784 8.192 21.504 22.016 69.632 22.016 70.144 4.096 14.336 16.896 23.552 30.72 23.552 3.072 0 5.632-0.512 8.704-1.536 16.896-5.12 27.136-23.04 22.016-39.936 0 0 0-0.512-0.512-1.024h0.512c-10.752-51.712-29.696-100.864-56.832-145.92l-1.536-3.584c-1.024-2.56-2.56-4.608-4.608-6.656-17.408-27.136-37.376-52.736-60.416-75.776A459.008 459.008 0 0 0 573.44 72.704c-18.944-5.632-48.128-5.632-66.56 13.312-20.48 20.48-17.92 50.176-9.728 69.12l1.024 1.536 1.024 1.536c69.632 118.784 50.688 269.824-46.08 367.616s-245.76 116.736-362.496 46.592l-1.536-1.024c-10.24-5.12-46.08-20.992-69.632 2.56-23.04 23.04-15.872 60.928-13.312 71.68l0.512 2.048c21.504 70.144 60.416 134.656 111.616 186.88 56.832 57.344 127.488 98.816 204.8 119.296 1.536 0.512 3.584 0.512 5.12 0.512 13.312 0 25.6-8.192 30.208-21.504 6.144-16.384-2.56-34.816-18.944-40.96z" />
              <path d="M531.968 671.744c-16.384 0-29.696 13.312-29.696 29.696 0 16.384 13.312 29.696 29.696 29.696h267.264c16.384 0 29.696-13.312 29.696-29.696 0-16.384-13.312-29.696-29.696-29.696h-267.264z m459.264 160.256h-573.44c-16.384 0-29.696 13.312-29.696 29.696 0 16.384 13.312 29.696 29.696 29.696h573.952c16.384 0 29.696-13.312 29.696-29.696-0.512-16.384-13.824-29.696-30.208-29.696z m-366.08-296.448c0 16.384 13.312 29.696 29.696 29.696h336.384c16.384 0 29.696-13.312 29.696-29.696s-13.312-29.696-29.696-29.696h-336.384c-16.384 0-29.696 13.312-29.696 29.696zM323.072 189.44h22.528v22.528c0 10.752 8.704 19.968 19.968 19.968 10.752 0 19.968-8.704 19.968-19.968V189.44h22.528c10.752 0 19.968-8.704 19.968-19.968s-8.704-19.968-19.968-19.968h-22.528v-22.528c0-10.752-8.704-19.968-19.968-19.968-10.752 0-19.968 8.704-19.968 19.968v22.528h-22.528c-10.752 0-19.968 8.704-19.968 19.968S312.32 189.44 323.072 189.44z m-204.8 275.968h37.376v37.376c0 13.824 11.264 25.088 24.576 25.088 13.824 0 24.576-11.264 24.576-25.088v-37.376h37.376c13.824 0 24.576-11.264 24.576-25.088s-11.264-25.088-24.576-25.088H204.8v-37.376c0-13.824-11.264-25.088-24.576-25.088-13.824 0-24.576 11.264-24.576 25.088v37.376h-37.376c-13.824 0-24.576 11.264-24.576 25.088s10.752 25.088 24.576 25.088z" />
            </svg>
            <span>深色模式</span>
          </button>
        </div>
      </div>

      <div class="user-area">
        <button class="user-chip" type="button" aria-haspopup="menu">
          <img class="user-avatar" :src="defaultAvatar" alt="" />
          <span class="user-name">{{ user.username }}</span>
          <i class="user-arrow" aria-hidden="true" />
        </button>
        <div class="user-menu" role="menu">
          <button type="button" role="menuitem" @click="showView('follows')">
            <svg class="user-menu-icon" viewBox="0 0 1024 1024" aria-hidden="true">
              <path d="M476.689655 617.931034m-370.758621 0a370.758621 370.758621 0 1 0 741.517242 0 370.758621 370.758621 0 1 0-741.517242 0Z" fill="#D8D8D8" />
              <path d="M20.391724 727.816828l0.459035-1.253518a259.831172 259.831172 0 0 1 7.062069-17.867034 310.060138 310.060138 0 0 1 11.564138-23.869793 372.930207 372.930207 0 0 1 10.257655-18.061242 438.23669 438.23669 0 0 1 7.379862-11.828965A476.407172 476.407172 0 0 1 35.310345 512C35.310345 248.726069 248.726069 35.310345 512 35.310345c149.963034 0 283.771586 69.261241 371.147034 177.522758 56.231724 5.826207 97.968552 25.900138 118.324966 61.157518 23.746207 41.136552 15.059862 97.068138-19.261793 159.161379 4.254897 25.63531 6.479448 51.976828 6.479448 78.848 0 263.273931-213.415724 476.689655-476.689655 476.689655-104.165517 0-200.509793-33.403586-278.951724-90.094345a505.114483 505.114483 0 0 1-47.951448 3.248552l-5.826207 0.070621c-75.422897 0.282483-131.61931-20.126897-156.336552-62.958345A95.514483 95.514483 0 0 1 12.358621 811.99669a109.726897 109.726897 0 0 1-2.189242-13.27669 132.890483 132.890483 0 0 1 0.723862-32.432552 177.858207 177.858207 0 0 1 4.043035-20.48 209.796414 209.796414 0 0 1 5.12-17.019586l-0.441379 1.271172 0.759172-2.242206z m878.697931-183.401931l-0.335448 0.353103a836.502069 836.502069 0 0 1-34.868966 35.398621 967.326897 967.326897 0 0 1-18.008275 16.843034l4.043034-3.707586a1012.18869 1012.18869 0 0 1-40.183172 35.239724 1128.041931 1128.041931 0 0 1-15.942621 13.064828 1214.322759 1214.322759 0 0 1-51.606069 39.265103 1302.121931 1302.121931 0 0 1-66.330483 44.808828 1386.107586 1386.107586 0 0 1-48.904827 29.572414c-90.394483 52.206345-180.859586 91.136-263.556414 115.712A387.10731 387.10731 0 0 0 512 900.413793c203.599448 0 370.599724-156.654345 387.072-355.998896zM103.45931 757.777655l-0.494344 1.412414a140.711724 140.711724 0 0 0-1.483035 4.766897c-5.031724 17.37269-4.502069 26.747586-2.118621 30.861241 2.365793 4.13131 10.222345 9.268966 27.789242 13.594483 4.237241 1.05931 8.792276 1.942069 13.594482 2.683586a482.198069 482.198069 0 0 1-37.287724-53.318621zM512 123.586207C297.489655 123.586207 123.586207 297.489655 123.586207 512c0 117.318621 52.012138 222.508138 134.249931 293.729103l5.420138-0.988689c95.19669-17.92 208.507586-61.810759 319.558621-125.934345 11.211034-6.479448 22.245517-13.064828 33.103448-19.773793l10.416552-6.497104a1288.651034 1288.651034 0 0 0 53.053793-35.310344 1204.612414 1204.612414 0 0 0 38.417655-28.177656l8.262621-6.391172a1103.960276 1103.960276 0 0 0 24.858482-20.00331l-6.17931 5.084689c3.03669-2.471724 6.038069-4.943448 9.004138-7.432827l-2.824828 2.348138c2.507034-2.065655 4.996414-4.148966 7.468138-6.232276l-4.625655 3.884138 8.721655-7.379862-4.096 3.495724c2.930759-2.471724 5.826207-4.961103 8.704-7.450483l-4.608 3.954759c2.807172-2.383448 5.579034-4.784552 8.315586-7.185656l-3.707586 3.230897 7.220966-6.302897-3.51338 3.072a946.970483 946.970483 0 0 0 20.268138-18.220137l-3.601655 3.319172c2.736552-2.52469 5.455448-5.031724 8.121379-7.538759l-4.519724 4.219587a869.075862 869.075862 0 0 0 19.950345-19.085242l-3.213241 3.177931a819.2 819.2 0 0 0 7.591724-7.503448l-4.360828 4.325517c2.436414-2.418759 4.855172-4.819862 7.238621-7.238621l-2.895448 2.913104c1.959724-1.977379 3.919448-3.937103 5.826207-5.914483l-2.948414 3.001379a757.195034 757.195034 0 0 0 17.213793-18.008275l-2.436414 2.612965c2.08331-2.224552 4.13131-4.431448 6.144-6.656l-3.707586 4.06069c3.177931-3.442759 6.267586-6.885517 9.321931-10.292966l-5.614345 6.232276c2.259862-2.471724 4.484414-4.961103 6.673655-7.432827l-1.05931 1.200551c1.57131-1.765517 3.124966-3.54869 4.660966-5.331862l-3.601656 4.131311c2.295172-2.612966 4.555034-5.208276 6.779587-7.803587l-3.177931 3.672276c2.59531-3.001379 5.15531-5.985103 7.644689-8.968827l-4.466758 5.296551c1.995034-2.348138 3.972414-4.678621 5.896827-7.009103l-1.430069 1.712552 4.590345-5.543725-3.160276 3.831173c2.189241-2.648276 4.325517-5.261241 6.426483-7.891862l-3.266207 4.060689c2.718897-3.354483 5.367172-6.673655 7.944828-9.992827 27.771586-35.663448 45.762207-68.148966 53.265655-94.066759 5.031724-17.390345 4.502069-26.747586 2.11862-30.878896-2.383448-4.113655-10.222345-9.25131-27.789241-13.594483-15.766069-3.884138-35.469241-5.649655-58.420965-5.12-20.709517-27.61269-40.448-50.582069-59.233104-68.961103A387.142621 387.142621 0 0 0 512 123.586207z" fill="#464646" />
              <path d="M635.586207 494.344828m-70.62069 0a70.62069 70.62069 0 1 0 141.24138 0 70.62069 70.62069 0 1 0-141.24138 0Z" fill="#6A6A6A" />
              <path d="M388.413793 494.344828m-70.62069 0a70.62069 70.62069 0 1 0 141.24138 0 70.62069 70.62069 0 1 0-141.24138 0Z" fill="#6A6A6A" />
            </svg>
            我的追番
          </button>
          <button type="button" role="menuitem" @click="showView('history')">
            <svg class="user-menu-icon" viewBox="0 0 1024 1024" aria-hidden="true">
              <path d="M582.82496 600.275862m-388.413793 0a388.413793 388.413793 0 1 0 776.827586 0 388.413793 388.413793 0 1 0-776.827586 0Z" fill="#D8D8D8" />
              <path d="M512.20427 0c282.765241 0 512 229.234759 512 512S794.969512 1024 512.20427 1024C315.59627 1024 144.870753 913.178483 59.08427 750.592a44.137931 44.137931 0 0 1 77.523862-42.231172C207.423029 843.511172 349.052822 935.724138 512.20427 935.724138c234.01931 0 423.724138-189.704828 423.724138-423.724138S746.223581 88.275862 512.20427 88.275862C310.193788 88.275862 141.180822 229.658483 98.755443 418.868966L174.990477 374.854621a44.137931 44.137931 0 0 1 44.137931 76.446896l-152.893793 88.275862a44.137931 44.137931 0 0 1-65.747862-44.738207l-0.123586 4.360828C7.142753 222.349241 233.711581 0 512.20427 0z" fill="#464646" />
              <path d="M603.658063 309.548138a44.137931 44.137931 0 0 1 16.154483 60.292414L503.253098 571.674483l65.677241 65.659586a44.137931 44.137931 0 1 1-62.428689 62.411034l-87.393104-87.393103a43.926069 43.926069 0 0 1-11.793655-21.186207 44.014345 44.014345 0 0 1 3.636966-36.104827l132.413793-229.34069a44.137931 44.137931 0 0 1 60.292413-16.172138z" fill="#6A6A6A" />
            </svg>
            观看历史
          </button>
          <button type="button" role="menuitem" @click="showView('settings')">
            <svg class="user-menu-icon" viewBox="0 0 1024 1024" aria-hidden="true">
              <path d="M459.034483 600.275862m-388.413793 0a388.413793 388.413793 0 1 0 776.827586 0 388.413793 388.413793 0 1 0-776.827586 0Z" fill="#D8D8D8" />
              <path d="M1001.524966 501.142069c8.651034 8.651034 12.976552 20.020966 12.923586 31.373241a492.173241 492.173241 0 0 1-144.34869 330.68138c-193.05931 193.076966-506.067862 193.076966-699.109517 0C-22.068966 670.17269-22.068966 357.164138 170.990345 164.122483a492.296828 492.296828 0 0 1 299.55531-142.26538 44.137931 44.137931 0 0 1 36.440276 75.052138 43.979034 43.979034 0 0 1-29.342897 12.888276A404.815448 404.815448 0 0 0 233.401379 226.515862c-158.578759 158.596414-158.578759 415.691034 0 574.269793s415.691034 158.578759 574.269793 0a404.603586 404.603586 0 0 0 118.678069-272.436965 44.137931 44.137931 0 0 1 75.175725-27.188966z" fill="#464646" />
              <path d="M343.889168 626.005957m31.210231-31.21023l436.943224-436.943225q31.21023-31.21023 62.420461 0l0 0q31.21023 31.21023 0 62.420461l-436.943225 436.943225q-31.21023 31.21023-62.42046 0l0 0q-31.21023-31.21023 0-62.420461Z" fill="#6A6A6A" />
            </svg>
            设置
          </button>
          <button :disabled="loading" type="button" role="menuitem" @click="emit('logout')">
            <span class="user-menu-icon-spacer" aria-hidden="true" />
            退出登录
          </button>
        </div>
      </div>
    </header>

    <AnimeDetailScreen
      v-if="detailAnimeId !== null"
      :bangumi-id="detailAnimeId"
      :initial-media-id="detailMediaId"
      :initial-position="detailPosition"
      @back="closeAnimeDetail"
      @follow-changed="loadHome"
    />
    <ScheduleScreen v-else-if="activeView === 'schedule'" @open-anime="openAnime" />
    <LibraryScreen
      v-else-if="activeView === 'library'"
      :initial-query="libraryQuery"
      :search-key="librarySearchKey"
      @open-anime="openAnime"
    />
    <HistoryScreen v-else-if="activeView === 'history'" @open-history="openHistoryItem" />
    <FollowScreen v-else-if="activeView === 'follows'" @open-follow="openFollowedAnime" />
    <SettingsScreen v-else-if="activeView === 'settings'" :user="user" />

    <section v-else class="home-stage" aria-label="首页">
      <ParticleField :count="16" palette="pink" :max-size="30" />
      <div class="stage-grid" aria-hidden="true" />
      <div class="stage-halo halo-a" aria-hidden="true" />
      <div class="stage-halo halo-b" aria-hidden="true" />

      <div class="content-wrap" :class="{ 'without-hero': homeInitialized && !currentHero }">
        <!-- ===== 首页轮播 ===== -->
        <section
          v-if="!homeInitialized || currentHero"
          class="hero-carousel"
          :class="{ 'has-slide': currentHero, 'is-loading': !homeInitialized, clickable: currentHero && homeInitialized }"
          aria-label="精选轮播"
          :aria-busy="!homeInitialized"
          @click="openCurrentHero"
        >
          <img
            v-if="homeInitialized && currentHero"
            class="hero-image"
            :src="carouselImageURL(currentHero.id, currentHero.imageUpdatedAt)"
            :alt="currentHero.title"
          />
          <div v-if="homeInitialized && currentHero" class="hero-shade" aria-hidden="true" />

          <div v-if="!homeInitialized" class="hero-skeleton-backdrop" aria-hidden="true" />

          <ParticleField v-if="homeInitialized" :count="14" palette="cool" :max-size="40" />

          <div v-if="!homeInitialized" class="hero-skeleton" role="status" aria-label="轮播内容加载中">
            <span class="hero-skeleton-block skeleton-tag" />
            <span class="hero-skeleton-block skeleton-kicker" />
            <span class="hero-skeleton-block skeleton-heading" />
            <span class="hero-skeleton-block skeleton-heading short" />
            <div class="skeleton-meta-row">
              <span class="hero-skeleton-block" />
              <span class="hero-skeleton-block" />
            </div>
            <span class="hero-skeleton-block skeleton-summary" />
            <span class="hero-skeleton-block skeleton-summary short" />
          </div>

          <div v-else-if="currentHero" :key="currentHero.id" class="hero-content">
            <span class="hero-index-tag">
              <i>{{ String(heroIndex + 1).padStart(2, '0') }}</i>
              <span>/ {{ String(heroSlides.length).padStart(2, '0') }}</span>
            </span>
            <p class="hero-kicker">FEATURED</p>
            <h1 class="hero-title">{{ currentHero.title }}</h1>
            <div class="hero-meta">
              <span class="meta-pill">
                <i class="meta-dot" aria-hidden="true" />
                {{ formatAirDate(currentHero.airDate) }}
              </span>
              <span v-if="currentHero.ratingScore !== null" class="meta-pill">
                <i class="meta-star" aria-hidden="true">★</i>
                {{ ratingText(currentHero.ratingScore) }}
              </span>
            </div>
            <p class="hero-summary">{{ currentHero.summary || '暂无剧情简介' }}</p>
          </div>

          <!-- 切换箭头 -->
          <button
            v-if="heroSlides.length > 1"
            class="hero-arrow arrow-prev"
            type="button"
            aria-label="上一个"
            @click.stop="turnHero(-1)"
          />
          <button
            v-if="heroSlides.length > 1"
            class="hero-arrow arrow-next"
            type="button"
            aria-label="下一个"
            @click.stop="turnHero(1)"
          />

          <!-- 指示器 -->
          <div v-if="heroSlides.length > 1" class="hero-dots" role="tablist" aria-label="切换轮播">
            <button
              v-for="(slide, index) in heroSlides"
              :key="slide.id"
              class="hero-dot"
              :class="{ active: index === heroIndex }"
              type="button"
              role="tab"
              :aria-selected="index === heroIndex"
              :aria-label="`第 ${index + 1} 张`"
              @click.stop="selectHero(index)"
            />
          </div>
        </section>

        <!-- ===== 我的追番 ===== -->
        <section v-if="homeLoading || unfinishedFollows.length > 0" class="anime-section follow-section">
          <div class="section-head">
            <div class="section-title">
              <p class="section-kicker">MY FOLLOWING</p>
              <h2>我的追番</h2>
              <i class="section-bar" aria-hidden="true" />
            </div>
            <div class="section-controls">
              <span class="page-count">{{ followPageIndicator }}</span>
              <button
                class="arrow-button page-arrow-prev"
                :disabled="!canTurnFollow"
                type="button"
                aria-label="上一页"
                @click="turnFollowPage(-1)"
              />
              <button
                class="arrow-button page-arrow-next"
                :disabled="!canTurnFollow"
                type="button"
                aria-label="下一页"
                @click="turnFollowPage(1)"
              />
            </div>
          </div>
          <div v-if="homeLoading" class="follow-home-grid">
            <article v-for="index in 6" :key="index" class="follow-home-skeleton">
              <div class="skeleton-follow-cover skeleton-block" />
              <div class="skeleton-title skeleton-block" />
            </article>
          </div>
          <div v-else class="follow-home-grid">
            <FollowCard
              v-for="item in currentFollowItems"
              :key="item.bangumiId"
              :item="item"
              @open="openFollowedAnime"
            />
          </div>
        </section>

        <!-- ===== 最近更新（统一大竖版卡 + NEW + 时间角标） ===== -->
        <section class="anime-section recent-section">
          <div class="section-head">
            <div class="section-title">
              <p class="section-kicker">RECENT DROPS</p>
              <h2>最近更新</h2>
              <i class="section-bar" aria-hidden="true" />
            </div>
          </div>

          <div v-if="homeLoading" class="recent-grid">
            <article v-for="n in recentSkeletonCount" :key="n" class="poster-card">
              <div class="skeleton-poster skeleton-block" />
              <div class="skeleton-title skeleton-block" />
            </article>
          </div>
          <div v-else-if="!homeError && recentItems.length === 0" class="state-panel compact">
            <strong>暂无最近更新</strong>
          </div>
          <div v-else-if="!homeError" class="recent-grid">
            <article
              v-for="(item, index) in recentItems"
              :key="item.bangumiId"
              class="poster-card recent-card"
              :style="{ '--stagger': stagger(index) }"
              role="link"
              tabindex="0"
              @click="openAnime(item.bangumiId)"
              @keydown.enter="openAnime(item.bangumiId)"
            >
              <div class="poster-frame">
                <span :class="item.isCompleted ? 'completed-tag' : 'new-tag'">
                  {{ item.isCompleted ? '完结' : 'NEW' }}
                </span>
                <img
                  v-if="hasCover(item)"
                  :src="coverURL(item)"
                  :alt="item.title"
                  loading="lazy"
                  @error="markCoverFailed(item)"
                />
                <div v-else class="cover-fallback">
                  <span>{{ item.title.slice(0, 2) }}</span>
                </div>
                <span class="time-pill">{{ formatUpdatedAt(item.updatedAt) }}</span>
              </div>
              <h3 class="poster-title">{{ item.title }}</h3>
              <p class="poster-sub">{{ updateText(item) }}</p>
              <p class="episode-title" :title="item.latestEpisodeTitle || '暂无分集标题'">
                {{ item.latestEpisodeTitle || '暂无分集标题' }}
              </p>
            </article>
          </div>
        </section>

        <!-- ===== 热播推荐 ===== -->
        <section class="anime-section">
          <div class="section-head">
            <div class="section-title">
              <p class="section-kicker">HOT ON AIR</p>
              <h2>热播推荐</h2>
              <i class="section-bar" aria-hidden="true" />
            </div>
            <div class="section-controls">
              <span class="page-count">{{ pageIndicator }}</span>
              <button
                class="arrow-button page-arrow-prev"
                :disabled="!canTurnHot"
                type="button"
                aria-label="上一页"
                @click="turnHotPage(-1)"
              />
              <button
                class="arrow-button page-arrow-next"
                :disabled="!canTurnHot"
                type="button"
                aria-label="下一页"
                @click="turnHotPage(1)"
              />
            </div>
          </div>

          <div v-if="homeLoading" class="poster-grid">
            <article v-for="n in hotSkeletonCount" :key="n" class="poster-card">
              <div class="skeleton-poster skeleton-block" />
              <div class="skeleton-title skeleton-block" />
            </article>
          </div>
          <div v-else-if="homeError" class="state-panel">
            <strong>{{ homeError }}</strong>
            <button type="button" @click="loadHome">重试</button>
          </div>
          <div v-else-if="currentHotItems.length === 0" class="state-panel">
            <strong>暂无热播推荐</strong>
          </div>
          <div v-else class="poster-grid">
            <article
              v-for="(item, index) in currentHotItems"
              :key="item.bangumiId"
              class="poster-card"
              :style="{ '--stagger': stagger(index) }"
              role="link"
              tabindex="0"
              @click="openAnime(item.bangumiId)"
              @keydown.enter="openAnime(item.bangumiId)"
            >
              <div class="poster-frame">
                <span v-if="item.isCompleted" class="completed-tag">完结</span>
                <img
                  v-if="hasCover(item)"
                  :src="coverURL(item)"
                  :alt="item.title"
                  loading="lazy"
                  @error="markCoverFailed(item)"
                />
                <div v-else class="cover-fallback">
                  <span>{{ item.title.slice(0, 2) }}</span>
                </div>
                <span class="score-overlay">{{ ratingText(item.ratingScore) }}</span>
              </div>
              <h3 class="poster-title">{{ item.title }}</h3>
              <p class="poster-sub">{{ updateText(item) }}</p>
              <p class="poster-sub">{{ formatPremiereDate(item.airDate) }}</p>
            </article>
          </div>
        </section>
      </div>
    </section>
  </main>
</template>

<style scoped>
.home-shell {
  position: relative;
  min-width: 1200px;
  min-height: 100vh;
  background:
    linear-gradient(135deg, rgba(255, 244, 248, 0.9), rgba(255, 255, 255, 0.96) 42%, rgba(236, 253, 255, 0.78)),
    repeating-linear-gradient(90deg, rgba(255, 95, 158, 0.08) 0 1px, transparent 1px 52px),
    #ffffff;
}

/* ============ 顶部导航 ============ */
.topbar {
  position: sticky;
  top: 0;
  z-index: 20;
  height: 86px;
  display: grid;
  grid-template-columns: minmax(140px, 1fr) auto minmax(140px, 230px) auto auto auto;
  align-items: center;
  gap: 12px;
  padding: 0 28px;
  background: var(--glass-strong);
  border-bottom: 1px solid var(--line-soft);
  backdrop-filter: blur(18px);
  animation: bp-rise 0.5s var(--ease-out) both;
}

.brand-row {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.brand-text {
  min-width: 0;
}

.brand-text p {
  color: var(--ink-400);
  font-size: 11px;
  letter-spacing: 2px;
}

.brand-text strong {
  display: block;
  max-width: 360px;
  margin-top: 2px;
  overflow: hidden;
  font-size: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 主导航：清透浅粉激活态（替代原纯粉渐变） */
.main-nav {
  display: grid;
  grid-template-columns: repeat(3, auto);
  gap: 4px;
  padding: 5px;
  background: rgba(255, 255, 255, 0.7);
  border: 1px solid var(--line-soft);
  clip-path: polygon(0 0, calc(100% - 14px) 0, 100% 14px, 100% 100%, 14px 100%, 0 calc(100% - 14px));
}

.nav-item {
  position: relative;
  height: 38px;
  padding: 0 16px;
  color: var(--ink-600);
  font-size: 14px;
  background: transparent;
  clip-path: polygon(var(--bevel-sm));
  transition: color 180ms var(--ease-soft), background 180ms var(--ease-soft);
}

.nav-item:hover {
  color: var(--pink-600);
  background: rgba(255, 244, 248, 0.7);
}

.nav-item.active {
  color: var(--pink-600);
}

/* 激活态底部指示条：横向、无倾斜、纯粉色 */
.nav-item.active::after {
  content: '';
  position: absolute;
  left: 50%;
  bottom: 5px;
  width: 22px;
  height: 3px;
  background: var(--pink-500);
  border-radius: 2px;
  transform: translateX(-50%);
}

/* 搜索框 */
.search-box {
  position: relative;
  display: flex;
  align-items: center;
  height: 42px;
  min-width: 0;
  padding: 0 14px 0 42px;
  background: #ffffff;
  border: 1px solid var(--line);
  box-shadow: 0 10px 24px rgba(255, 95, 158, 0.08);
  clip-path: polygon(var(--bevel-chip));
}

.search-symbol {
  position: absolute;
  left: 16px;
  width: 14px;
  height: 14px;
  border: 2px solid var(--pink-300);
  border-radius: 50%;
}

.search-symbol::after {
  content: '';
  position: absolute;
  right: -6px;
  bottom: -4px;
  width: 8px;
  height: 2px;
  background: var(--pink-300);
  transform: rotate(45deg);
}

.search-box input {
  width: 100%;
  color: var(--ink-700);
  font-size: 14px;
}

.search-box input::placeholder {
  color: var(--ink-300);
}

.app-download-link,
.theme-button {
  height: 44px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 0 14px;
  color: #ee3f86;
  font-size: 13px;
  white-space: nowrap;
  background: #ffffff;
  border: 1px solid var(--line-soft);
  box-shadow: 0 10px 24px rgba(255, 95, 158, 0.08);
  clip-path: polygon(var(--bevel-chip));
  transition: color 180ms var(--ease-soft), background 180ms var(--ease-soft), box-shadow 180ms var(--ease-soft), transform 180ms var(--ease-soft);
}

.app-download-link:hover {
  color: var(--pink-600);
  background: var(--pink-50);
  box-shadow: 0 13px 28px rgba(255, 95, 158, 0.16);
  transform: translateY(-1px);
}

.app-download-link:focus-visible {
  outline: 2px solid rgba(238, 63, 134, 0.38);
  outline-offset: 3px;
}

.app-download-icon {
  width: 23px;
  height: 23px;
  flex: 0 0 auto;
}

.theme-area {
  position: relative;
  height: 44px;
}

.theme-area::after {
  content: '';
  position: absolute;
  top: 100%;
  right: 0;
  left: 0;
  height: 9px;
}

.theme-button {
  color: var(--ink-600);
}

.theme-button-icon {
  width: 21px;
  height: 21px;
  flex: 0 0 auto;
}

.theme-button:hover {
  color: var(--pink-600);
  background: var(--pink-50);
}

.theme-button:focus-visible {
  outline: 0;
  box-shadow: inset 0 0 0 2px var(--pink-400);
}

.theme-arrow {
  width: 7px;
  height: 7px;
  margin: 0 2px 4px 3px;
  border-right: 1px solid currentColor;
  border-bottom: 1px solid currentColor;
  transform: rotate(45deg);
  transition: transform 180ms var(--ease-soft);
}

.theme-area:hover .theme-arrow,
.theme-area:focus-within .theme-arrow {
  transform: translateY(3px) rotate(225deg);
}

.theme-menu {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 30;
  width: 156px;
  padding: 7px;
  visibility: hidden;
  opacity: 0;
  background: rgba(255, 255, 255, 0.96);
  border: 1px solid var(--line-soft);
  box-shadow: 0 18px 38px rgba(85, 119, 217, 0.16);
  backdrop-filter: blur(16px);
  clip-path: polygon(0 0, calc(100% - 12px) 0, 100% 12px, 100% 100%, 12px 100%, 0 calc(100% - 12px));
  transform: translateY(-7px);
  transition: opacity 160ms ease, transform 160ms ease, visibility 160ms ease;
}

.theme-menu::before {
  content: '';
  position: absolute;
  right: 0;
  bottom: 100%;
  left: 0;
  height: 9px;
}

.theme-area:hover .theme-menu,
.theme-area:focus-within .theme-menu {
  visibility: visible;
  opacity: 1;
  transform: translateY(0);
}

.theme-menu button {
  position: relative;
  width: 100%;
  height: 40px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 28px 0 11px;
  color: var(--ink-600);
  font-size: 13px;
  text-align: left;
  clip-path: polygon(var(--bevel-sm));
}

.theme-menu button:hover,
.theme-menu button.active {
  color: var(--pink-600);
  background: var(--pink-50);
}

.theme-menu button.active::after {
  content: '';
  position: absolute;
  right: 13px;
  width: 7px;
  height: 7px;
  background: var(--pink-500);
  border-radius: 50%;
  box-shadow: 0 0 0 3px rgba(238, 63, 134, 0.13);
}

.theme-menu button:focus-visible {
  outline: 0;
  box-shadow: inset 0 0 0 2px var(--pink-400);
}

.theme-option-icon {
  width: 19px;
  height: 19px;
  display: block;
  flex: 0 0 19px;
  color: #ee3f86;
}

/* 用户区 */
.user-area {
  position: relative;
  height: 44px;
}

.user-area::after {
  content: '';
  position: absolute;
  top: 100%;
  right: 0;
  left: 0;
  height: 9px;
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 44px;
  padding: 4px 13px 4px 6px;
  background: #ffffff;
  border: 1px solid var(--line-soft);
  box-shadow: 0 10px 24px rgba(255, 95, 158, 0.08);
  clip-path: polygon(0 0, calc(100% - 12px) 0, 100% 12px, 100% 100%, 12px 100%, 0 calc(100% - 12px));
}

.user-arrow {
  width: 8px;
  height: 8px;
  margin: 0 3px 4px 0;
  border-right: 1px solid var(--ink-400);
  border-bottom: 1px solid var(--ink-400);
  transform: rotate(45deg);
  transition: transform 180ms var(--ease-soft);
}

.user-area:hover .user-arrow,
.user-area:focus-within .user-arrow {
  transform: translateY(3px) rotate(225deg);
}

.user-menu {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 30;
  width: 172px;
  padding: 7px;
  visibility: hidden;
  opacity: 0;
  background: rgba(255, 255, 255, 0.96);
  border: 1px solid var(--line-soft);
  box-shadow: 0 18px 38px rgba(85, 119, 217, 0.16);
  backdrop-filter: blur(16px);
  clip-path: polygon(0 0, calc(100% - 12px) 0, 100% 12px, 100% 100%, 12px 100%, 0 calc(100% - 12px));
  transform: translateY(-7px);
  transition: opacity 160ms ease, transform 160ms ease, visibility 160ms ease;
}

.user-menu::before {
  content: '';
  position: absolute;
  right: 0;
  bottom: 100%;
  left: 0;
  height: 9px;
}

.user-area:hover .user-menu,
.user-area:focus-within .user-menu {
  visibility: visible;
  opacity: 1;
  transform: translateY(0);
}

.user-menu button {
  width: 100%;
  height: 38px;
  display: flex;
  align-items: center;
  gap: 11px;
  padding: 0 12px;
  color: var(--ink-600);
  font-size: 13px;
  text-align: left;
  clip-path: polygon(var(--bevel-sm));
}

.user-menu button:hover:not(:disabled) {
  color: var(--pink-600);
  background: var(--pink-50);
}

.user-menu button:disabled {
  opacity: 0.5;
}

.user-menu-icon,
.user-menu-icon-spacer {
  width: 18px;
  height: 18px;
  display: block;
  flex: 0 0 18px;
}

.user-avatar {
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  object-fit: cover;
  background: linear-gradient(135deg, var(--cyan-400), var(--blue-500));
  clip-path: polygon(8px 0, 100% 0, 100% calc(100% - 8px), calc(100% - 8px) 100%, 0 100%, 0 8px);
}

.user-name {
  max-width: 120px;
  overflow: hidden;
  color: var(--ink-900);
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 退出按钮：玻璃描边风（替代原纯粉渐变），hover 才转实粉 */
.logout-button {
  position: relative;
  height: 32px;
  padding: 0 16px;
  overflow: hidden;
  color: var(--pink-600);
  font-size: 13px;
  letter-spacing: 1px;
  background: var(--glass-strong);
  border: 1px solid var(--pink-200);
  clip-path: polygon(var(--bevel-sm));
  transition: color 180ms var(--ease-soft), background 180ms var(--ease-soft);
}

.logout-label {
  position: relative;
  z-index: 2;
}

.logout-sweep {
  position: absolute;
  inset: 0;
  z-index: 1;
  background: linear-gradient(110deg, transparent 0 30%, rgba(255, 255, 255, 0.6) 42%, transparent 56%);
  transform: translateX(-130%);
}

.logout-button:hover:not(:disabled) {
  color: #ffffff;
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600));
}

.logout-button:hover:not(:disabled) .logout-sweep {
  animation: bp-sweep 0.9s var(--ease-soft);
}

/* ============ 舞台 ============ */
.home-stage {
  position: relative;
  min-height: calc(100vh - 86px);
  overflow: hidden;
}

.stage-grid {
  position: absolute;
  inset: 0;
  z-index: 0;
  background:
    linear-gradient(transparent 95%, rgba(85, 119, 217, 0.07) 95%),
    linear-gradient(90deg, transparent 95%, rgba(255, 95, 158, 0.06) 95%);
  background-size: 72px 72px;
  mask-image: linear-gradient(to bottom, rgba(0, 0, 0, 0.7), transparent 86%);
  pointer-events: none;
}

.stage-halo {
  position: absolute;
  z-index: 0;
  border-radius: 50%;
  filter: blur(80px);
  pointer-events: none;
  animation: bp-halo 8s ease-in-out infinite;
}

.halo-a {
  width: 520px;
  height: 520px;
  left: -8%;
  top: 80px;
  background: radial-gradient(circle, rgba(255, 159, 189, 0.42), transparent 70%);
}

.halo-b {
  width: 560px;
  height: 560px;
  right: -6%;
  top: 360px;
  background: radial-gradient(circle, rgba(73, 214, 233, 0.32), transparent 70%);
  animation-delay: 3.5s;
}

.content-wrap {
  position: relative;
  z-index: 4;
  width: min(1440px, calc(100% - 84px));
  margin: 0 auto;
  padding: 34px 0 64px;
}

.content-wrap.without-hero > .anime-section:first-of-type {
  margin-top: 0;
}

/* ============ 首页轮播 ============ */
.hero-carousel {
  position: relative;
  display: grid;
  align-items: center;
  height: 420px;
  overflow: hidden;
  background: linear-gradient(120deg, rgba(255, 255, 255, 0.92), rgba(255, 244, 248, 0.84) 42%, rgba(236, 253, 255, 0.9));
  border: 1px solid var(--line-soft);
  box-shadow: 0 26px 60px rgba(255, 95, 158, 0.12);
  clip-path: polygon(0 0, calc(100% - 30px) 0, 100% 30px, 100% 100%, 30px 100%, 0 calc(100% - 30px));
  animation: bp-rise 0.58s var(--ease-out) 0.04s both;
}

.hero-carousel.has-slide {
  background: #101624;
  border-color: rgba(255, 255, 255, 0.2);
  box-shadow: 0 28px 64px rgba(26, 36, 58, 0.28);
}

.hero-carousel.is-loading {
  background: #111722;
  border-color: rgba(255, 255, 255, 0.14);
  box-shadow: 0 28px 64px rgba(26, 36, 58, 0.24);
}

.hero-carousel.clickable {
  cursor: pointer;
}

.hero-image {
  position: absolute;
  inset: 0;
  z-index: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.hero-shade {
  position: absolute;
  inset: 0;
  z-index: 2;
  pointer-events: none;
  background:
    linear-gradient(90deg, rgba(6, 11, 22, 0.94) 0%, rgba(8, 14, 27, 0.8) 34%, rgba(8, 14, 27, 0.38) 58%, transparent 82%),
    linear-gradient(0deg, rgba(6, 11, 22, 0.28), transparent 56%);
}

.hero-skeleton-backdrop {
  position: absolute;
  inset: 0;
  z-index: 0;
  overflow: hidden;
  background:
    linear-gradient(90deg, rgba(6, 10, 18, 0.96) 0%, rgba(9, 14, 24, 0.82) 40%, rgba(14, 20, 30, 0.5) 72%, rgba(23, 29, 39, 0.34) 100%),
    linear-gradient(135deg, #313844, #1c2330 48%, #39404a);
}

.hero-skeleton-backdrop::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(105deg, transparent 20%, rgba(255, 255, 255, 0.075) 44%, transparent 68%);
  animation: bp-skeleton 1.5s ease-in-out infinite;
}

.hero-skeleton {
  position: relative;
  z-index: 3;
  width: min(620px, 100%);
  padding: 48px 48px 48px 56px;
}

.hero-skeleton-block {
  position: relative;
  display: block;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.14);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.025) inset;
}

.hero-skeleton-block::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(100deg, transparent 18%, rgba(255, 255, 255, 0.18) 46%, transparent 72%);
  animation: bp-skeleton 1.25s ease-in-out infinite;
}

.hero-skeleton .skeleton-tag { width: 78px; height: 28px; margin-bottom: 18px; clip-path: polygon(0 0, 100% 0, calc(100% - 10px) 100%, 0 100%); }
.hero-skeleton .skeleton-kicker { width: 112px; height: 11px; margin-bottom: 15px; }
.hero-skeleton .skeleton-heading { width: min(440px, 88%); height: 34px; }
.hero-skeleton .skeleton-heading.short { width: min(310px, 64%); margin-top: 10px; }
.skeleton-meta-row { display: flex; gap: 10px; margin-top: 21px; }
.skeleton-meta-row .hero-skeleton-block { width: 112px; height: 30px; clip-path: polygon(var(--bevel-tag)); }
.hero-skeleton .skeleton-summary { width: min(520px, 96%); height: 13px; margin-top: 20px; }
.hero-skeleton .skeleton-summary.short { width: min(380px, 72%); margin-top: 10px; }

.hero-content {
  position: relative;
  z-index: 3;
  max-width: 620px;
  padding: 48px 48px 48px 56px;
  animation: bp-hero-in 0.55s var(--ease-out) both;
}

.hero-index-tag {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  margin-bottom: 16px;
  padding: 6px 12px;
  color: var(--ink-900);
  font-size: 12px;
  letter-spacing: 1px;
  background: var(--yellow-300);
  box-shadow: 0 8px 18px rgba(255, 229, 122, 0.4);
  clip-path: polygon(0 0, 100% 0, calc(100% - 12px) 100%, 0 100%);
  transform: rotate(-3deg);
}

.hero-index-tag i {
  font-size: 15px;
  font-style: normal;
}

.hero-index-tag span {
  color: var(--ink-600);
}

.hero-kicker {
  color: var(--pink-600);
  font-size: 12px;
  letter-spacing: 2px;
}

.hero-title {
  margin-top: 12px;
  max-width: 560px;
  min-height: 100px; /* 固定两行高度，避免轮播切换时卡片跳动 */
  color: var(--ink-900);
  font-size: 42px;
  line-height: 1.19;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow: hidden;
}

.hero-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 18px;
}

.meta-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 12px;
  color: var(--ink-700);
  font-size: 13px;
  background: rgba(255, 255, 255, 0.7);
  border: 1px solid rgba(255, 255, 255, 0.9);
  box-shadow: 0 6px 16px rgba(255, 95, 158, 0.08);
  backdrop-filter: blur(8px);
  clip-path: polygon(var(--bevel-tag));
}

.meta-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--pink-500);
}

.meta-star {
  color: var(--yellow-400);
  font-size: 14px;
  font-style: normal;
}

.hero-summary {
  margin-top: 18px;
  max-width: 540px;
  color: var(--ink-600);
  font-size: 15px;
  line-height: 1.8;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  overflow: hidden;
}

.has-slide .hero-kicker {
  color: rgba(255, 255, 255, 0.72);
}

.has-slide .hero-title {
  color: #ffffff;
  text-shadow: 0 3px 16px rgba(0, 0, 0, 0.52);
}

.has-slide .meta-pill {
  color: #ffffff;
  background: rgba(8, 14, 27, 0.42);
  border-color: rgba(255, 255, 255, 0.22);
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.18);
}

.has-slide .hero-summary {
  color: rgba(255, 255, 255, 0.84);
  text-shadow: 0 2px 12px rgba(0, 0, 0, 0.5);
}

/* 切换箭头 */
.hero-arrow {
  position: absolute;
  top: 50%;
  z-index: 5;
  display: grid;
  place-items: center;
  width: 42px;
  height: 42px;
  color: var(--pink-600);
  background: rgba(255, 255, 255, 0.85);
  border: 1px solid rgba(255, 255, 255, 0.9);
  box-shadow: 0 10px 22px rgba(255, 95, 158, 0.12);
  backdrop-filter: blur(8px);
  clip-path: polygon(var(--bevel-sm));
  transform: translateY(-50%);
  transition: background 170ms var(--ease-soft), color 170ms var(--ease-soft), transform 170ms var(--ease-soft);
}

.hero-arrow::before,
.arrow-button::before {
  content: '';
  width: 9px;
  height: 9px;
  border-top: 2px solid currentColor;
  border-right: 2px solid currentColor;
}

.arrow-prev::before,
.page-arrow-prev::before {
  transform: rotate(-135deg);
}

.arrow-next::before,
.page-arrow-next::before {
  transform: rotate(45deg);
}

.arrow-prev {
  left: 14px;
}

.arrow-next {
  right: 14px;
}

.hero-arrow:hover {
  color: #ffffff;
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600));
  transform: translateY(-50%) scale(1.06);
}

/* 指示器 */
.hero-dots {
  position: absolute;
  left: 56px;
  bottom: 30px;
  z-index: 5;
  display: flex;
  gap: 8px;
}

.hero-dot {
  width: 26px;
  height: 4px;
  background: rgba(139, 149, 173, 0.3);
  clip-path: polygon(var(--bevel-sm));
  transition: width 320ms var(--ease-bounce), background 320ms var(--ease-soft);
}

.hero-dot.active {
  width: 48px;
  background: linear-gradient(90deg, var(--pink-500), var(--cyan-400));
}

/* ============ 区块标题 ============ */
.anime-section {
  margin-top: 42px;
  animation: bp-rise 0.58s var(--ease-out) 0.12s both;
}

.recent-section {
  margin-top: 48px;
}

.section-head {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 20px;
}

.section-title {
  position: relative;
}

.section-kicker {
  color: var(--pink-500);
  font-size: 12px;
  letter-spacing: 2px;
}

.section-title h2 {
  margin-top: 4px;
  color: var(--ink-900);
  font-size: 28px;
  font-weight: 900;
}

.section-bar {
  display: block;
  margin-top: 10px;
  width: 56px;
  height: 4px;
  background: linear-gradient(90deg, var(--pink-500), var(--cyan-400));
}

.section-controls {
  display: flex;
  align-items: center;
  gap: 10px;
}

.page-count {
  min-width: 52px;
  color: var(--ink-400);
  font-size: 12px;
  text-align: right;
}

.arrow-button {
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  color: var(--pink-600);
  background: #ffffff;
  border: 1px solid var(--line-soft);
  box-shadow: 0 8px 20px rgba(255, 95, 158, 0.1);
  clip-path: polygon(var(--bevel-sm));
  transition: transform 170ms var(--ease-soft), background 170ms var(--ease-soft), color 170ms var(--ease-soft);
}

.arrow-button:hover:not(:disabled) {
  color: #ffffff;
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600));
  transform: translateY(-2px);
}

.arrow-button:disabled {
  color: var(--ink-300);
  cursor: default;
  filter: grayscale(0.3);
}

/* ============ 海报网格 ============ */
.follow-home-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 20px;
}

.follow-home-skeleton {
  min-width: 0;
}

.skeleton-follow-cover {
  aspect-ratio: 16 / 9;
}

.poster-grid {
  display: grid;
  grid-template-columns: repeat(8, minmax(0, 1fr));
  gap: 18px;
}

.recent-grid {
  display: grid;
  grid-template-columns: repeat(8, minmax(0, 1fr));
  gap: 18px;
}

.poster-card {
  min-width: 0;
  cursor: pointer;
  outline: 0;
  animation: bp-rise 0.42s var(--ease-out) both;
  animation-delay: var(--stagger, 0s);
}

.poster-frame {
  position: relative;
  aspect-ratio: 2 / 3;
  overflow: hidden;
  background: rgba(255, 244, 248, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.82);
  box-shadow: 0 14px 30px rgba(85, 119, 217, 0.1);
  clip-path: polygon(0 0, calc(100% - 16px) 0, 100% 16px, 100% 100%, 16px 100%, 0 calc(100% - 16px));
  transition: transform 220ms var(--ease-soft), box-shadow 220ms var(--ease-soft), filter 220ms var(--ease-soft);
}

.poster-frame::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(105deg, transparent 28%, rgba(255, 255, 255, 0.32) 45%, transparent 62%);
  transform: translateX(-120%);
  transition: transform 460ms var(--ease-out);
  pointer-events: none;
}

.poster-card:hover .poster-frame {
  transform: translateY(-6px);
  box-shadow: 0 26px 50px rgba(255, 95, 158, 0.2);
  filter: saturate(1.08);
}

.poster-card:hover .poster-frame::after {
  transform: translateX(120%);
}

.poster-frame img {
  width: 100%;
  height: 100%;
  object-fit: unset;
}

.poster-card:focus-visible .poster-frame {
  box-shadow: 0 0 0 3px rgba(255, 95, 158, 0.24), 0 26px 50px rgba(255, 95, 158, 0.2);
}

/* 热播海报底部评分：使用渐变衬底保证浅色封面上的可读性 */
.score-overlay {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  z-index: 2;
  display: flex;
  align-items: flex-end;
  justify-content: flex-end;
  min-height: 58px;
  padding: 20px 10px 8px;
  color: #ffffff;
  font-size: 23px;
  font-style: italic;
  line-height: 1;
  background: linear-gradient(to top, rgba(32, 40, 62, 0.76), rgba(32, 40, 62, 0));
  text-shadow: 0 2px 7px rgba(20, 25, 40, 0.85);
}

.poster-title {
  margin-top: 11px;
  overflow: hidden;
  color: var(--ink-900);
  font-size: 14px;
  line-height: 1.45;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.poster-sub {
  margin-top: 4px;
  color: var(--ink-400);
  font-size: 12px;
}

.episode-title {
  margin-top: 3px;
  overflow: hidden;
  color: var(--ink-600);
  font-size: 12px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 封面兜底 */
.cover-fallback {
  display: grid;
  place-items: center;
  width: 100%;
  height: 100%;
  padding: 16px;
  color: var(--pink-600);
  font-size: 22px;
  text-align: center;
  background:
    linear-gradient(135deg, rgba(255, 244, 248, 0.92), rgba(236, 253, 255, 0.82)),
    repeating-linear-gradient(135deg, rgba(255, 95, 158, 0.12) 0 2px, transparent 2px 12px);
}

/* ============ 最近更新专属：NEW 贴片 + 时间玻璃药丸 ============ */
.new-tag {
  position: absolute;
  top: 10px;
  left: 10px;
  z-index: 2;
  display: grid;
  place-items: center;
  min-width: 48px;
  height: 24px;
  padding: 0 10px;
  color: var(--ink-900);
  font-size: 12px;
  letter-spacing: 1px;
  background: var(--yellow-300);
  box-shadow: 0 8px 18px rgba(255, 229, 122, 0.45);
  clip-path: polygon(0 0, 100% 0, calc(100% - 10px) 100%, 0 100%);
  transform: rotate(-3deg);
  animation: bp-tag-in 0.5s var(--ease-bounce) 0.3s both;
}

.time-pill {
  position: absolute;
  bottom: 10px;
  left: 50%;
  display: inline-flex;
  align-items: center;
  height: 24px;
  padding: 0 10px;
  color: #ffffff;
  font-size: 12px;
  white-space: nowrap;
  background: rgba(32, 40, 62, 0.62);
  box-shadow: 0 6px 16px rgba(32, 40, 62, 0.28);
  text-shadow: 0 1px 4px rgba(0, 0, 0, 0.6);
  clip-path: polygon(var(--bevel-sm));
  transform: translateX(-50%);
}

/* ============ 状态面板（错误 / 空） ============ */
.state-panel {
  display: grid;
  place-items: center;
  min-height: 214px;
  gap: 14px;
  color: var(--ink-600);
  background: rgba(255, 255, 255, 0.74);
  border: 1px dashed var(--line);
  clip-path: polygon(0 0, calc(100% - 18px) 0, 100% 18px, 100% 100%, 18px 100%, 0 calc(100% - 18px));
}

.state-panel.compact {
  min-height: 180px;
}

.state-panel strong {
  color: var(--ink-600);
  font-size: 15px;
}

.state-panel button {
  height: 36px;
  padding: 0 20px;
  color: #ffffff;
  font-size: 13px;
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600));
  clip-path: polygon(var(--bevel-sm));
}

/* ============ 骨架屏 ============ */
.skeleton-block {
  position: relative;
  overflow: hidden;
  background: rgba(255, 244, 248, 0.7);
}

.skeleton-block::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(100deg, transparent 20%, rgba(255, 255, 255, 0.7) 45%, transparent 70%);
  animation: bp-skeleton 1.2s ease-in-out infinite;
}

.skeleton-poster {
  aspect-ratio: 2 / 3;
  clip-path: polygon(0 0, calc(100% - 16px) 0, 100% 16px, 100% 100%, 16px 100%, 0 calc(100% - 16px));
}

.skeleton-title {
  width: 82%;
  height: 18px;
  margin-top: 14px;
  clip-path: polygon(var(--bevel-sm));
}

@keyframes bp-skeleton {
  from {
    transform: translateX(-110%);
  }
  to {
    transform: translateX(110%);
  }
}
</style>
