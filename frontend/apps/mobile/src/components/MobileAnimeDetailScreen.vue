<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import {
  api,
  buildAuthenticatedMediaURL,
  type ViewerAnimeActor,
  type ViewerAnimeCharacter,
  type ViewerAnimeDetail,
  type ViewerDetailEpisode,
  type ViewerEpisodeComments,
} from '../api'
import MobileEpisodeCommentItem from './MobileEpisodeCommentItem.vue'
import MobileVideoPlayer from './MobileVideoPlayer.vue'

interface Props {
  bangumiId: number
  initialMediaId: number
  initialPosition: number
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (event: 'back'): void
  (event: 'follow-changed'): void
}>()

const anime = ref<ViewerAnimeDetail | null>(null)
const selectedEpisodeKey = ref('')
const resumePosition = ref(0)
const loading = ref(true)
const errorMessage = ref('')
const followed = ref(false)
const followSaving = ref(false)
const followError = ref('')
const summaryExpanded = ref(false)
const metadataExpanded = ref(false)
const failedImages = ref<Set<string>>(new Set())
const episodeRail = ref<HTMLElement | null>(null)
const episodeSheetOpen = ref(false)
const episodeSheetList = ref<HTMLElement | null>(null)
const detailTab = ref<'info' | 'comments'>('info')
const detailTabPanels = ref<HTMLElement | null>(null)
const detailInfoPanel = ref<HTMLElement | null>(null)
const detailCommentsPanel = ref<HTMLElement | null>(null)
const infoPanelHeight = ref(0)
const commentsPanelHeight = ref(0)
const tabDragging = ref(false)
const tabDragOffset = ref(0)
const tabSwipeWidth = ref(0)
const episodeComments = ref<ViewerEpisodeComments | null>(null)
const commentSmiles = ref<Record<string, string>>({})
const commentsLoading = ref(false)
const commentsError = ref('')
const episodeCardRefs = new Map<string, HTMLElement>()
const commentsCache = new Map<number, { episode: ViewerEpisodeComments; smiles: Record<string, string> }>()
let commentsController: AbortController | null = null
let commentsRequestID = 0
let commentsLoadingMediaID = 0
let tabSwipeStart: {
  x: number
  y: number
  startedAt: number
  direction: 'pending' | 'horizontal' | 'vertical'
} | null = null
let tabPanelResizeObserver: ResizeObserver | null = null
let progressSaving = false
let queuedProgress: { mediaId: number; positionSeconds: number; durationSeconds: number } | null = null

const weekdays = ['', '周一', '周二', '周三', '周四', '周五', '周六', '周日']

const selectedEpisode = computed(() =>
  anime.value?.episodes.find((episode) => episode.key === selectedEpisodeKey.value) ?? null,
)
const selectedCommentCount = computed(() =>
  episodeComments.value?.commentCount ?? selectedEpisode.value?.commentCount ?? 0,
)
const detailTabIndex = computed(() => detailTab.value === 'info' ? 0 : 1)
const detailTabTrackStyle = computed(() => ({
  transform: `translate3d(calc(${-detailTabIndex.value * 100}% + ${tabDragOffset.value}px), 0, 0)`,
}))
const detailTabIndicatorStyle = computed(() => {
  const dragProgress = tabDragging.value && tabSwipeWidth.value
    ? tabDragOffset.value / tabSwipeWidth.value
    : 0
  const position = Math.max(0, Math.min(1, detailTabIndex.value - dragProgress))
  return { transform: `translate3d(${position * 100}%, 0, 0)` }
})
const displayedPanelHeight = computed(() => {
  const heights = [infoPanelHeight.value, commentsPanelHeight.value]
  const currentIndex = detailTabIndex.value
  const currentHeight = heights[currentIndex] ?? 0
  if (!tabDragging.value || !tabDragOffset.value || !tabSwipeWidth.value) return currentHeight

  const targetIndex = currentIndex + (tabDragOffset.value < 0 ? 1 : -1)
  const targetHeight = heights[targetIndex]
  if (!targetHeight) return currentHeight
  const progress = Math.min(Math.abs(tabDragOffset.value) / tabSwipeWidth.value, 1)
  return currentHeight + (targetHeight - currentHeight) * progress
})
const detailTabPanelsStyle = computed(() => {
  const height = Math.ceil(displayedPanelHeight.value)
  return height > 0 ? { height: `${height}px` } : undefined
})
const playableEpisodes = computed(() => anime.value?.episodes.filter((episode) => episode.hasMedia) ?? [])
const playerEpisodes = computed(() =>
  playableEpisodes.value.map((episode) => ({
    key: episode.key,
    mediaId: episode.mediaId,
    label: episode.label,
    title: episode.title || episode.originalTitle,
    summary: episode.summary,
    hasCover: episode.hasCover,
    coverURL: episodeCoverURL(episode),
  })),
)
const streamURL = computed(() => {
  const episode = selectedEpisode.value
  return episode?.hasMedia ? buildAuthenticatedMediaURL(`/api/anime/${props.bangumiId}/media/${episode.mediaId}/stream`) : ''
})
const playerPoster = computed(() => {
  const episode = selectedEpisode.value
  return episode?.hasMedia && episode.hasCover
    ? buildAuthenticatedMediaURL(`/api/anime/${props.bangumiId}/media/${episode.mediaId}/cover`)
    : ''
})
const playerTitle = computed(() => {
  const episode = selectedEpisode.value
  if (!episode) return anime.value?.title ?? '番剧播放'
  return `${episode.label} · ${episode.title || anime.value?.title || '番剧播放'}`
})
const basicFacts = computed(() => {
  const detail = anime.value
  if (!detail) return []
  return [
    formatAirDate(detail.airDate),
    weekdays[detail.airWeekday] || '播出日未定',
    detail.totalEpisodes > 0 ? `全 ${detail.totalEpisodes} 话` : '话数未定',
  ]
})
const metadataItems = computed(() => {
  const detail = anime.value
  if (!detail) return []
  const items = [
    { key: '开播日期', value: formatAirDate(detail.airDate) },
    { key: '播出星期', value: weekdays[detail.airWeekday] || '未定' },
    { key: '播放平台', value: detail.platform || '未定' },
    { key: '总话数', value: detail.totalEpisodes > 0 ? `${detail.totalEpisodes} 话` : '未定' },
    { key: 'Bangumi 评分', value: detail.ratingScore === null ? '暂无评分' : detail.ratingScore.toFixed(1) },
  ]
  for (const entry of detail.infobox.slice(0, 14)) {
    const key = String(entry.key ?? '').trim()
    const value = formatInfoValue(entry.value)
    if (key && value && !items.some((item) => item.key === key)) {
      items.push({ key, value })
    }
  }
  return items
})
const displayMetaTags = computed(() => {
  const seen = new Set<string>()
  return (anime.value?.metaTags ?? []).filter((tag) => {
    const key = normalizeTagName(tag)
    if (!key || seen.has(key)) return false
    seen.add(key)
    return true
  })
})
const displayTags = computed(() => {
  const metaTagNames = new Set(displayMetaTags.value.map(normalizeTagName))
  const seen = new Set<string>()
  return (anime.value?.tags ?? []).filter((tag) => {
    const key = normalizeTagName(tag.name)
    if (!key || metaTagNames.has(key) || seen.has(key)) return false
    seen.add(key)
    return true
  })
})

onMounted(loadDetail)
onMounted(() => {
  tabPanelResizeObserver = new ResizeObserver(measureDetailPanelHeights)
  observeDetailPanels()
})
watch(() => props.bangumiId, loadDetail)
watch([detailInfoPanel, detailCommentsPanel], observeDetailPanels, { flush: 'post' })
watch(() => selectedEpisode.value?.mediaId ?? 0, (mediaID, previousMediaID) => {
  if (mediaID === previousMediaID) return
  if (mediaID > 0) {
    void loadEpisodeComments(mediaID)
    return
  }
  resetEpisodeComments()
})
onBeforeUnmount(() => {
  commentsController?.abort()
  tabPanelResizeObserver?.disconnect()
})

