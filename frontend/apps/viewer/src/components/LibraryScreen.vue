<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'

import {
  api,
  type ViewerFilterDimension,
  type ViewerLibrary,
  type ViewerScheduleCard,
} from '../api'
import ParticleField from './ParticleField.vue'

interface Props {
  initialQuery: string
  searchKey: number
}

const props = defineProps<Props>()
const dimensions = ref<ViewerFilterDimension[]>([])
const selectedFilters = ref<Record<number, string[]>>({})
const searchDraft = ref(props.initialQuery)
const appliedQuery = ref(props.initialQuery.trim())
const library = ref<ViewerLibrary>({ items: [], total: 0 })
const filtersLoading = ref(false)
const filtersError = ref('')
const loading = ref(false)
const errorMessage = ref('')
const failedCovers = ref<Set<number>>(new Set())
let requestID = 0

const selectedTagCount = computed(() =>
  Object.values(selectedFilters.value).reduce((total, tags) => total + tags.length, 0),
)

onMounted(async () => {
  await Promise.all([loadFilters(), loadLibrary()])
})

watch(
  () => props.searchKey,
  () => {
    const query = props.initialQuery.trim()
    searchDraft.value = props.initialQuery
    appliedQuery.value = query
    void loadLibrary()
  },
)

async function loadFilters() {
  filtersLoading.value = true
  filtersError.value = ''
  try {
    const result = await api.libraryFilters()
    dimensions.value = result.items
  } catch (error) {
    filtersError.value = error instanceof Error ? error.message : '筛选标签加载失败'
  } finally {
    filtersLoading.value = false
  }
}

async function loadLibrary() {
  const currentRequest = ++requestID
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await api.animeLibrary(appliedQuery.value, selectedFilters.value)
    if (currentRequest === requestID) {
      library.value = result.library
      failedCovers.value = new Set()
    }
  } catch (error) {
    if (currentRequest === requestID) {
      errorMessage.value = error instanceof Error ? error.message : '番剧图书馆加载失败'
    }
  } finally {
    if (currentRequest === requestID) loading.value = false
  }
}

function isSelected(dimensionID: number, tag: string) {
  return selectedFilters.value[dimensionID]?.includes(tag) ?? false
}

function toggleTag(dimensionID: number, tag: string) {
  const next = { ...selectedFilters.value }
  const tags = [...(next[dimensionID] ?? [])]
  const index = tags.indexOf(tag)
  if (index >= 0) tags.splice(index, 1)
  else tags.push(tag)
  if (tags.length) next[dimensionID] = tags
  else delete next[dimensionID]
  selectedFilters.value = next
  void loadLibrary()
}

function resetFilters() {
  if (selectedTagCount.value === 0) return
  selectedFilters.value = {}
  void loadLibrary()
}

function submitSearch() {
  appliedQuery.value = searchDraft.value.trim()
  void loadLibrary()
}

function clearSearch() {
  if (!searchDraft.value && !appliedQuery.value) return
  searchDraft.value = ''
  appliedQuery.value = ''
  void loadLibrary()
}

function coverURL(item: ViewerScheduleCard) {
  return `/api/anime/${item.bangumiId}/cover`
}

function hasCover(item: ViewerScheduleCard) {
  return item.hasCover && !failedCovers.value.has(item.bangumiId)
}

function markCoverFailed(item: ViewerScheduleCard) {
  const next = new Set(failedCovers.value)
  next.add(item.bangumiId)
  failedCovers.value = next
}

function formatAirDate(value: string) {
  return value ? `${value.replaceAll('-', '.')} 放送开始` : '放送日期未定'
}

function totalEpisodesText(value: number) {
  return value > 0 ? `全 ${value} 话` : '话数未定'
}

function progressText(item: ViewerScheduleCard) {
  if (item.latestEpisodeLabel) return `更新至 ${item.latestEpisodeLabel}`
  if (!item.airDate) return '开播时间未定'
  const premiere = new Date(`${item.airDate}T00:00:00`)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  return !Number.isNaN(premiere.getTime()) && premiere > today ? '尚未开播' : '尚未放流'
}

function stagger(index: number) {
  return `${0.03 + (index % 16) * 0.035}s`
}
</script>

