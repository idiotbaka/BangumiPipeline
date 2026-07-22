<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

import type { ViewerOPSkipSegment } from '../api'
import episodePickerIcon from '../assets/episode-picker.svg?raw'
import fullscreenIcon from '../assets/fullscreen.svg?raw'
import { enterNativeFullscreen, exitNativeFullscreen, setNativeKeepScreenOn } from '../native/player'

interface SelectableEpisode {
  key: string
  mediaId: number
  label: string
  title: string
  summary: string
  hasCover: boolean
  coverURL: string
}

interface Props {
  mediaId: number
  src: string
  poster: string
  title: string
  startTime: number
  opSkip: ViewerOPSkipSegment | null
  episodes: SelectableEpisode[]
  selectedEpisodeKey: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (event: 'progress', value: { mediaId: number; positionSeconds: number; durationSeconds: number }): void
  (event: 'open-episode-sheet'): void
  (event: 'select-episode', episodeKey: string): void
}>()

const video = ref<HTMLVideoElement | null>(null)
const player = ref<HTMLElement | null>(null)
const playing = ref(false)
const buffering = ref(false)
const controlsVisible = ref(true)
const currentTime = ref(0)
const duration = ref(0)
const bufferedEnd = ref(0)
const errorMessage = ref('')
const mediaReady = ref(false)
const resumeApplied = ref(false)
const hasPlayed = ref(false)
const autoplayAttempted = ref(false)
const nativeFullscreen = ref(false)
const episodePickerOpen = ref(false)
const episodePickerList = ref<HTMLElement | null>(null)
const failedEpisodeCovers = ref<Set<string>>(new Set())
const opSkipDismissed = ref(false)
const seekGestureActive = ref(false)
const seekGestureDelta = ref(0)
const seekGestureTarget = ref(0)

let progressTimer: ReturnType<typeof setInterval> | null = null
let controlsTimer: ReturnType<typeof setTimeout> | null = null
let bufferTimer: ReturnType<typeof setInterval> | null = null
let tapTimer: ReturnType<typeof setTimeout> | null = null
let fullscreenHistoryPushed = false
let episodePickerHistoryPushed = false
let ignoreNextPopState = false
let ignoreNextEpisodePickerPopState = false
let lastTap:
  | {
      time: number
      x: number
      y: number
    }
  | null = null
let seekGesture:
  | {
      pointerId: number
      startX: number
      startY: number
      startTime: number
      controlsVisibleAtStart: boolean
      canSeek: boolean
      tracking: boolean
      moved: boolean
    }
  | null = null

const seekGestureThreshold = 28
const seekGestureClickTolerance = 6
const doubleTapDelay = 260
const doubleTapDistance = 42
const maxSeekGestureSeconds = 600

const playerLoading = computed(() => Boolean(props.src) && !mediaReady.value && !errorMessage.value)
const canControlPlayback = computed(() => Boolean(props.src) && mediaReady.value && !errorMessage.value)
const hasEpisodes = computed(() => props.episodes.length > 0)
const activeOPSkip = computed(() => normalizeOPSkip(props.opSkip))
const progressStyle = computed(() => {
  const progress = duration.value > 0 ? (currentTime.value / duration.value) * 100 : 0
  const buffered = duration.value > 0 ? (bufferedEnd.value / duration.value) * 100 : progress
  const clampedProgress = clampPercent(progress)
  const clampedBuffered = Math.max(clampedProgress, clampPercent(buffered))
  const opStart =
    activeOPSkip.value && duration.value > 0 ? clampPercent((activeOPSkip.value.startSeconds / duration.value) * 100) : 0
  const opEnd =
    activeOPSkip.value && duration.value > 0 ? clampPercent((activeOPSkip.value.endSeconds / duration.value) * 100) : 0
  return {
    '--progress': `${clampedProgress}%`,
    '--buffered': `${clampedBuffered}%`,
    '--op-start': `${opStart}%`,
    '--op-end': `${opEnd}%`,
  }
})
const withinOPSkipPrompt = computed(() => {
  const segment = activeOPSkip.value
  return Boolean(
    segment &&
      currentTime.value >= segment.promptStartSeconds &&
      currentTime.value < segment.promptEndSeconds,
  )
})
const opSkipVisible = computed(() => canControlPlayback.value && withinOPSkipPrompt.value && !opSkipDismissed.value)
const seekGestureText = computed(() => {
  const prefix = seekGestureDelta.value >= 0 ? '+' : '-'
  return `${prefix}${formatDuration(Math.abs(seekGestureDelta.value))}`
})
const seekGestureDirection = computed(() => (seekGestureDelta.value >= 0 ? 'forward' : 'backward'))
const seekGestureTargetText = computed(() => formatTime(seekGestureTarget.value))

watch(
  () => [props.src, props.mediaId] as const,
  async (_current, previous) => {
    reportProgress(previous[1])
    resetMediaState()
    await nextTick()
    video.value?.load()
  },
)

watch(
  () => props.opSkip,
  () => {
    opSkipDismissed.value = false
  },
)

watch(withinOPSkipPrompt, (inside) => {
  if (!inside) opSkipDismissed.value = false
})

window.addEventListener('popstate', handlePopState)