async function loadDetail() {
  commentsCache.clear()
  resetEpisodeComments()
  detailTab.value = 'info'
  selectedEpisodeKey.value = ''
  loading.value = true
  errorMessage.value = ''
  followError.value = ''
  summaryExpanded.value = false
  metadataExpanded.value = false
  episodeSheetOpen.value = false
  episodeCardRefs.clear()
  try {
    const result = await api.animeDetail(props.bangumiId)
    anime.value = result.anime
    followed.value = result.followed
    const requestedMediaID = props.initialMediaId || result.watchProgress?.mediaId || 0
    const requestedEpisode = result.anime.episodes.find(
      (episode) => episode.hasMedia && episode.mediaId === requestedMediaID,
    )
    const defaultEpisode = requestedEpisode ?? result.anime.episodes.find((episode) => episode.hasMedia)
    selectedEpisodeKey.value = defaultEpisode?.key ?? ''
    if (requestedEpisode && props.initialMediaId) {
      resumePosition.value = Math.max(props.initialPosition, 0)
    } else if (requestedEpisode && result.watchProgress) {
      resumePosition.value = result.watchProgress.completed ? 0 : Math.max(result.watchProgress.positionSeconds, 0)
    } else {
      resumePosition.value = 0
    }
    failedImages.value = new Set()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '番剧详情加载失败'
  } finally {
    loading.value = false
    void scrollSelectedEpisodeIntoView('auto')
  }
}

async function toggleFollow() {
  if (followSaving.value) return
  followSaving.value = true
  followError.value = ''
  try {
    const result = await api.updateAnimeFollow(props.bangumiId, !followed.value)
    followed.value = result.followed
    emit('follow-changed')
  } catch (error) {
    followError.value = error instanceof Error ? error.message : '追番状态更新失败'
  } finally {
    followSaving.value = false
  }
}

function selectEpisode(episode: ViewerDetailEpisode) {
  if (!episode.hasMedia) return
  selectedEpisodeKey.value = episode.key
  resumePosition.value = 0
  void scrollSelectedEpisodeIntoView('smooth')
}

function selectEpisodeByKey(key: string) {
  const episode = anime.value?.episodes.find((item) => item.key === key)
  if (episode) selectEpisode(episode)
}

function selectDetailTab(tab: 'info' | 'comments') {
  detailTab.value = tab
  if (tab === 'comments' && selectedEpisode.value?.mediaId && !episodeComments.value && !commentsLoading.value) {
    void loadEpisodeComments(selectedEpisode.value.mediaId)
  }
}

async function loadEpisodeComments(mediaID = selectedEpisode.value?.mediaId ?? 0, force = false) {
  if (!mediaID) {
    resetEpisodeComments()
    return
  }
  if (!force && commentsLoadingMediaID === mediaID) return

  commentsController?.abort()
  commentsController = null
  const requestID = ++commentsRequestID
  commentsLoadingMediaID = 0

  const cached = commentsCache.get(mediaID)
  if (cached && !force) {
    episodeComments.value = cached.episode
    commentSmiles.value = cached.smiles
    commentsLoading.value = false
    commentsError.value = ''
    return
  }

  const controller = new AbortController()
  commentsController = controller
  commentsLoadingMediaID = mediaID
  commentsLoading.value = true
  commentsError.value = ''
  if (!cached) {
    episodeComments.value = null
    commentSmiles.value = {}
  }

  try {
    const result = await api.episodeComments(props.bangumiId, mediaID, controller.signal)
    if (requestID !== commentsRequestID || selectedEpisode.value?.mediaId !== mediaID) return
    const smiles = authenticatedSmileURLs(result.smiles)
    commentsCache.set(mediaID, { episode: result.episode, smiles })
    episodeComments.value = result.episode
    commentSmiles.value = smiles
  } catch (error) {
    if (controller.signal.aborted || requestID !== commentsRequestID) return
    commentsError.value = error instanceof Error ? error.message : '评论加载失败'
  } finally {
    if (requestID === commentsRequestID) {
      commentsController = null
      commentsLoadingMediaID = 0
      commentsLoading.value = false
    }
  }
}

function retryEpisodeComments() {
  void loadEpisodeComments(selectedEpisode.value?.mediaId ?? 0, true)
}

function resetEpisodeComments() {
  commentsController?.abort()
  commentsController = null
  commentsRequestID++
  commentsLoadingMediaID = 0
  commentsLoading.value = false
  commentsError.value = ''
  episodeComments.value = null
  commentSmiles.value = {}
}

function authenticatedSmileURLs(smiles: Record<string, string>) {
  const result: Record<string, string> = {}
  for (const [code, source] of Object.entries(smiles)) {
    result[code] = source.startsWith('/') ? buildAuthenticatedMediaURL(source) : source
  }
  return result
}

function startTabSwipe(event: TouchEvent) {
  const target = event.target
  if (target instanceof Element && target.closest('button, a, input, [role="button"], .episode-rail')) {
    tabSwipeStart = null
    return
  }
  const touch = event.touches[0]
  if (!touch) return
  tabSwipeWidth.value = detailTabPanels.value?.clientWidth || window.innerWidth
  tabSwipeStart = {
    x: touch.clientX,
    y: touch.clientY,
    startedAt: performance.now(),
    direction: 'pending',
  }
}

function moveTabSwipe(event: TouchEvent) {
  const start = tabSwipeStart
  if (!start) return
  const touch = event.touches[0]
  if (!touch) return
  const horizontal = touch.clientX - start.x
  const vertical = touch.clientY - start.y

  if (start.direction === 'pending') {
    if (Math.hypot(horizontal, vertical) < 8) return
    start.direction = Math.abs(horizontal) > Math.abs(vertical) * 1.08 ? 'horizontal' : 'vertical'
    if (start.direction === 'vertical') return
    tabDragging.value = true
  }
  if (start.direction !== 'horizontal') return

  const beyondFirstPanel = detailTab.value === 'info' && horizontal > 0
  const beyondLastPanel = detailTab.value === 'comments' && horizontal < 0
  tabDragOffset.value = beyondFirstPanel || beyondLastPanel ? horizontal * 0.18 : horizontal
  event.preventDefault()
}

function finishTabSwipe(event: TouchEvent) {
  const start = tabSwipeStart
  const touch = event.changedTouches[0]
  if (!start || start.direction !== 'horizontal' || !touch) {
    cancelTabSwipe()
    return
  }

  const horizontal = touch.clientX - start.x
  const elapsed = Math.max(performance.now() - start.startedAt, 1)
  const velocity = Math.abs(horizontal) / elapsed
  const distanceThreshold = Math.max(tabSwipeWidth.value * 0.22, 64)
  const shouldSwitch = Math.abs(horizontal) >= distanceThreshold || velocity >= 0.55
  const canSwitchToComments = horizontal < 0 && detailTab.value === 'info'
  const canSwitchToInfo = horizontal > 0 && detailTab.value === 'comments'

  tabSwipeStart = null
  tabDragging.value = false
  tabDragOffset.value = 0
  if (shouldSwitch && canSwitchToComments) selectDetailTab('comments')
  if (shouldSwitch && canSwitchToInfo) selectDetailTab('info')
}

function cancelTabSwipe() {
  tabSwipeStart = null
  tabDragging.value = false
  tabDragOffset.value = 0
}

function observeDetailPanels() {
  if (!tabPanelResizeObserver) return
  tabPanelResizeObserver.disconnect()
  if (detailInfoPanel.value) tabPanelResizeObserver.observe(detailInfoPanel.value)
  if (detailCommentsPanel.value) tabPanelResizeObserver.observe(detailCommentsPanel.value)
  measureDetailPanelHeights()
}

