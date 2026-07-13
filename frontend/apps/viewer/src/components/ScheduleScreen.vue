<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import { api, type ViewerSchedule, type ViewerScheduleCard } from '../api'
import ParticleField from './ParticleField.vue'

const emit = defineEmits<{ (event: 'open-anime', bangumiId: number): void }>()

const weekdays = [
  { value: 1, label: '周一', kana: 'MON' },
  { value: 2, label: '周二', kana: 'TUE' },
  { value: 3, label: '周三', kana: 'WED' },
  { value: 4, label: '周四', kana: 'THU' },
  { value: 5, label: '周五', kana: 'FRI' },
  { value: 6, label: '周六', kana: 'SAT' },
  { value: 7, label: '周日', kana: 'SUN' },
  { value: 8, label: '其他', kana: 'ETC' },
]

const now = new Date()
const initialMonth = Math.floor(now.getMonth() / 3) * 3 + 1
const seasonYear = ref(now.getFullYear())
const seasonMonth = ref(initialMonth)
const selectedWeekday = ref(now.getDay() === 0 ? 7 : now.getDay())
const schedule = ref<ViewerSchedule | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const failedCovers = ref<Set<number>>(new Set())
const relativeTimeNow = ref(Date.now())
let requestID = 0
let relativeTimeTimer: ReturnType<typeof setInterval> | null = null

const seasonKey = computed(() => `${seasonYear.value.toString().padStart(4, '0')}-${seasonMonth.value.toString().padStart(2, '0')}`)
const fallbackSeasonLabel = computed(() => `${seasonYear.value}年${seasonMonth.value}月`)
const selectedDay = computed(() => weekdays.find((day) => day.value === selectedWeekday.value) ?? weekdays[0])
const selectedItems = computed(() => itemsForDay(selectedWeekday.value))

onMounted(() => {
  void loadSchedule()
  relativeTimeTimer = setInterval(() => {
    relativeTimeNow.value = Date.now()
  }, 60_000)
})

onBeforeUnmount(() => {
  if (relativeTimeTimer !== null) clearInterval(relativeTimeTimer)
})

function normalizedWeekday(value: number) {
  return value >= 1 && value <= 7 ? value : 8
}

function itemsForDay(weekday: number) {
  return (schedule.value?.items ?? [])
    .filter((item) => normalizedWeekday(item.airWeekday) === weekday)
    .sort((left, right) => Number(isUnavailable(left)) - Number(isUnavailable(right)))
}

async function loadSchedule() {
  const currentRequest = ++requestID
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await api.animeSchedule(seasonKey.value)
    if (currentRequest === requestID) {
      schedule.value = result.schedule
      failedCovers.value = new Set()
    }
  } catch (error) {
    if (currentRequest === requestID) {
      errorMessage.value = error instanceof Error ? error.message : '番剧时间表加载失败'
    }
  } finally {
    if (currentRequest === requestID) {
      loading.value = false
    }
  }
}

function changeSeason(direction: number) {
  const next = new Date(seasonYear.value, seasonMonth.value - 1 + direction * 3, 1)
  seasonYear.value = next.getFullYear()
  seasonMonth.value = next.getMonth() + 1
  void loadSchedule()
}

function selectWeekday(value: number) {
  selectedWeekday.value = value
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
  if (item.latestEpisodeLabel) {
    return `更新至 ${item.latestEpisodeLabel}`
  }
  const status = availabilityStatus(item)
  if (status === 'unknown') return '开播时间未定'
  return status === 'not-aired' ? '尚未开播' : '尚未放流'
}

function scheduleCardMeta(item: ViewerScheduleCard) {
  const updateAge = formatScheduleUpdateAge(item.latestEpisodeUpdatedAt)
  return updateAge ? `于 ${updateAge} 更新` : formatAirDate(item.airDate)
}

function formatScheduleUpdateAge(value: number | null) {
  if (!value) return ''
  const elapsedSeconds = Math.max(Math.floor(relativeTimeNow.value / 1000) - value, 0)
  const hourSeconds = 60 * 60
  const daySeconds = 24 * hourSeconds
  if (elapsedSeconds < hourSeconds) return '刚刚'
  if (elapsedSeconds < daySeconds) return `${Math.floor(elapsedSeconds / hourSeconds)} 小时前`
  if (elapsedSeconds > 30 * daySeconds) return ''
  const days = Math.floor(elapsedSeconds / daySeconds)
  return days === 1 ? '昨天' : `${days} 天前`
}

function availabilityStatus(item: ViewerScheduleCard) {
  if (item.latestEpisodeLabel) return 'available'
  if (!item.airDate) return 'unknown'
  const premiere = new Date(`${item.airDate}T00:00:00`)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  return !Number.isNaN(premiere.getTime()) && premiere > today ? 'not-aired' : 'not-released'
}

