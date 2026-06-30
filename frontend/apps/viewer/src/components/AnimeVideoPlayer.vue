<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

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
const player = ref<HTMLElement | null>(null)
const video = ref<HTMLVideoElement | null>(null)
const playing = ref(false)
const buffering = ref(false)
const currentTime = ref(0)
const duration = ref(0)
const volume = ref(1)
const muted = ref(false)
const playbackRate = ref(1)
const errorMessage = ref('')
const webFullscreen = ref(false)
const controlsVisible = ref(true)
const resumeApplied = ref(false)
const hasPlayed = ref(false)
const bufferEnd = ref(0)
let previousBodyOverflow = ''
let progressTimer: ReturnType<typeof setInterval> | null = null
let controlsTimer: ReturnType<typeof setTimeout> | null = null

const progressStyle = computed(() => {
  const progress = duration.value > 0 ? (currentTime.value / duration.value) * 100 : 0
  const buffered = duration.value > 0 ? (bufferEnd.value / duration.value) * 100 : progress
  return {
    '--progress': `${Math.max(0, Math.min(100, progress))}%`,
    '--buffered': `${Math.max(progress, Math.min(100, buffered))}%`,
  }
})

watch(
  () => props.src,
  async () => {
    playing.value = false
    buffering.value = false
    currentTime.value = 0
    duration.value = 0
    bufferEnd.value = 0
    errorMessage.value = ''
    resumeApplied.value = false
    hasPlayed.value = false
    stopProgressTimer()
    showControls()
    await nextTick()
    video.value?.load()
  },
)

onMounted(() => window.addEventListener('keydown', handleWindowKeydown))

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleWindowKeydown)
  reportProgress()
  stopProgressTimer()
  stopControlsTimer()
  exitWebFullscreen()
})

async function togglePlay() {
  const element = video.value
  if (!element || !props.src) return
  if (element.paused) {
    try {
      await element.play()
    } catch {
      errorMessage.value = '浏览器暂时无法播放该视频'
    }
  } else {
    element.pause()
  }
}

function updateDuration() {
  duration.value = Number.isFinite(video.value?.duration) ? video.value?.duration ?? 0 : 0
  applyResumePosition()
  updateBuffered()
}

function updateTime() {
  currentTime.value = video.value?.currentTime ?? 0
  updateBuffered()
}

function updateBuffered() {
  const element = video.value
  if (!element || duration.value <= 0 || element.buffered.length === 0) {
    bufferEnd.value = currentTime.value
    return
  }
  const time = element.currentTime
  let end = time
  for (let index = 0; index < element.buffered.length; index++) {
    const start = element.buffered.start(index)
    const rangeEnd = element.buffered.end(index)
    if (time >= start && time <= rangeEnd) {
      end = rangeEnd
      break
    }
    if (rangeEnd < time) {
      end = Math.max(end, rangeEnd)
      continue
    }
    if (start > time) {
      break
    }
  }
  bufferEnd.value = Math.max(time, Math.min(duration.value, end))
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
    updateBuffered()
  }
}

function handlePlay() {
  playing.value = true
  hasPlayed.value = true
  startProgressTimer()
  scheduleControlsHide()
}

function handlePause() {
  playing.value = false
  showControls()
  reportProgress()
  stopProgressTimer()
}

function handleEnded() {
  playing.value = false
  showControls()
  currentTime.value = duration.value
  reportProgress()
  stopProgressTimer()
}

function handleWaiting() {
  buffering.value = true
  showControls()
}

function handlePlaying() {
  buffering.value = false
  scheduleControlsHide()
}

function handleCanPlay() {
  buffering.value = false
  updateBuffered()
  if (playing.value) scheduleControlsHide()
}

function handleError() {
  errorMessage.value = '视频加载失败，请稍后重试'
  showControls()
}

function handlePlayerInteraction() {
  controlsVisible.value = true
  scheduleControlsHide()
}

function showControls() {
  controlsVisible.value = true
  stopControlsTimer()
}

