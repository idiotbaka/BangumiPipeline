<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { api, type ViewerAnimeCard, type ViewerHome, type ViewerUser } from '../api'
import ParticleField from './ParticleField.vue'

interface Props {
  user: ViewerUser
  siteName: string
  loading: boolean
}

defineProps<Props>()
const emit = defineEmits<{ (e: 'logout'): void }>()

const hotPageSize = 8
const maxHotPages = 4
const hotSkeletonCount = 8
const recentSkeletonCount = 4

const searchQuery = ref('')
const homeLoading = ref(false)
const homeError = ref('')
const hotPage = ref(0)
const failedCovers = ref<Set<number>>(new Set())
const home = ref<ViewerHome>({
  hotRecommendations: [],
  recentUpdates: [],
})

const hotPages = computed(() => {
  const items = home.value.hotRecommendations.slice(0, hotPageSize * maxHotPages)
  const pages: ViewerAnimeCard[][] = []
  for (let index = 0; index < items.length; index += hotPageSize) {
    pages.push(items.slice(index, index + hotPageSize))
  }
  return pages
})

const currentHotItems = computed(() => hotPages.value[hotPage.value] ?? [])
const canTurnHot = computed(() => hotPages.value.length > 1)
const pageIndicator = computed(() => {
  const total = Math.max(hotPages.value.length, 1)
  return `${Math.min(hotPage.value + 1, total)} / ${total}`
})

onMounted(() => {
  void loadHome()
})

async function loadHome() {
  if (homeLoading.value) {
    return
  }
  homeLoading.value = true
  homeError.value = ''
  try {
    const result = await api.home()
    home.value = result.home
    hotPage.value = 0
  } catch (error) {
    homeError.value = error instanceof Error ? error.message : '首页数据加载失败'
  } finally {
    homeLoading.value = false
  }
}

function turnHotPage(direction: number) {
  if (!canTurnHot.value) {
    return
  }
  const total = hotPages.value.length
  hotPage.value = (hotPage.value + direction + total) % total
}

function coverURL(item: ViewerAnimeCard) {
  return `/api/anime/${item.bangumiId}/cover`
}

function hasCover(item: ViewerAnimeCard) {
  return item.hasCover && !failedCovers.value.has(item.bangumiId)
}

function markCoverFailed(item: ViewerAnimeCard) {
  const next = new Set(failedCovers.value)
  next.add(item.bangumiId)
  failedCovers.value = next
}

function ratingText(score: number | null) {
  return score === null ? '--' : score.toFixed(1)
}

function updateText(item: ViewerAnimeCard) {
  return item.latestEpisodeLabel ? `更新至 ${item.latestEpisodeLabel}` : '更新至 ?'
}

function formatUpdatedAt(value: number | null) {
  if (!value) {
    return '--'
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(value * 1000))
}

function formatAirDate(value: string) {
  return value ? value.split('-').join('.') : 'ON AIR'
}
</script>

