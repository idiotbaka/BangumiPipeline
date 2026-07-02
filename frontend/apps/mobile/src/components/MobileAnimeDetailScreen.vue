<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'

import {
  api,
  buildAuthenticatedMediaURL,
  type ViewerAnimeActor,
  type ViewerAnimeCharacter,
  type ViewerAnimeDetail,
  type ViewerDetailEpisode,
} from '../api'
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
const loading = ref(false)
const errorMessage = ref('')
const followed = ref(false)
const followSaving = ref(false)
const followError = ref('')
const summaryExpanded = ref(false)
const metadataExpanded = ref(false)
const failedImages = ref<Set<string>>(new Set())
const episodeRail = ref<HTMLElement | null>(null)
const episodeCardRefs = new Map<string, HTMLElement>()
let progressSaving = false
let queuedProgress: { mediaId: number; positionSeconds: number; durationSeconds: number } | null = null

const weekdays = ['', '周一', '周二', '周三', '周四', '周五', '周六', '周日']

const selectedEpisode = computed(() =>
  anime.value?.episodes.find((episode) => episode.key === selectedEpisodeKey.value) ?? null,
)
const playableEpisodes = computed(() => anime.value?.episodes.filter((episode) => episode.hasMedia) ?? [])
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
watch(() => props.bangumiId, loadDetail)

async function loadDetail() {
  loading.value = true
  errorMessage.value = ''
  followError.value = ''
  summaryExpanded.value = false
  metadataExpanded.value = false
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
    card.scrollIntoView({ behavior, block: 'nearest', inline: 'center' })
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
    <div v-if="loading" class="detail-state">
      <i aria-hidden="true" />
      <p>正在读取番剧详情...</p>
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
      <div class="player-wrap">
        <MobileVideoPlayer
          v-if="selectedEpisode"
          :key="selectedEpisode.mediaId"
          :media-id="selectedEpisode.mediaId"
          :src="streamURL"
          :poster="playerPoster"
          :title="playerTitle"
          :start-time="resumePosition"
          @progress="saveProgress"
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
          <span>{{ playableEpisodes.length }} / {{ anime.episodes.length }}</span>
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
  padding: 3px 0 3px;
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

.detail-state > i {
  width: 38px;
  height: 38px;
  border: 2px solid var(--line);
  border-top-color: var(--pink-500);
  border-radius: 50%;
  animation: detail-spin 0.8s linear infinite;
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

@keyframes detail-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