function scheduleControlsHide() {
  stopControlsTimer()
  if (!playing.value || buffering.value || errorMessage.value) return
  controlsTimer = setTimeout(() => {
    if (playing.value && !buffering.value && !errorMessage.value) controlsVisible.value = false
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

function changeVolume(event: Event) {
  const value = Number((event.target as HTMLInputElement).value)
  if (!video.value || !Number.isFinite(value)) return
  video.value.volume = value
  video.value.muted = value === 0
  volume.value = value
  muted.value = value === 0
}

function toggleMute() {
  if (!video.value) return
  video.value.muted = !video.value.muted
  muted.value = video.value.muted
}

function cyclePlaybackRate() {
  const rates = [1, 1.25, 1.5, 2]
  const index = rates.indexOf(playbackRate.value)
  playbackRate.value = rates[(index + 1) % rates.length]
  if (video.value) video.value.playbackRate = playbackRate.value
}

async function toggleFullscreen() {
  if (!player.value) return
  if (document.fullscreenElement) await document.exitFullscreen()
  else {
    exitWebFullscreen()
    await nextTick()
    await player.value.requestFullscreen()
  }
}

function toggleWebFullscreen() {
  if (webFullscreen.value) {
    exitWebFullscreen()
    return
  }
  previousBodyOverflow = document.body.style.overflow
  document.body.style.overflow = 'hidden'
  document.body.classList.add('bp-web-fullscreen')
  webFullscreen.value = true
  void nextTick(() => player.value?.focus())
}

function exitWebFullscreen() {
  if (!webFullscreen.value) return
  webFullscreen.value = false
  document.body.style.overflow = previousBodyOverflow
  document.body.classList.remove('bp-web-fullscreen')
}

function handleWindowKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && webFullscreen.value) exitWebFullscreen()
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
</script>

<template>
  <section
    ref="player"
    class="anime-player"
    :class="{ 'web-fullscreen': webFullscreen, 'ui-hidden': !controlsVisible && playing }"
    tabindex="0"
    :aria-label="`正在播放 ${title}`"
    @pointermove="handlePlayerInteraction"
    @pointerdown="handlePlayerInteraction"
    @focusin="handlePlayerInteraction"
    @keydown="handlePlayerInteraction"
    @keydown.space.prevent="togglePlay"
  >
    <video
      ref="video"
      :src="src"
      :poster="poster || undefined"
      preload="auto"
      playsinline
      @click="togglePlay"
      @play="handlePlay"
      @pause="handlePause"
      @ended="handleEnded"
      @waiting="handleWaiting"
      @playing="handlePlaying"
      @canplay="handleCanPlay"
      @canplaythrough="updateBuffered"
      @loadedmetadata="updateDuration"
      @durationchange="updateDuration"
      @timeupdate="updateTime"
      @progress="updateBuffered"
      @seeking="updateBuffered"
      @seeked="updateBuffered"
      @error="handleError"
    />

    <div class="screen-shade" aria-hidden="true" />
    <div class="player-corners" aria-hidden="true"><i /><i /><i /><i /></div>

    <button v-if="!playing && !buffering && !errorMessage" class="center-play" type="button" aria-label="播放" @click="togglePlay">
      <i aria-hidden="true" />
    </button>
    <div v-if="buffering" class="buffering-mark" aria-label="正在缓冲"><i /><span>BUFFERING</span></div>
    <div v-if="errorMessage" class="player-error"><span>!</span><p>{{ errorMessage }}</p></div>

    <div class="player-heading">
      <span>NOW PLAYING</span>
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
        aria-label="播放进度"
        @input="seek"
      />
      <div class="control-row">
        <button class="play-control" type="button" :aria-label="playing ? '暂停' : '播放'" @click="togglePlay">
          <i :class="{ pause: playing }" aria-hidden="true" />
        </button>
        <span class="time-code">{{ formatTime(currentTime) }} <i>/</i> {{ formatTime(duration) }}</span>
        <div class="control-spacer" />
        <button class="text-control" type="button" @click="toggleMute">{{ muted ? 'MUTE' : 'VOL' }}</button>
        <input
          class="volume-range"
          type="range"
          min="0"
          max="1"
          step="0.05"
          :value="muted ? 0 : volume"
          aria-label="音量"
          @input="changeVolume"
        />
        <button class="text-control rate" type="button" @click="cyclePlaybackRate">{{ playbackRate }}×</button>
        <button
          class="web-fullscreen-control"
          :class="{ active: webFullscreen }"
          type="button"
          :aria-label="webFullscreen ? '退出网页全屏' : '进入网页全屏'"
          @click="toggleWebFullscreen"
        >
          <i aria-hidden="true" />{{ webFullscreen ? '退出网页全屏' : '网页全屏' }}
        </button>
        <button class="fullscreen-control" type="button" aria-label="全屏" @click="toggleFullscreen"><i aria-hidden="true" /></button>
      </div>
    </div>
    <div class="hidden-progress" :style="progressStyle" aria-hidden="true" />
  </section>
</template>

<style scoped>
.anime-player { position: relative; width: 100%; height: 100%; min-height: 480px; overflow: hidden; color: #fff; background: #101522; outline: 0; clip-path: polygon(0 0, calc(100% - 20px) 0, 100% 20px, 100% 100%, 20px 100%, 0 calc(100% - 20px)); }
.anime-player video { width: 100%; height: 100%; object-fit: contain; background: #090d17; cursor: pointer; }
.screen-shade { position: absolute; inset: 0; pointer-events: none; background: linear-gradient(to bottom, rgba(8,12,22,.48), transparent 20%, transparent 68%, rgba(8,12,22,.86)); transition: opacity 180ms ease; }
.player-corners { position: absolute; inset: 12px; pointer-events: none; transition: opacity 180ms ease; }
.player-corners i { position: absolute; width: 18px; height: 18px; opacity: .72; }
.player-corners i:nth-child(1) { top: 0; left: 0; border-top: 1px solid var(--cyan-300); border-left: 1px solid var(--cyan-300); }
.player-corners i:nth-child(2) { top: 0; right: 0; border-top: 1px solid var(--pink-300); border-right: 1px solid var(--pink-300); }
.player-corners i:nth-child(3) { right: 0; bottom: 0; border-right: 1px solid var(--cyan-300); border-bottom: 1px solid var(--cyan-300); }
.player-corners i:nth-child(4) { bottom: 0; left: 0; border-bottom: 1px solid var(--pink-300); border-left: 1px solid var(--pink-300); }
.center-play { position: absolute; top: 50%; left: 50%; width: 74px; height: 74px; display: grid; place-items: center; color: #fff; background: rgba(255,95,158,.88); border: 1px solid rgba(255,255,255,.55); box-shadow: 0 0 0 10px rgba(255,255,255,.08), 0 15px 38px rgba(0,0,0,.34); backdrop-filter: blur(10px); transform: translate(-50%, -50%); clip-path: polygon(16px 0, 100% 0, 100% calc(100% - 16px), calc(100% - 16px) 100%, 0 100%, 0 16px); transition: transform 180ms ease, background 180ms ease; }
.center-play:hover { background: var(--pink-500); transform: translate(-50%, -50%) scale(1.05); }
.center-play i { width: 0; height: 0; margin-left: 5px; border-top: 11px solid transparent; border-bottom: 11px solid transparent; border-left: 17px solid #fff; }
.player-heading { position: absolute; top: 24px; left: 28px; max-width: 70%; text-shadow: 0 2px 8px rgba(0,0,0,.7); pointer-events: none; transition: opacity 180ms ease, transform 180ms ease; }
.player-heading span { color: var(--cyan-300); font-family: var(--font-mono); font-size: 13px; letter-spacing: 2px; }
.player-heading p { margin-top: 3px; overflow: hidden; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.player-controls { position: absolute; right: 24px; bottom: 20px; left: 24px; transition: opacity 180ms ease, transform 180ms ease; }
.timeline { width: 100%; height: 4px; appearance: none; cursor: pointer; background: linear-gradient(90deg, var(--pink-400) 0 var(--progress), rgba(142,232,242,.58) var(--progress) var(--buffered), rgba(255,255,255,.28) var(--buffered) 100%); }
.timeline::-webkit-slider-thumb { width: 13px; height: 13px; appearance: none; background: #fff; border: 3px solid var(--pink-400); transform: rotate(45deg); box-shadow: 0 2px 8px rgba(0,0,0,.3); }
.control-row { height: 46px; display: flex; align-items: flex-end; gap: 14px; }
.play-control { width: 30px; height: 30px; display: grid; place-items: center; color: #fff; }
.play-control i:not(.pause) { width: 0; height: 0; margin-left: 3px; border-top: 7px solid transparent; border-bottom: 7px solid transparent; border-left: 11px solid currentColor; }
.play-control i.pause { width: 11px; height: 14px; border-right: 4px solid currentColor; border-left: 4px solid currentColor; }
.time-code { margin-bottom: 5px; color: rgba(255,255,255,.82); font-family: var(--font-mono); font-size: 13px; }
.time-code i { padding: 0 4px; color: var(--pink-300); font-style: normal; }
.control-spacer { flex: 1; }
.text-control { margin-bottom: 3px; color: rgba(255,255,255,.75); font-family: var(--font-mono); font-size: 13px; letter-spacing: .5px; }
.text-control:hover { color: var(--cyan-300); }
.text-control.rate { min-width: 32px; }
.volume-range { width: 70px; height: 3px; margin-bottom: 10px; appearance: none; background: rgba(255,255,255,.3); }
.volume-range::-webkit-slider-thumb { width: 9px; height: 9px; appearance: none; background: var(--cyan-300); transform: rotate(45deg); }
.fullscreen-control { width: 28px; height: 28px; display: grid; place-items: center; }
.fullscreen-control i { width: 14px; height: 14px; border: 1px solid rgba(255,255,255,.84); clip-path: polygon(0 0, 42% 0, 42% 12%, 12% 12%, 12% 42%, 0 42%, 0 0, 100% 0, 100% 42%, 88% 42%, 88% 12%, 58% 12%, 58% 0, 100% 0, 100% 100%, 58% 100%, 58% 88%, 88% 88%, 88% 58%, 100% 58%, 100% 100%, 0 100%, 0 58%, 12% 58%, 12% 88%, 42% 88%, 42% 100%, 0 100%); }
.web-fullscreen-control { height: 29px; display: inline-flex; align-items: center; gap: 7px; margin-bottom: -1px; padding: 0 9px; color: rgba(255,255,255,.78); font-size: 13px; white-space: nowrap; border: 1px solid rgba(255,255,255,.24); background: rgba(9,13,23,.38); clip-path: polygon(var(--bevel-sm)); }
.web-fullscreen-control:hover, .web-fullscreen-control.active { color: #fff; border-color: rgba(142,232,242,.55); background: rgba(73,214,233,.18); }
.web-fullscreen-control i { width: 12px; height: 9px; border: 1px solid currentColor; box-shadow: inset 0 2px 0 rgba(142,232,242,.45); }
.buffering-mark, .player-error { position: absolute; top: 50%; left: 50%; display: grid; place-items: center; gap: 10px; transform: translate(-50%, -50%); }
.buffering-mark i { width: 38px; height: 38px; border: 2px solid rgba(255,255,255,.2); border-top-color: var(--cyan-300); border-radius: 50%; animation: bp-spin .8s linear infinite; }
.buffering-mark span { font-family: var(--font-mono); font-size: 13px; letter-spacing: 2px; }
.player-error span { width: 42px; height: 42px; display: grid; place-items: center; color: var(--pink-300); font-family: var(--font-mono); font-size: 22px; border: 1px solid var(--pink-300); transform: rotate(45deg); }
.player-error p { font-size: 13px; }
.hidden-progress { position: absolute; right: 0; bottom: 0; left: 0; z-index: 2; height: 3px; pointer-events: none; opacity: 0; background: linear-gradient(90deg, var(--pink-400) 0 var(--progress), rgba(142,232,242,.5) var(--progress) var(--buffered), transparent var(--buffered) 100%); filter: drop-shadow(0 -1px 3px rgba(255,95,158,.42)) drop-shadow(0 -1px 4px rgba(142,232,242,.26)); transition: opacity 180ms ease; }
.anime-player.ui-hidden { cursor: none; }
.anime-player.ui-hidden video { cursor: none; }
.anime-player.ui-hidden .screen-shade,
.anime-player.ui-hidden .player-corners,
.anime-player.ui-hidden .player-heading,
.anime-player.ui-hidden .player-controls { opacity: 0; pointer-events: none; }
.anime-player.ui-hidden .player-heading { transform: translateY(-8px); }
.anime-player.ui-hidden .player-controls { transform: translateY(8px); }
.anime-player.ui-hidden .hidden-progress { opacity: 1; }
.anime-player:fullscreen { clip-path: none; }
.anime-player.web-fullscreen { position: fixed; inset: 0; z-index: 1000; width: 100vw; height: 100vh; height: 100dvh; min-height: 0; clip-path: none; }
</style>