<template>
  <main class="home-shell">
    <header class="topbar">
      <div class="brand-row">
        <div class="brand-mark">BP</div>
        <div class="brand-text">
          <p>VIEWER PORTAL</p>
          <strong>{{ siteName }}</strong>
        </div>
      </div>

      <nav class="main-nav" aria-label="主导航">
        <button class="nav-item active" type="button">首页</button>
        <button class="nav-item" type="button">番剧时间表</button>
        <button class="nav-item" type="button">番剧图书馆</button>
      </nav>

      <label class="search-box">
        <span class="search-symbol" aria-hidden="true" />
        <input v-model="searchQuery" type="search" placeholder="搜索番剧" />
      </label>

      <div class="user-chip">
        <span class="user-avatar" aria-hidden="true">{{ user.username.slice(0, 1).toUpperCase() }}</span>
        <span class="user-name">{{ user.username }}</span>
        <button class="logout-button" :disabled="loading" type="button" @click="emit('logout')">
          退出
        </button>
      </div>
    </header>

    <section class="home-stage" aria-label="首页">
      <ParticleField :count="14" palette="pink" :max-size="28" />
      <div class="stage-grid" aria-hidden="true" />
      <div class="stage-shape shape-ribbon" aria-hidden="true" />
      <div class="stage-shape shape-tile" aria-hidden="true" />

      <div class="content-wrap">
        <section class="hero-carousel" aria-label="轮播图">
          <div class="hero-copy">
            <span class="hero-kicker">BANNER 01</span>
            <h1>{{ siteName }}</h1>
            <p>今日放送、近期完结与收藏中的动画将在这里汇合。</p>
          </div>
          <div class="hero-art" aria-hidden="true">
            <span class="art-panel panel-a" />
            <span class="art-panel panel-b" />
            <span class="art-panel panel-c" />
            <span class="art-line line-a" />
            <span class="art-line line-b" />
          </div>
          <div class="hero-dots" aria-hidden="true">
            <span class="dot active" />
            <span class="dot" />
            <span class="dot" />
          </div>
        </section>

        <section class="anime-section">
          <div class="section-head">
            <div>
              <p class="section-kicker">HOT ON AIR</p>
              <h2>热播推荐</h2>
            </div>
            <div class="section-controls">
              <span class="page-count">{{ pageIndicator }}</span>
              <button class="arrow-button" :disabled="!canTurnHot" type="button" aria-label="上一页" @click="turnHotPage(-1)">
                ‹
              </button>
              <button class="arrow-button" :disabled="!canTurnHot" type="button" aria-label="下一页" @click="turnHotPage(1)">
                ›
              </button>
            </div>
          </div>

          <div v-if="homeLoading" class="poster-grid">
            <article v-for="n in hotSkeletonCount" :key="n" class="poster-card skeleton-card">
              <div class="poster-frame skeleton-block" />
              <div class="skeleton-title skeleton-block" />
            </article>
          </div>
          <div v-else-if="homeError" class="state-panel">
            <strong>{{ homeError }}</strong>
            <button type="button" @click="loadHome">重试</button>
          </div>
          <div v-else-if="currentHotItems.length === 0" class="state-panel">
            <strong>暂无热播推荐</strong>
          </div>
          <div v-else class="poster-grid">
            <article
              v-for="item in currentHotItems"
              :key="item.bangumiId"
              class="poster-card"
            >
              <div class="poster-frame">
                <img
                  v-if="hasCover(item)"
                  :src="coverURL(item)"
                  :alt="item.title"
                  loading="lazy"
                  @error="markCoverFailed(item)"
                />
                <div v-else class="cover-fallback">
                  <span>{{ item.title.slice(0, 2) }}</span>
                </div>
                <span class="rating-badge">{{ ratingText(item.ratingScore) }}</span>
                <span class="episode-shadow">{{ updateText(item) }}</span>
              </div>
              <h3>{{ item.title }}</h3>
              <p>{{ formatAirDate(item.airDate) }}</p>
            </article>
          </div>
        </section>

        <section class="anime-section recent-section">
          <div class="section-head">
            <div>
              <p class="section-kicker">RECENT DROPS</p>
              <h2>最近更新</h2>
            </div>
          </div>

          <div v-if="homeLoading" class="recent-grid">
            <article v-for="n in recentSkeletonCount" :key="n" class="update-card skeleton-update">
              <div class="update-cover skeleton-block" />
              <div class="update-body">
                <div class="skeleton-line skeleton-block" />
                <div class="skeleton-line short skeleton-block" />
              </div>
            </article>
          </div>
          <div v-else-if="!homeError && home.recentUpdates.length === 0" class="state-panel compact">
            <strong>暂无最近更新</strong>
          </div>
          <div v-else-if="!homeError" class="recent-grid">
            <article v-for="item in home.recentUpdates" :key="item.bangumiId" class="update-card">
              <div class="update-cover">
                <img
                  v-if="hasCover(item)"
                  :src="coverURL(item)"
                  :alt="item.title"
                  loading="lazy"
                  @error="markCoverFailed(item)"
                />
                <div v-else class="cover-fallback small">
                  <span>{{ item.title.slice(0, 2) }}</span>
                </div>
              </div>
              <div class="update-body">
                <span class="update-chip">{{ updateText(item) }}</span>
                <h3>{{ item.title }}</h3>
                <time :datetime="String(item.updatedAt ?? '')">{{ formatUpdatedAt(item.updatedAt) }}</time>
              </div>
            </article>
          </div>
        </section>
      </div>
    </section>
  </main>
</template>

<style scoped>
.home-shell {
  position: relative;
  min-width: 1200px;
  min-height: 100vh;
  background:
    linear-gradient(135deg, rgba(255, 244, 248, 0.88), rgba(255, 255, 255, 0.96) 42%, rgba(236, 253, 255, 0.72)),
    repeating-linear-gradient(90deg, var(--line-soft) 0 1px, transparent 1px 46px),
    #ffffff;
}

