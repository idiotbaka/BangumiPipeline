<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import fullscreenIcon from '../assets/player-icons/fullscreen.svg?raw'
import fullscreenExitIcon from '../assets/player-icons/fullscreen-exit.svg?raw'
import episodePickerIcon from '../assets/player-icons/episode-picker.svg?raw'
import volumeIcon from '../assets/player-icons/volume.svg?raw'
import volumeMutedIcon from '../assets/player-icons/volume-muted.svg?raw'
import webFullscreenIcon from '../assets/player-icons/web-fullscreen.svg?raw'
import webFullscreenExitIcon from '../assets/player-icons/web-fullscreen-exit.svg?raw'

interface OPSkipSegment {
  startSeconds: number
  endSeconds: number
  promptStartSeconds: number
  promptEndSeconds: number
  seekToSeconds: number
}

interface SelectableEpisode {
  key: string
  mediaId: number
  label: string
  title: string
  summary: string
  hasCover: boolean
  coverURL: string
}

interface MediaInfo {
  format: string
  videoCodec: string
  audioCodec: string
  hasInternalSubtitles: boolean
  hasExternalSubtitles: boolean
  action: string
}

interface Props {
  mediaId: number
  src: string
  poster: string
  title: string
  startTime: number
  opSkip: OPSkipSegment | null
  mediaInfo: MediaInfo | null
  episodes: SelectableEpisode[]
  selectedEpisodeKey: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (event: 'progress', value: { mediaId: number; positionSeconds: number; durationSeconds: number }): void
  (event: 'select-episode', episode: SelectableEpisode): void
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
const fullscreenActive = ref(false)
const timelineBubbleVisible = ref(false)
const timelineBubblePercent = ref(0)
const timelineBubbleTime = ref(0)
const volumeBubbleVisible = ref(false)
const controlsVisible = ref(true)
const resumeApplied = ref(false)
const hasPlayed = ref(false)
const mediaReady = ref(false)
const bufferEnd = ref(0)
const opSkipDismissed = ref(false)
const episodePickerOpen = ref(false)
const failedEpisodeCovers = ref<Set<string>>(new Set())
const contextMenuVisible = ref(false)
const contextMenuPosition = ref({ left: 16, top: 16 })
const statisticsVisible = ref(false)
const copyFeedbackVisible = ref(false)
const copyFeedbackMessage = ref('')
const debugState = ref({
  width: 0,
  height: 0,
  readyState: 0,
  networkState: 0,
  totalFrames: 0,
  droppedFrames: 0,
  corruptedFrames: 0,
})
let previousBodyOverflow = ''
let progressTimer: ReturnType<typeof setInterval> | null = null
let controlsTimer: ReturnType<typeof setTimeout> | null = null
let bufferTimer: ReturnType<typeof setInterval> | null = null
let volumeBubbleTimer: ReturnType<typeof setTimeout> | null = null
let copyFeedbackTimer: ReturnType<typeof setTimeout> | null = null
const bufferRangeToleranceSeconds = 0.5

const activeOPSkip = computed(() => normalizeOPSkip(props.opSkip))
const progressStyle = computed(() => {
  const progress = duration.value > 0 ? (currentTime.value / duration.value) * 100 : 0
  const buffered = duration.value > 0 ? (bufferEnd.value / duration.value) * 100 : progress
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
    '--op-width': `${Math.max(0, opEnd - opStart)}%`,
    '--op-end': `${opEnd}%`,
    '--op-opacity': activeOPSkip.value && opEnd > opStart ? '1' : '0',
  }
})
const volumeStyle = computed(() => {
  const level = muted.value ? 0 : Math.max(0, Math.min(1, volume.value)) * 100
  return { '--volume': `${level}%` }
})
const volumePercent = computed(() => Math.round((muted.value ? 0 : Math.max(0, Math.min(1, volume.value))) * 100))
const timelineBubbleStyle = computed(() => ({ '--timeline-hover': `${timelineBubblePercent.value}%` }))
const timelineBubbleLabel = computed(() => formatTime(timelineBubbleTime.value))
const playerLoading = computed(() => Boolean(props.src) && !mediaReady.value && !errorMessage.value)
const canControlPlayback = computed(() => mediaReady.value && !errorMessage.value)
const withinOPSkipPrompt = computed(() => {
  const segment = activeOPSkip.value
  return Boolean(
    segment &&
      currentTime.value >= segment.promptStartSeconds &&
      currentTime.value < segment.promptEndSeconds,
  )
})
const opSkipVisible = computed(() => canControlPlayback.value && withinOPSkipPrompt.value && !opSkipDismissed.value)
const volumeIconSrc = computed(() => muted.value ? volumeMutedIcon : volumeIcon)
const volumeLabel = computed(() => muted.value ? '取消静音' : '静音')
const webFullscreenIconSrc = computed(() => webFullscreen.value ? webFullscreenExitIcon : webFullscreenIcon)
const webFullscreenLabel = computed(() => webFullscreen.value ? '退出网页全屏' : '网页全屏')
const fullscreenIconSrc = computed(() => fullscreenActive.value ? fullscreenExitIcon : fullscreenIcon)
const fullscreenLabel = computed(() => fullscreenActive.value ? '退出全屏' : '全屏')
const hasEpisodes = computed(() => props.episodes.length > 0)
const contextMenuStyle = computed(() => ({
  left: `${contextMenuPosition.value.left}px`,
  top: `${contextMenuPosition.value.top}px`,
}))
const currentMediaRows = computed(() => [
  { label: '当前影片', value: props.title || '未命名媒体' },
  {
    label: '媒体 / 进度',
    value: `${props.mediaId > 0 ? `#${props.mediaId}` : '未分配'} · ${formatTime(currentTime.value)} / ${formatTime(duration.value)}`,
  },
])
const mediaInfoRows = computed(() => {
  const info = props.mediaInfo
  return [
    {
      label: '封装 / 编码',
      value: `${formatMediaLabel(info?.format, '浏览器媒体流')} · ${formatCodecLabel(info?.videoCodec)} / ${formatCodecLabel(info?.audioCodec)}`,
    },
    { label: '字幕 / 处理', value: `${subtitleLabel(info)} · ${mediaActionLabel(info?.action)}` },
  ]
})
const debugRows = computed(() => {
  const bufferedPercent = duration.value > 0 ? Math.round((bufferEnd.value / duration.value) * 100) : 0
  const resolution = debugState.value.width > 0 ? `${debugState.value.width} × ${debugState.value.height}` : '等待媒体元数据'
  return [
    {
      label: '画面 / 缓冲',
      value: `${resolution} · ${formatTime(bufferEnd.value)} (${Math.max(0, Math.min(100, bufferedPercent))}%)`,
    },
    { label: '速率 / 音量', value: `${playbackRate.value.toFixed(2)}× · ${volumePercent.value}%${muted.value ? '（静音）' : ''}` },
    {
      label: '帧统计',
      value: `渲染 ${debugState.value.totalFrames.toLocaleString()} · 丢失 ${debugState.value.droppedFrames.toLocaleString()} · 损坏 ${debugState.value.corruptedFrames.toLocaleString()}`,
    },
    { label: '状态', value: `${readyStateLabel(debugState.value.readyState)} · ${networkStateLabel(debugState.value.networkState)}` },
  ]
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
    mediaReady.value = false
    opSkipDismissed.value = false
    episodePickerOpen.value = false
    contextMenuVisible.value = false
    resetDebugState()
    stopProgressTimer()
    stopBufferTimer()
    showControls()
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

onMounted(() => {
  window.addEventListener('keydown', handleWindowKeydown)
  document.addEventListener('fullscreenchange', handleFullscreenChange)
  document.addEventListener('pointerdown', closeContextMenu)
  handleFullscreenChange()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleWindowKeydown)
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  document.removeEventListener('pointerdown', closeContextMenu)
  reportProgress()
  stopProgressTimer()
  stopBufferTimer()
  stopControlsTimer()
  stopVolumeBubbleTimer()
  stopCopyFeedbackTimer()
  exitWebFullscreen()
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
      errorMessage.value = '浏览器暂时无法播放该视频'
    }
  } else {
    element.pause()
  }
}