function measureDetailPanelHeights() {
  infoPanelHeight.value = detailInfoPanel.value ? Math.ceil(detailInfoPanel.value.scrollHeight) : 0
  commentsPanelHeight.value = detailCommentsPanel.value ? Math.ceil(detailCommentsPanel.value.scrollHeight) : 0
}

async function openEpisodeSheet() {
  if (!anime.value?.episodes.length) return
  episodeSheetOpen.value = true
  await nextTick()
  window.requestAnimationFrame(() => scrollSelectedSheetEpisodeIntoView('auto'))
}

function closeEpisodeSheet() {
  episodeSheetOpen.value = false
}

function selectEpisodeFromSheet(episode: ViewerDetailEpisode) {
  if (!episode.hasMedia) return
  closeEpisodeSheet()
  selectEpisode(episode)
}

function scrollSelectedSheetEpisodeIntoView(behavior: ScrollBehavior) {
  const list = episodeSheetList.value
  const selectedItem = list?.querySelector<HTMLElement>('.episode-sheet-item.selected')
  if (!list || !selectedItem) return

  const listBounds = list.getBoundingClientRect()
  const itemBounds = selectedItem.getBoundingClientRect()
  const target = list.scrollTop + itemBounds.top - listBounds.top - (list.clientHeight - itemBounds.height) / 2
  list.scrollTo({ top: Math.max(target, 0), behavior })
}

function setEpisodeCardRef(key: string, element: unknown) {
  if (element instanceof HTMLElement) {
    episodeCardRefs.set(key, element)
    return
  }
  episodeCardRefs.delete(key)
}

async function scrollSelectedEpisodeIntoView(behavior: ScrollBehavior) {
  const key = selectedEpisodeKey.value
  if (!key) return
  await nextTick()
  window.requestAnimationFrame(() => {
    const rail = episodeRail.value
    const card = episodeCardRefs.get(key)
    if (!rail || !card) return

    const railBounds = rail.getBoundingClientRect()
    const cardBounds = card.getBoundingClientRect()
    const target = rail.scrollLeft + cardBounds.left - railBounds.left - (rail.clientWidth - cardBounds.width) / 2
    const maxScrollLeft = Math.max(rail.scrollWidth - rail.clientWidth, 0)
    rail.scrollTo({ left: Math.max(0, Math.min(target, maxScrollLeft)), behavior })
  })
}

async function saveProgress(progress: { mediaId: number; positionSeconds: number; durationSeconds: number }) {
  queuedProgress = progress
  if (progressSaving) return
  progressSaving = true
  while (queuedProgress) {
    const current = queuedProgress
    queuedProgress = null
    try {
      await api.updateWatchProgress(
        props.bangumiId,
        current.mediaId,
        current.positionSeconds,
        current.durationSeconds,
      )
    } catch {
      // 播放进度记录失败不阻断播放体验，下一次上报会继续尝试。
    }
  }
  progressSaving = false
}

function animeCoverURL() {
  return buildAuthenticatedMediaURL(`/api/anime/${props.bangumiId}/cover`)
}

function episodeCoverURL(episode: ViewerDetailEpisode) {
  return buildAuthenticatedMediaURL(`/api/anime/${props.bangumiId}/media/${episode.mediaId}/cover`)
}

function characterImageURL(character: ViewerAnimeCharacter) {
  return buildAuthenticatedMediaURL(`/api/anime/${props.bangumiId}/characters/${character.characterId}/image`)
}

function actorImageURL(actor: ViewerAnimeActor) {
  return buildAuthenticatedMediaURL(`/api/actors/${actor.actorId}/image`)
}

function imageAvailable(key: string, available: boolean) {
  return available && !failedImages.value.has(key)
}

function markImageFailed(key: string) {
  const next = new Set(failedImages.value)
  next.add(key)
  failedImages.value = next
}

function episodeAvailability(episode: ViewerDetailEpisode) {
  return availabilityText(episode.airDate || anime.value?.airDate || '')
}

function availabilityText(airDate: string) {
  if (!airDate) return '开播时间未定'
  const premiere = new Date(`${airDate}T00:00:00`)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  return !Number.isNaN(premiere.getTime()) && premiere > today ? '尚未开播' : '尚未放流'
}

function formatAirDate(value: string) {
  return value ? value.replaceAll('-', '.') : '日期未定'
}

function normalizeTagName(value: string) {
  return value.trim().normalize('NFKC').toLocaleLowerCase()
}

function formatInfoValue(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value)) {
    return value.map((item) => formatInfoValue(item)).filter(Boolean).join('、')
  }
  if (typeof value === 'object') {
    const object = value as Record<string, unknown>
    if (object.v !== undefined) return formatInfoValue(object.v)
    if (object.value !== undefined) return formatInfoValue(object.value)
    return Object.values(object).map((item) => formatInfoValue(item)).filter(Boolean).join(' ')
  }
  return ''
}
</script>