.topbar {
  position: sticky;
  top: 0;
  z-index: 20;
  height: 86px;
  display: grid;
  grid-template-columns: minmax(220px, 1fr) auto 230px auto;
  align-items: center;
  gap: 16px;
  padding: 0 32px;
  background: var(--glass-strong);
  border-bottom: 1px solid var(--line);
  backdrop-filter: blur(18px);
  animation: bp-rise 0.5s var(--ease-out) both;
}

.brand-row {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.brand-mark {
  display: grid;
  place-items: center;
  width: 46px;
  height: 46px;
  flex: 0 0 auto;
  color: #ffffff;
  font-size: 14px;
  font-weight: 900;
  letter-spacing: 0.5px;
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600) 58%, var(--blue-500));
  box-shadow: 0 12px 28px rgba(255, 95, 158, 0.28);
  clip-path: polygon(var(--bevel-md));
}

.brand-text {
  min-width: 0;
}

.brand-text p,
.section-kicker,
.hero-kicker {
  color: var(--ink-400);
  font-size: 11px;
  font-weight: 900;
  letter-spacing: 2px;
}

.brand-text strong {
  display: block;
  max-width: 360px;
  margin-top: 2px;
  overflow: hidden;
  font-size: 20px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.main-nav {
  display: grid;
  grid-template-columns: repeat(3, auto);
  gap: 8px;
  padding: 5px;
  background: rgba(255, 255, 255, 0.74);
  border: 1px solid var(--line-soft);
  clip-path: polygon(0 0, calc(100% - 14px) 0, 100% 14px, 100% 100%, 14px 100%, 0 calc(100% - 14px));
}

.nav-item {
  height: 38px;
  padding: 0 14px;
  color: var(--ink-600);
  font-size: 14px;
  font-weight: 900;
  border: 1px solid transparent;
  clip-path: polygon(var(--bevel-sm));
  transition: transform 180ms var(--ease-soft), color 180ms var(--ease-soft), background 180ms var(--ease-soft);
}

.nav-item:hover,
.nav-item.active {
  color: #ffffff;
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600));
  box-shadow: 0 10px 20px rgba(255, 95, 158, 0.2);
  transform: translateY(-1px);
}

.search-box {
  position: relative;
  display: flex;
  align-items: center;
  height: 42px;
  min-width: 0;
  padding: 0 14px 0 42px;
  background: #ffffff;
  border: 1px solid var(--line);
  box-shadow: 0 12px 28px rgba(85, 119, 217, 0.08);
  clip-path: polygon(var(--bevel-chip));
}

.search-symbol {
  position: absolute;
  left: 16px;
  width: 14px;
  height: 14px;
  border: 2px solid var(--blue-400);
  border-radius: 50%;
}

.search-symbol::after {
  content: '';
  position: absolute;
  right: -6px;
  bottom: -4px;
  width: 8px;
  height: 2px;
  background: var(--blue-400);
  transform: rotate(45deg);
}

.search-box input {
  width: 100%;
  color: var(--ink-700);
  font-size: 14px;
  font-weight: 700;
}

.search-box input::placeholder {
  color: var(--ink-300);
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 44px;
  padding: 4px 4px 4px 6px;
  background: #ffffff;
  border: 1px solid var(--line);
  box-shadow: 0 14px 30px rgba(255, 95, 158, 0.1);
  clip-path: polygon(0 0, calc(100% - 12px) 0, 100% 12px, 100% 100%, 12px 100%, 0 calc(100% - 12px));
}

.user-avatar {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  color: #ffffff;
  font-size: 14px;
  font-weight: 900;
  background: linear-gradient(135deg, var(--cyan-400), var(--blue-500));
  clip-path: polygon(8px 0, 100% 0, 100% calc(100% - 8px), calc(100% - 8px) 100%, 0 100%, 0 8px);
}

.user-name {
  max-width: 120px;
  overflow: hidden;
  color: var(--ink-900);
  font-size: 14px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.logout-button {
  height: 32px;
  padding: 0 16px;
  color: #ffffff;
  font-size: 13px;
  font-weight: 900;
  letter-spacing: 1px;
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600));
  box-shadow: 0 8px 18px rgba(255, 95, 158, 0.28);
  clip-path: polygon(var(--bevel-sm));
  transition: transform 180ms var(--ease-soft), box-shadow 180ms var(--ease-soft), filter 180ms var(--ease-soft);
}

.logout-button:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 12px 22px rgba(255, 95, 158, 0.34);
  filter: saturate(1.08);
}