function updateDuration() {
  const nextDuration = Number.isFinite(video.value?.duration) ? video.value?.duration ?? 0 : 0
  duration.value = nextDuration
  mediaReady.value = nextDuration > 0
  applyResumePosition()
  updateBuffered()
  updateDebugState()
}

function updateTime() {
  currentTime.value = video.value?.currentTime ?? 0
  updateBuffered()
}

function updateBuffered() {
  const element = video.value
  if (!element || duration.value <= 0 || element.buffered.length === 0) {
    bufferEnd.value = currentTime.value
    updateDebugState()
    return
  }
  const time = element.currentTime
  let end = time
  for (let index = 0; index < element.buffered.length; index++) {
    const start = element.buffered.start(index)
    const rangeEnd = element.buffered.end(index)
    if (time + bufferRangeToleranceSeconds >= start && time - bufferRangeToleranceSeconds <= rangeEnd) {
      end = Math.max(end, rangeEnd)
      continue
    }
    if (rangeEnd < time) {
      continue
    }
    if (start > time) {
      end = Math.max(end, rangeEnd)
      break
    }
  }
  bufferEnd.value = Math.max(time, Math.min(duration.value, end))
  updateDebugState()
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
  startBufferTimer()
  updateBuffered()
  scheduleControlsHide()
}

function handlePause() {
  playing.value = false
  showControls()
  reportProgress()
  stopProgressTimer()
  stopBufferTimer()
}

function handleEnded() {
  playing.value = false
  showControls()
  currentTime.value = duration.value
  reportProgress()
  stopProgressTimer()
  stopBufferTimer()
}

function handleWaiting() {
  if (!mediaReady.value) {
    showControls()
    return
  }
  buffering.value = true
  updateBuffered()
  showControls()
}

function handlePlaying() {
  if (!mediaReady.value) updateDuration()
  buffering.value = false
  startBufferTimer()
  updateBuffered()
  scheduleControlsHide()
}

function handleCanPlay() {
  updateDuration()
  buffering.value = false
  updateBuffered()
  if (playing.value) scheduleControlsHide()
}