<template>
  <section class="library-stage" aria-label="番剧图书馆">
    <ParticleField :count="18" palette="pink" :max-size="34" />
    <div class="library-grid" aria-hidden="true" />
    <div class="library-halo pink" aria-hidden="true" />
    <div class="library-halo cyan" aria-hidden="true" />

    <div class="library-wrap">
      <header class="library-hero">
        <div>
          <p class="eyebrow"><span>ANIME ARCHIVE</span><i /></p>
          <h1>番剧图书馆</h1>
          <p class="hero-note">アニメーション・コレクション</p>
        </div>
      </header>

      <section class="filter-panel" aria-label="标签筛选">
        <div class="filter-panel-head">
          <div>
            <p>FILTER MATRIX</p>
            <h2>标签筛选</h2>
          </div>
          <button type="button" :disabled="selectedTagCount === 0 || loading" @click="resetFilters">
            清除筛选 <span>{{ String(selectedTagCount).padStart(2, '0') }}</span>
          </button>
        </div>

        <div v-if="filtersLoading" class="filter-loading">正在读取筛选配置...</div>
        <div v-else-if="filtersError" class="filter-error">
          <span>{{ filtersError }}</span>
          <button type="button" @click="loadFilters">重试</button>
        </div>
        <div v-else-if="dimensions.length === 0" class="filter-empty">管理员尚未配置筛选维度</div>
        <div v-else class="filter-dimensions">
          <div v-for="(dimension, index) in dimensions" :key="dimension.id" class="filter-row">
            <div class="dimension-name">
              <span>{{ String(index + 1).padStart(2, '0') }}</span>
              <strong>{{ dimension.name }}</strong>
            </div>
            <div class="filter-tags">
              <button
                v-for="tag in dimension.tags"
                :key="tag"
                type="button"
                :class="{ active: isSelected(dimension.id, tag) }"
                :disabled="loading"
                @click="toggleTag(dimension.id, tag)"
              >
                <i aria-hidden="true" />{{ tag }}
              </button>
            </div>
          </div>
        </div>
      </section>

      <form v-if="!filtersLoading" class="library-search" role="search" @submit.prevent="submitSearch">
        <span class="search-icon" aria-hidden="true" />
        <input v-model="searchDraft" maxlength="100" type="search" placeholder="输入番剧标题、中文名或别名" />
        <button v-if="searchDraft" class="clear-button" type="button" aria-label="清除搜索" @click="clearSearch">×</button>
        <button class="search-button" type="submit" :disabled="loading">搜索番剧</button>
      </form>

      <div class="result-heading">
        <div>
          <span class="result-index">{{ String(library.total).padStart(2, '0') }}</span>
          <p>ARCHIVE RESULTS</p>
          <h2>{{ appliedQuery ? `「${appliedQuery}」的搜索结果` : '全部收录番剧' }}</h2>
        </div>
        <span class="result-count">共 {{ library.total }} 部</span>
      </div>

      <div v-if="loading" class="library-cards" aria-label="加载中">
        <article v-for="index in 16" :key="index" class="library-card skeleton-card">
          <div class="card-cover skeleton-block" />
          <div class="skeleton-line skeleton-block" />
          <div class="skeleton-line short skeleton-block" />
        </article>
      </div>

      <div v-else-if="errorMessage" class="library-state error-state">
        <span>!</span>
        <p>{{ errorMessage }}</p>
        <button type="button" @click="loadLibrary">重新加载</button>
      </div>

      <div v-else-if="library.items.length === 0" class="library-state">
        <span>00</span>
        <p>没有找到符合条件的番剧</p>
        <small>NO TITLES MATCHED THE CURRENT FILTERS</small>
      </div>

      <div v-else class="library-cards">
        <article
          v-for="(item, index) in library.items"
          :key="item.bangumiId"
          class="library-card"
          :style="{ '--stagger': stagger(index) }"
        >
          <div class="card-cover">
            <img
              v-if="hasCover(item)"
              :src="coverURL(item)"
              :alt="item.title"
              loading="lazy"
              @error="markCoverFailed(item)"
            />
            <div v-else class="cover-fallback"><span>{{ item.title.slice(0, 2) }}</span></div>
            <span class="episode-total">{{ totalEpisodesText(item.totalEpisodes) }}</span>
            <div class="progress-shade">
              <i aria-hidden="true" />
              <span>{{ progressText(item) }}</span>
            </div>
          </div>
          <h3 :title="item.title">{{ item.title }}</h3>
          <p class="air-date"><i aria-hidden="true" />{{ formatAirDate(item.airDate) }}</p>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.library-stage { position: relative; min-height: calc(100vh - 86px); overflow: hidden; background: linear-gradient(145deg, rgba(255,249,252,.94), rgba(246,252,255,.92)); }