.home-stage {
  position: relative;
  min-height: calc(100vh - 86px);
  overflow: hidden;
}

.stage-grid {
  position: absolute;
  inset: 0;
  z-index: 0;
  background:
    linear-gradient(transparent 95%, rgba(85, 119, 217, 0.12) 95%),
    linear-gradient(90deg, transparent 95%, rgba(255, 95, 158, 0.1) 95%);
  background-size: 72px 72px;
  mask-image: linear-gradient(to bottom, rgba(0, 0, 0, 0.85), transparent 86%);
  pointer-events: none;
}

.stage-shape {
  position: absolute;
  z-index: 1;
  pointer-events: none;
  animation: bp-sway 7s ease-in-out infinite alternate;
}

.shape-ribbon {
  left: 4%;
  top: 180px;
  width: 170px;
  height: 58px;
  background: rgba(255, 229, 122, 0.34);
  border: 1px solid rgba(255, 214, 74, 0.42);
  clip-path: polygon(0 0, 100% 0, calc(100% - 34px) 100%, 0 100%);
}

.shape-tile {
  right: 5%;
  top: 360px;
  width: 150px;
  height: 150px;
  background: rgba(73, 214, 233, 0.18);
  border: 1px solid rgba(73, 214, 233, 0.36);
  clip-path: polygon(18px 0, 100% 0, 100% calc(100% - 18px), calc(100% - 18px) 100%, 0 100%, 0 18px);
  animation-delay: 1.4s;
}

.content-wrap {
  position: relative;
  z-index: 4;
  width: min(1440px, calc(100% - 84px));
  margin: 0 auto;
  padding: 34px 0 64px;
}

.hero-carousel {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 0.92fr) minmax(420px, 1fr);
  min-height: 318px;
  overflow: hidden;
  background:
    linear-gradient(120deg, rgba(255, 255, 255, 0.92), rgba(255, 244, 248, 0.84) 42%, rgba(236, 253, 255, 0.9)),
    linear-gradient(90deg, rgba(255, 95, 158, 0.12), rgba(85, 119, 217, 0.1));
  border: 1px solid var(--line);
  box-shadow: 0 26px 60px rgba(85, 119, 217, 0.13);
  clip-path: polygon(0 0, calc(100% - 30px) 0, 100% 30px, 100% 100%, 30px 100%, 0 calc(100% - 30px));
  animation: bp-rise 0.58s var(--ease-out) 0.04s both;
}

.hero-carousel::before {
  content: '';
  position: absolute;
  inset: 18px;
  border: 1px solid rgba(255, 255, 255, 0.82);
  clip-path: polygon(0 0, calc(100% - 22px) 0, 100% 22px, 100% 100%, 22px 100%, 0 calc(100% - 22px));
  pointer-events: none;
}

.hero-copy {
  position: relative;
  z-index: 2;
  align-self: center;
  padding: 48px 0 48px 56px;
}

.hero-kicker {
  display: inline-flex;
  padding: 7px 14px;
  color: var(--pink-600);
  background: rgba(255, 255, 255, 0.74);
  border: 1px solid var(--pink-100);
  clip-path: polygon(var(--bevel-tag));
}

.hero-copy h1 {
  max-width: 640px;
  margin-top: 22px;
  color: var(--ink-900);
  font-size: 44px;
  font-weight: 900;
  letter-spacing: 0;
  line-height: 1.1;
}

.hero-copy p {
  max-width: 520px;
  margin-top: 16px;
  color: var(--ink-600);
  font-size: 16px;
  font-weight: 700;
  line-height: 1.75;
}

.hero-art {
  position: relative;
  min-height: 318px;
}

.art-panel {
  position: absolute;
  display: block;
  border: 1px solid rgba(255, 255, 255, 0.78);
  box-shadow: 0 18px 46px rgba(85, 119, 217, 0.12);
  animation: bp-float 4.6s ease-in-out infinite alternate;
}

.panel-a {
  right: 108px;
  top: 38px;
  width: 270px;
  height: 188px;
  background: linear-gradient(135deg, rgba(255, 127, 166, 0.34), rgba(255, 255, 255, 0.88));
  clip-path: polygon(0 0, calc(100% - 24px) 0, 100% 24px, 100% 100%, 24px 100%, 0 calc(100% - 24px));
}