onBeforeUnmount(() => {
  window.removeEventListener('popstate', handlePopState)
  reportProgress()
  stopProgressTimer()
  stopBufferTimer()
  stopControlsTimer()
  stopTapTimer()
  void setNativeKeepScreenOn(false)
  if (nativeFullscreen.value) {
    void exitFullscreen({ fromUnmount: true })
  }
})

async function togglePlay() {
  const element = video.value
  if (!element || !props.src) return
  if (!canControlPlayback.value) {
    showControls()
    return
  }
  if (element.paused) {
    await requestPlayback(false)
  } else {
    element.pause()
  }
}

async function requestPlayback(silent: boolean) {
  const element = video.value
  if (!element || !props.src || !canControlPlayback.value) return
  try {
    await element.play()
  } catch {
    if (!silent) {
      errorMessage.value = '视频暂时无法播放'
    }
    showControls()
  }
}

function resetMediaState() {
  cancelSeekGesture()
  episodePickerOpen.value = false
  playing.value = false
  buffering.value = false
  currentTime.value = 0
  duration.value = 0
  bufferedEnd.value = 0
  errorMessage.value = ''
  mediaReady.value = false
  resumeApplied.value = false
  hasPlayed.value = false
  autoplayAttempted.value = false
  opSkipDismissed.value = false
  stopProgressTimer()
  stopBufferTimer()
  stopTapTimer()
  showControls()
}

function handleLoadedMetadata() {
  const element = video.value
  duration.value = Number.isFinite(element?.duration) ? element?.duration ?? 0 : 0
  mediaReady.value = duration.value > 0
  applyResumePosition()
  updateBuffered()
  attemptAutoplay()
}

function applyResumePosition() {
  const element = video.value
  if (!element || resumeApplied.value || duration.value <= 0) return
  resumeApplied.value = true
  if (props.startTime <= 0) return
  const target = Math.min(props.startTime, Math.max(duration.value - 1, 0))
  if (target > 0) {
    element.currentTime = target
    currentTime.value = target
  }
}

function updateTime() {
  currentTime.value = video.value?.currentTime ?? 0
  updateBuffered()
}

function updateBuffered() {
  const element = video.value
  if (!element || duration.value <= 0 || element.buffered.length === 0) {
    bufferedEnd.value = currentTime.value
    return
  }
  const time = element.currentTime
  let nextEnd = time
  for (let index = 0; index < element.buffered.length; index += 1) {
    const start = element.buffered.start(index)
    const end = element.buffered.end(index)
    if (time + 0.5 >= start && time - 0.5 <= end) {
      nextEnd = Math.max(nextEnd, end)
      continue
    }
    if (start > time) {
      nextEnd = Math.max(nextEnd, end)
      break
    }
  }
  bufferedEnd.value = Math.max(time, Math.min(duration.value, nextEnd))
}

function handlePlay() {
  playing.value = true
  hasPlayed.value = true
  buffering.value = false
  void setNativeKeepScreenOn(true)
  startProgressTimer()
  startBufferTimer()
  scheduleControlsHide()
}

function handlePause() {
  playing.value = false
  void setNativeKeepScreenOn(false)
  reportProgress()
  stopProgressTimer()
  stopBufferTimer()
  showControls()
}

function handleEnded() {
  playing.value = false
  currentTime.value = duration.value
  void setNativeKeepScreenOn(false)
  reportProgress()
  stopProgressTimer()
  stopBufferTimer()
  showControls()
}

function handleWaiting() {
  if (!mediaReady.value) return
  buffering.value = true
  showControls()
}

function handlePlaying() {
  buffering.value = false
  updateBuffered()
  scheduleControlsHide()
}

function handleCanPlay() {
  if (!mediaReady.value) handleLoadedMetadata()
  buffering.value = false
  updateBuffered()
  attemptAutoplay()
  if (playing.value) scheduleControlsHide()
}

function attemptAutoplay() {
  if (autoplayAttempted.value || !canControlPlayback.value || !video.value?.paused) {
    return
  }
  autoplayAttempted.value = true
  void requestPlayback(true)
}

function handleError() {
  errorMessage.value = '视频加载失败，请稍后重试'
  mediaReady.value = false
  buffering.value = false
  stopProgressTimer()
  stopBufferTimer()
  showControls()
}

function handleInteraction() {
  controlsVisible.value = true
  scheduleControlsHide()
}

function handlePointerDown(event: PointerEvent) {
  resetSeekGesturePreview()
  if (!event.isPrimary || isInteractiveTarget(event.target)) {
    seekGesture = null
    return
  }
  seekGesture = {
    pointerId: event.pointerId,
    startX: event.clientX,
    startY: event.clientY,
    startTime: currentTime.value,
    controlsVisibleAtStart: controlsVisible.value,
    canSeek: canStartSeekGesture(event),
    tracking: false,
    moved: false,
  }
  if (seekGesture.canSeek) {
    player.value?.setPointerCapture?.(event.pointerId)
  }
}