<template>
  <section class="detail-screen" aria-label="番剧详情播放页">
    <div v-if="loading" class="detail-skeleton" aria-label="正在读取番剧详情" aria-busy="true">
      <div class="detail-skeleton-player detail-skeleton-shimmer">
        <button class="floating-back skeleton-back" type="button" aria-label="返回" @click="emit('back')">‹</button>
        <div class="detail-skeleton-player-copy" aria-hidden="true">
          <span />
          <span />
        </div>
        <div class="detail-skeleton-player-controls" aria-hidden="true">
          <span class="skeleton-play-button" />
          <span class="skeleton-timeline" />
          <span class="skeleton-time" />
        </div>
      </div>

      <section class="detail-skeleton-title" aria-hidden="true">
        <div>
          <span class="detail-skeleton-line title detail-skeleton-shimmer" />
          <span class="detail-skeleton-line subtitle detail-skeleton-shimmer" />
        </div>
        <span class="detail-skeleton-follow detail-skeleton-shimmer" />
      </section>

      <div class="detail-skeleton-facts" aria-hidden="true">
        <span v-for="index in 3" :key="`skeleton-fact-${index}`" class="detail-skeleton-pill detail-skeleton-shimmer" />
      </div>

      <section class="detail-skeleton-card" aria-hidden="true">
        <span class="detail-skeleton-heading detail-skeleton-shimmer" />
        <div class="detail-skeleton-copy">
          <span class="detail-skeleton-line detail-skeleton-shimmer" />
          <span class="detail-skeleton-line detail-skeleton-shimmer" />
          <span class="detail-skeleton-line short detail-skeleton-shimmer" />
        </div>
      </section>

      <section class="detail-skeleton-card" aria-hidden="true">
        <div class="detail-skeleton-card-head">
          <span class="detail-skeleton-heading detail-skeleton-shimmer" />
          <span class="detail-skeleton-count detail-skeleton-shimmer" />
        </div>
        <div class="detail-skeleton-episodes">
          <article v-for="index in 3" :key="`skeleton-episode-${index}`">
            <div class="detail-skeleton-episode-cover detail-skeleton-shimmer" />
            <span class="detail-skeleton-line episode-label detail-skeleton-shimmer" />
            <span class="detail-skeleton-line episode-title detail-skeleton-shimmer" />
          </article>
        </div>
      </section>
    </div>

    <div v-else-if="errorMessage" class="detail-state error">
      <span>!</span>
      <p>{{ errorMessage }}</p>
      <div>
        <button type="button" @click="emit('back')">返回</button>
        <button type="button" @click="loadDetail">重试</button>
      </div>
    </div>

    <div v-else-if="anime" class="detail-content">
      <div class="playback-sticky">
      <div class="player-wrap">
        <MobileVideoPlayer
          v-if="selectedEpisode"
          :media-id="selectedEpisode.mediaId"
          :src="streamURL"
          :poster="playerPoster"
          :title="playerTitle"
          :start-time="resumePosition"
          :op-skip="selectedEpisode.opSkip"
          :episodes="playerEpisodes"
          :selected-episode-key="selectedEpisodeKey"
          @progress="saveProgress"
          @open-episode-sheet="openEpisodeSheet"
          @select-episode="selectEpisodeByKey"
        />
        <div v-else class="player-empty">
          <img
            v-if="imageAvailable('anime-cover', anime.hasCover)"
            :src="animeCoverURL()"
            alt=""
            @error="markImageFailed('anime-cover')"
          />
          <div class="empty-shade" />
          <button class="player-back" type="button" aria-label="返回" @click="emit('back')">‹</button>
          <p>{{ availabilityText(anime.airDate) }}</p>
          <span>{{ anime.episodes.length ? '当前没有可播放的成品视频' : '分集信息尚未收录' }}</span>
        </div>
        <button class="floating-back" type="button" aria-label="返回" @click="emit('back')">‹</button>
      </div>

      <nav class="detail-tabs" role="tablist" aria-label="播放器页面内容">
        <button
          id="mobile-detail-info-tab"
          class="detail-tab"
          :class="{ active: detailTab === 'info' }"
          type="button"
          role="tab"
          :aria-selected="detailTab === 'info'"
          aria-controls="mobile-detail-info-panel"
          @click="selectDetailTab('info')"
        >
          <span>番剧信息</span>
        </button>
        <button
          id="mobile-detail-comments-tab"
          class="detail-tab"
          :class="{ active: detailTab === 'comments' }"
          type="button"
          role="tab"
          :aria-selected="detailTab === 'comments'"
          aria-controls="mobile-detail-comments-panel"
          @click="selectDetailTab('comments')"
        >
          <span>该集吐槽</span>
          <em v-if="selectedCommentCount > 0">{{ selectedCommentCount }}</em>
          <i v-else-if="commentsLoading" aria-label="正在加载评论" />
        </button>
        <span
          class="detail-tab-indicator"
          :class="{ dragging: tabDragging }"
          :style="detailTabIndicatorStyle"
          aria-hidden="true"
        ><i /></span>
      </nav>
      </div>

      <div
        ref="detailTabPanels"
        class="detail-tab-panels"
        :class="{ dragging: tabDragging }"
        :style="detailTabPanelsStyle"
        @touchstart.passive="startTabSwipe"
        @touchmove="moveTabSwipe"
        @touchend.passive="finishTabSwipe"
        @touchcancel.passive="cancelTabSwipe"
      >
        <div class="detail-tab-track" :class="{ dragging: tabDragging }" :style="detailTabTrackStyle">
        <div
          ref="detailInfoPanel"
          id="mobile-detail-info-panel"
          class="detail-tab-panel detail-info-panel"
          :class="{ active: detailTab === 'info' }"
          role="tabpanel"
          aria-labelledby="mobile-detail-info-tab"
          :aria-hidden="detailTab !== 'info'"
          :inert="detailTab !== 'info'"
        >
      <section class="title-section">
        <div class="title-copy">
          <p class="detail-title">{{ anime.title }}</p>
          <span v-if="anime.originalTitle && anime.originalTitle !== anime.title">{{ anime.originalTitle }}</span>
        </div>
        <button
          class="follow-button"
          :class="{ followed }"
          type="button"
          :disabled="followSaving"
          @click="toggleFollow"
        >
          {{ followed ? '已追番' : '追番' }}
        </button>
      </section>
      <p v-if="followError" class="follow-error">{{ followError }}</p>

      <div class="fact-row">
        <span v-for="fact in basicFacts" :key="fact">{{ fact }}</span>
      </div>

      <section class="detail-block summary-block">
        <div class="block-title">简介</div>
        <div class="summary-box" :class="{ expanded: summaryExpanded }">
          <p>{{ anime.summary || '该番剧暂无剧情简介。' }}</p>
        </div>
        <div
          class="summary-arrow"
          :class="{ expanded: summaryExpanded }"
          role="button"
          tabindex="0"
          :aria-label="summaryExpanded ? '收起简介' : '展开简介'"
          @click="summaryExpanded = !summaryExpanded"
          @keydown.enter.prevent="summaryExpanded = !summaryExpanded"
          @keydown.space.prevent="summaryExpanded = !summaryExpanded"
        />
      </section>

      <section class="detail-block">
        <div class="block-head">
          <div class="block-title">选集</div>
          <button
            class="episode-list-trigger"
            type="button"
            :disabled="!anime.episodes.length"
            aria-haspopup="dialog"
            :aria-expanded="episodeSheetOpen"
            aria-controls="mobile-episode-sheet"
            @click="openEpisodeSheet"
          >
            <span>{{ playableEpisodes.length }} / {{ anime.episodes.length }}</span>
            <i aria-hidden="true" />
          </button>
        </div>
        <div v-if="anime.episodes.length" ref="episodeRail" class="episode-rail">
          <article
            v-for="episode in anime.episodes"
            :key="episode.key"
            :ref="(element) => setEpisodeCardRef(episode.key, element)"
            class="episode-card"
            :class="{ selected: selectedEpisodeKey === episode.key, unavailable: !episode.hasMedia }"
            role="button"
            tabindex="0"
            @click="selectEpisode(episode)"
            @keydown.enter.prevent="selectEpisode(episode)"
            @keydown.space.prevent="selectEpisode(episode)"
          >
            <div class="episode-cover">
              <img
                v-if="imageAvailable(`episode-${episode.mediaId}`, episode.hasCover)"
                :src="episodeCoverURL(episode)"
                :alt="episode.title || episode.label"
                loading="lazy"
                @error="markImageFailed(`episode-${episode.mediaId}`)"
              />
              <div v-else class="episode-fallback">{{ episode.label }}</div>
              <span v-if="!episode.hasMedia">{{ episodeAvailability(episode) }}</span>
            </div>
            <p>{{ episode.label }}</p>
            <small>{{ episode.title || episode.originalTitle || episode.label }}</small>
          </article>
        </div>
        <div v-else class="detail-empty">暂无分集信息</div>
      </section>

      <section class="detail-block">
        <div class="block-title">标签</div>
        <div class="tag-cloud">
          <span v-for="tag in displayMetaTags" :key="`meta-${tag}`" class="meta-tag">{{ tag }}</span>
          <span v-for="tag in displayTags" :key="tag.name">{{ tag.name }}</span>
        </div>
      </section>

      <section class="detail-block">
        <div class="block-title">元数据</div>
        <div class="metadata-box" :class="{ expanded: metadataExpanded }">
          <dl class="metadata-list">
            <div v-for="item in metadataItems" :key="item.key">
              <dt>{{ item.key }}</dt>
              <dd>{{ item.value }}</dd>
            </div>
          </dl>
        </div>
        <div
          class="summary-arrow"
          :class="{ expanded: metadataExpanded }"
          role="button"
          tabindex="0"
          :aria-label="metadataExpanded ? '收起元数据' : '展开元数据'"
          @click="metadataExpanded = !metadataExpanded"
          @keydown.enter.prevent="metadataExpanded = !metadataExpanded"
          @keydown.space.prevent="metadataExpanded = !metadataExpanded"
        />
      </section>

      <section class="detail-block cast-block">
        <div class="block-head">
          <div class="block-title">角色与声优</div>
          <span>{{ anime.characters.length }} 位角色</span>
        </div>
        <div v-if="anime.characters.length" class="character-list">
          <article v-for="character in anime.characters" :key="character.characterId" class="character-card">
            <div class="character-image">
              <img
                v-if="imageAvailable(`character-${character.characterId}`, character.hasImage)"
                :src="characterImageURL(character)"
                :alt="character.name"
                loading="lazy"
                @error="markImageFailed(`character-${character.characterId}`)"
              />
              <span v-else>{{ character.name.slice(0, 1) }}</span>
            </div>
            <div class="character-main">
              <span class="relation-tag">{{ character.relation || '角色' }}</span>
              <p>{{ character.name }}</p>
              <small>{{ character.summary || '暂无角色简介。' }}</small>
              <div v-if="character.actors.length" class="actor-row">
                <div v-for="actor in character.actors" :key="actor.actorId" class="actor-chip">
                  <img
                    v-if="imageAvailable(`actor-${actor.actorId}`, actor.hasImage)"
                    :src="actorImageURL(actor)"
                    :alt="actor.name"
                    loading="lazy"
                    @error="markImageFailed(`actor-${actor.actorId}`)"
                  />
                  <span v-else>CV</span>
                  <p>{{ actor.name }}</p>
                </div>
              </div>
            </div>
          </article>
        </div>
        <div v-else class="detail-empty">暂无角色与声优资料</div>
      </section>
        </div>

        <section
          ref="detailCommentsPanel"
          id="mobile-detail-comments-panel"
          class="detail-tab-panel episode-comments-panel"
          :class="{ active: detailTab === 'comments' }"
          role="tabpanel"
          aria-labelledby="mobile-detail-comments-tab"
          :aria-busy="commentsLoading"
          :aria-hidden="detailTab !== 'comments'"
          :inert="detailTab !== 'comments'"
        >
          <header class="comments-heading">
            <div>
              <span>{{ selectedEpisode?.label || '当前选集' }}</span>
              <h2>{{ selectedEpisode?.title || selectedEpisode?.originalTitle || anime.title }}</h2>
            </div>
            <small v-if="episodeComments">共 {{ episodeComments.totalCount }} 条内容</small>
            <small v-else-if="selectedCommentCount > 0">{{ selectedCommentCount }} 条吐槽</small>
          </header>

          <div v-if="commentsLoading && !episodeComments" class="comments-loading-list" aria-hidden="true">
            <article v-for="index in 3" :key="`comment-skeleton-${index}`">
              <i />
              <div><span /><span /></div>
              <p />
            </article>
          </div>
          <div v-else-if="commentsError" class="comments-state comments-error-state">
            <span>!</span>
            <p>{{ commentsError }}</p>
            <button type="button" @click="retryEpisodeComments">重新加载</button>
          </div>
          <div v-else-if="episodeComments?.comments.length" class="comments-list">
            <MobileEpisodeCommentItem
              v-for="comment in episodeComments.comments"
              :key="comment.commentId"
              :comment="comment"
              :smiles="commentSmiles"
            />
          </div>
          <div v-else class="comments-state">
            <span>···</span>
            <p v-if="!selectedEpisode">请先选择一个可播放话数</p>
            <p v-else-if="episodeComments?.syncStatus === 'not_started' || episodeComments?.syncStatus === 'pending'">
              本话评论正在等待首次同步
            </p>
            <p v-else-if="episodeComments?.syncStatus === 'not_found'">Bangumi 暂无该话评论数据</p>
            <p v-else>本话暂时没有评论</p>
          </div>
        </section>
        </div>
      </div>
    </div>

    <Teleport to="body">
      <Transition name="episode-sheet">
        <div
          v-if="episodeSheetOpen && anime"
          class="episode-sheet-layer"
          role="presentation"
          @click.self="closeEpisodeSheet"
        >
          <section
            id="mobile-episode-sheet"
            class="episode-sheet-panel"
            role="dialog"
            aria-modal="true"
            aria-label="选集"
            @keydown.esc="closeEpisodeSheet"
          >
            <div class="episode-sheet-handle" aria-hidden="true" />
            <header>
              <div>
                <h2>选集</h2>
                <span>{{ playableEpisodes.length }} / {{ anime.episodes.length }} 集可播放</span>
              </div>
              <button type="button" aria-label="关闭选集列表" @click="closeEpisodeSheet">×</button>
            </header>

            <div ref="episodeSheetList" class="episode-sheet-list">
              <button
                v-for="episode in anime.episodes"
                :key="episode.key"
                class="episode-sheet-item"
                :class="{ selected: selectedEpisodeKey === episode.key, unavailable: !episode.hasMedia }"
                type="button"
                :disabled="!episode.hasMedia"
                :aria-current="selectedEpisodeKey === episode.key ? 'true' : undefined"
                @click="selectEpisodeFromSheet(episode)"
              >
                <div class="episode-sheet-thumb">
                  <img
                    v-if="imageAvailable(`episode-${episode.mediaId}`, episode.hasCover)"
                    :src="episodeCoverURL(episode)"
                    :alt="episode.title || episode.label"
                    loading="lazy"
                    @error="markImageFailed(`episode-${episode.mediaId}`)"
                  />
                  <span v-else>{{ episode.label }}</span>
                </div>
                <div class="episode-sheet-copy">
                  <div class="episode-sheet-meta">
                    <span>{{ episode.label }}</span>
                    <i v-if="selectedEpisodeKey === episode.key">正在播放</i>
                    <i v-else-if="!episode.hasMedia">{{ episodeAvailability(episode) }}</i>
                  </div>
                  <strong>{{ episode.title || episode.originalTitle || episode.label }}</strong>
                  <p>{{ episode.summary || '该话暂无剧情简介。' }}</p>
                </div>
              </button>
            </div>
          </section>
        </div>
      </Transition>
    </Teleport>
  </section>