function isUnavailable(item: ViewerScheduleCard) {
  const status = availabilityStatus(item)
  return status === 'not-aired' || status === 'not-released'
}

function stagger(index: number) {
  return `${0.04 + index * 0.045}s`
}
</script>

<template>
  <section class="schedule-stage" aria-label="番剧时间表">
    <ParticleField :count="18" palette="cool" :max-size="32" />
    <div class="schedule-grid" aria-hidden="true" />
    <div class="schedule-halo halo-pink" aria-hidden="true" />
    <div class="schedule-halo halo-cyan" aria-hidden="true" />

    <div class="schedule-wrap">
      <header class="schedule-hero">
        <div class="hero-copy">
          <p class="eyebrow"><span>SEASON PROGRAM</span><i /></p>
          <h1>番剧时间表</h1>
          <p class="hero-note">アニメーション・ウィークリーガイド</p>
        </div>

        <div class="season-selector" aria-label="季度切换">
          <button type="button" :disabled="loading" aria-label="上一季度" @click="changeSeason(-1)">
            <i class="selector-arrow prev" aria-hidden="true" />
          </button>
          <div class="season-label">
            <span>{{ schedule?.seasonLabel || fallbackSeasonLabel }}</span>
            <small>{{ seasonKey }} / QUARTER</small>
          </div>
          <button type="button" :disabled="loading" aria-label="下一季度" @click="changeSeason(1)">
            <i class="selector-arrow next" aria-hidden="true" />
          </button>
        </div>
      </header>

      <nav class="weekday-tabs" aria-label="按星期筛选" role="tablist">
        <button
          v-for="day in weekdays"
          :key="day.value"
          class="weekday-tab"
          :class="{ active: selectedWeekday === day.value }"
          type="button"
          role="tab"
          :aria-selected="selectedWeekday === day.value"
          @click="selectWeekday(day.value)"
        >
          <span class="day-kana">{{ day.kana }}</span>
          <span class="day-label">{{ day.label }}</span>
          <span class="day-count">{{ itemsForDay(day.value).length }}</span>
        </button>
      </nav>

      <div class="day-heading">
        <div>
          <span class="day-index">{{ String(selectedDay.value).padStart(2, '0') }}</span>
          <p>{{ selectedDay.kana }} LINEUP</p>
          <h2>{{ selectedDay.label }}放送</h2>
        </div>
        <span class="lineup-count">共 {{ selectedItems.length }} 部</span>
      </div>

      <div v-if="loading" class="schedule-cards" aria-label="加载中">
        <article v-for="index in 8" :key="index" class="schedule-card skeleton-card">
          <div class="card-cover skeleton-block" />
          <div class="skeleton-line skeleton-block" />
          <div class="skeleton-line short skeleton-block" />
        </article>
      </div>

      <div v-else-if="errorMessage" class="schedule-state error-state">
        <span>!</span>
        <p>{{ errorMessage }}</p>
        <button type="button" @click="loadSchedule">重新加载</button>
      </div>

      <div v-else-if="selectedItems.length === 0" class="schedule-state">
        <span>00</span>
        <p>该分类暂时没有收录番剧</p>
        <small>NO PROGRAMS IN THIS LINEUP</small>
      </div>

      <div v-else class="schedule-cards">
        <article
          v-for="(item, index) in selectedItems"
          :key="item.bangumiId"
          class="schedule-card"
          :class="{ unavailable: isUnavailable(item) }"
          :style="{ '--stagger': stagger(index) }"
          role="link"
          tabindex="0"
          @click="emit('open-anime', item.bangumiId)"
          @keydown.enter="emit('open-anime', item.bangumiId)"
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
          <p class="air-date"><i aria-hidden="true" />{{ scheduleCardMeta(item) }}</p>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.schedule-stage {
  position: relative;
  min-height: calc(100vh - 86px);
  overflow: hidden;
  background: linear-gradient(145deg, rgba(255, 249, 252, 0.9), rgba(246, 252, 255, 0.92));
}

.schedule-grid {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(rgba(85, 119, 217, 0.055) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 95, 158, 0.05) 1px, transparent 1px);
  background-size: 64px 64px;
  mask-image: linear-gradient(to bottom, #000 0%, transparent 78%);
  pointer-events: none;
}

.schedule-halo {
  position: absolute;
  width: 430px;
  height: 430px;
  border-radius: 50%;
  filter: blur(80px);
  pointer-events: none;
}

.halo-pink { top: -250px; right: 5%; background: rgba(255, 159, 189, 0.28); }
.halo-cyan { top: 240px; left: -260px; background: rgba(73, 214, 233, 0.2); }

.schedule-wrap {
  position: relative;
  z-index: 2;
  width: min(1500px, calc(100% - 72px));
  margin: 0 auto;
  padding: 54px 0 88px;
}

