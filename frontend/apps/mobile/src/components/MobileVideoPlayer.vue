<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

import fullscreenIcon from '../assets/fullscreen.svg?raw'
import { enterNativeFullscreen, exitNativeFullscreen, setNativeKeepScreenOn } from '../native/player'

interface Props {
  mediaId: number
  src: string
  poster: string
  title: string
  startTime: number
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (event: 'progress', value: { mediaId: number; positionSeconds: number; durationSeconds: number }): void
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
const nativeFullscreen = ref(false)
const seekGestureActive = ref(false)
const seekGestureDelta = ref(0)
const seekGestureTarget = ref(0)

let progressTimer: ReturnType<typeof setInterval> | null = null
let controlsTimer: ReturnType<typeof setTimeout> | null = null
let bufferTimer: ReturnType<typeof setInterval> | null = null
let fullscreenHistoryPushed = false
let ignoreNextPopState = false
let suppressNextVideoClick = false
let seekGesture:
  | {
      pointerId: number
      startX: number
      startY: number
      startTime: number
      tracking: boolean
      moved: boolean
    }
  | null = null

const seekGestureThreshold = 28
const seekGestureClickTolerance = 6
const maxSeekGestureSeconds = 600

const playerLoading = computed(() => Boolean(props.src) && !mediaReady.value && !errorMessage.value)
const canControlPlayback = computed(() => Boolean(props.src) && mediaReady.value && !errorMessage.value)
const progressStyle = computed(() => {
  const progress = duration.value > 0 ? (currentTime.value / duration.value) * 100 : 0
  const buffered = duration.value > 0 ? (bufferedEnd.value / duration.value) * 100 : progress
  return {
    '--progress': `${clampPercent(progress)}%`,
    '--buffered': `${Math.max(clampPercent(progress), clampPercent(buffered))}%`,
  }
})
const seekGestureText = computed(() => {
  const prefix = seekGestureDelta.value >= 0 ? '+' : '-'
  return `${prefix}${formatDuration(Math.abs(seekGestureDelta.value))}`
})
const seekGestureDirection = computed(() => (seekGestureDelta.value >= 0 ? 'forward' : 'backward'))
const seekGestureTargetText = computed(() => formatTime(seekGestureTarget.value))

watch(
  () => props.src,
  async () => {
    resetMediaState()
    await nextTick()
    video.value?.load()
  },
)

window.addEventListener('popstate', handlePopState)

onBeforeUnmount(() => {
  window.removeEventListener('popstate', handlePopState)
  reportProgress()
  stopProgressTimer()
  stopBufferTimer()
  stopControlsTimer()
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
    try {
      await element.play()
    } catch {
      errorMessage.value = '视频暂时无法播放'
    }
  } else {
    element.pause()
  }
}

function handleVideoClick() {
  if (suppressNextVideoClick) {
    suppressNextVideoClick = false
    return
  }
  void togglePlay()
}

function resetMediaState() {
  cancelSeekGesture()
  playing.value = false
  buffering.value = false
  currentTime.value = 0
  duration.value = 0
  bufferedEnd.value = 0
  errorMessage.value = ''
  mediaReady.value = false
  resumeApplied.value = false
  hasPlayed.value = false
  stopProgressTimer()
  stopBufferTimer()
  showControls()
}

function handleLoadedMetadata() {
  const element = video.value
  duration.value = Number.isFinite(element?.duration) ? element?.duration ?? 0 : 0
  mediaReady.value = duration.value > 0
  applyResumePosition()
  updateBuffered()
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
  if (playing.value) scheduleControlsHide()
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
  handleInteraction()
  resetSeekGesturePreview()
  if (!canStartSeekGesture(event)) {
    seekGesture = null
    return
  }
  seekGesture = {
    pointerId: event.pointerId,
    startX: event.clientX,
    startY: event.clientY,
    startTime: currentTime.value,
    tracking: false,
    moved: false,
  }
  player.value?.setPointerCapture?.(event.pointerId)
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
  if (seekGesture.moved) {
    suppressNextVideoClick = true
    window.setTimeout(() => {
      suppressNextVideoClick = false
    }, 0)
  }
  if (seekGesture.tracking && Math.abs(seekGestureDelta.value) >= 5) {
    applySeekGesture()
  }
  player.value?.releasePointerCapture?.(event.pointerId)
  seekGesture = null
  resetSeekGesturePreview()
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
  return target instanceof Element && Boolean(target.closest('button, input, a, [role="button"], .player-controls'))
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
    player.value?.releasePointerCapture?.(seekGesture.pointerId)
  }
  seekGesture = null
  resetSeekGesturePreview()
}