.panel-b {
  right: 34px;
  bottom: 46px;
  width: 250px;
  height: 140px;
  background: linear-gradient(135deg, rgba(73, 214, 233, 0.32), rgba(255, 255, 255, 0.9));
  clip-path: polygon(26px 0, 100% 0, 100% calc(100% - 26px), calc(100% - 26px) 100%, 0 100%, 0 26px);
  animation-delay: 0.6s;
}

.panel-c {
  right: 360px;
  bottom: 54px;
  width: 120px;
  height: 120px;
  background: linear-gradient(135deg, rgba(255, 229, 122, 0.42), rgba(255, 255, 255, 0.86));
  clip-path: polygon(var(--bevel-diamond));
  animation-delay: 1s;
}

.art-line {
  position: absolute;
  right: 74px;
  display: block;
  height: 2px;
  background: linear-gradient(90deg, transparent, var(--pink-300), var(--cyan-400), transparent);
}

.line-a {
  top: 86px;
  width: 420px;
}

.line-b {
  top: 238px;
  width: 320px;
}

.hero-dots {
  position: absolute;
  left: 56px;
  bottom: 34px;
  display: flex;
  gap: 10px;
}

.dot {
  width: 28px;
  height: 4px;
  background: rgba(139, 149, 173, 0.25);
  clip-path: polygon(var(--bevel-sm));
}

.dot.active {
  width: 48px;
  background: linear-gradient(90deg, var(--pink-500), var(--cyan-400));
}

.anime-section {
  margin-top: 38px;
  animation: bp-rise 0.58s var(--ease-out) 0.12s both;
}

.recent-section {
  margin-top: 44px;
}

.section-head {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 18px;
}

.section-head h2 {
  margin-top: 4px;
  color: var(--ink-900);
  font-size: 28px;
  font-weight: 900;
  letter-spacing: 0;
}

.section-controls {
  display: flex;
  align-items: center;
  gap: 10px;
}

.page-count {
  min-width: 52px;
  color: var(--ink-400);
  font-size: 12px;
  font-weight: 900;
  text-align: right;
}

.arrow-button {
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  color: var(--pink-600);
  font-size: 25px;
  font-weight: 900;
  line-height: 1;
  background: #ffffff;
  border: 1px solid var(--line);
  box-shadow: 0 10px 22px rgba(255, 95, 158, 0.12);
  clip-path: polygon(var(--bevel-sm));
  transition: transform 170ms var(--ease-soft), background 170ms var(--ease-soft), color 170ms var(--ease-soft);
}

.arrow-button:hover:not(:disabled) {
  color: #ffffff;
  background: linear-gradient(135deg, var(--pink-500), var(--blue-500));
  transform: translateY(-2px);
}

.arrow-button:disabled {
  color: var(--ink-300);
  cursor: default;
  filter: grayscale(0.3);
}

.poster-grid {
  display: grid;
  grid-template-columns: repeat(8, minmax(0, 1fr));
  gap: 18px;
}

.poster-card {
  min-width: 0;
  animation: bp-rise 0.4s var(--ease-out) both;
}

.poster-frame {
  position: relative;
  aspect-ratio: 2 / 3;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.72);
  border: 1px solid rgba(255, 255, 255, 0.82);
  box-shadow: 0 18px 36px rgba(85, 119, 217, 0.12);
  clip-path: polygon(0 0, calc(100% - 16px) 0, 100% 16px, 100% 100%, 16px 100%, 0 calc(100% - 16px));
  transition: transform 220ms var(--ease-soft), box-shadow 220ms var(--ease-soft), filter 220ms var(--ease-soft);
}

.poster-frame::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(105deg, transparent 28%, rgba(255, 255, 255, 0.32) 45%, transparent 62%);
  transform: translateX(-120%);
  transition: transform 460ms var(--ease-out);
  pointer-events: none;
}

.poster-card:hover .poster-frame {
  transform: translateY(-6px);
  box-shadow: 0 28px 54px rgba(255, 95, 158, 0.18);
  filter: saturate(1.08);
}

.poster-card:hover .poster-frame::after {
  transform: translateX(120%);
}

.poster-frame img,
.update-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.rating-badge {
  position: absolute;
  top: 8px;
  right: 8px;
  min-width: 44px;
  padding: 4px 8px;
  color: #ffffff;
  font-size: 13px;
  font-weight: 900;
  text-align: center;
  background: linear-gradient(135deg, var(--yellow-400), var(--pink-500));
  box-shadow: 0 8px 18px rgba(238, 63, 134, 0.25);
  clip-path: polygon(var(--bevel-sm));
}