.schedule-hero {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  min-height: 124px;
  padding: 0 8px 28px;
  border-bottom: 1px solid var(--line-cool);
}

.eyebrow { display: flex; align-items: center; gap: 12px; color: var(--pink-500); font-size: 11px; letter-spacing: 3px; }
.eyebrow i { width: 62px; height: 1px; background: linear-gradient(90deg, var(--pink-400), transparent); }
.hero-copy h1 { margin-top: 7px; color: var(--ink-900); font-size: 34px; line-height: 1.2; letter-spacing: 2px; }
.hero-note { margin-top: 6px; color: var(--ink-400); font-size: 12px; letter-spacing: 2px; }

.season-selector {
  display: grid;
  grid-template-columns: 46px minmax(180px, auto) 46px;
  align-items: stretch;
  height: 68px;
  background: rgba(255, 255, 255, 0.78);
  border: 1px solid var(--line);
  box-shadow: 0 16px 34px rgba(85, 119, 217, 0.09);
  clip-path: polygon(0 0, calc(100% - 15px) 0, 100% 15px, 100% 100%, 15px 100%, 0 calc(100% - 15px));
}

.season-selector button { position: relative; display: grid; place-items: center; color: var(--pink-500); transition: background 180ms ease; }
.season-selector button:hover:not(:disabled) { background: var(--pink-50); }
.season-selector button:disabled { cursor: wait; opacity: 0.45; }
.selector-arrow { width: 11px; height: 11px; border-top: 2px solid currentColor; border-left: 2px solid currentColor; }
.selector-arrow.prev { transform: rotate(-45deg); }
.selector-arrow.next { transform: rotate(135deg); }
.season-label { display: grid; place-items: center; align-content: center; padding: 0 24px; text-align: center; border-inline: 1px solid var(--line-soft); }
.season-label span { color: var(--ink-900); font-size: 21px; letter-spacing: 1px; white-space: nowrap; }
.season-label small { margin-top: 3px; color: var(--ink-400); font-family: var(--font-mono); font-size: 9px; letter-spacing: 1.5px; }

.weekday-tabs {
  display: grid;
  grid-template-columns: repeat(8, minmax(0, 1fr));
  gap: 7px;
  margin-top: 28px;
  padding: 7px;
  background: rgba(255, 255, 255, 0.63);
  border: 1px solid rgba(85, 119, 217, 0.12);
  backdrop-filter: blur(12px);
  clip-path: polygon(0 0, calc(100% - 13px) 0, 100% 13px, 100% 100%, 13px 100%, 0 calc(100% - 13px));
}

.weekday-tab {
  position: relative;
  min-height: 68px;
  display: grid;
  grid-template-columns: 1fr auto;
  grid-template-rows: auto auto;
  align-content: center;
  padding: 9px 14px;
  color: var(--ink-600);
  text-align: left;
  transition: color 180ms ease, background 180ms ease, transform 180ms ease;
  clip-path: polygon(0 0, calc(100% - 9px) 0, 100% 9px, 100% 100%, 9px 100%, 0 calc(100% - 9px));
}