function resetSeekGesturePreview() {
  seekGestureActive.value = false
  seekGestureDelta.value = 0
  seekGestureTarget.value = currentTime.value
}

function showControls() {
  controlsVisible.value = true
  stopControlsTimer()
}

function scheduleControlsHide() {
  stopControlsTimer()
  if (!playing.value || buffering.value || errorMessage.value) return
  controlsTimer = setTimeout(() => {
    if (playing.value && !buffering.value && !errorMessage.value) {
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

function reportProgress() {
  if (!hasPlayed.value || props.mediaId < 1 || currentTime.value <= 0 || duration.value <= 0) return
  emit('progress', {
    mediaId: props.mediaId,
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
  nativeFullscreen.value = false
  document.body.classList.remove('mobile-player-fullscreen')
  await exitNativeFullscreen()
  if (!options.fromPopState && !options.fromUnmount && fullscreenHistoryPushed) {
    ignoreNextPopState = true
    window.history.back()
  }
  fullscreenHistoryPushed = false
}

function handlePopState() {
  if (ignoreNextPopState) {
    ignoreNextPopState = false
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
</script>

<template>
  <section
    ref="player"
    class="mobile-player"
    :class="{ fullscreen: nativeFullscreen, 'ui-hidden': !controlsVisible && playing, loading: playerLoading }"
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
      @click="handleVideoClick"
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

    <button
      v-if="canControlPlayback && !playing && !buffering && !errorMessage"
      class="center-play"
      type="button"
      aria-label="播放"
      @click="togglePlay"
    >
      <i aria-hidden="true" />
    </button>

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
      <span>{{ nativeFullscreen ? '正在播放' : 'BakaVip 2.0' }}</span>
      <p>{{ title }}</p>
    </div>

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
        <button type="button" class="fullscreen-button" :aria-label="nativeFullscreen ? '退出全屏' : '横屏全屏'" @click="toggleFullscreen">
          <i aria-hidden="true" v-html="fullscreenIcon" />
        </button>
      </div>
    </div>

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

.center-play {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 58px;
  height: 58px;
  display: grid;
  place-items: center;
  color: #ffffff;
  background: rgba(238, 63, 134, 0.92);
  border: 1px solid rgba(255, 255, 255, 0.48);
  border-radius: 50%;
  box-shadow: 0 14px 34px rgba(0, 0, 0, 0.36);
  transform: translate(-50%, -50%);
  transition: transform 160ms var(--ease-soft), filter 160ms var(--ease-soft);
}

.center-play:active {
  transform: translate(-50%, -50%) scale(0.95);
}

.center-play i,
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
  background: rgba(7, 10, 18, 0.72);
  border: 1px solid rgba(255, 255, 255, 0.16);
  border-radius: 9px;
  box-shadow: 0 14px 34px rgba(0, 0, 0, 0.28);
  text-shadow: 0 2px 8px rgba(0, 0, 0, 0.55);
  backdrop-filter: blur(14px);
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
  background: linear-gradient(
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
.fullscreen-button {
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  color: #ffffff;
}

.play-button:disabled {
  cursor: wait;
  opacity: 0.42;
}

.control-row span {
  flex: 1;
  color: rgba(255, 255, 255, 0.82);
  font-size: 12px;
}

.fullscreen-button i {
  width: 20px;
  height: 20px;
  display: grid;
  place-items: center;
}

.fullscreen-button :deep(svg) {
  width: 100%;
  height: 100%;
  display: block;
}

.fullscreen-button :deep(path) {
  fill: currentColor;
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
  background: linear-gradient(90deg, var(--pink-500) 0 var(--progress), rgba(142, 232, 242, 0.5) var(--progress) var(--buffered), transparent var(--buffered) 100%);
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