function handlePointerMove(event: PointerEvent) {
  handleInteraction()
  if (!seekGesture || event.pointerId !== seekGesture.pointerId) {
    return
  }
  const deltaX = event.clientX - seekGesture.startX
  const deltaY = event.clientY - seekGesture.startY
  const absX = Math.abs(deltaX)
  const absY = Math.abs(deltaY)
  if (absX > seekGestureClickTolerance || absY > seekGestureClickTolerance) {
    seekGesture.moved = true
  }
  if (!seekGesture.canSeek) {
    return
  }
  if (!seekGesture.tracking) {
    if (absX < seekGestureThreshold) {
      return
    }
    if (absX <= absY * 1.25) {
      cancelSeekGesture()
      return
    }
    seekGesture.tracking = true
    seekGestureActive.value = true
    showControls()
  }
  event.preventDefault()
  updateSeekGesturePreview(deltaX)
}

function handlePointerUp(event: PointerEvent) {
  if (!seekGesture || event.pointerId !== seekGesture.pointerId) {
    return
  }
  if (seekGesture.tracking && Math.abs(seekGestureDelta.value) >= 5) {
    applySeekGesture()
  }
  const controlsVisibleAtStart = seekGesture.controlsVisibleAtStart
  const shouldHandleTap = !seekGesture.tracking && !seekGesture.moved && !isInteractiveTarget(event.target)
  releaseGesturePointerCapture(event.pointerId)
  seekGesture = null
  resetSeekGesturePreview()
  if (shouldHandleTap) {
    handlePlayerTap(event, controlsVisibleAtStart)
  }
}

function handlePointerCancel(event?: PointerEvent) {
  if (event && seekGesture && event.pointerId !== seekGesture.pointerId) {
    return
  }
  cancelSeekGesture()
}

function canStartSeekGesture(event: PointerEvent) {
  return (
    playing.value &&
    canControlPlayback.value &&
    duration.value > 0 &&
    event.isPrimary &&
    !isInteractiveTarget(event.target)
  )
}

function isInteractiveTarget(target: EventTarget | null) {
  return (
    target instanceof Element &&
    Boolean(target.closest('button, input, a, [role="button"], .player-controls, .fullscreen-episode-picker'))
  )
}

function updateSeekGesturePreview(deltaX: number) {
  const elementWidth = Math.max(player.value?.clientWidth ?? 0, 1)
  const effectiveDistance = Math.max(Math.abs(deltaX) - seekGestureThreshold, 0)
  const gestureRange = Math.max(elementWidth * 0.72 - seekGestureThreshold, 1)
  const rawSeconds = (effectiveDistance / gestureRange) * maxSeekGestureSeconds
  const signedSeconds = Math.sign(deltaX) * Math.min(maxSeekGestureSeconds, rawSeconds)
  const target = clampTime((seekGesture?.startTime ?? currentTime.value) + signedSeconds)
  seekGestureTarget.value = target
  seekGestureDelta.value = Math.round(target - (seekGesture?.startTime ?? currentTime.value))
}

function applySeekGesture() {
  const element = video.value
  if (!element) return
  element.currentTime = seekGestureTarget.value
  currentTime.value = seekGestureTarget.value
  updateBuffered()
  reportProgress()
}

function cancelSeekGesture() {
  if (seekGesture?.pointerId !== undefined) {
    releaseGesturePointerCapture(seekGesture.pointerId)
  }
  seekGesture = null
  resetSeekGesturePreview()
}

function releaseGesturePointerCapture(pointerId: number) {
  const element = player.value
  if (!element?.hasPointerCapture?.(pointerId)) {
    return
  }
  element.releasePointerCapture(pointerId)
}

function resetSeekGesturePreview() {
  seekGestureActive.value = false
  seekGestureDelta.value = 0
  seekGestureTarget.value = currentTime.value
}

function handlePlayerTap(event: PointerEvent, controlsVisibleAtStart: boolean) {
  if (playerLoading.value || errorMessage.value || buffering.value) {
    showControls()
    return
  }

  const now = window.performance.now()
  const isDoubleTap =
    lastTap !== null &&
    now - lastTap.time <= doubleTapDelay &&
    Math.hypot(event.clientX - lastTap.x, event.clientY - lastTap.y) <= doubleTapDistance

  if (isDoubleTap) {
    stopTapTimer()
    lastTap = null
    showControls()
    void togglePlay()
    return
  }

  lastTap = { time: now, x: event.clientX, y: event.clientY }
  stopTapTimer()
  tapTimer = window.setTimeout(() => {
    applySingleTap(controlsVisibleAtStart)
    lastTap = null
    tapTimer = null
  }, doubleTapDelay)
}

function applySingleTap(controlsWereVisible: boolean) {
  if (controlsWereVisible) {
    controlsVisible.value = false
    stopControlsTimer()
    return
  }
  showControls()
  scheduleControlsHide()
}

function showControls() {
  controlsVisible.value = true
  stopControlsTimer()
}

function scheduleControlsHide() {
  stopControlsTimer()
  if (!playing.value || buffering.value || errorMessage.value || episodePickerOpen.value) return
  controlsTimer = setTimeout(() => {
    if (playing.value && !buffering.value && !errorMessage.value && !episodePickerOpen.value) {
      controlsVisible.value = false
    }
    controlsTimer = null
  }, 3_000)
}

function stopControlsTimer() {
  if (controlsTimer !== null) {
    clearTimeout(controlsTimer)
    controlsTimer = null
  }
}

function stopTapTimer() {
  if (tapTimer !== null) {
    clearTimeout(tapTimer)
    tapTimer = null
  }
}