.library-grid { position: absolute; inset: 0; background: linear-gradient(rgba(85,119,217,.052) 1px, transparent 1px), linear-gradient(90deg, rgba(255,95,158,.05) 1px, transparent 1px); background-size: 64px 64px; mask-image: linear-gradient(to bottom, #000, transparent 86%); pointer-events: none; }
.library-halo { position: absolute; width: 460px; height: 460px; border-radius: 50%; filter: blur(85px); pointer-events: none; }
.library-halo.pink { top: -290px; right: 3%; background: rgba(255,159,189,.3); }
.library-halo.cyan { top: 440px; left: -300px; background: rgba(73,214,233,.19); }
.library-wrap { position: relative; z-index: 2; width: min(1500px, calc(100% - 72px)); margin: 0 auto; padding: 54px 0 88px; }

.library-hero { min-height: 124px; display: flex; align-items: flex-end; justify-content: space-between; padding: 0 8px 28px; border-bottom: 1px solid var(--line-cool); }
.eyebrow { display: flex; align-items: center; gap: 12px; color: var(--pink-500); font-size: 11px; letter-spacing: 3px; }
.eyebrow i { width: 62px; height: 1px; background: linear-gradient(90deg, var(--pink-400), transparent); }
.library-hero h1 { margin-top: 7px; color: var(--ink-900); font-size: 34px; line-height: 1.2; letter-spacing: 2px; }
.hero-note { margin-top: 6px; color: var(--ink-400); font-size: 12px; letter-spacing: 2px; }
.filter-panel { margin-top: 28px; overflow: hidden; background: rgba(255,255,255,.72); border: 1px solid rgba(85,119,217,.13); box-shadow: 0 18px 38px rgba(85,119,217,.07); backdrop-filter: blur(14px); clip-path: polygon(0 0, calc(100% - 17px) 0, 100% 17px, 100% 100%, 17px 100%, 0 calc(100% - 17px)); }
.filter-panel-head { min-height: 74px; display: flex; align-items: center; justify-content: space-between; padding: 13px 22px; border-bottom: 1px solid var(--line-soft); background: linear-gradient(90deg, rgba(255,244,248,.72), rgba(236,253,255,.32)); }
.filter-panel-head p { color: var(--blue-500); font-family: var(--font-mono); font-size: 9px; letter-spacing: 2px; }
.filter-panel-head h2 { margin-top: 2px; color: var(--ink-900); font-size: 18px; }
.filter-panel-head > button { height: 34px; padding: 0 13px; color: var(--ink-400); font-size: 11px; border: 1px solid var(--line); background: rgba(255,255,255,.76); clip-path: polygon(var(--bevel-sm)); }
.filter-panel-head > button:not(:disabled):hover { color: var(--pink-600); background: var(--pink-50); }
.filter-panel-head > button:disabled { cursor: default; opacity: .5; }
.filter-panel-head > button span { display: inline-grid; place-items: center; min-width: 22px; height: 20px; margin-left: 6px; color: var(--pink-600); background: #fff; }
.filter-dimensions { padding: 5px 22px 10px; }
.filter-row { min-height: 65px; display: grid; grid-template-columns: 150px 1fr; align-items: center; gap: 18px; padding: 10px 0; border-bottom: 1px dashed rgba(85,119,217,.12); }
.filter-row:last-child { border-bottom: 0; }
.dimension-name { display: flex; align-items: center; gap: 10px; color: var(--ink-700); }
.dimension-name span { color: rgba(255,95,158,.34); font-family: var(--font-mono); font-size: 19px; }
.dimension-name strong { overflow: hidden; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.filter-tags { display: flex; flex-wrap: wrap; gap: 7px; }
.filter-tags button { min-height: 31px; display: inline-flex; align-items: center; gap: 7px; padding: 0 12px; color: var(--ink-600); font-size: 12px; background: rgba(248,250,253,.9); border: 1px solid rgba(85,119,217,.14); clip-path: polygon(0 0, calc(100% - 7px) 0, 100% 7px, 100% 100%, 7px 100%, 0 calc(100% - 7px)); transition: color 160ms ease, background 160ms ease, transform 160ms ease; }
.filter-tags button:hover:not(:disabled) { color: var(--pink-600); background: var(--pink-50); transform: translateY(-1px); }
.filter-tags button.active { color: #fff; border-color: transparent; background: linear-gradient(135deg, var(--pink-400), var(--pink-600)); box-shadow: 0 7px 16px rgba(255,95,158,.2); }
.filter-tags button.active:hover:not(:disabled) { color: #fff; background: linear-gradient(135deg, var(--pink-400), var(--pink-600)); transform: none; }
.filter-tags button:disabled { cursor: wait; }
.filter-tags button i { width: 5px; height: 5px; background: currentColor; transform: rotate(45deg); opacity: .62; }
.filter-loading, .filter-empty { min-height: 82px; display: grid; place-items: center; color: var(--ink-400); font-size: 12px; }
.filter-error { min-height: 82px; display: flex; align-items: center; justify-content: center; gap: 12px; color: var(--ink-400); font-size: 12px; }
.filter-error button { padding: 6px 14px; color: var(--pink-600); border: 1px solid var(--line); background: var(--pink-50); clip-path: polygon(var(--bevel-sm)); }

.library-search { position: relative; height: 58px; display: flex; align-items: center; margin-top: 20px; padding-left: 52px; background: rgba(255,255,255,.82); border: 1px solid var(--line); box-shadow: 0 14px 30px rgba(255,95,158,.07); clip-path: polygon(0 0, calc(100% - 13px) 0, 100% 13px, 100% 100%, 13px 100%, 0 calc(100% - 13px)); }
.search-icon { position: absolute; left: 21px; width: 15px; height: 15px; border: 2px solid var(--pink-300); border-radius: 50%; }
.search-icon::after { content: ''; position: absolute; right: -6px; bottom: -4px; width: 8px; height: 2px; background: var(--pink-300); transform: rotate(45deg); }
.library-search input { flex: 1; height: 100%; min-width: 0; color: var(--ink-700); font-size: 14px; }
.library-search input::placeholder { color: var(--ink-300); }
.clear-button { width: 40px; height: 40px; color: var(--ink-300); font-size: 22px; }
.clear-button:hover { color: var(--pink-500); }
.search-button { align-self: stretch; min-width: 128px; color: #fff; font-size: 13px; letter-spacing: 1px; background: linear-gradient(135deg, var(--pink-500), var(--pink-600)); clip-path: polygon(0 0, 100% 0, 100% 100%, 13px 100%); }
.search-button:disabled { opacity: .65; }

.result-heading { display: flex; justify-content: space-between; align-items: flex-end; margin: 46px 4px 22px; }
.result-heading > div { position: relative; min-height: 48px; padding-left: 72px; }
.result-index { position: absolute; left: 0; top: 1px; color: rgba(255,95,158,.16); font-family: var(--font-mono); font-size: 45px; line-height: 1; }
.result-heading p { color: var(--blue-500); font-family: var(--font-mono); font-size: 9px; letter-spacing: 2px; }
.result-heading h2 { max-width: 760px; margin-top: 2px; overflow: hidden; color: var(--ink-900); font-size: 22px; letter-spacing: 1px; text-overflow: ellipsis; white-space: nowrap; }
.result-count { padding: 5px 13px; color: var(--ink-400); font-size: 11px; border-left: 2px solid var(--cyan-400); background: rgba(255,255,255,.65); }

.library-cards { display: grid; grid-template-columns: repeat(8, minmax(0, 1fr)); gap: 22px 18px; }
.library-card { min-width: 0; animation: bp-rise .42s var(--ease-out) both; animation-delay: var(--stagger, 0s); }
.card-cover { position: relative; aspect-ratio: 2 / 3; overflow: hidden; background: var(--pink-50); border: 1px solid rgba(255,255,255,.9); box-shadow: 0 15px 32px rgba(85,119,217,.1); clip-path: polygon(0 0, calc(100% - 15px) 0, 100% 15px, 100% 100%, 15px 100%, 0 calc(100% - 15px)); transition: transform 220ms var(--ease-soft), box-shadow 220ms var(--ease-soft); }
.library-card:hover .card-cover { transform: translateY(-6px); box-shadow: 0 24px 48px rgba(255,95,158,.18); }
.card-cover img { width: 100%; height: 100%; object-fit: unset; }
.cover-fallback { display: grid; place-items: center; width: 100%; height: 100%; padding: 16px; color: var(--pink-600); font-size: 21px; background: linear-gradient(145deg, rgba(255,244,248,.96), rgba(236,253,255,.9)), repeating-linear-gradient(135deg, rgba(255,95,158,.1) 0 2px, transparent 2px 12px); }
.episode-total { position: absolute; top: 9px; right: 8px; z-index: 2; height: 23px; padding: 0 9px; display: inline-flex; align-items: center; color: var(--ink-700); font-size: 10px; white-space: nowrap; background: rgba(255,255,255,.88); box-shadow: 0 5px 13px rgba(32,40,62,.12); backdrop-filter: blur(8px); clip-path: polygon(0 0, calc(100% - 6px) 0, 100% 6px, 100% 100%, 6px 100%, 0 calc(100% - 6px)); }
.progress-shade { position: absolute; right: 0; bottom: 0; left: 0; z-index: 2; min-height: 66px; display: flex; align-items: flex-end; gap: 7px; padding: 24px 10px 9px; color: #fff; font-size: 12px; background: linear-gradient(to top, rgba(25,32,53,.84), rgba(25,32,53,0)); text-shadow: 0 2px 5px rgba(0,0,0,.65); }
.progress-shade i { width: 6px; height: 6px; margin-bottom: 5px; flex: 0 0 auto; background: var(--cyan-300); transform: rotate(45deg); box-shadow: 0 0 10px rgba(142,232,242,.8); }
.library-card h3 { margin-top: 11px; overflow: hidden; color: var(--ink-900); font-size: 14px; line-height: 1.45; text-overflow: ellipsis; white-space: nowrap; }
.air-date { display: flex; align-items: center; gap: 7px; margin-top: 5px; overflow: hidden; color: var(--ink-400); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.air-date i { width: 13px; height: 1px; flex: 0 0 auto; background: linear-gradient(90deg, var(--pink-400), var(--cyan-400)); }

.library-state { min-height: 270px; display: grid; place-items: center; align-content: center; gap: 5px; color: var(--ink-400); background: rgba(255,255,255,.62); border: 1px dashed var(--line); clip-path: polygon(0 0, calc(100% - 20px) 0, 100% 20px, 100% 100%, 20px 100%, 0 calc(100% - 20px)); }
.library-state span { color: rgba(255,95,158,.22); font-family: var(--font-mono); font-size: 46px; line-height: 1; }
.library-state p { color: var(--ink-600); font-size: 14px; }
.library-state small { margin-top: 3px; font-family: var(--font-mono); font-size: 9px; letter-spacing: 2px; }
.library-state button { margin-top: 12px; padding: 8px 20px; color: #fff; font-size: 12px; background: linear-gradient(135deg, var(--pink-500), var(--pink-600)); clip-path: polygon(var(--bevel-sm)); }
.skeleton-block { position: relative; overflow: hidden; background: rgba(255,244,248,.75); }
.skeleton-block::after { content: ''; position: absolute; inset: 0; background: linear-gradient(100deg, transparent 20%, rgba(255,255,255,.75) 45%, transparent 70%); animation: library-skeleton 1.2s ease-in-out infinite; }
.skeleton-line { width: 90%; height: 12px; margin-top: 12px; }
.skeleton-line.short { width: 62%; height: 9px; margin-top: 7px; }
@keyframes library-skeleton { from { transform: translateX(-100%); } to { transform: translateX(100%); } }
</style>
