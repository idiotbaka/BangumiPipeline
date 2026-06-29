<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

import { api, type ViewerWatchHistoryItem } from '../api'
import ParticleField from './ParticleField.vue'

const emit = defineEmits<{
  (event: 'open-history', item: ViewerWatchHistoryItem): void
}>()
const items = ref<ViewerWatchHistoryItem[]>([])
const loading = ref(false)
const errorMessage = ref('')
const failedCovers = ref<Set<number>>(new Set())
const relativeTimeNow = ref(Date.now())
let relativeTimeTimer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  void loadHistory()
  relativeTimeTimer = setInterval(() => {
    relativeTimeNow.value = Date.now()
  }, 60_000)
})

onBeforeUnmount(() => {
  if (relativeTimeTimer !== null) clearInterval(relativeTimeTimer)
})

async function loadHistory() {
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await api.watchHistory()
    items.value = result.items
    failedCovers.value = new Set()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '观看历史加载失败'
  } finally {
    loading.value = false
  }
}

function coverURL(item: ViewerWatchHistoryItem) {
  return `/api/anime/${item.bangumiId}/media/${item.mediaId}/cover`
}

function hasCover(item: ViewerWatchHistoryItem) {
  return item.hasCover && !failedCovers.value.has(item.mediaId)
}

function markCoverFailed(item: ViewerWatchHistoryItem) {
  const next = new Set(failedCovers.value)
  next.add(item.mediaId)
  failedCovers.value = next
}

function progressText(item: ViewerWatchHistoryItem) {
  return item.completed
    ? `已看完 ${item.episodeLabel}`
    : `看到 ${item.episodeLabel} ${item.progressPercent}%`
}

function totalText(item: ViewerWatchHistoryItem) {
  const total = item.totalEpisodes > 0 ? `全 ${item.totalEpisodes} 话` : '话数未定'
  const latest = item.latestEpisodeLabel ? `更新至 ${item.latestEpisodeLabel}` : '更新话数未知'
  return `${total} / ${latest}`
}

