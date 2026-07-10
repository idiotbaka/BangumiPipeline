<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'

import {
  api,
  type ViewerAnimeActor,
  type ViewerAnimeCharacter,
  type ViewerAnimeDetail,
  type ViewerDetailEpisode,
} from '../api'
import AnimeVideoPlayer from './AnimeVideoPlayer.vue'
import ParticleField from './ParticleField.vue'

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
const failedImages = ref<Set<string>>(new Set())
let progressSaving = false
let queuedProgress: { mediaId: number; positionSeconds: number; durationSeconds: number } | null = null

const weekdays = ['', '周一', '周二', '周三', '周四', '周五', '周六', '周日']
const selectedEpisode = computed(() =>
  anime.value?.episodes.find((episode) => episode.key === selectedEpisodeKey.value) ?? null,
)
const playableEpisodes = computed(() => anime.value?.episodes.filter((episode) => episode.hasMedia) ?? [])
const playerEpisodes = computed(() =>
  playableEpisodes.value.map((episode) => ({
    key: episode.key,
    mediaId: episode.mediaId,
    label: episode.label,
    title: episode.title,
    summary: episode.summary,
    hasCover: episode.hasCover,
    coverURL: episodeCoverURL(episode),
  })),
)
const streamURL = computed(() => {
  const episode = selectedEpisode.value
  return episode?.hasMedia ? `/api/anime/${props.bangumiId}/media/${episode.mediaId}/stream` : ''
})
const metadataItems = computed(() => {
  const detail = anime.value
  if (!detail) return []
  const items = [
    { key: '放送开始', value: formatAirDate(detail.airDate) },
    { key: '放送周期', value: weekdays[detail.airWeekday] || '未定' },
    { key: '播放平台', value: detail.platform || '未定' },
    { key: '总话数', value: detail.totalEpisodes > 0 ? `${detail.totalEpisodes} 话` : '未定' },
    { key: 'Bangumi 评分', value: detail.ratingScore === null ? '暂无评分' : detail.ratingScore.toFixed(1) },
  ]
  for (const entry of detail.infobox.slice(0, 16)) {
    const key = String(entry.key ?? '').trim()
    const value = formatInfoValue(entry.value)
    if (key && value && !items.some((item) => item.key === key)) items.push({ key, value })
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

function selectEpisode(episode: Pick<ViewerDetailEpisode, 'key'>) {
  selectedEpisodeKey.value = episode.key
  resumePosition.value = 0
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
      // 进度记录失败不打断视频播放，下一次轮询会继续尝试。
    }
  }
  progressSaving = false
}

function episodeCoverURL(episode: ViewerDetailEpisode) {
  return `/api/anime/${props.bangumiId}/media/${episode.mediaId}/cover`
}

function characterImageURL(character: ViewerAnimeCharacter) {
  return `/api/anime/${props.bangumiId}/characters/${character.characterId}/image`
}

function actorImageURL(actor: ViewerAnimeActor) {
  return `/api/actors/${actor.actorId}/image`
}

function imageAvailable(key: string, available: boolean) {
  return available && !failedImages.value.has(key)
}

function markImageFailed(key: string) {
  const next = new Set(failedImages.value)
  next.add(key)
  failedImages.value = next
}

function formatAirDate(value: string) {
  return value ? value.replaceAll('-', '.') : '日期未定'
}

function normalizeTagName(value: string) {
  return value.trim().normalize('NFKC').toLocaleLowerCase()
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
  <section class="detail-stage" aria-label="番剧详情">
    <ParticleField :count="20" palette="cool" :max-size="38" />
    <div class="detail-grid" aria-hidden="true" />
    <div class="detail-halo pink" aria-hidden="true" />
    <div class="detail-halo cyan" aria-hidden="true" />

    <div v-if="loading" class="detail-state loading-state">
      <i aria-hidden="true" />
      <p>正在读取番剧档案...</p>
    </div>

    <div v-else-if="errorMessage" class="detail-state">
      <span>!</span>
      <p>{{ errorMessage }}</p>
      <div><button type="button" @click="emit('back')">返回</button><button type="button" @click="loadDetail">重试</button></div>
    </div>

    <div v-else-if="anime" class="detail-wrap">
      <header class="anime-heading">
        <button class="back-button" type="button" @click="emit('back')"><i aria-hidden="true" />返回列表</button>
        <div class="title-copy">
          <p>ANIME PLAYBACK ARCHIVE <i /></p>
          <h1>{{ anime.title }}</h1>
          <span v-if="anime.originalTitle && anime.originalTitle !== anime.title">{{ anime.originalTitle }}</span>
        </div>
        <div class="heading-meta">
          <button
            class="follow-button"
            :class="{ followed }"
            type="button"
            :disabled="followSaving"
            @click="toggleFollow"
          >
            <i aria-hidden="true" />{{ followed ? '已追番' : '追番' }}
          </button>
          <small v-if="followError" class="follow-error">{{ followError }}</small>
          <div class="heading-facts">
            <span>{{ formatAirDate(anime.airDate) }}</span>
            <i />
            <span>{{ weekdays[anime.airWeekday] || '放送日未定' }}</span>
            <i />
            <span>{{ anime.totalEpisodes > 0 ? `全 ${anime.totalEpisodes} 话` : '话数未定' }}</span>
          </div>
        </div>
      </header>

      <section class="playback-layout">
        <div class="player-column">
          <AnimeVideoPlayer
            v-if="selectedEpisode"
            :key="selectedEpisode.mediaId"
            :media-id="selectedEpisode.mediaId"
            :src="streamURL"
            poster=""
            :title="`${selectedEpisode.label} · ${selectedEpisode.title || anime.title}`"
            :start-time="resumePosition"
            :op-skip="selectedEpisode.opSkip"
            :media-info="selectedEpisode.mediaInfo"
            :episodes="playerEpisodes"
            :selected-episode-key="selectedEpisodeKey"
            @progress="saveProgress"
            @select-episode="selectEpisode"
          />
          <div v-else class="player-empty">
            <div class="empty-symbol"><i /><i /></div>
            <p>{{ availabilityText(anime.airDate) }}</p>
            <span>{{ anime.episodes.length ? '当前没有可播放的成品视频' : '分集信息尚未收录' }}</span>
          </div>
        </div>

        <aside class="episode-panel">
          <header>
            <div><p>EPISODE LIST</p><h2>选集</h2></div>
            <span>{{ playableEpisodes.length }} EPISODES</span>
          </header>
          <div v-if="playableEpisodes.length" class="episode-list">
            <button
              v-for="episode in playableEpisodes"
              :key="episode.key"
              class="episode-item"
              :class="{ selected: selectedEpisodeKey === episode.key }"
              type="button"
              @click="selectEpisode(episode)"
            >
              <div class="episode-thumb">
                <img
                  v-if="imageAvailable(`episode-${episode.mediaId}`, episode.hasCover)"
                  :src="episodeCoverURL(episode)"
                  :alt="episode.title"
                  loading="lazy"
                  @error="markImageFailed(`episode-${episode.mediaId}`)"
                />
                <div v-else class="episode-thumb-fallback"><span>{{ episode.label }}</span></div>
              </div>
              <div class="episode-copy">
                <span class="episode-label">{{ episode.label }}</span>
                <strong>{{ episode.title || episode.label }}</strong>
                <small v-if="episode.originalTitle && episode.originalTitle !== episode.title">{{ episode.originalTitle }}</small>
                <small v-else>{{ episode.airDate ? `${formatAirDate(episode.airDate)} 放送` : '放送日期未定' }}</small>
              </div>
              <p v-if="selectedEpisodeKey === episode.key" class="episode-summary">
                {{ episode.summary || '该话暂无剧情简介。' }}
              </p>
            </button>
          </div>
          <div v-else class="episode-list-empty">暂无可播放的成品视频</div>
        </aside>
      </section>

      <section class="information-layout">
        <article class="summary-panel info-panel">
          <header><p>STORY</p><h2>剧情简介</h2></header>
          <p class="anime-summary">{{ anime.summary || '该番剧暂无剧情简介。' }}</p>
          <div class="anime-tags">
            <span v-for="tag in displayMetaTags" :key="`meta-${tag}`" class="meta-tag">{{ tag }}</span>
            <span v-for="tag in displayTags" :key="tag.name">{{ tag.name }}</span>
          </div>
        </article>

        <article class="metadata-panel info-panel">
          <header><p>METADATA</p><h2>档案信息</h2></header>
          <dl>
            <div v-for="item in metadataItems" :key="item.key">
              <dt>{{ item.key }}</dt><dd>{{ item.value }}</dd>
            </div>
          </dl>
        </article>
      </section>

      <section class="characters-section">
        <div class="section-heading">
          <div><p>CHARACTER & CAST</p><h2>角色与声优</h2></div>
          <span>{{ anime.characters.length }} CHARACTERS</span>
        </div>
        <div v-if="anime.characters.length" class="character-grid">
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
            <div class="character-copy">
              <span class="relation-tag">{{ character.relation || '角色' }}</span>
              <h3>{{ character.name }}</h3>
              <p>{{ character.summary || '暂无角色简介。' }}</p>
              <div v-if="character.actors.length" class="actor-list">
                <div v-for="actor in character.actors" :key="actor.actorId" class="actor-chip">
                  <div>
                    <img
                      v-if="imageAvailable(`actor-${actor.actorId}`, actor.hasImage)"
                      :src="actorImageURL(actor)"
                      :alt="actor.name"
                      loading="lazy"
                      @error="markImageFailed(`actor-${actor.actorId}`)"
                    />
                    <span v-else>CV</span>
                  </div>
                  <p><small>CAST</small>{{ actor.name }}</p>
                </div>
              </div>
            </div>
          </article>
        </div>
        <div v-else class="characters-empty">暂无角色与声优资料</div>
      </section>
    </div>
  </section>
</template>

<style scoped>
.detail-stage { position: relative; min-height: calc(100vh - 86px); overflow: hidden; background: linear-gradient(145deg, rgba(255,249,252,.95), rgba(245,252,255,.94)); }
.detail-grid { position: absolute; inset: 0; background: linear-gradient(rgba(85,119,217,.05) 1px, transparent 1px), linear-gradient(90deg, rgba(255,95,158,.045) 1px, transparent 1px); background-size: 64px 64px; mask-image: linear-gradient(to bottom, #000, transparent 90%); pointer-events: none; }
.detail-halo { position: absolute; width: 500px; height: 500px; border-radius: 50%; filter: blur(90px); pointer-events: none; }
.detail-halo.pink { top: -340px; right: 4%; background: rgba(255,159,189,.3); }
.detail-halo.cyan { top: 550px; left: -340px; background: rgba(73,214,233,.18); }
.detail-wrap { position: relative; z-index: 2; width: min(1500px, calc(100% - 72px)); margin: 0 auto; padding: 42px 0 96px; }

.anime-heading { position: relative; min-height: 190px; display: grid; grid-template-columns: minmax(0, 1fr) auto; grid-template-rows: auto 1fr; align-items: end; gap: 22px 24px; padding: 0 8px 28px; border-bottom: 1px solid var(--line-cool); }
.back-button { grid-column: 1 / -1; align-self: start; justify-self: start; height: 38px; display: inline-flex; align-items: center; gap: 9px; padding: 0 15px; color: var(--ink-600); font-size: 13px; border: 1px solid var(--line); background: rgba(255,255,255,.74); clip-path: polygon(var(--bevel-sm)); }
.back-button:hover { color: var(--pink-600); background: var(--pink-50); }
.back-button i { width: 8px; height: 8px; border-left: 1px solid currentColor; border-bottom: 1px solid currentColor; transform: rotate(45deg); }
.title-copy { min-width: 0; }
.title-copy > p { display: flex; align-items: center; gap: 12px; color: var(--pink-500); font-family: var(--font-mono); font-size: 13px; letter-spacing: 2px; }
.title-copy > p i { width: 54px; height: 1px; background: linear-gradient(90deg, var(--pink-400), transparent); }
.title-copy h1 { margin-top: 7px; overflow: hidden; color: var(--ink-900); font-size: 34px; line-height: 1.25; letter-spacing: 1px; text-overflow: ellipsis; white-space: nowrap; }
.title-copy > span { display: block; margin-top: 5px; overflow: hidden; color: var(--ink-400); font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.heading-meta { display: grid; justify-items: end; gap: 11px; padding-bottom: 5px; color: var(--ink-600); font-size: 13px; white-space: nowrap; }
.heading-facts { display: flex; align-items: center; gap: 10px; }
.heading-facts i { width: 5px; height: 5px; background: var(--pink-300); transform: rotate(45deg); }
.follow-button { min-width: 96px; height: 38px; display: inline-flex; align-items: center; justify-content: center; gap: 9px; padding: 0 17px; color: var(--pink-600); font-size: 14px; border: 1px solid var(--pink-200); background: rgba(255,255,255,.76); box-shadow: 0 10px 22px rgba(255,95,158,.1); clip-path: polygon(var(--bevel-sm)); transition: color 160ms ease, background 160ms ease; }
.follow-button:hover:not(:disabled), .follow-button.followed { color: #fff; background: linear-gradient(135deg, var(--pink-400), var(--pink-600)); }
.follow-button:disabled { cursor: wait; opacity: .65; }
.follow-button i { width: 11px; height: 11px; border: 1px solid currentColor; transform: rotate(45deg); }
.follow-button.followed i { background: rgba(255,255,255,.86); box-shadow: inset 0 0 0 3px var(--pink-500); }
.follow-error { max-width: 280px; overflow: hidden; color: #d92d20; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }

.playback-layout { display: grid; grid-template-columns: minmax(0, 2fr) minmax(400px, 1fr); gap: 18px; height: 650px; margin-top: 28px; }
.player-column { min-width: 0; height: 100%; }
.player-empty { height: 100%; display: grid; place-items: center; align-content: center; gap: 8px; color: #fff; background: radial-gradient(circle at 50% 42%, rgba(85,119,217,.22), transparent 33%), linear-gradient(145deg, #151b2c, #090d17); clip-path: polygon(0 0, calc(100% - 20px) 0, 100% 20px, 100% 100%, 20px 100%, 0 calc(100% - 20px)); }
.empty-symbol { position: relative; width: 64px; height: 64px; margin-bottom: 8px; border: 1px solid rgba(142,232,242,.55); transform: rotate(45deg); }
.empty-symbol i { position: absolute; background: var(--pink-300); }
.empty-symbol i:first-child { top: 50%; left: 17px; width: 28px; height: 1px; }
.empty-symbol i:last-child { top: 17px; left: 50%; width: 1px; height: 28px; }
.player-empty p { font-size: 17px; letter-spacing: 1px; }
.player-empty span { color: rgba(255,255,255,.48); font-size: 13px; }

.episode-panel { height: 100%; display: grid; grid-template-rows: 70px minmax(0, 1fr); overflow: hidden; background: rgba(255,255,255,.8); border: 1px solid rgba(85,119,217,.14); box-shadow: 0 18px 40px rgba(85,119,217,.09); backdrop-filter: blur(14px); clip-path: polygon(0 0, calc(100% - 17px) 0, 100% 17px, 100% 100%, 17px 100%, 0 calc(100% - 17px)); }
.episode-panel > header { display: flex; align-items: center; justify-content: space-between; padding: 12px 19px; border-bottom: 1px solid var(--line-soft); background: linear-gradient(90deg, rgba(255,244,248,.8), rgba(236,253,255,.36)); }
.episode-panel > header p { color: var(--blue-500); font-family: var(--font-mono); font-size: 13px; letter-spacing: 1.5px; }
.episode-panel > header h2 { margin-top: 1px; color: var(--ink-900); font-size: 18px; }
.episode-panel > header > span { color: var(--pink-500); font-family: var(--font-mono); font-size: 13px; }
.episode-list { overflow-y: auto; padding: 7px; }
.episode-item { width: 100%; display: grid; grid-template-columns: 128px minmax(0, 1fr); gap: 13px; padding: 10px; color: var(--ink-700); text-align: left; border-bottom: 1px dashed rgba(85,119,217,.12); transition: background 160ms ease; }
.episode-item:hover:not(:disabled) { background: rgba(255,244,248,.7); }
.episode-item.selected { background: linear-gradient(110deg, rgba(255,225,236,.78), rgba(236,253,255,.52)); }
.episode-item.unavailable { cursor: not-allowed; opacity: .62; }
.episode-thumb { position: relative; aspect-ratio: 16 / 9; overflow: hidden; background: #eef2f8; clip-path: polygon(0 0, calc(100% - 8px) 0, 100% 8px, 100% 100%, 8px 100%, 0 calc(100% - 8px)); }
.episode-thumb img { width: 100%; height: 100%; object-fit: cover; }
.episode-thumb-fallback { width: 100%; height: 100%; display: grid; place-items: center; padding: 7px; color: var(--pink-500); font-size: 13px; text-align: center; background: linear-gradient(135deg, var(--pink-50), var(--cyan-50)); }
.availability-badge { position: absolute; inset: auto 0 0; padding: 5px; color: #fff; font-size: 11px; text-align: center; background: rgba(25,32,53,.76); }
.episode-copy { min-width: 0; align-self: center; }
.episode-label { color: var(--pink-500); font-family: var(--font-mono); font-size: 13px; letter-spacing: .4px; }
.episode-copy strong, .episode-copy small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.episode-copy strong { margin-top: 4px; color: var(--ink-900); font-size: 14px; }
.episode-copy small { margin-top: 4px; color: var(--ink-400); font-size: 13px; }
.episode-summary { grid-column: 1 / -1; padding: 9px 11px 7px 12px; color: var(--ink-600); font-size: 13px; line-height: 1.7; border-left: 2px solid var(--cyan-400); background: rgba(255,255,255,.52); }
.episode-list-empty { display: grid; place-items: center; color: var(--ink-400); font-size: 13px; }

.information-layout { display: grid; grid-template-columns: minmax(0, 1.2fr) minmax(420px, .8fr); gap: 18px; margin-top: 42px; }
.info-panel { padding: 24px 26px; background: rgba(255,255,255,.74); border: 1px solid rgba(85,119,217,.12); box-shadow: 0 16px 36px rgba(85,119,217,.06); clip-path: polygon(0 0, calc(100% - 17px) 0, 100% 17px, 100% 100%, 17px 100%, 0 calc(100% - 17px)); }
.info-panel header p, .section-heading p { color: var(--blue-500); font-family: var(--font-mono); font-size: 13px; letter-spacing: 1.5px; }
.info-panel header h2, .section-heading h2 { margin-top: 2px; color: var(--ink-900); font-size: 20px; }
.anime-summary { margin-top: 19px; color: var(--ink-600); font-size: 13px; line-height: 1.9; white-space: pre-line; }
.anime-tags { display: flex; flex-wrap: wrap; gap: 7px; margin-top: 20px; }
.anime-tags span { min-height: 31px; display: inline-flex; align-items: center; padding: 0 12px; color: var(--pink-600); font-size: 13px; border: 1px solid var(--line); background: var(--pink-50); clip-path: polygon(var(--bevel-sm)); }
.anime-tags span.meta-tag { color: var(--blue-500); border-color: var(--line-cool); background: var(--cyan-50); }
.metadata-panel dl { display: grid; grid-template-columns: 1fr 1fr; gap: 0 22px; margin-top: 16px; }
.metadata-panel dl > div { min-width: 0; display: grid; grid-template-columns: 105px 1fr; gap: 10px; padding: 11px 0; border-bottom: 1px dashed rgba(85,119,217,.12); }
.metadata-panel dt { color: var(--ink-400); font-size: 13px; }
.metadata-panel dd { overflow: hidden; color: var(--ink-700); font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }

.characters-section { margin-top: 52px; }
.section-heading { display: flex; align-items: flex-end; justify-content: space-between; margin: 0 4px 20px; padding-bottom: 13px; border-bottom: 1px solid var(--line-cool); }
.section-heading > span { color: var(--ink-400); font-family: var(--font-mono); font-size: 13px; letter-spacing: 1px; }
.character-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.character-card { min-width: 0; min-height: 238px; display: grid; grid-template-columns: 150px minmax(0, 1fr); gap: 18px; padding: 14px; background: rgba(255,255,255,.76); border: 1px solid rgba(85,119,217,.12); box-shadow: 0 14px 30px rgba(85,119,217,.07); clip-path: polygon(0 0, calc(100% - 15px) 0, 100% 15px, 100% 100%, 15px 100%, 0 calc(100% - 15px)); }
.character-image { height: 210px; overflow: hidden; display: grid; place-items: center; color: var(--pink-500); font-size: 32px; background: linear-gradient(145deg, var(--pink-50), var(--cyan-50)); clip-path: polygon(0 0, calc(100% - 11px) 0, 100% 11px, 100% 100%, 11px 100%, 0 calc(100% - 11px)); }
.character-image img { width: 100%; height: 100%; object-fit: cover; object-position: top center; }
.character-copy { min-width: 0; padding: 4px 5px 2px 0; }
.relation-tag { display: inline-flex; min-height: 27px; align-items: center; padding: 0 9px; color: var(--pink-600); font-size: 13px; background: var(--pink-50); border-left: 2px solid var(--pink-400); }
.character-copy h3 { margin-top: 7px; color: var(--ink-900); font-size: 16px; }
.character-copy > p { margin-top: 8px; display: -webkit-box; overflow: hidden; color: var(--ink-400); font-size: 13px; line-height: 1.65; -webkit-box-orient: vertical; -webkit-line-clamp: 3; }
.actor-list { display: flex; flex-wrap: wrap; gap: 7px; margin-top: 13px; }
.actor-chip { min-width: 155px; max-width: 220px; display: flex; align-items: center; gap: 9px; padding: 6px 10px 6px 6px; background: rgba(246,249,253,.9); border: 1px solid rgba(85,119,217,.12); clip-path: polygon(0 0, calc(100% - 7px) 0, 100% 7px, 100% 100%, 7px 100%, 0 calc(100% - 7px)); }
.actor-chip > div { width: 38px; height: 38px; flex: 0 0 auto; overflow: hidden; display: grid; place-items: center; color: var(--blue-500); font-family: var(--font-mono); font-size: 13px; background: var(--cyan-50); }
.actor-chip img { width: 100%; height: 100%; object-fit: cover; }
.actor-chip p { min-width: 0; overflow: hidden; color: var(--ink-700); font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.actor-chip small { display: block; color: var(--blue-400); font-family: var(--font-mono); font-size: 13px; letter-spacing: .5px; }
.characters-empty { min-height: 180px; display: grid; place-items: center; color: var(--ink-400); font-size: 13px; border: 1px dashed var(--line); background: rgba(255,255,255,.58); }

.detail-state { position: relative; z-index: 2; min-height: calc(100vh - 86px); display: grid; place-items: center; align-content: center; gap: 10px; color: var(--ink-600); }
.detail-state > span { width: 48px; height: 48px; display: grid; place-items: center; color: var(--pink-500); font-family: var(--font-mono); font-size: 24px; border: 1px solid var(--pink-300); transform: rotate(45deg); }
.detail-state > div { display: flex; gap: 9px; margin-top: 7px; }
.detail-state button { padding: 9px 17px; color: var(--pink-600); font-size: 13px; border: 1px solid var(--line); background: #fff; clip-path: polygon(var(--bevel-sm)); }
.loading-state > i { width: 44px; height: 44px; border: 2px solid var(--line); border-top-color: var(--pink-400); border-radius: 50%; animation: bp-spin .8s linear infinite; }
</style>