.episode-shadow {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  min-height: 48px;
  display: flex;
  align-items: end;
  padding: 18px 10px 9px;
  color: #ffffff;
  font-size: 13px;
  font-weight: 900;
  line-height: 1.25;
  background: linear-gradient(to top, rgba(32, 40, 62, 0.82), rgba(32, 40, 62, 0));
  text-shadow: 0 1px 6px rgba(32, 40, 62, 0.6);
}

.poster-card h3 {
  min-height: 42px;
  margin-top: 11px;
  overflow: hidden;
  color: var(--ink-900);
  font-size: 14px;
  font-weight: 900;
  line-height: 1.45;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.poster-card p {
  margin-top: 4px;
  color: var(--ink-400);
  font-size: 12px;
  font-weight: 800;
}

.cover-fallback {
  display: grid;
  place-items: center;
  width: 100%;
  height: 100%;
  padding: 16px;
  color: var(--pink-600);
  font-size: 22px;
  font-weight: 900;
  text-align: center;
  background:
    linear-gradient(135deg, rgba(255, 244, 248, 0.92), rgba(236, 253, 255, 0.82)),
    repeating-linear-gradient(135deg, rgba(255, 95, 158, 0.12) 0 2px, transparent 2px 12px);
}

.cover-fallback.small {
  font-size: 18px;
}

.recent-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 18px;
}

.update-card {
  display: grid;
  grid-template-columns: 102px minmax(0, 1fr);
  min-height: 146px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.82);
  border: 1px solid var(--line-soft);
  box-shadow: 0 18px 38px rgba(85, 119, 217, 0.1);
  clip-path: polygon(0 0, calc(100% - 18px) 0, 100% 18px, 100% 100%, 18px 100%, 0 calc(100% - 18px));
  transition: transform 220ms var(--ease-soft), box-shadow 220ms var(--ease-soft), border-color 220ms var(--ease-soft);
}

.update-card:hover {
  border-color: var(--line);
  box-shadow: 0 26px 50px rgba(255, 95, 158, 0.16);
  transform: translateY(-4px);
}

.update-cover {
  position: relative;
  min-height: 146px;
  overflow: hidden;
  background: var(--glass-pink);
}

.update-body {
  min-width: 0;
  padding: 18px 18px 16px;
}

.update-chip {
  display: inline-flex;
  max-width: 100%;
  padding: 5px 10px;
  overflow: hidden;
  color: var(--pink-600);
  font-size: 12px;
  font-weight: 900;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: var(--pink-50);
  border: 1px solid var(--pink-100);
  clip-path: polygon(var(--bevel-tag));
}

.update-body h3 {
  min-height: 44px;
  margin-top: 12px;
  overflow: hidden;
  color: var(--ink-900);
  font-size: 15px;
  font-weight: 900;
  line-height: 1.45;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.update-body time {
  display: block;
  margin-top: 12px;
  color: var(--ink-400);
  font-size: 12px;
  font-weight: 900;
}

.state-panel {
  display: grid;
  place-items: center;
  min-height: 214px;
  gap: 14px;
  color: var(--ink-600);
  background: rgba(255, 255, 255, 0.74);
  border: 1px dashed var(--line);
  clip-path: polygon(0 0, calc(100% - 18px) 0, 100% 18px, 100% 100%, 18px 100%, 0 calc(100% - 18px));
}

.state-panel.compact {
  min-height: 146px;
}

.state-panel strong {
  color: var(--ink-600);
  font-size: 15px;
  font-weight: 900;
}

.state-panel button {
  height: 36px;
  padding: 0 20px;
  color: #ffffff;
  font-size: 13px;
  font-weight: 900;
  background: linear-gradient(135deg, var(--pink-500), var(--blue-500));
  clip-path: polygon(var(--bevel-sm));
}

.skeleton-block {
  position: relative;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.68);
}

.skeleton-block::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(100deg, transparent 20%, rgba(255, 255, 255, 0.74) 45%, transparent 70%);
  animation: bp-skeleton 1.2s ease-in-out infinite;
}

.skeleton-title {
  width: 82%;
  height: 18px;
  margin-top: 14px;
  clip-path: polygon(var(--bevel-sm));
}

.skeleton-line {
  width: 84%;
  height: 18px;
  margin-top: 16px;
  clip-path: polygon(var(--bevel-sm));
}

.skeleton-line.short {
  width: 52%;
}

@keyframes bp-skeleton {
  from {
    transform: translateX(-110%);
  }
  to {
    transform: translateX(110%);
  }
}
</style>