function handleError() {
  errorMessage.value = '视频加载失败，请稍后重试'
  mediaReady.value = false
  stopBufferTimer()
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
  if (!playing.value || buffering.value || errorMessage.value || episodePickerOpen.value || contextMenuVisible.value) return
  controlsTimer = setTimeout(() => {
    if (playing.value && !buffering.value && !errorMessage.value && !episodePickerOpen.value && !contextMenuVisible.value) controlsVisible.value = false
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

function showVolumeBubble() {
  volumeBubbleVisible.value = true
  stopVolumeBubbleTimer()
  volumeBubbleTimer = setTimeout(() => {
    volumeBubbleVisible.value = false
    volumeBubbleTimer = null
  }, 1_100)
}

function stopVolumeBubbleTimer() {
  if (volumeBubbleTimer !== null) {
    clearTimeout(volumeBubbleTimer)
    volumeBubbleTimer = null
  }
}

function showTimelineBubble(event: PointerEvent) {
  updateTimelineBubble(event)
  if (duration.value > 0 && canControlPlayback.value) {
    timelineBubbleVisible.value = true
  }
}

function updateTimelineBubble(event: PointerEvent) {
  if (duration.value <= 0 || !canControlPlayback.value) {
    timelineBubbleVisible.value = false
    return
  }
  const target = event.currentTarget
  if (!(target instanceof HTMLElement)) return
  const rect = target.getBoundingClientRect()
  const ratio = rect.width > 0 ? (event.clientX - rect.left) / rect.width : 0
  const clampedRatio = Math.max(0, Math.min(1, ratio))
  timelineBubblePercent.value = clampedRatio * 100
  timelineBubbleTime.value = clampedRatio * duration.value
  timelineBubbleVisible.value = true
}

function hideTimelineBubble() {
  timelineBubbleVisible.value = false
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

function changeVolume(event: Event) {
  const value = Number((event.target as HTMLInputElement).value)
  if (!video.value || !Number.isFinite(value)) return
  video.value.volume = value
  video.value.muted = value === 0
  volume.value = value
  muted.value = value === 0
  showVolumeBubble()
}

function toggleMute() {
  if (!video.value) return
  if (video.value.muted && volume.value === 0) {
    video.value.volume = 0.6
    volume.value = 0.6
  }
  video.value.muted = !video.value.muted
  muted.value = video.value.muted
  showVolumeBubble()
}

function cyclePlaybackRate() {
  const rates = [1, 1.25, 1.5, 2]
  const index = rates.indexOf(playbackRate.value)
  playbackRate.value = rates[(index + 1) % rates.length]
  if (video.value) video.value.playbackRate = playbackRate.value
}

async function toggleFullscreen() {
  if (!player.value) return
  try {
    if (document.fullscreenElement) {
      await document.exitFullscreen()
      return
    }
    exitWebFullscreen()
    await nextTick()
    await player.value.requestFullscreen()
  } catch {
    showControls()
  } finally {
    handleFullscreenChange()
  }
}

function toggleEpisodePicker() {
  if (!hasEpisodes.value) return
  episodePickerOpen.value = !episodePickerOpen.value
  showControls()
  if (!episodePickerOpen.value) scheduleControlsHide()
}

function selectEpisode(episode: SelectableEpisode) {
  episodePickerOpen.value = false
  emit('select-episode', episode)
  scheduleControlsHide()
}

function openContextMenu(event: MouseEvent) {
  const element = player.value
  if (!element) return
  const rect = element.getBoundingClientRect()
  const margin = 16
  const menuWidth = Math.min(248, Math.max(0, rect.width - margin * 2))
  const menuHeight = 116
  contextMenuPosition.value = {
    left: Math.round(clampMenuPosition(event.clientX - rect.left, margin, rect.width - menuWidth - margin)),
    top: Math.round(clampMenuPosition(event.clientY - rect.top, margin, rect.height - menuHeight - margin)),
  }
  updateDebugState()
  contextMenuVisible.value = true
  showControls()
}

function closeContextMenu() {
  if (!contextMenuVisible.value) return
  contextMenuVisible.value = false
  scheduleControlsHide()
}

function showStatistics() {
  updateDebugState()
  statisticsVisible.value = true
  closeContextMenu()
}

function closeStatistics() {
  statisticsVisible.value = false
}

async function copyVideoLink() {
  closeContextMenu()
  try {
    await copyText(window.location.href)
    showCopyFeedback('视频链接已复制')
  } catch {
    showCopyFeedback('复制失败，请检查浏览器权限')
  }
}

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }
  const field = document.createElement('textarea')
  field.value = value
  field.setAttribute('readonly', '')
  field.style.position = 'fixed'
  field.style.opacity = '0'
  document.body.append(field)
  field.select()
  const copied = document.execCommand('copy')
  field.remove()
  if (!copied) throw new Error('copy failed')
}

function showCopyFeedback(message: string) {
  copyFeedbackMessage.value = message
  copyFeedbackVisible.value = true
  stopCopyFeedbackTimer()
  copyFeedbackTimer = setTimeout(() => {
    copyFeedbackVisible.value = false
    copyFeedbackTimer = null
  }, 1_800)
}

function stopCopyFeedbackTimer() {
  if (copyFeedbackTimer !== null) {
    clearTimeout(copyFeedbackTimer)
    copyFeedbackTimer = null
  }
}

function episodeCoverAvailable(episode: SelectableEpisode) {
  return episode.hasCover && Boolean(episode.coverURL) && !failedEpisodeCovers.value.has(episode.key)
}

function markEpisodeCoverFailed(key: string) {
  const failed = new Set(failedEpisodeCovers.value)
  failed.add(key)
  failedEpisodeCovers.value = failed
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

function handleFullscreenChange() {
  fullscreenActive.value = document.fullscreenElement === player.value
}

function handleWindowKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && episodePickerOpen.value) {
    episodePickerOpen.value = false
    scheduleControlsHide()
    event.preventDefault()
    return
  }
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

function clampPercent(value: number) {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, value))
}

function clampMenuPosition(value: number, minimum: number, maximum: number) {
  return Math.max(minimum, Math.min(value, Math.max(minimum, maximum)))
}

function updateDebugState() {
  const element = video.value
  if (!element) {
    resetDebugState()
    return
  }
  const quality = element.getVideoPlaybackQuality?.()
  debugState.value = {
    width: element.videoWidth,
    height: element.videoHeight,
    readyState: element.readyState,
    networkState: element.networkState,
    totalFrames: quality?.totalVideoFrames ?? 0,
    droppedFrames: quality?.droppedVideoFrames ?? 0,
    corruptedFrames: quality?.corruptedVideoFrames ?? 0,
  }
}

function resetDebugState() {
  debugState.value = {
    width: 0,
    height: 0,
    readyState: 0,
    networkState: 0,
    totalFrames: 0,
    droppedFrames: 0,
    corruptedFrames: 0,
  }
}

function formatMediaLabel(value: string | undefined, fallback: string) {
  return value?.trim() ? value.trim().toUpperCase() : fallback
}

function formatCodecLabel(value: string | undefined) {
  const normalized = value?.trim().toLowerCase() ?? ''
  const labels: Record<string, string> = {
    h264: 'H.264 / AVC',
    hevc: 'H.265 / HEVC',
    h265: 'H.265 / HEVC',
    av1: 'AV1',
    vp9: 'VP9',
    aac: 'AAC',
    opus: 'Opus',
    mp3: 'MP3',
    ac3: 'AC-3',
    eac3: 'E-AC-3',
  }
  return labels[normalized] ?? (normalized ? normalized.toUpperCase() : '未知')
}

function subtitleLabel(info: MediaInfo | null) {
  if (!info) return '未知'
  if (info.action === 'burn_subtitles') return '已压制到画面'
  if (info.hasInternalSubtitles && info.hasExternalSubtitles) return '内嵌 + 外挂'
  if (info.hasInternalSubtitles) return '内嵌字幕'
  if (info.hasExternalSubtitles) return '外挂字幕'
  return '未检测到字幕'
}

function mediaActionLabel(action: string | undefined) {
  const labels: Record<string, string> = {
    copy: '直接复制',
    remux: '快速重封装',
    transcode: 'H.264 / AAC 转码',
    burn_subtitles: '字幕压制转码',
  }
  return labels[action?.trim() ?? ''] ?? '媒体处理完成'
}

