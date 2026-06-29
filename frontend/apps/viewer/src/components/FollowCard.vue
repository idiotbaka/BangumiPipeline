<script setup lang="ts">
import { ref } from 'vue'

import type { ViewerFollowedAnime } from '../api'

interface Props {
  item: ViewerFollowedAnime
}

const props = defineProps<Props>()
const emit = defineEmits<{ (event: 'open', item: ViewerFollowedAnime): void }>()
const coverFailed = ref(false)

function coverURL() {
  return `/api/anime/${props.item.bangumiId}/media/${props.item.mediaId}/cover`
}

function progressText() {
  const item = props.item
  if (!item.hasWatchProgress) return '尚未开始观看'
  if (item.watchCompleted && !item.caughtUp) return `已看完 ${item.watchedEpisodeLabel}，有新内容`
  if (item.watchCompleted) return `已追完 ${item.watchedEpisodeLabel}`
  return `看到 ${item.watchedEpisodeLabel} ${item.progressPercent}%`
}

function updateText() {
  const total = props.item.totalEpisodes > 0 ? `全 ${props.item.totalEpisodes} 话` : '话数未定'
  const latest = props.item.latestEpisodeLabel ? `更新至 ${props.item.latestEpisodeLabel}` : '尚无成品'
  return `${total} / ${latest}`
}
</script>

<template>
  <article
    class="follow-card"
    role="link"
    tabindex="0"
    @click="emit('open', item)"
    @keydown.enter="emit('open', item)"
  >
    <div class="follow-cover">
      <img
        v-if="item.mediaId > 0 && item.hasCover && !coverFailed"
        :src="coverURL()"
        :alt="`${item.animeTitle} ${item.episodeLabel}`"
        loading="lazy"
        @error="coverFailed = true"
      />
      <div v-else class="cover-fallback"><span>{{ item.episodeLabel || item.animeTitle.slice(0, 4) }}</span></div>
      <div class="progress-overlay">
        <span>{{ progressText() }}</span>
        <div><i :style="{ width: `${item.hasWatchProgress ? item.progressPercent : 0}%` }" /></div>
      </div>
    </div>
    <h3 :title="item.animeTitle">{{ item.animeTitle }}</h3>
    <p :title="item.episodeTitle || item.episodeLabel">{{ item.episodeTitle || item.episodeLabel || '等待放流' }}</p>
    <small>{{ updateText() }}</small>
  </article>
</template>

<style scoped>
.follow-card { min-width: 0; cursor: pointer; outline: 0; animation: bp-rise .4s var(--ease-out) both; }
.follow-cover { position: relative; aspect-ratio: 16 / 9; overflow: hidden; background: #edf2f8; box-shadow: 0 14px 30px rgba(85,119,217,.1); clip-path: polygon(0 0, calc(100% - 13px) 0, 100% 13px, 100% 100%, 13px 100%, 0 calc(100% - 13px)); transition: transform 180ms ease, box-shadow 180ms ease; }
.follow-card:hover .follow-cover, .follow-card:focus-visible .follow-cover { transform: translateY(-5px); box-shadow: 0 23px 43px rgba(255,95,158,.18); }
.follow-cover img { width: 100%; height: 100%; object-fit: cover; }
.cover-fallback { width: 100%; height: 100%; display: grid; place-items: center; padding: 14px; color: var(--pink-500); font-size: 14px; text-align: center; background: linear-gradient(145deg, var(--pink-50), var(--cyan-50)); }
.progress-overlay { position: absolute; right: 0; bottom: 0; left: 0; padding: 24px 10px 8px; color: #fff; font-size: 13px; background: linear-gradient(to top, rgba(20,26,43,.88), transparent); text-shadow: 0 2px 5px rgba(0,0,0,.62); }
.progress-overlay > div { height: 4px; margin-top: 6px; overflow: hidden; background: rgba(255,255,255,.35); }
.progress-overlay i { display: block; height: 100%; background: linear-gradient(90deg, var(--pink-400), var(--cyan-300)); }
.follow-card h3, .follow-card p, .follow-card small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.follow-card h3 { margin-top: 11px; color: var(--ink-900); font-size: 15px; }
.follow-card p { margin-top: 4px; color: var(--ink-600); font-size: 13px; }
.follow-card small { margin-top: 4px; color: var(--ink-400); font-size: 13px; }
</style>