function startProgressTimer() {
  stopProgressTimer()
  progressTimer = setInterval(reportProgress, 10_000)
}

function stopProgressTimer() {
  if (progressTimer !== null) {
    clearInterval(progressTimer)
    progressTimer = null
  }
}

function startBufferTimer() {
  stopBufferTimer()
  updateBuffered()
  bufferTimer = setInterval(updateBuffered, 1_000)
}

function stopBufferTimer() {
  if (bufferTimer !== null) {
    clearInterval(bufferTimer)
    bufferTimer = null
  }
}

function reportProgress(mediaId = props.mediaId) {
  if (!hasPlayed.value || mediaId < 1 || currentTime.value <= 0 || duration.value <= 0) return
  emit('progress', {
    mediaId,
    positionSeconds: currentTime.value,
    durationSeconds: duration.value,
  })
}

function seek(event: Event) {
  const value = Number((event.target as HTMLInputElement).value)
  if (!video.value || !Number.isFinite(value)) return
  video.value.currentTime = value
  currentTime.value = value
  updateBuffered()
}

function skipOP() {
  const element = video.value
  const segment = activeOPSkip.value
  if (!element || !segment) return
  const maxSeek = duration.value > 0 ? Math.max(duration.value - 0.1, 0) : segment.seekToSeconds
  const target = Math.max(0, Math.min(segment.seekToSeconds, maxSeek))
  opSkipDismissed.value = true
  element.currentTime = target
  currentTime.value = target
  updateBuffered()
  showControls()
}

async function toggleEpisodePicker() {
  if (!hasEpisodes.value) return
  if (!nativeFullscreen.value) {
    emit('open-episode-sheet')
    return
  }

  if (episodePickerOpen.value) {
    closeFullscreenEpisodePicker()
    return
  }

  episodePickerOpen.value = true
  if (!episodePickerHistoryPushed) {
    const currentState = window.history.state
    const baseState = currentState && typeof currentState === 'object' ? currentState : {}
    window.history.pushState({ ...baseState, bpMobilePlayerEpisodePicker: true }, '', window.location.href)
    episodePickerHistoryPushed = true
  }
  showControls()

  await nextTick()
  window.requestAnimationFrame(() => scrollSelectedPickerEpisodeIntoView('auto'))
}

function closeFullscreenEpisodePicker(options: { fromPopState?: boolean } = {}) {
  if (!episodePickerOpen.value) return
  episodePickerOpen.value = false
  if (!options.fromPopState && episodePickerHistoryPushed) {
    ignoreNextEpisodePickerPopState = true
    window.history.back()
  }
  episodePickerHistoryPushed = false
  showControls()
  scheduleControlsHide()
}

function selectFullscreenEpisode(episode: SelectableEpisode) {
  closeFullscreenEpisodePicker()
  if (episode.key !== props.selectedEpisodeKey) {
    emit('select-episode', episode.key)
  }
}

function scrollSelectedPickerEpisodeIntoView(behavior: ScrollBehavior) {
  const list = episodePickerList.value
  const selectedItem = list?.querySelector<HTMLElement>('.fullscreen-episode-item.selected')
  if (!list || !selectedItem) return

  const listBounds = list.getBoundingClientRect()
  const itemBounds = selectedItem.getBoundingClientRect()
  const target = list.scrollTop + itemBounds.top - listBounds.top - (list.clientHeight - itemBounds.height) / 2
  list.scrollTo({ top: Math.max(target, 0), behavior })
}

function episodeCoverAvailable(episode: SelectableEpisode) {
  return episode.hasCover && Boolean(episode.coverURL) && !failedEpisodeCovers.value.has(episode.key)
}

function markEpisodeCoverFailed(key: string) {
  const failed = new Set(failedEpisodeCovers.value)
  failed.add(key)
  failedEpisodeCovers.value = failed
}

async function toggleFullscreen() {
  if (nativeFullscreen.value) {
    await exitFullscreen()
  } else {
    await enterFullscreen()
  }
}

async function enterFullscreen() {
  if (nativeFullscreen.value) return
  nativeFullscreen.value = true
  document.body.classList.add('mobile-player-fullscreen')
  if (!fullscreenHistoryPushed) {
    window.history.pushState({ bpMobilePlayerFullscreen: true }, '', window.location.href)
    fullscreenHistoryPushed = true
  }
  await nextTick()
  player.value?.focus()
  await enterNativeFullscreen()
}

async function exitFullscreen(options: { fromPopState?: boolean; fromUnmount?: boolean } = {}) {
  if (!nativeFullscreen.value) return
  episodePickerOpen.value = false
  episodePickerHistoryPushed = false
  nativeFullscreen.value = false
  document.body.classList.remove('mobile-player-fullscreen')
  await exitNativeFullscreen()
  if (!options.fromPopState && !options.fromUnmount && fullscreenHistoryPushed) {
    ignoreNextPopState = true
    window.history.back()
  }
  fullscreenHistoryPushed = false
}