function readyStateLabel(state: number) {
  const labels = ['未载入', '元数据就绪', '当前帧可用', '后续数据可用', '可连续播放']
  return labels[state] ?? '未知'
}

function networkStateLabel(state: number) {
  const labels = ['空闲', '空闲', '加载中', '无可用媒体源']
  return labels[state] ?? '未知'
}

function normalizeOPSkip(segment: OPSkipSegment | null) {
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
    class="anime-player"
    :class="{ 'web-fullscreen': webFullscreen, 'ui-hidden': !controlsVisible && playing, 'is-loading': playerLoading }"
    tabindex="0"
    :aria-label="`正在播放 ${title}`"
    @pointermove="handlePlayerInteraction"
    @pointerdown="handlePlayerInteraction"
    @focusin="handlePlayerInteraction"
    @keydown="handlePlayerInteraction"
    @keydown.space.prevent="togglePlay"
    @contextmenu.prevent="openContextMenu"
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

    <button
      v-if="canControlPlayback && !playing && !buffering && !errorMessage"
      class="center-play"
      type="button"
      aria-label="播放"
      @click="togglePlay"
    >
      <i aria-hidden="true" />
    </button>
    <div v-if="playerLoading" class="loading-mark" aria-live="polite"><i /><span>播放器加载中</span></div>
    <div v-if="buffering" class="buffering-mark" aria-label="正在缓冲"><i /><span>BUFFERING</span></div>
    <div v-if="errorMessage" class="player-error"><span>!</span><p>{{ errorMessage }}</p></div>

    <div class="player-heading">
      <span>NOW PLAYING</span>
      <p>{{ title }}</p>
    </div>

    <Transition name="op-skip">
      <button v-if="opSkipVisible" class="op-skip-button" type="button" aria-label="跳过 OP" @click.stop="skipOP">
        <span>跳过 OP</span>
        <i aria-hidden="true" />
      </button>
    </Transition>

    <Transition name="statistics-card">
      <aside
        v-if="statisticsVisible"
        class="player-statistics-card"
        role="dialog"
        aria-label="播放器信息与调试数据"
        @pointerdown.stop
        @click.stop
        @contextmenu.prevent.stop
      >
        <header class="statistics-card-header">
          <div>
            <span>BakaVIP H5Player</span>
            <strong>统计信息</strong>
          </div>
          <button class="statistics-card-close" type="button" aria-label="关闭统计信息" @click="closeStatistics">×</button>
        </header>
        <div class="context-menu-content">
          <section class="context-menu-section">
            <h2>媒体信息</h2>
            <dl>
              <div v-for="row in currentMediaRows" :key="row.label" class="context-menu-data-row">
                <dt>{{ row.label }}</dt><dd>{{ row.value }}</dd>
              </div>
              <div v-for="row in mediaInfoRows" :key="row.label" class="context-menu-data-row">
                <dt>{{ row.label }}</dt><dd>{{ row.value }}</dd>
              </div>
            </dl>
          </section>
          <section class="context-menu-section debug-section">
            <h2>调试信息</h2>
            <dl>
              <div v-for="row in debugRows" :key="row.label" class="context-menu-data-row">
                <dt>{{ row.label }}</dt><dd>{{ row.value }}</dd>
              </div>
            </dl>
          </section>
        </div>
      </aside>
    </Transition>

    <Transition name="context-menu">
      <aside
        v-if="contextMenuVisible"
        class="player-context-menu"
        :style="contextMenuStyle"
        role="menu"
        aria-label="BakaVIP H5Player 播放器菜单"
        @pointerdown.stop
        @click.stop
        @contextmenu.prevent.stop
      >
        <p class="context-menu-brand">BakaVIP H5Player</p>
        <button class="context-menu-action" type="button" role="menuitem" @click="showStatistics">
          <i aria-hidden="true" />
          <span><strong>显示统计信息</strong><small>显示媒体与播放调试数据</small></span>
        </button>
        <button class="context-menu-action" type="button" role="menuitem" @click="copyVideoLink">
          <i aria-hidden="true" />
          <span><strong>复制视频链接</strong><small>复制当前页面地址</small></span>
        </button>
      </aside>
    </Transition>

    <Transition name="copy-feedback">
      <div v-if="copyFeedbackVisible" class="copy-feedback" role="status">{{ copyFeedbackMessage }}</div>
    </Transition>

    <div class="player-controls">
      <div
        class="timeline-wrap"
        :style="progressStyle"
        @pointerenter="showTimelineBubble"
        @pointermove="updateTimelineBubble"
        @pointerleave="hideTimelineBubble"
      >
        <div
          class="timeline-bubble"
          :class="{ visible: timelineBubbleVisible }"
          :style="timelineBubbleStyle"
          aria-hidden="true"
        >
          {{ timelineBubbleLabel }}
        </div>
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
      </div>
      <div class="control-row">
        <button
          class="play-control"
          type="button"
          :aria-label="playing ? '暂停' : '播放'"
          :disabled="!canControlPlayback"
          @click="togglePlay"
        >
          <i :class="{ pause: playing }" aria-hidden="true" />
        </button>
        <span class="time-code">{{ formatTime(currentTime) }} <i>/</i> {{ formatTime(duration) }}</span>
        <div class="control-spacer" />
        <button
          class="icon-control episode-picker-control"
          :class="{ active: episodePickerOpen }"
          type="button"
          :disabled="!hasEpisodes"
          :aria-expanded="episodePickerOpen"
          aria-controls="episode-picker"
          aria-label="选集"
          title="选集"
          @click="toggleEpisodePicker"
        >
          <i aria-hidden="true" v-html="episodePickerIcon" />
        </button>
        <div class="volume-cluster">
          <div class="volume-bubble" :class="{ visible: volumeBubbleVisible }" :aria-hidden="!volumeBubbleVisible" aria-live="polite">{{ volumePercent }}%</div>
          <button class="icon-control volume-control" type="button" :aria-label="volumeLabel" :title="volumeLabel" @click="toggleMute">
            <i aria-hidden="true" v-html="volumeIconSrc" />
          </button>
          <input
            class="volume-range"
            type="range"
            min="0"
            max="1"
            step="0.05"
            :value="muted ? 0 : volume"
            :style="volumeStyle"
            aria-label="音量"
            @pointerdown="showVolumeBubble"
            @input="changeVolume"
          />
        </div>
        <button class="text-control rate" type="button" @click="cyclePlaybackRate">{{ playbackRate }}×</button>
        <button
          class="icon-control web-fullscreen-control"
          :class="{ active: webFullscreen }"
          type="button"
          :aria-label="webFullscreenLabel"
          :title="webFullscreenLabel"
          @click="toggleWebFullscreen"
        >
          <i aria-hidden="true" v-html="webFullscreenIconSrc" />
        </button>
        <button
          class="icon-control fullscreen-control"
          :class="{ active: fullscreenActive }"
          type="button"
          :aria-label="fullscreenLabel"
          :title="fullscreenLabel"
          @click="toggleFullscreen"
        >
          <i aria-hidden="true" v-html="fullscreenIconSrc" />
        </button>
      </div>
      <Transition name="episode-picker">
        <aside v-if="episodePickerOpen" id="episode-picker" class="episode-picker" aria-label="选集">
          <header>
            <div><span>EPISODES</span><strong>选集</strong></div>
            <small>{{ episodes.length }} 集可播放</small>
          </header>
          <div class="episode-picker-list">
            <button
              v-for="episode in episodes"
              :key="episode.key"
              class="episode-picker-item"
              :class="{ selected: selectedEpisodeKey === episode.key }"
              type="button"
              :aria-current="selectedEpisodeKey === episode.key ? 'true' : undefined"
              @click="selectEpisode(episode)"
            >
              <div class="episode-picker-thumb">
                <img
                  v-if="episodeCoverAvailable(episode)"
                  :src="episode.coverURL"
                  :alt="episode.title || episode.label"
                  loading="lazy"
                  @error="markEpisodeCoverFailed(episode.key)"
                />
                <span v-else>{{ episode.label }}</span>
              </div>
              <div class="episode-picker-copy">
                <span class="episode-picker-label">{{ episode.label }}</span>
                <strong>{{ episode.title || episode.label }}</strong>
                <p>{{ episode.summary || '该话暂无剧情简介。' }}</p>
              </div>
              <i v-if="selectedEpisodeKey === episode.key" class="episode-playing" aria-label="正在播放" />
            </button>
          </div>
        </aside>
      </Transition>
    </div>
    <div class="hidden-progress" :style="progressStyle" aria-hidden="true" />
  </section>
</template>

<style scoped>
.anime-player { position: relative; width: 100%; height: 100%; min-height: 480px; overflow: hidden; color: #fff; background: #101522; outline: 0; clip-path: polygon(0 0, calc(100% - 20px) 0, 100% 20px, 100% 100%, 20px 100%, 0 calc(100% - 20px)); }
.anime-player video { width: 100%; height: 100%; object-fit: contain; background: #090d17; cursor: pointer; }
.anime-player.is-loading video { cursor: progress; }
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
.op-skip-button { position: absolute; right: 32px; bottom: 104px; z-index: 5; min-width: 116px; height: 42px; display: inline-flex; align-items: center; justify-content: center; gap: 10px; padding: 0 17px 0 18px; color: #fff; font-size: 14px; font-weight: 700; border: 1px solid rgba(255,255,255,.48); border-radius: 8px; background: linear-gradient(135deg, rgba(255,95,158,.92), rgba(255,189,77,.9)); box-shadow: 0 12px 28px rgba(0,0,0,.34), 0 0 0 1px rgba(255,255,255,.12) inset; backdrop-filter: blur(12px); }
.op-skip-button:hover { border-color: rgba(255,255,255,.72); box-shadow: 0 16px 34px rgba(0,0,0,.4), 0 0 18px rgba(255,189,77,.22); transform: translateY(-1px); }
.op-skip-button i { width: 0; height: 0; border-top: 6px solid transparent; border-bottom: 6px solid transparent; border-left: 9px solid currentColor; filter: drop-shadow(0 1px 3px rgba(0,0,0,.25)); }
.op-skip-enter-active, .op-skip-leave-active { transition: opacity 180ms ease, transform 180ms ease; }
.op-skip-enter-from, .op-skip-leave-to { opacity: 0; transform: translateX(24px) scale(.98); }
.player-context-menu { position: absolute; z-index: 10; width: min(248px, calc(100% - 32px)); overflow: hidden; color: rgba(255,255,255,.94); border: 1px solid rgba(142,232,242,.42); background: linear-gradient(145deg, rgba(19,27,45,.98), rgba(10,15,27,.97)); box-shadow: 0 18px 42px rgba(0,0,0,.54), 0 0 0 1px rgba(255,255,255,.06) inset; clip-path: polygon(0 0, calc(100% - 14px) 0, 100% 14px, 100% 100%, 14px 100%, 0 calc(100% - 14px)); }
.context-menu-brand { padding: 10px 13px 9px; color: var(--cyan-300); font-family: var(--font-mono); font-size: 11px; letter-spacing: 1px; border-bottom: 1px solid rgba(142,232,242,.2); background: rgba(4,9,18,.36); }
.context-menu-action { width: 100%; display: grid; grid-template-columns: 18px minmax(0, 1fr); align-items: center; gap: 9px; padding: 10px 13px; color: inherit; text-align: left; background: transparent; border-bottom: 1px solid rgba(255,255,255,.07); transition: color 150ms ease, background 150ms ease; }
.context-menu-action:last-child { border-bottom: 0; }
.context-menu-action:hover { color: #fff; background: linear-gradient(90deg, rgba(255,95,158,.19), rgba(73,214,233,.08)); }
.context-menu-action > i { width: 7px; height: 7px; background: var(--pink-300); box-shadow: 0 0 0 3px rgba(255,159,189,.12); transform: rotate(45deg); }
.context-menu-action:last-child > i { background: var(--cyan-300); box-shadow: 0 0 0 3px rgba(142,232,242,.1); }
.context-menu-action span { min-width: 0; display: grid; gap: 2px; }
.context-menu-action strong { font-size: 12px; font-weight: 500; }
.context-menu-action small { overflow: hidden; color: rgba(255,255,255,.46); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.player-statistics-card { position: absolute; top: 18px; left: 18px; z-index: 9; width: min(520px, calc(100% - 36px)); max-height: min(260px, calc(100% - 36px)); overflow: hidden; color: rgba(255,255,255,.94); border: 1px solid rgba(142,232,242,.42); background: linear-gradient(145deg, rgba(19,27,45,.98), rgba(10,15,27,.97)); box-shadow: 0 22px 52px rgba(0,0,0,.58), 0 0 0 1px rgba(255,255,255,.06) inset; clip-path: polygon(0 0, calc(100% - 17px) 0, 100% 17px, 100% 100%, 17px 100%, 0 calc(100% - 17px)); }
.player-statistics-card::before { content: ''; position: absolute; inset: 0; z-index: -1; opacity: .32; pointer-events: none; background: linear-gradient(rgba(142,232,242,.08) 1px, transparent 1px), linear-gradient(90deg, rgba(255,159,189,.07) 1px, transparent 1px); background-size: 28px 28px; }
.statistics-card-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 9px 12px 8px; border-bottom: 1px solid rgba(142,232,242,.2); background: rgba(4,9,18,.36); }
.statistics-card-header div { display: flex; align-items: baseline; gap: 10px; min-width: 0; }
.statistics-card-header span { overflow: hidden; color: var(--cyan-300); font-family: var(--font-mono); font-size: 11px; letter-spacing: 1px; text-overflow: ellipsis; white-space: nowrap; }
.statistics-card-header strong { color: #fff; font-size: 13px; letter-spacing: .4px; white-space: nowrap; }
.statistics-card-close { width: 24px; height: 24px; display: grid; place-items: center; flex: 0 0 auto; color: rgba(255,255,255,.76); font-family: var(--font-mono); font-size: 18px; line-height: 1; border: 1px solid rgba(255,255,255,.12); background: rgba(255,255,255,.04); clip-path: polygon(0 0, calc(100% - 6px) 0, 100% 6px, 100% 100%, 6px 100%, 0 calc(100% - 6px)); transition: color 160ms ease, border-color 160ms ease, background 160ms ease; }
.statistics-card-close:hover { color: #fff; border-color: rgba(255,159,189,.62); background: rgba(255,95,158,.18); }
.context-menu-content { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
.context-menu-section { min-width: 0; padding: 8px 11px 10px; }
.context-menu-section + .context-menu-section { border-left: 1px dashed rgba(142,232,242,.18); }
.context-menu-section h2 { display: flex; align-items: center; gap: 6px; margin-bottom: 6px; color: var(--pink-200); font-family: var(--font-mono); font-size: 10px; font-weight: 500; letter-spacing: 1px; }
.context-menu-section h2::before { content: ''; width: 5px; height: 5px; background: var(--pink-300); transform: rotate(45deg); }
.context-menu-section dl { display: grid; gap: 4px; font-size: 10px; line-height: 1.35; }
.context-menu-data-row { min-width: 0; display: grid; grid-template-columns: 64px minmax(0, 1fr); gap: 7px; }
.context-menu-section dt { color: rgba(255,255,255,.48); white-space: nowrap; }
.context-menu-section dd { min-width: 0; overflow: hidden; color: rgba(255,255,255,.88); text-align: right; text-overflow: ellipsis; white-space: nowrap; }
.context-menu-section:first-child .context-menu-data-row:first-child dd { color: #fff; }
.debug-section { background: rgba(73,214,233,.035); }
.context-menu-enter-active, .context-menu-leave-active { transition: opacity 150ms ease, transform 150ms ease; }
.context-menu-enter-from, .context-menu-leave-to { opacity: 0; transform: translateY(8px) scale(.985); }
.statistics-card-enter-active, .statistics-card-leave-active { transition: opacity 150ms ease, transform 150ms ease; }
.statistics-card-enter-from, .statistics-card-leave-to { opacity: 0; transform: translateY(-8px) scale(.985); }
.copy-feedback { position: absolute; bottom: 84px; left: 50%; z-index: 11; padding: 9px 14px; color: #fff; font-size: 12px; border: 1px solid rgba(142,232,242,.48); background: rgba(16,26,43,.94); box-shadow: 0 12px 28px rgba(0,0,0,.36); clip-path: polygon(0 0, calc(100% - 9px) 0, 100% 9px, 100% 100%, 9px 100%, 0 calc(100% - 9px)); transform: translateX(-50%); }
.copy-feedback-enter-active, .copy-feedback-leave-active { transition: opacity 150ms ease, transform 150ms ease; }
.copy-feedback-enter-from, .copy-feedback-leave-to { opacity: 0; transform: translate(-50%, 7px); }
.player-controls { position: absolute; right: 24px; bottom: 20px; left: 24px; transition: opacity 180ms ease, transform 180ms ease; }
.timeline-wrap { position: relative; height: 18px; display: flex; align-items: center; --op-start: 0%; --op-width: 0%; --op-opacity: 0; }
.timeline-wrap::before { content: ''; position: absolute; left: var(--op-start); z-index: 1; width: var(--op-width); height: 8px; border-radius: 999px; pointer-events: none; opacity: var(--op-opacity); background: linear-gradient(90deg, rgba(255,241,170,.88), rgba(255,189,77,.88)); box-shadow: 0 0 12px rgba(255,189,77,.38), 0 0 0 1px rgba(255,255,255,.22) inset; }
.timeline-bubble { position: absolute; bottom: calc(100% + 8px); left: var(--timeline-hover); z-index: 4; min-width: 58px; padding: 6px 10px 7px; color: rgba(255,255,255,.94); font-family: var(--font-mono); font-size: 12px; line-height: 1; text-align: center; pointer-events: none; opacity: 0; background: rgba(78,82,92,.92); border: 1px solid rgba(255,255,255,.14); border-radius: 7px; box-shadow: 0 10px 22px rgba(0,0,0,.24); transform: translate(-50%, 4px); transition: opacity 140ms ease, transform 140ms ease; }
.timeline-bubble::after { content: ''; position: absolute; top: 100%; left: 50%; width: 8px; height: 8px; background: rgba(78,82,92,.92); border-right: 1px solid rgba(255,255,255,.14); border-bottom: 1px solid rgba(255,255,255,.14); transform: translate(-50%, -4px) rotate(45deg); }
.timeline-bubble.visible { opacity: 1; transform: translate(-50%, 0); }
.timeline { position: relative; z-index: 2; width: 100%; height: 4px; display: block; appearance: none; cursor: pointer; background: linear-gradient(90deg, var(--pink-400) 0 var(--progress), rgba(142,232,242,.58) var(--progress) var(--buffered), rgba(255,255,255,.28) var(--buffered) 100%); }
.timeline:disabled { cursor: wait; opacity: .62; }
.timeline::-webkit-slider-thumb { width: 13px; height: 13px; appearance: none; background: #fff; border: 3px solid var(--pink-400); transform: rotate(45deg); box-shadow: 0 2px 8px rgba(0,0,0,.3); }
.control-row { height: 48px; display: flex; align-items: center; gap: 12px; padding-top: 4px; }
.play-control, .icon-control { width: 36px; height: 36px; display: grid; place-items: center; flex: 0 0 36px; color: rgba(255,255,255,.86); border: 1px solid transparent; border-radius: 8px; background: rgba(9,13,23,.2); transition: color 160ms ease, border-color 160ms ease, background 160ms ease, transform 160ms ease; }
.play-control:hover:not(:disabled), .icon-control:hover, .icon-control.active { color: #fff; border-color: rgba(142,232,242,.44); background: rgba(73,214,233,.16); box-shadow: 0 0 0 1px rgba(255,255,255,.06) inset; transform: translateY(-1px); }
.play-control:disabled { cursor: wait; opacity: .45; transform: none; }
.icon-control:disabled { cursor: not-allowed; opacity: .4; transform: none; }
.play-control i:not(.pause) { width: 0; height: 0; margin-left: 3px; border-top: 7px solid transparent; border-bottom: 7px solid transparent; border-left: 11px solid currentColor; }
.play-control i.pause { width: 11px; height: 14px; border-right: 4px solid currentColor; border-left: 4px solid currentColor; }
.time-code { min-width: 104px; color: rgba(255,255,255,.82); font-family: var(--font-mono); font-size: 13px; white-space: nowrap; }
.time-code i { padding: 0 4px; color: var(--pink-300); font-style: normal; }
.control-spacer { flex: 1; }
.volume-cluster { position: relative; display: inline-flex; align-items: center; gap: 10px; }
.volume-bubble { position: absolute; bottom: calc(100% + 10px); left: 50%; z-index: 3; min-width: 46px; padding: 5px 9px 6px; color: rgba(255,255,255,.94); font-family: var(--font-mono); font-size: 12px; line-height: 1; text-align: center; pointer-events: none; opacity: 0; background: rgba(78,82,92,.92); border: 1px solid rgba(255,255,255,.14); border-radius: 7px; box-shadow: 0 10px 22px rgba(0,0,0,.24); transform: translate(-50%, 4px); transition: opacity 140ms ease, transform 140ms ease; }
.volume-bubble::after { content: ''; position: absolute; top: 100%; left: 50%; width: 8px; height: 8px; background: rgba(78,82,92,.92); border-right: 1px solid rgba(255,255,255,.14); border-bottom: 1px solid rgba(255,255,255,.14); transform: translate(-50%, -4px) rotate(45deg); }
.volume-bubble.visible { opacity: 1; transform: translate(-50%, 0); }
.icon-control > i { width: 20px; height: 20px; display: grid; place-items: center; color: currentColor; filter: drop-shadow(0 2px 6px rgba(0,0,0,.35)); }
.icon-control :deep(svg) { width: 100%; height: 100%; display: block; }
.icon-control :deep(path) { fill: currentColor; }
.volume-control > i { width: 22px; height: 22px; }
.episode-picker-control { border-radius: 0; clip-path: polygon(0 0, calc(100% - 8px) 0, 100% 8px, 100% 100%, 8px 100%, 0 calc(100% - 8px)); }
.episode-picker-control > i { width: 18px; height: 18px; }
.text-control { height: 32px; min-width: 42px; padding: 0 8px; color: rgba(255,255,255,.78); font-family: var(--font-mono); font-size: 13px; letter-spacing: .2px; border: 1px solid transparent; border-radius: 8px; background: rgba(9,13,23,.2); transition: color 160ms ease, border-color 160ms ease, background 160ms ease; }
.text-control:hover { color: #fff; border-color: rgba(255,95,158,.42); background: rgba(255,95,158,.14); }
.text-control.rate { min-width: 46px; }
.volume-range { width: 84px; height: 3px; appearance: none; cursor: pointer; background: linear-gradient(90deg, rgba(142,232,242,.86) 0 var(--volume), rgba(255,255,255,.3) var(--volume) 100%); }
.volume-range::-webkit-slider-thumb { width: 9px; height: 9px; appearance: none; background: var(--cyan-300); transform: rotate(45deg); }
.episode-picker { position: absolute; right: 0; bottom: calc(100% + 12px); z-index: 6; width: min(430px, calc(100vw - 48px)); max-height: min(390px, calc(100vh - 176px)); display: grid; grid-template-rows: auto minmax(0, 1fr); overflow: hidden; color: rgba(255,255,255,.94); border: 1px solid rgba(142,232,242,.34); border-radius: 0; background: linear-gradient(145deg, rgba(24,32,50,.97), rgba(13,18,31,.96)); box-shadow: 0 20px 48px rgba(0,0,0,.48), 0 0 0 1px rgba(255,255,255,.05) inset; clip-path: polygon(0 0, calc(100% - 18px) 0, 100% 18px, 100% 100%, 18px 100%, 0 calc(100% - 18px)); }
.episode-picker::before { content: ''; position: absolute; inset: 0; pointer-events: none; opacity: .35; background: linear-gradient(rgba(142,232,242,.08) 1px, transparent 1px), linear-gradient(90deg, rgba(255,159,189,.07) 1px, transparent 1px); background-size: 28px 28px; }
.episode-picker::after { content: ''; position: absolute; inset: 11px; z-index: 0; pointer-events: none; border: 1px solid rgba(255,255,255,.1); clip-path: polygon(0 0, calc(100% - 9px) 0, 100% 9px, 100% 100%, 9px 100%, 0 calc(100% - 9px)); }
.episode-picker > header { position: relative; z-index: 1; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 13px 16px 12px; border-bottom: 1px solid rgba(255,255,255,.1); background: rgba(8,12,22,.36); }
.episode-picker > header div { display: grid; gap: 2px; }
.episode-picker > header span { color: var(--cyan-300); font-family: var(--font-mono); font-size: 11px; letter-spacing: 1.4px; }
.episode-picker > header strong { font-size: 15px; }
.episode-picker > header small { color: rgba(255,255,255,.54); font-family: var(--font-mono); font-size: 12px; white-space: nowrap; }
.episode-picker-list { position: relative; z-index: 1; min-height: 0; overflow-y: auto; padding: 7px; }
.episode-picker-item { position: relative; width: 100%; display: grid; grid-template-columns: 102px minmax(0, 1fr); gap: 11px; padding: 8px; color: inherit; text-align: left; border: 1px solid transparent; border-radius: 0; background: transparent; clip-path: polygon(0 0, calc(100% - 9px) 0, 100% 9px, 100% 100%, 9px 100%, 0 calc(100% - 9px)); transition: border-color 160ms ease, background 160ms ease; }
.episode-picker-item:hover { border-color: rgba(142,232,242,.36); background: rgba(142,232,242,.09); }
.episode-picker-item.selected { border-color: rgba(255,159,189,.58); background: linear-gradient(100deg, rgba(255,95,158,.22), rgba(73,214,233,.11)); }
.episode-picker-item.selected::before { content: ''; position: absolute; top: 50%; left: 4px; width: 7px; height: 7px; background: var(--pink-300); box-shadow: 0 0 0 3px rgba(255,159,189,.15); transform: translateY(-50%) rotate(45deg); }
.episode-picker-thumb { aspect-ratio: 16 / 9; overflow: hidden; display: grid; place-items: center; color: var(--pink-200); font-family: var(--font-mono); font-size: 12px; text-align: center; background: linear-gradient(135deg, rgba(255,95,158,.2), rgba(73,214,233,.15)); border-radius: 0; clip-path: polygon(0 0, calc(100% - 7px) 0, 100% 7px, 100% 100%, 7px 100%, 0 calc(100% - 7px)); }
.episode-picker-thumb img { width: 100%; height: 100%; object-fit: cover; }
.episode-picker-copy { min-width: 0; align-self: center; padding-right: 16px; }
.episode-picker-label { color: var(--pink-200); font-family: var(--font-mono); font-size: 12px; }
.episode-picker-copy strong { display: block; margin-top: 2px; overflow: hidden; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.episode-picker-copy p { margin-top: 4px; display: -webkit-box; overflow: hidden; color: rgba(255,255,255,.56); font-size: 12px; line-height: 1.45; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.episode-playing { position: absolute; top: 50%; right: 12px; width: 8px; height: 8px; background: var(--cyan-300); box-shadow: 0 0 0 3px rgba(142,232,242,.12), 0 0 12px var(--cyan-300); transform: translateY(-50%) rotate(45deg); }
.episode-picker-enter-active, .episode-picker-leave-active { transition: opacity 160ms ease, transform 160ms ease; }
.episode-picker-enter-from, .episode-picker-leave-to { opacity: 0; transform: translateY(10px) scale(.98); }
.loading-mark, .buffering-mark, .player-error { position: absolute; top: 50%; left: 50%; display: grid; place-items: center; gap: 10px; transform: translate(-50%, -50%); }
.loading-mark i { width: 46px; height: 46px; border: 1px solid rgba(142,232,242,.35); box-shadow: inset 0 0 0 6px rgba(142,232,242,.08), 0 0 18px rgba(142,232,242,.22); clip-path: polygon(var(--bevel-sm)); animation: bp-player-pulse 1.1s ease-in-out infinite; }
.loading-mark span { color: rgba(255,255,255,.86); font-size: 13px; letter-spacing: 1px; }
.buffering-mark i { width: 38px; height: 38px; border: 2px solid rgba(255,255,255,.2); border-top-color: var(--cyan-300); border-radius: 50%; animation: bp-spin .8s linear infinite; }
.buffering-mark span { font-family: var(--font-mono); font-size: 13px; letter-spacing: 2px; }
.player-error span { width: 42px; height: 42px; display: grid; place-items: center; color: var(--pink-300); font-family: var(--font-mono); font-size: 22px; border: 1px solid var(--pink-300); transform: rotate(45deg); }
.player-error p { font-size: 13px; }
.hidden-progress { position: absolute; right: 0; bottom: 0; left: 0; z-index: 2; height: 3px; pointer-events: none; opacity: 0; background: linear-gradient(90deg, transparent 0 var(--op-start), rgba(255,189,77,.72) var(--op-start) var(--op-end), transparent var(--op-end) 100%), linear-gradient(90deg, var(--pink-400) 0 var(--progress), rgba(142,232,242,.5) var(--progress) var(--buffered), transparent var(--buffered) 100%); filter: drop-shadow(0 -1px 3px rgba(255,95,158,.42)) drop-shadow(0 -1px 4px rgba(142,232,242,.26)); transition: opacity 180ms ease; }
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
.anime-player:fullscreen .op-skip-button,
.anime-player.web-fullscreen .op-skip-button { right: 44px; bottom: 118px; }
.anime-player:fullscreen .episode-picker,
.anime-player.web-fullscreen .episode-picker { width: min(470px, calc(100vw - 88px)); max-height: min(470px, calc(100vh - 220px)); }
.anime-player:fullscreen .player-context-menu,
.anime-player.web-fullscreen .player-context-menu { width: min(248px, calc(100vw - 88px)); }
.anime-player:fullscreen .player-statistics-card,
.anime-player.web-fullscreen .player-statistics-card { width: min(540px, calc(100vw - 88px)); max-height: 260px; }
.anime-player.web-fullscreen { position: fixed; inset: 0; z-index: 1000; width: 100vw; height: 100vh; height: 100dvh; min-height: 0; clip-path: none; }
@keyframes bp-player-pulse { 0%, 100% { opacity: .52; transform: scale(.92); } 50% { opacity: 1; transform: scale(1); } }
</style>