function formatLastWatched(value: number) {
  if (!value) return '观看时间未知'
  const elapsedSeconds = Math.max(Math.floor(relativeTimeNow.value / 1000) - value, 0)
  const minutes = Math.max(Math.floor(elapsedSeconds / 60), 1)
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}天前`
  const months = Math.floor(days / 30)
  if (months < 12) return `${months}个月前`
  return `${Math.floor(months / 12)}年前`
}
</script>

<template>
  <section class="history-stage" aria-label="观看历史">
    <ParticleField :count="18" palette="cool" :max-size="34" />
    <div class="history-grid" aria-hidden="true" />
    <div class="history-halo pink" aria-hidden="true" />
    <div class="history-halo cyan" aria-hidden="true" />

    <div class="history-wrap">
      <header class="history-heading">
        <div>
          <p>PLAYBACK ARCHIVE <i /></p>
          <h1>观看历史</h1>
          <span>視聴履歴・CONTINUE WATCHING</span>
        </div>
        <button type="button" :disabled="loading" @click="loadHistory">刷新记录</button>
      </header>

      <div class="result-heading">
        <div><p>RECENTLY WATCHED</p><h2>最近观看</h2></div>
        <span>共 {{ items.length }} 条记录</span>
      </div>

      <div v-if="loading" class="history-cards">
        <article v-for="index in 8" :key="index" class="history-card skeleton-card">
          <div class="history-cover skeleton-block" />
          <div class="skeleton-line skeleton-block" />
          <div class="skeleton-line short skeleton-block" />
        </article>
      </div>

      <div v-else-if="errorMessage" class="history-state">
        <span>!</span><p>{{ errorMessage }}</p><button type="button" @click="loadHistory">重新加载</button>
      </div>

      <div v-else-if="items.length === 0" class="history-state">
        <span>00</span><p>还没有超过 15 秒的观看记录</p><small>NO PLAYBACK HISTORY YET</small>
      </div>

      <div v-else class="history-cards">
        <article
          v-for="item in items"
          :key="`${item.bangumiId}-${item.mediaId}`"
          class="history-card"
          role="link"
          tabindex="0"
          @click="emit('open-history', item)"
          @keydown.enter="emit('open-history', item)"
        >
          <div class="history-cover">
            <img
              v-if="hasCover(item)"
              :src="coverURL(item)"
              :alt="`${item.animeTitle} ${item.episodeLabel}`"
              loading="lazy"
              @error="markCoverFailed(item)"
            />
            <div v-else class="cover-fallback"><span>{{ item.episodeLabel }}</span></div>
            <div class="watch-overlay">
              <span>{{ progressText(item) }}</span>
              <div><i :style="{ width: `${item.progressPercent}%` }" /></div>
            </div>
          </div>
          <h3 :title="item.animeTitle">{{ item.animeTitle }}</h3>
          <p class="episode-title" :title="item.episodeTitle || item.episodeLabel">
            {{ item.episodeTitle || item.episodeLabel }}
          </p>
          <div class="history-meta"><span>{{ totalText(item) }}</span><time>{{ formatLastWatched(item.lastWatchedAt) }}</time></div>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.history-stage { position: relative; min-height: calc(100vh - 86px); overflow: hidden; background: linear-gradient(145deg, rgba(255,249,252,.95), rgba(245,252,255,.93)); }
.history-grid { position: absolute; inset: 0; background: linear-gradient(rgba(85,119,217,.05) 1px, transparent 1px), linear-gradient(90deg, rgba(255,95,158,.045) 1px, transparent 1px); background-size: 64px 64px; mask-image: linear-gradient(to bottom, #000, transparent 88%); pointer-events: none; }
.history-halo { position: absolute; width: 470px; height: 470px; border-radius: 50%; filter: blur(90px); pointer-events: none; }
.history-halo.pink { top: -320px; right: 4%; background: rgba(255,159,189,.28); }
.history-halo.cyan { top: 410px; left: -320px; background: rgba(73,214,233,.18); }
.history-wrap { position: relative; z-index: 2; width: min(1500px, calc(100% - 72px)); margin: 0 auto; padding: 54px 0 92px; }
.history-heading { min-height: 124px; display: flex; align-items: flex-end; justify-content: space-between; padding: 0 8px 28px; border-bottom: 1px solid var(--line-cool); }
.history-heading p { display: flex; align-items: center; gap: 12px; color: var(--pink-500); font-family: var(--font-mono); font-size: 13px; letter-spacing: 2px; }
.history-heading p i { width: 60px; height: 1px; background: linear-gradient(90deg, var(--pink-400), transparent); }
.history-heading h1 { margin-top: 7px; color: var(--ink-900); font-size: 34px; line-height: 1.2; letter-spacing: 2px; }
.history-heading div > span { display: block; margin-top: 6px; color: var(--ink-400); font-size: 13px; letter-spacing: 1.5px; }
.history-heading > button { height: 38px; padding: 0 17px; color: var(--pink-600); font-size: 13px; border: 1px solid var(--line); background: rgba(255,255,255,.76); clip-path: polygon(var(--bevel-sm)); }
.history-heading > button:hover:not(:disabled) { color: #fff; background: linear-gradient(135deg, var(--pink-500), var(--pink-600)); }
.result-heading { display: flex; align-items: flex-end; justify-content: space-between; margin: 42px 4px 22px; }
.result-heading p { color: var(--blue-500); font-family: var(--font-mono); font-size: 13px; letter-spacing: 1.5px; }
.result-heading h2 { margin-top: 2px; color: var(--ink-900); font-size: 22px; }
.result-heading > span { padding: 5px 13px; color: var(--ink-400); font-size: 13px; border-left: 2px solid var(--cyan-400); background: rgba(255,255,255,.65); }
.history-cards { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 30px 20px; }
.history-card { min-width: 0; cursor: pointer; outline: 0; animation: bp-rise .42s var(--ease-out) both; }
.history-cover { position: relative; aspect-ratio: 16 / 9; overflow: hidden; background: #edf2f8; box-shadow: 0 15px 32px rgba(85,119,217,.11); clip-path: polygon(0 0, calc(100% - 14px) 0, 100% 14px, 100% 100%, 14px 100%, 0 calc(100% - 14px)); transition: transform 180ms ease, box-shadow 180ms ease; }
.history-card:hover .history-cover, .history-card:focus-visible .history-cover { transform: translateY(-5px); box-shadow: 0 24px 45px rgba(255,95,158,.18); }
.history-cover img { width: 100%; height: 100%; object-fit: cover; }
.cover-fallback { width: 100%; height: 100%; display: grid; place-items: center; color: var(--pink-500); font-size: 16px; background: linear-gradient(145deg, var(--pink-50), var(--cyan-50)); }
.watch-overlay { position: absolute; right: 0; bottom: 0; left: 0; padding: 26px 11px 9px; color: #fff; font-size: 13px; background: linear-gradient(to top, rgba(20,26,43,.88), transparent); text-shadow: 0 2px 5px rgba(0,0,0,.62); }
.watch-overlay > div { height: 4px; margin-top: 6px; overflow: hidden; background: rgba(255,255,255,.35); }
.watch-overlay i { display: block; height: 100%; background: linear-gradient(90deg, var(--pink-400), var(--cyan-300)); }
.history-card h3 { margin-top: 12px; overflow: hidden; color: var(--ink-900); font-size: 16px; text-overflow: ellipsis; white-space: nowrap; }
.episode-title { margin-top: 4px; overflow: hidden; color: var(--ink-600); font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.history-meta { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-top: 5px; color: var(--ink-400); font-size: 13px; }
.history-meta time { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.history-state { min-height: 280px; display: grid; place-items: center; align-content: center; gap: 7px; color: var(--ink-400); border: 1px dashed var(--line); background: rgba(255,255,255,.62); clip-path: polygon(0 0, calc(100% - 20px) 0, 100% 20px, 100% 100%, 20px 100%, 0 calc(100% - 20px)); }
.history-state > span { color: rgba(255,95,158,.24); font-family: var(--font-mono); font-size: 46px; }
.history-state p { color: var(--ink-600); font-size: 14px; }
.history-state small { font-family: var(--font-mono); font-size: 13px; letter-spacing: 1.5px; }
.history-state button { margin-top: 8px; padding: 9px 17px; color: #fff; font-size: 13px; background: linear-gradient(135deg, var(--pink-500), var(--pink-600)); clip-path: polygon(var(--bevel-sm)); }
.skeleton-block { position: relative; overflow: hidden; background: rgba(255,244,248,.75); }
.skeleton-block::after { content: ''; position: absolute; inset: 0; background: linear-gradient(100deg, transparent 20%, rgba(255,255,255,.75) 45%, transparent 70%); animation: history-skeleton 1.2s ease-in-out infinite; }
.skeleton-line { width: 90%; height: 13px; margin-top: 12px; }
.skeleton-line.short { width: 62%; height: 10px; margin-top: 7px; }
@keyframes history-skeleton { from { transform: translateX(-100%); } to { transform: translateX(100%); } }
</style>