</template>

<style scoped>
.detail-screen {
  min-height: 100vh;
  min-height: 100dvh;
  color: var(--ink-900);
  background:
    linear-gradient(180deg, #090d17 0 214px, rgba(255, 244, 248, 0.84) 214px, #f6f7fb 55%),
    #f6f7fb;
}

.detail-content {
  padding-bottom: calc(26px + env(safe-area-inset-bottom));
}

.detail-skeleton {
  min-height: 100vh;
  min-height: 100dvh;
  padding-bottom: calc(26px + env(safe-area-inset-bottom));
  background: #f6f7fb;
}

.detail-skeleton-shimmer {
  position: relative;
  overflow: hidden;
  background: #edf0f5;
}

.detail-skeleton-shimmer::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(100deg, transparent 18%, rgba(255, 255, 255, 0.82) 45%, transparent 72%);
  animation: detail-skeleton-sweep 1.1s ease-in-out infinite;
}

.detail-skeleton-player {
  position: relative;
  width: 100%;
  min-height: 210px;
  aspect-ratio: 16 / 9;
  background: #101522;
}

.detail-skeleton-player::after {
  background: linear-gradient(100deg, transparent 18%, rgba(255, 255, 255, 0.11) 45%, transparent 72%);
}

.skeleton-back {
  background: rgba(255, 255, 255, 0.12);
}

.detail-skeleton-player-copy {
  position: absolute;
  top: calc(19px + env(safe-area-inset-top));
  right: 18px;
  left: 62px;
  z-index: 1;
  display: grid;
  gap: 7px;
}

.detail-skeleton-player-copy span {
  width: 48%;
  height: 9px;
  background: rgba(255, 255, 255, 0.16);
  border-radius: 999px;
}

.detail-skeleton-player-copy span:last-child {
  width: 31%;
  height: 7px;
  background: rgba(255, 255, 255, 0.1);
}

.detail-skeleton-player-controls {
  position: absolute;
  right: 16px;
  bottom: 17px;
  left: 16px;
  z-index: 1;
  display: flex;
  align-items: center;
  gap: 10px;
}

.skeleton-play-button {
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  background: rgba(255, 255, 255, 0.16);
  border-radius: 50%;
}

.skeleton-timeline {
  height: 4px;
  flex: 1;
  background: rgba(255, 255, 255, 0.14);
  border-radius: 999px;
}