function handlePopState(event: PopStateEvent) {
  if (ignoreNextEpisodePickerPopState) {
    ignoreNextEpisodePickerPopState = false
    return
  }
  if (ignoreNextPopState) {
    ignoreNextPopState = false
    return
  }
  const state = event.state as { bpMobilePlayerEpisodePicker?: boolean } | null
  if (episodePickerOpen.value && state?.bpMobilePlayerEpisodePicker !== true) {
    episodePickerHistoryPushed = false
    closeFullscreenEpisodePicker({ fromPopState: true })
    return
  }
  if (nativeFullscreen.value && !episodePickerOpen.value && state?.bpMobilePlayerEpisodePicker === true) {
    episodePickerHistoryPushed = true
    episodePickerOpen.value = true
    showControls()
    void nextTick(() => window.requestAnimationFrame(() => scrollSelectedPickerEpisodeIntoView('auto')))
    return
  }
  if (nativeFullscreen.value) {
    void exitFullscreen({ fromPopState: true })
  }
}

function formatTime(value: number) {
  if (!Number.isFinite(value) || value < 0) return '00:00'
  const total = Math.floor(value)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60
  const base = `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
  return hours > 0 ? `${String(hours).padStart(2, '0')}:${base}` : base
}

function formatDuration(value: number) {
  const total = Math.floor(Math.max(value, 0))
  const minutes = Math.floor(total / 60)
  const seconds = total % 60
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
}

function clampPercent(value: number) {
  return Math.max(0, Math.min(100, value))
}

function clampTime(value: number) {
  return Math.max(0, Math.min(duration.value || 0, value))
}

function normalizeOPSkip(segment: ViewerOPSkipSegment | null) {
  if (!segment) return null
  const startSeconds = Math.max(0, segment.startSeconds)
  const endSeconds = Math.max(0, segment.endSeconds)
  if (!Number.isFinite(startSeconds) || !Number.isFinite(endSeconds) || endSeconds <= startSeconds) return null
  const fallbackPromptStart = Math.max(0, startSeconds - 2)
  const fallbackSeekTo = Math.max(0, endSeconds - 2)
  const promptStartSeconds = Number.isFinite(segment.promptStartSeconds)
    ? Math.max(0, segment.promptStartSeconds)
    : fallbackPromptStart
  const seekToSeconds = Number.isFinite(segment.seekToSeconds)
    ? Math.max(0, Math.min(segment.seekToSeconds, endSeconds))
    : fallbackSeekTo
  const promptEndSeconds = Number.isFinite(segment.promptEndSeconds)
    ? Math.max(promptStartSeconds, Math.min(segment.promptEndSeconds, endSeconds))
    : Math.max(promptStartSeconds, seekToSeconds)
  if (promptEndSeconds <= promptStartSeconds) return null
  return { startSeconds, endSeconds, promptStartSeconds, promptEndSeconds, seekToSeconds }
}
</script>

<template>
  <section
    ref="player"
    class="mobile-player"
    :class="{ fullscreen: nativeFullscreen, 'ui-hidden': !controlsVisible, loading: playerLoading }"
    tabindex="0"
    :aria-label="`正在播放 ${title}`"
    @pointerdown="handlePointerDown"
    @pointermove="handlePointerMove"
    @pointerup="handlePointerUp"
    @pointercancel="handlePointerCancel"
    @lostpointercapture="handlePointerCancel"
    @focusin="handleInteraction"
  >
    <video
      ref="video"
      :src="src"
      :poster="poster || undefined"
      preload="auto"
      playsinline
      webkit-playsinline
      x5-playsinline
      @play="handlePlay"
      @pause="handlePause"
      @ended="handleEnded"
      @waiting="handleWaiting"
      @playing="handlePlaying"
      @canplay="handleCanPlay"
      @canplaythrough="updateBuffered"
      @loadedmetadata="handleLoadedMetadata"
      @durationchange="handleLoadedMetadata"
      @timeupdate="updateTime"
      @progress="updateBuffered"
      @seeked="updateBuffered"
      @error="handleError"
    />

    <div class="player-shade" aria-hidden="true" />

    <div v-if="playerLoading" class="player-state" aria-live="polite">
      <i aria-hidden="true" /><span>播放器加载中</span>
    </div>
    <div v-if="buffering" class="player-state"><i aria-hidden="true" /><span>缓冲中</span></div>
    <div v-if="errorMessage" class="player-error"><span>!</span><p>{{ errorMessage }}</p></div>

    <div v-if="seekGestureActive" class="seek-gesture" :class="seekGestureDirection" aria-live="polite">
      <p>{{ seekGestureText }}</p>
      <small>{{ seekGestureDirection === 'forward' ? '快进到' : '快退到' }} {{ seekGestureTargetText }}</small>
    </div>

    <div class="player-title">
      <span>{{ nativeFullscreen ? '正在播放' : 'BakaVip2' }}</span>
      <p>{{ title }}</p>
    </div>

    <Transition name="op-skip">
      <button v-if="opSkipVisible" class="op-skip-button" type="button" aria-label="跳过 OP" @click.stop="skipOP">
        <span>跳过 OP</span>
      </button>
    </Transition>

    <div class="player-controls">
      <input
        class="timeline"
        type="range"
        min="0"
        :max="duration || 0"
        step="0.1"
        :value="currentTime"
        :style="progressStyle"
        :disabled="!canControlPlayback"
        aria-label="播放进度"
        @input="seek"
      />
      <div class="control-row">
        <button
          type="button"
          class="play-button"
          :aria-label="playing ? '暂停' : '播放'"
          :disabled="!canControlPlayback"
          @click="togglePlay"
        >
          <i :class="{ pause: playing }" aria-hidden="true" />
        </button>
        <span>{{ formatTime(currentTime) }} / {{ formatTime(duration) }}</span>
        <button
          type="button"
          class="episode-picker-button"
          :class="{ active: episodePickerOpen }"
          :disabled="!hasEpisodes"
          :aria-expanded="nativeFullscreen ? episodePickerOpen : undefined"
          aria-controls="mobile-player-episode-picker"
          aria-label="选集"
          @click="toggleEpisodePicker"
        >
          <i aria-hidden="true" v-html="episodePickerIcon" />
        </button>
        <button type="button" class="fullscreen-button" :aria-label="nativeFullscreen ? '退出全屏' : '横屏全屏'" @click="toggleFullscreen">
          <i aria-hidden="true" v-html="fullscreenIcon" />
        </button>
      </div>
    </div>

    <Transition name="fullscreen-episode-picker" :duration="210">
      <div
        v-if="nativeFullscreen && episodePickerOpen"
        class="fullscreen-episode-picker"
        role="presentation"
        @click.self="closeFullscreenEpisodePicker()"
      >
        <aside id="mobile-player-episode-picker" role="dialog" aria-modal="true" aria-label="选集">
          <header>
            <div>
              <span>EPISODES</span>
              <h2>选集</h2>
            </div>
            <small>{{ episodes.length }} 集可播放</small>
            <button type="button" aria-label="关闭选集列表" @click="closeFullscreenEpisodePicker()">×</button>
          </header>

          <div ref="episodePickerList" class="fullscreen-episode-list">
            <button
              v-for="episode in episodes"
              :key="episode.key"
              class="fullscreen-episode-item"
              :class="{ selected: selectedEpisodeKey === episode.key }"
              type="button"
              :aria-current="selectedEpisodeKey === episode.key ? 'true' : undefined"
              @click="selectFullscreenEpisode(episode)"
            >
              <div class="fullscreen-episode-thumb">
                <img
                  v-if="episodeCoverAvailable(episode)"
                  :src="episode.coverURL"
                  :alt="episode.title || episode.label"
                  loading="lazy"
                  decoding="async"
                  @error="markEpisodeCoverFailed(episode.key)"
                />
                <span v-else>{{ episode.label }}</span>
              </div>
              <div class="fullscreen-episode-copy">
                <span>{{ episode.label }}</span>
                <strong>{{ episode.title || episode.label }}</strong>
                <p>{{ episode.summary || '该话暂无剧情简介。' }}</p>
              </div>
              <i v-if="selectedEpisodeKey === episode.key" aria-label="正在播放" />
            </button>
          </div>
        </aside>
      </div>
    </Transition>

    <div class="hidden-progress" :style="progressStyle" aria-hidden="true" />
  </section>
</template>

<style scoped>
.mobile-player {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 9;
  min-height: 210px;
  overflow: hidden;
  color: #ffffff;
  background: #070a12;
  outline: 0;
  touch-action: pan-y;
  contain: layout paint;
  isolation: isolate;
}

.mobile-player.fullscreen {
  position: fixed;
  inset: 0;
  z-index: 1000;
  width: 100vw;
  height: 100vh;
  height: 100dvh;
  min-height: 0;
  aspect-ratio: auto;
  background: #000000;
}

.mobile-player video {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #000000;
}

.mobile-player.loading video {
  cursor: progress;
}

.player-shade {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(to bottom, rgba(7, 10, 18, 0.68), transparent 28%, transparent 58%, rgba(7, 10, 18, 0.9));
  transition: opacity 180ms var(--ease-soft);
}

.play-button i:not(.pause) {
  width: 0;
  height: 0;
  margin-left: 4px;
  border-top: 10px solid transparent;
  border-bottom: 10px solid transparent;
  border-left: 15px solid currentColor;
}

.play-button i:not(.pause) {
  border-top-width: 7px;
  border-bottom-width: 7px;
  border-left-width: 11px;
}

.play-button i.pause {
  width: 12px;
  height: 15px;
  border-right: 4px solid currentColor;
  border-left: 4px solid currentColor;
}

.player-state,
.player-error {
  position: absolute;
  left: 50%;
  top: 50%;
  display: grid;
  place-items: center;
  gap: 8px;
  color: rgba(255, 255, 255, 0.9);
  font-size: 12px;
  transform: translate(-50%, -50%);
}

.player-state i {
  width: 34px;
  height: 34px;
  border: 2px solid rgba(255, 255, 255, 0.2);
  border-top-color: var(--pink-300);
  border-radius: 50%;
  animation: player-spin 0.8s linear infinite;
}

.player-error span {
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  color: var(--pink-300);
  border: 1px solid var(--pink-300);
  border-radius: 50%;
}

.seek-gesture {
  position: absolute;
  left: 50%;
  top: 50%;
  z-index: 4;
  min-width: 112px;
  display: grid;
  place-items: center;
  gap: 3px;
  padding: 10px 16px 9px;
  color: #ffffff;
  background: rgba(7, 10, 18, 0.9);
  border: 1px solid rgba(255, 255, 255, 0.16);
  border-radius: 9px;
  box-shadow: 0 14px 34px rgba(0, 0, 0, 0.28);
  text-shadow: 0 2px 8px rgba(0, 0, 0, 0.55);
  pointer-events: none;
  transform: translate(-50%, -50%);
}

.seek-gesture.forward {
  color: var(--pink-300);
}

.seek-gesture.backward {
  color: var(--cyan-300);
}

.seek-gesture small {
  color: rgba(255, 255, 255, 0.72);
  font-size: 11px;
}

.seek-gesture p {
  color: #ffffff;
  font-size: 23px;
  line-height: 1.18;
  letter-spacing: 0;
}

.player-title {
  position: absolute;
  top: calc(12px + env(safe-area-inset-top));
  right: 14px;
  left: auto;
  z-index: 2;
  width: min(64%, 360px);
  text-align: right;
  pointer-events: none;
  text-shadow: 0 2px 10px rgba(0, 0, 0, 0.72);
  transition: opacity 180ms var(--ease-soft), transform 180ms var(--ease-soft);
}

.player-title span {
  display: block;
  color: var(--cyan-300);
  font-size: 11px;
  letter-spacing: 1.6px;
}

.player-title p {
  margin-top: 2px;
  overflow: hidden;
  font-size: 13px;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.player-controls {
  position: absolute;
  right: 14px;
  bottom: calc(12px + env(safe-area-inset-bottom));
  left: 14px;
  z-index: 2;
  transition: opacity 180ms var(--ease-soft), transform 180ms var(--ease-soft);
}

.timeline {
  width: 100%;
  height: 4px;
  display: block;
  appearance: none;
  background:
    linear-gradient(
      90deg,
      transparent 0 var(--op-start),
      rgba(255, 189, 77, 0.88) var(--op-start) var(--op-end),
      transparent var(--op-end) 100%
    ),
    linear-gradient(
      90deg,
      var(--pink-500) 0 var(--progress),
      rgba(142, 232, 242, 0.58) var(--progress) var(--buffered),
      rgba(255, 255, 255, 0.3) var(--buffered) 100%
    );
}

.timeline::-webkit-slider-thumb {
  width: 15px;
  height: 15px;
  appearance: none;
  background: #ffffff;
  border: 3px solid var(--pink-500);
  border-radius: 50%;
  box-shadow: 0 2px 9px rgba(0, 0, 0, 0.4);
}

.control-row {
  min-height: 40px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding-top: 6px;
}

.play-button,
.episode-picker-button,
.fullscreen-button {
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  color: #ffffff;
}

.play-button:disabled,
.episode-picker-button:disabled {
  cursor: wait;
  opacity: 0.42;
}

.control-row span {
  flex: 1;
  color: rgba(255, 255, 255, 0.82);
  font-size: 12px;
}

.episode-picker-button i,
.fullscreen-button i {
  width: 20px;
  height: 20px;
  display: grid;
  place-items: center;
}

.episode-picker-button :deep(svg),
.fullscreen-button :deep(svg) {
  width: 100%;
  height: 100%;
  display: block;
}

.episode-picker-button :deep(path),
.fullscreen-button :deep(path) {
  fill: currentColor;
}

.episode-picker-button i {
  width: 18px;
  height: 18px;
}

.episode-picker-button {
  border-radius: 7px;
  transition: color 140ms var(--ease-soft), background 140ms var(--ease-soft);
}

.episode-picker-button.active {
  color: var(--cyan-300);
  background: rgba(142, 232, 242, 0.12);
}

.fullscreen-episode-picker {
  position: absolute;
  inset: 0;
  z-index: 8;
  display: flex;
  justify-content: flex-end;
  padding:
    max(10px, env(safe-area-inset-top))
    max(10px, env(safe-area-inset-right))
    max(10px, env(safe-area-inset-bottom))
    max(10px, env(safe-area-inset-left));
  background: rgba(3, 5, 10, 0.84);
  touch-action: pan-y;
}

.fullscreen-episode-picker > aside {
  width: min(480px, 58vw);
  min-width: min(320px, calc(100vw - 28px));
  height: 100%;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  overflow: hidden;
  color: rgba(255, 255, 255, 0.94);
  background: linear-gradient(150deg, rgba(30, 36, 49, 0.98), rgba(15, 19, 29, 0.98));
  border: 1px solid rgba(255, 255, 255, 0.13);
  border-radius: 12px;
  box-shadow: 0 18px 48px rgba(0, 0, 0, 0.48), inset 0 0 0 1px rgba(142, 232, 242, 0.04);
}

.fullscreen-episode-picker > aside > header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 14px;
  padding: 10px 11px 10px 14px;
  background: rgba(7, 10, 18, 0.42);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.fullscreen-episode-picker > aside > header > div {
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 9px;
}

.fullscreen-episode-picker > aside > header span {
  color: var(--cyan-300);
  font-size: 9px;
  letter-spacing: 1.4px;
}

.fullscreen-episode-picker > aside > header h2 {
  font-size: 16px;
}

.fullscreen-episode-picker > aside > header small {
  color: rgba(255, 255, 255, 0.5);
  font-size: 10px;
  white-space: nowrap;
}

.fullscreen-episode-picker > aside > header > button {
  width: 30px;
  height: 30px;
  display: grid;
  place-items: center;
  padding-bottom: 3px;
  color: rgba(255, 255, 255, 0.7);
  font-size: 24px;
  line-height: 1;
  background: rgba(255, 255, 255, 0.08);
  border-radius: 50%;
}

.fullscreen-episode-list {
  min-height: 0;
  overflow-y: auto;
  padding: 6px;
  overscroll-behavior: contain;
  scroll-padding: 8px 0;
}

.fullscreen-episode-item {
  position: relative;
  width: 100%;
  display: grid;
  grid-template-columns: 108px minmax(0, 1fr);
  gap: 11px;
  padding: 8px;
  color: inherit;
  text-align: left;
  border: 1px solid transparent;
  border-bottom-color: rgba(255, 255, 255, 0.07);
  border-radius: 8px;
  contain: layout paint style;
  content-visibility: auto;
  contain-intrinsic-size: auto 94px;
  transition: border-color 140ms var(--ease-soft), background 140ms var(--ease-soft), transform 140ms var(--ease-soft);
}

.fullscreen-episode-item.selected {
  border-color: rgba(255, 159, 189, 0.5);
  background: linear-gradient(100deg, rgba(255, 95, 158, 0.2), rgba(73, 214, 233, 0.1));
}

.fullscreen-episode-item:active {
  transform: scale(0.985);
}

.fullscreen-episode-thumb {
  aspect-ratio: 16 / 9;
  align-self: center;
  overflow: hidden;
  display: grid;
  place-items: center;
  color: var(--pink-200);
  font-size: 11px;
  text-align: center;
  background: linear-gradient(135deg, rgba(255, 95, 158, 0.2), rgba(73, 214, 233, 0.14));
  border-radius: 6px;
}

.fullscreen-episode-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.fullscreen-episode-copy {
  min-width: 0;
  align-self: center;
  padding-right: 15px;
}

.fullscreen-episode-copy > span {
  color: var(--pink-200);
  font-size: 10px;
}

.fullscreen-episode-copy > strong {
  display: block;
  overflow: hidden;
  margin-top: 1px;
  font-size: 12px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.fullscreen-episode-copy > p {
  display: -webkit-box;
  overflow: hidden;
  margin-top: 3px;
  color: rgba(255, 255, 255, 0.5);
  font-size: 10px;
  line-height: 1.45;
  white-space: pre-line;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.fullscreen-episode-item > i {
  position: absolute;
  top: 50%;
  right: 10px;
  width: 7px;
  height: 7px;
  background: var(--cyan-300);
  box-shadow: 0 0 0 3px rgba(142, 232, 242, 0.12), 0 0 10px var(--cyan-300);
  transform: translateY(-50%) rotate(45deg);
}

.fullscreen-episode-picker-enter-active > aside,
.fullscreen-episode-picker-leave-active > aside {
  transition: transform 210ms cubic-bezier(0.2, 0.82, 0.2, 1);
  will-change: transform;
}

.fullscreen-episode-picker-enter-from > aside,
.fullscreen-episode-picker-leave-to > aside {
  transform: translateX(28px);
}

.op-skip-button {
  position: absolute;
  right: 12px;
  bottom: 66px;
  z-index: 5;
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0 10px;
  color: rgba(255, 255, 255, 0.9);
  font-size: 11px;
  font-weight: 600;
  background: rgba(54, 59, 68, 0.9);
  border: 1px solid rgba(255, 255, 255, 0.2);
  border-radius: 6px;
  box-shadow: 0 5px 14px rgba(0, 0, 0, 0.24);
}

.op-skip-button:active {
  transform: scale(0.97);
}

.mobile-player.fullscreen .op-skip-button {
  right: max(20px, env(safe-area-inset-right));
  bottom: calc(76px + env(safe-area-inset-bottom));
}

.op-skip-enter-active,
.op-skip-leave-active {
  transition: opacity 160ms var(--ease-soft), transform 160ms var(--ease-soft);
}

.op-skip-enter-from,
.op-skip-leave-to {
  opacity: 0;
  transform: translateY(7px);
}

.hidden-progress {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  z-index: 3;
  height: 3px;
  pointer-events: none;
  opacity: 0;
  background:
    linear-gradient(
      90deg,
      transparent 0 var(--op-start),
      rgba(255, 189, 77, 0.74) var(--op-start) var(--op-end),
      transparent var(--op-end) 100%
    ),
    linear-gradient(
      90deg,
      var(--pink-500) 0 var(--progress),
      rgba(142, 232, 242, 0.5) var(--progress) var(--buffered),
      transparent var(--buffered) 100%
    );
  transition: opacity 180ms var(--ease-soft);
}

.mobile-player.ui-hidden {
  cursor: none;
}

.mobile-player.ui-hidden .player-shade,
.mobile-player.ui-hidden .player-title,
.mobile-player.ui-hidden .player-controls {
  opacity: 0;
  pointer-events: none;
}

.mobile-player.ui-hidden .player-title {
  transform: translateY(-8px);
}

.mobile-player.ui-hidden .player-controls {
  transform: translateY(8px);
}

.mobile-player.ui-hidden .hidden-progress {
  opacity: 1;
}

@keyframes player-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