.weekday-tab:hover { color: var(--pink-600); background: rgba(255, 244, 248, 0.8); }
.weekday-tab.active { color: #fff; background: linear-gradient(135deg, var(--pink-400), var(--pink-600) 72%, #de4e94); box-shadow: 0 10px 22px rgba(255, 95, 158, 0.22); transform: translateY(-2px); }
.day-kana { font-family: var(--font-mono); font-size: 9px; letter-spacing: 1.5px; opacity: 0.68; }
.day-label { grid-column: 1; font-size: 14px; }
.day-count { grid-column: 2; grid-row: 1 / 3; align-self: center; min-width: 27px; height: 27px; display: grid; place-items: center; color: var(--pink-600); font-family: var(--font-mono); font-size: 11px; background: rgba(255,255,255,.9); clip-path: polygon(7px 0, 100% 0, 100% calc(100% - 7px), calc(100% - 7px) 100%, 0 100%, 0 7px); }

.day-heading { display: flex; justify-content: space-between; align-items: flex-end; margin: 46px 4px 22px; }
.day-heading > div { position: relative; min-height: 48px; padding-left: 66px; }
.day-index { position: absolute; left: 0; top: 1px; color: rgba(255, 95, 158, 0.16); font-family: var(--font-mono); font-size: 47px; line-height: 1; }
.day-heading p { color: var(--blue-500); font-family: var(--font-mono); font-size: 9px; letter-spacing: 2px; }
.day-heading h2 { margin-top: 2px; color: var(--ink-900); font-size: 23px; letter-spacing: 1px; }
.lineup-count { padding: 5px 13px; color: var(--ink-400); font-size: 11px; border-left: 2px solid var(--cyan-400); background: rgba(255,255,255,.65); }

.schedule-cards { display: grid; grid-template-columns: repeat(8, minmax(0, 1fr)); gap: 18px; }
.schedule-card { min-width: 0; cursor: pointer; outline: 0; animation: bp-rise .42s var(--ease-out) both; animation-delay: var(--stagger, 0s); }
.schedule-card:focus-visible .card-cover { box-shadow: 0 0 0 3px rgba(255,95,158,.24), 0 24px 48px rgba(255,95,158,.18); }
.card-cover { position: relative; aspect-ratio: 2 / 3; overflow: hidden; background: var(--pink-50); border: 1px solid rgba(255,255,255,.9); box-shadow: 0 15px 32px rgba(85,119,217,.1); clip-path: polygon(0 0, calc(100% - 15px) 0, 100% 15px, 100% 100%, 15px 100%, 0 calc(100% - 15px)); transition: transform 220ms var(--ease-soft), box-shadow 220ms var(--ease-soft); }
.schedule-card:hover .card-cover { transform: translateY(-6px); box-shadow: 0 24px 48px rgba(255,95,158,.18); }
.card-cover img { width: 100%; height: 100%; object-fit: unset; }
.schedule-card.unavailable .card-cover img,
.schedule-card.unavailable .cover-fallback { filter: grayscale(1) contrast(.88); }
.cover-fallback { display: grid; place-items: center; width: 100%; height: 100%; padding: 16px; color: var(--pink-600); font-size: 21px; background: linear-gradient(145deg, rgba(255,244,248,.96), rgba(236,253,255,.9)), repeating-linear-gradient(135deg, rgba(255,95,158,.1) 0 2px, transparent 2px 12px); }
.episode-total { position: absolute; top: 9px; right: 8px; z-index: 2; height: 23px; padding: 0 9px; display: inline-flex; align-items: center; color: var(--ink-700); font-size: 10px; white-space: nowrap; background: rgba(255,255,255,.88); box-shadow: 0 5px 13px rgba(32,40,62,.12); backdrop-filter: blur(8px); clip-path: polygon(0 0, calc(100% - 6px) 0, 100% 6px, 100% 100%, 6px 100%, 0 calc(100% - 6px)); }
.progress-shade { position: absolute; right: 0; bottom: 0; left: 0; z-index: 2; min-height: 66px; display: flex; align-items: flex-end; gap: 7px; padding: 24px 10px 9px; color: #fff; font-size: 12px; background: linear-gradient(to top, rgba(25,32,53,.84), rgba(25,32,53,0)); text-shadow: 0 2px 5px rgba(0,0,0,.65); }
.progress-shade i { width: 6px; height: 6px; margin-bottom: 5px; flex: 0 0 auto; background: var(--cyan-300); transform: rotate(45deg); box-shadow: 0 0 10px rgba(142,232,242,.8); }
.schedule-card h3 { margin-top: 11px; overflow: hidden; color: var(--ink-900); font-size: 14px; line-height: 1.45; text-overflow: ellipsis; white-space: nowrap; }
.air-date { display: flex; align-items: center; gap: 7px; margin-top: 5px; overflow: hidden; color: var(--ink-400); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.air-date i { width: 13px; height: 1px; flex: 0 0 auto; background: linear-gradient(90deg, var(--pink-400), var(--cyan-400)); }

.schedule-state { min-height: 270px; display: grid; place-items: center; align-content: center; gap: 5px; color: var(--ink-400); background: rgba(255,255,255,.62); border: 1px dashed var(--line); clip-path: polygon(0 0, calc(100% - 20px) 0, 100% 20px, 100% 100%, 20px 100%, 0 calc(100% - 20px)); }
.schedule-state span { color: rgba(255,95,158,.22); font-family: var(--font-mono); font-size: 46px; line-height: 1; }
.schedule-state p { color: var(--ink-600); font-size: 14px; }
.schedule-state small { margin-top: 3px; font-family: var(--font-mono); font-size: 9px; letter-spacing: 2px; }
.schedule-state button { margin-top: 12px; padding: 8px 20px; color: #fff; font-size: 12px; background: linear-gradient(135deg, var(--pink-500), var(--pink-600)); clip-path: polygon(var(--bevel-sm)); }

.skeleton-block { position: relative; overflow: hidden; background: rgba(255,244,248,.75); }
.skeleton-block::after { content: ''; position: absolute; inset: 0; background: linear-gradient(100deg, transparent 20%, rgba(255,255,255,.75) 45%, transparent 70%); animation: schedule-skeleton 1.2s ease-in-out infinite; }
.skeleton-line { width: 90%; height: 12px; margin-top: 12px; }
.skeleton-line.short { width: 62%; height: 9px; margin-top: 7px; }
@keyframes schedule-skeleton { from { transform: translateX(-100%); } to { transform: translateX(100%); } }
</style>