.skeleton-time {
  width: 48px;
  height: 8px;
  flex: 0 0 auto;
  background: rgba(255, 255, 255, 0.12);
  border-radius: 999px;
}

.detail-skeleton-title {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  align-items: start;
  padding: 18px 16px 0;
}

.detail-skeleton-line {
  display: block;
  width: 100%;
  height: 11px;
  border-radius: 999px;
}

.detail-skeleton-line.title {
  width: min(78%, 250px);
  height: 21px;
}

.detail-skeleton-line.subtitle {
  width: min(56%, 180px);
  height: 10px;
  margin-top: 7px;
}

.detail-skeleton-line.short {
  width: 64%;
}

.detail-skeleton-follow {
  width: 72px;
  height: 32px;
  border-radius: 999px;
}

.detail-skeleton-facts {
  display: flex;
  gap: 8px;
  overflow: hidden;
  padding: 12px 16px 2px;
}

.detail-skeleton-pill {
  width: 82px;
  height: 28px;
  flex: 0 0 auto;
  border-radius: 999px;
}

.detail-skeleton-pill:nth-child(2) {
  width: 68px;
}

.detail-skeleton-pill:nth-child(3) {
  width: 74px;
}

.detail-skeleton-card {
  margin: 14px 12px 0;
  padding: 15px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(32, 40, 62, 0.06);
  border-radius: 8px;
  box-shadow: 0 12px 28px rgba(32, 40, 62, 0.04);
}

.detail-skeleton-heading {
  display: block;
  width: 58px;
  height: 17px;
  border-radius: 999px;
}

.detail-skeleton-copy {
  display: grid;
  gap: 10px;
  margin-top: 14px;
}

.detail-skeleton-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.detail-skeleton-count {
  width: 46px;
  height: 10px;
  border-radius: 999px;
}

.detail-skeleton-episodes {
  display: flex;
  gap: 10px;
  overflow: hidden;
  margin-top: 12px;
}

.detail-skeleton-episodes article {
  flex: 0 0 154px;
}

.detail-skeleton-episode-cover {
  aspect-ratio: 16 / 9;
  border-radius: 8px;
}

.detail-skeleton-line.episode-label {
  width: 48px;
  height: 10px;
  margin-top: 8px;
}

.detail-skeleton-line.episode-title {
  width: 78%;
  height: 9px;
  margin-top: 6px;
}

.playback-sticky {
  position: sticky;
  top: 0;
  z-index: 30;
  width: 100%;
  background: #ffffff;
  box-shadow: 0 10px 26px rgba(32, 40, 62, 0.12);
}

:global(body.mobile-player-fullscreen) .playback-sticky {
  z-index: 1100;
}

.player-wrap {
  position: relative;
  background: #070a12;
}

.floating-back,
.player-back {
  position: absolute;
  top: calc(12px + env(safe-area-inset-top));
  left: 12px;
  z-index: 8;
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  color: #ffffff;
  font-size: 28px;
  line-height: 0;
  background: rgba(7, 10, 18, 0.42);
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 999px;
  backdrop-filter: blur(10px);
  padding-bottom: 8px;
  transition: transform 140ms var(--ease-soft), background 140ms var(--ease-soft);
}

.floating-back:active,
.player-back:active {
  transform: scale(0.94);
  background: rgba(238, 63, 134, 0.72);
}

.player-empty {
  position: relative;
  min-height: 220px;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 6px;
  overflow: hidden;
  color: #ffffff;
  background: #101522;
}

.player-empty img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  opacity: 0.36;
  filter: blur(2px);
}

.empty-shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, rgba(7, 10, 18, 0.38), rgba(7, 10, 18, 0.86));
}

.player-empty p,
.player-empty span {
  position: relative;
  z-index: 2;
}

.player-empty p {
  font-size: 15px;
}

.player-empty span {
  color: rgba(255, 255, 255, 0.62);
  font-size: 12px;
}

.detail-tabs {
  position: relative;
  z-index: 3;
  display: grid;
  grid-template-columns: 1fr 1fr;
  padding: 0 12px;
  background: rgba(255, 255, 255, 0.96);
  border-bottom: 1px solid rgba(85, 119, 217, 0.1);
}

.detail-tab {
  position: relative;
  min-height: 50px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  color: var(--ink-400);
  font-size: 14px;
  transition: color 160ms var(--ease-soft), background 160ms var(--ease-soft);
}

.detail-tab.active {
  color: var(--pink-600);
  font-weight: 600;
  background: linear-gradient(180deg, transparent, rgba(255, 244, 248, 0.62));
}

.detail-tab-indicator {
  position: absolute;
  bottom: 0;
  left: 12px;
  width: calc((100% - 24px) / 2);
  height: 2px;
  padding: 0 18px;
  pointer-events: none;
  will-change: transform;
  transition: transform 260ms cubic-bezier(0.22, 1, 0.36, 1);
}

.detail-tab-indicator.dragging {
  transition: none;
}

.detail-tab-indicator > i {
  width: 100%;
  height: 100%;
  display: block;
  background: linear-gradient(90deg, var(--pink-500), var(--cyan-400));
  border-radius: 999px 999px 0 0;
}

.detail-tab em {
  min-width: 19px;
  height: 19px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0 5px;
  color: var(--pink-600);
  font-size: 10px;
  font-style: normal;
  font-weight: 500;
  background: var(--pink-50);
  border: 1px solid var(--line-soft);
  border-radius: 999px;
}

.detail-tab > i {
  width: 13px;
  height: 13px;
  border: 1.5px solid var(--line);
  border-top-color: var(--pink-500);
  border-radius: 50%;
  animation: bp-spin 0.8s linear infinite;
}

.detail-tab-panels {
  position: relative;
  min-width: 0;
  overflow: hidden;
  touch-action: pan-y;
  transition: height 260ms cubic-bezier(0.22, 1, 0.36, 1);
}

.detail-tab-panels.dragging {
  user-select: none;
  transition: none;
}

.detail-tab-track {
  width: 100%;
  display: flex;
  align-items: flex-start;
  will-change: transform;
  transition: transform 260ms cubic-bezier(0.22, 1, 0.36, 1);
}

.detail-tab-track.dragging {
  transition: none;
}

.detail-tab-panel {
  min-width: 0;
  flex: 0 0 100%;
  pointer-events: none;
}

.detail-tab-panel.active {
  pointer-events: auto;
}

.detail-info-panel {
  min-width: 0;
}

.episode-comments-panel {
  min-height: 430px;
  padding: 15px 12px 24px;
}

.comments-heading {
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 3px 4px 13px;
  border-bottom: 1px solid rgba(85, 119, 217, 0.1);
}

.comments-heading > div {
  min-width: 0;
}

.comments-heading span,
.comments-heading h2 {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.comments-heading span {
  color: var(--pink-600);
  font-size: 12px;
}

.comments-heading h2 {
  margin-top: 2px;
  color: var(--ink-900);
  font-size: 16px;
  font-weight: 600;
}

.comments-heading small {
  flex: 0 0 auto;
  color: var(--ink-400);
  font-size: 11px;
  white-space: nowrap;
}

.comments-list {
  display: grid;
  gap: 10px;
  margin-top: 12px;
}

.comments-state {
  min-height: 330px;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 10px;
  padding: 24px;
  color: var(--ink-400);
  font-size: 13px;
  text-align: center;
}

.comments-state > span {
  min-width: 38px;
  height: 38px;
  display: grid;
  place-items: center;
  color: var(--pink-500);
  font-size: 14px;
  background: linear-gradient(145deg, var(--pink-50), var(--cyan-50));
  border: 1px solid var(--line);
  border-radius: 50%;
}

.comments-error-state {
  color: #c73567;
}

.comments-error-state button {
  min-height: 34px;
  padding: 0 15px;
  color: var(--pink-600);
  background: #ffffff;
  border: 1px solid var(--line);
  border-radius: 999px;
}

.comments-loading-list {
  display: grid;
  gap: 10px;
  margin-top: 12px;
}

.comments-loading-list article {
  min-height: 116px;
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr);
  gap: 9px;
  padding: 14px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(32, 40, 62, 0.05);
  border-radius: 9px;
}

.comments-loading-list i,
.comments-loading-list span,
.comments-loading-list p {
  position: relative;
  overflow: hidden;
  background: #e9edf3;
}

.comments-loading-list i::after,
.comments-loading-list span::after,
.comments-loading-list p::after {
  position: absolute;
  inset: 0;
  content: '';
  background: linear-gradient(100deg, transparent 18%, rgba(255, 255, 255, 0.82) 45%, transparent 72%);
  animation: detail-skeleton-sweep 1.1s ease-in-out infinite;
}

.comments-loading-list i {
  width: 38px;
  height: 38px;
  border-radius: 8px;
}

.comments-loading-list article > div {
  display: grid;
  align-content: center;
  gap: 7px;
}

.comments-loading-list span {
  display: block;
  width: 38%;
  height: 9px;
  border-radius: 999px;
}

.comments-loading-list span:last-child {
  width: 62%;
  height: 7px;
}

.comments-loading-list p {
  grid-column: 1 / -1;
  width: 84%;
  height: 30px;
  margin-top: 6px;
  border-radius: 5px;
}

.title-section {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px;
  align-items: start;
  padding: 18px 16px 0;
}

.title-copy {
  min-width: 0;
}

.detail-title {
  color: var(--ink-900);
  font-size: 21px;
  line-height: 1.28;
}

.title-copy > span {
  display: block;
  margin-top: 5px;
  color: var(--ink-400);
  font-size: 12px;
  line-height: 1.4;
}

.follow-button {
  min-width: 72px;
  min-height: 32px;
  padding: 0 12px;
  color: var(--pink-600);
  font-size: 13px;
  background: #ffffff;
  border: 1px solid var(--line);
  border-radius: 999px;
  box-shadow: 0 10px 24px rgba(255, 95, 158, 0.12);
  transition: transform 140ms var(--ease-soft), background 140ms var(--ease-soft), color 140ms var(--ease-soft);
}

.follow-button.followed {
  color: #ffffff;
  background: var(--pink-600);
  border-color: transparent;
}

.follow-button:active:not(:disabled) {
  transform: scale(0.96);
}

.follow-error {
  margin: 8px 16px 0;
  color: #d92d20;
  font-size: 12px;
}

.fact-row {
  display: flex;
  gap: 8px;
  overflow-x: auto;
  padding: 12px 16px 2px;
}

.fact-row span {
  flex: 0 0 auto;
  min-height: 28px;
  display: inline-flex;
  align-items: center;
  padding: 0 10px;
  color: var(--ink-600);
  font-size: 12px;
  background: rgba(255, 255, 255, 0.78);
  border: 1px solid rgba(32, 40, 62, 0.06);
  border-radius: 999px;
}

.detail-block {
  margin: 14px 12px 0;
  padding: 15px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(32, 40, 62, 0.06);
  border-radius: 8px;
  box-shadow: 0 12px 28px rgba(32, 40, 62, 0.04);
}

.block-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.block-title {
  color: var(--ink-900);
  font-size: 17px;
  line-height: 1.3;
}

.block-head > span {
  color: var(--ink-400);
  font-size: 12px;
  white-space: nowrap;
}

.episode-list-trigger {
  min-height: 30px;
  display: inline-flex;
  align-items: center;
  gap: 9px;
  padding: 0 5px 0 10px;
  color: var(--ink-400);
  font-size: 12px;
  border-radius: 999px;
  transition: color 140ms var(--ease-soft), background 140ms var(--ease-soft), transform 140ms var(--ease-soft);
}

.episode-list-trigger i {
  width: 7px;
  height: 7px;
  border-top: 1.5px solid currentColor;
  border-right: 1.5px solid currentColor;
  transform: rotate(45deg);
}

.episode-list-trigger:active:not(:disabled) {
  color: var(--pink-600);
  background: var(--pink-50);
  transform: scale(0.96);
}

.summary-box {
  position: relative;
  max-height: 76px;
  margin-top: 10px;
  overflow: hidden;
  transition: max-height 260ms var(--ease-soft);
}

.summary-box.expanded {
  max-height: 620px;
}

.summary-box::after {
  content: '';
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 34px;
  pointer-events: none;
  background: linear-gradient(transparent, rgba(255, 255, 255, 0.96));
  opacity: 1;
  transition: opacity 180ms var(--ease-soft);
}

.summary-box.expanded::after {
  opacity: 0;
}

.summary-box p {
  color: var(--ink-600);
  font-size: 13px;
  line-height: 1.9;
  white-space: pre-line;
}

.summary-arrow {
  width: 38px;
  height: 24px;
  display: grid;
  place-items: center;
  margin: 7px auto -4px;
  color: var(--pink-600);
}

.summary-arrow::before {
  content: '';
  width: 10px;
  height: 10px;
  border-right: 1.8px solid currentColor;
  border-bottom: 1.8px solid currentColor;
  transform: rotate(45deg);
  transition: transform 180ms var(--ease-soft);
}

.summary-arrow.expanded::before {
  transform: rotate(225deg) translate(-2px, -2px);
}

.episode-rail {
  display: flex;
  gap: 10px;
  overflow-x: auto;
  margin: 9px 0 -3px;
  padding: 3px;
  scroll-padding-inline: 3px;
  scroll-snap-type: x proximity;
}

.episode-card {
  flex: 0 0 154px;
  min-width: 0;
  scroll-snap-align: start;
  color: var(--ink-700);
  outline: 0;
}

.episode-card.unavailable {
  opacity: 0.58;
}

.episode-cover {
  position: relative;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  background: #eef2f8;
  border-radius: 8px;
}

.episode-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.episode-fallback {
  width: 100%;
  height: 100%;
  display: grid;
  place-items: center;
  padding: 8px;
  color: var(--pink-600);
  font-size: 13px;
  text-align: center;
  background: linear-gradient(135deg, var(--pink-50), var(--cyan-50));
}

.episode-cover span {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  padding: 5px;
  color: #ffffff;
  font-size: 11px;
  text-align: center;
  background: rgba(7, 10, 18, 0.7);
}

.episode-card.selected .episode-cover {
  box-shadow: 0 0 0 2px var(--pink-500);
}

.episode-card p {
  margin-top: 7px;
  color: var(--pink-600);
  font-size: 12px;
}

.episode-card small {
  display: -webkit-box;
  overflow: hidden;
  margin-top: 2px;
  color: var(--ink-700);
  font-size: 12px;
  line-height: 1.45;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.episode-sheet-layer {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  padding-top: max(48px, env(safe-area-inset-top));
  background: rgba(7, 10, 18, 0.48);
  backdrop-filter: blur(2px);
  overscroll-behavior: contain;
}

.episode-sheet-panel {
  width: min(100%, 560px);
  max-height: min(82vh, 760px);
  max-height: min(82dvh, 760px);
  display: grid;
  grid-template-rows: auto auto minmax(0, 1fr);
  overflow: hidden;
  color: var(--ink-900);
  background: rgba(250, 251, 255, 0.98);
  border: 1px solid rgba(32, 40, 62, 0.08);
  border-bottom: 0;
  border-radius: 18px 18px 0 0;
  box-shadow: 0 -18px 48px rgba(7, 10, 18, 0.22);
}

.episode-sheet-handle {
  width: 38px;
  height: 4px;
  margin: 9px auto 4px;
  background: rgba(32, 40, 62, 0.16);
  border-radius: 999px;
}

.episode-sheet-panel > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 8px 16px 12px;
  border-bottom: 1px solid rgba(32, 40, 62, 0.07);
}

.episode-sheet-panel > header > div {
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 10px;
}

.episode-sheet-panel > header h2 {
  font-size: 18px;
}

.episode-sheet-panel > header span {
  color: var(--ink-400);
  font-size: 12px;
}

.episode-sheet-panel > header button {
  width: 32px;
  height: 32px;
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  padding-bottom: 3px;
  color: var(--ink-400);
  font-size: 25px;
  line-height: 1;
  background: rgba(32, 40, 62, 0.05);
  border-radius: 50%;
}

.episode-sheet-list {
  min-height: 0;
  overflow-y: auto;
  padding: 7px 10px calc(12px + env(safe-area-inset-bottom));
  overscroll-behavior: contain;
  scroll-padding: 12px 0;
}

.episode-sheet-item {
  position: relative;
  width: 100%;
  display: grid;
  grid-template-columns: 92px minmax(0, 1fr);
  gap: 11px;
  padding: 9px 7px;
  color: var(--ink-700);
  text-align: left;
  border: 1px solid transparent;
  border-bottom-color: rgba(32, 40, 62, 0.06);
  border-radius: 9px;
  transition: background 140ms var(--ease-soft), border-color 140ms var(--ease-soft), transform 140ms var(--ease-soft);
}

.episode-sheet-item.selected {
  border-color: rgba(255, 95, 158, 0.28);
  background: linear-gradient(105deg, rgba(255, 225, 236, 0.72), rgba(236, 253, 255, 0.56));
}

.episode-sheet-item:active:not(:disabled) {
  transform: scale(0.985);
}

.episode-sheet-item.unavailable {
  opacity: 0.58;
}

.episode-sheet-thumb {
  position: relative;
  align-self: start;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  display: grid;
  place-items: center;
  color: var(--pink-600);
  font-size: 11px;
  text-align: center;
  background: linear-gradient(135deg, var(--pink-50), var(--cyan-50));
  border-radius: 7px;
}

.episode-sheet-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.episode-sheet-copy {
  min-width: 0;
}

.episode-sheet-meta {
  min-height: 18px;
  display: flex;
  align-items: center;
  gap: 7px;
}

.episode-sheet-meta > span {
  color: var(--pink-600);
  font-size: 11px;
}

.episode-sheet-meta > i {
  overflow: hidden;
  padding: 2px 6px;
  color: var(--blue-500);
  font-size: 9px;
  font-style: normal;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: var(--cyan-50);
  border-radius: 999px;
}

.episode-sheet-item.selected .episode-sheet-meta > i {
  color: var(--pink-600);
  background: rgba(255, 255, 255, 0.72);
}

.episode-sheet-copy > strong {
  display: -webkit-box;
  overflow: hidden;
  margin-top: 2px;
  color: var(--ink-900);
  font-size: 13px;
  line-height: 1.45;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.episode-sheet-copy > p {
  display: -webkit-box;
  overflow: hidden;
  margin-top: 4px;
  color: var(--ink-400);
  font-size: 11px;
  line-height: 1.55;
  white-space: pre-line;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
}

.episode-sheet-enter-active,
.episode-sheet-leave-active {
  transition: opacity 180ms var(--ease-soft);
}

.episode-sheet-enter-active .episode-sheet-panel,
.episode-sheet-leave-active .episode-sheet-panel {
  transition: transform 220ms cubic-bezier(0.2, 0.82, 0.2, 1);
}

.episode-sheet-enter-from,
.episode-sheet-leave-to {
  opacity: 0;
}

.episode-sheet-enter-from .episode-sheet-panel,
.episode-sheet-leave-to .episode-sheet-panel {
  transform: translateY(100%);
}

.tag-cloud {
  max-height: 62px;
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
  overflow: hidden;
  margin-top: 11px;
}

.tag-cloud span {
  min-height: 27px;
  display: inline-flex;
  align-items: center;
  padding: 0 10px;
  color: var(--pink-600);
  font-size: 12px;
  background: var(--pink-50);
  border: 1px solid var(--line-soft);
  border-radius: 999px;
}

.tag-cloud .meta-tag {
  color: var(--blue-500);
  background: var(--cyan-50);
  border-color: var(--line-cool);
}

.metadata-list {
  display: grid;
  gap: 0;
  margin-top: 8px;
}

.metadata-box {
  position: relative;
  max-height: 205px;
  overflow: hidden;
  transition: max-height 260ms var(--ease-soft);
}

.metadata-box.expanded {
  max-height: 760px;
}

.metadata-box::after {
  content: '';
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 36px;
  pointer-events: none;
  background: linear-gradient(transparent, rgba(255, 255, 255, 0.96));
  opacity: 1;
  transition: opacity 180ms var(--ease-soft);
}

.metadata-box.expanded::after {
  opacity: 0;
}

.metadata-list > div {
  min-width: 0;
  display: grid;
  grid-template-columns: 82px minmax(0, 1fr);
  gap: 10px;
  padding: 10px 0;
  border-bottom: 1px solid rgba(32, 40, 62, 0.06);
}

.metadata-list > div:last-child {
  border-bottom: 0;
}

.metadata-list dt {
  color: var(--ink-400);
  font-size: 12px;
}

.metadata-list dd {
  min-width: 0;
  overflow: hidden;
  color: var(--ink-700);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.character-list {
  display: grid;
  gap: 11px;
  margin-top: 12px;
}

.character-card {
  display: grid;
  grid-template-columns: 72px minmax(0, 1fr);
  gap: 11px;
  padding: 9px;
  background: #f8faff;
  border-radius: 8px;
}

.character-image {
  height: 92px;
  display: grid;
  place-items: center;
  overflow: hidden;
  color: var(--pink-600);
  background: linear-gradient(135deg, var(--pink-50), var(--cyan-50));
  border-radius: 8px;
}

.character-image img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: top center;
}

.character-main {
  min-width: 0;
}

.relation-tag {
  display: inline-flex;
  min-height: 21px;
  align-items: center;
  padding: 0 7px;
  color: var(--pink-600);
  font-size: 11px;
  background: #ffffff;
  border-radius: 999px;
}

.character-main > p {
  margin-top: 5px;
  color: var(--ink-900);
  font-size: 14px;
}

.character-main > small {
  display: -webkit-box;
  overflow: hidden;
  margin-top: 4px;
  color: var(--ink-400);
  font-size: 12px;
  line-height: 1.55;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.actor-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

.actor-chip {
  min-width: 0;
  max-width: 100%;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 7px 4px 4px;
  background: #ffffff;
  border-radius: 999px;
}

.actor-chip img,
.actor-chip > span {
  width: 24px;
  height: 24px;
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  border-radius: 50%;
  background: var(--cyan-50);
  object-fit: cover;
}

.actor-chip > span {
  color: var(--blue-500);
  font-size: 10px;
}

.actor-chip p {
  min-width: 0;
  overflow: hidden;
  color: var(--ink-600);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-empty {
  min-height: 88px;
  display: grid;
  place-items: center;
  color: var(--ink-400);
  font-size: 13px;
}

.detail-state {
  min-height: 100vh;
  min-height: 100dvh;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 10px;
  color: var(--ink-600);
  background: #f6f7fb;
}

.detail-state > span {
  width: 42px;
  height: 42px;
  display: grid;
  place-items: center;
  color: var(--pink-600);
  border: 1px solid var(--line);
  border-radius: 50%;
}

.detail-state p {
  font-size: 13px;
}

.detail-state > div {
  display: flex;
  gap: 8px;
}

.detail-state button {
  min-height: 34px;
  padding: 0 14px;
  color: var(--pink-600);
  background: #ffffff;
  border: 1px solid var(--line);
  border-radius: 999px;
}

@keyframes detail-skeleton-sweep {
  from {
    transform: translateX(-100%);
  }
  to {
    transform: translateX(100%);
  }
}
</style>
