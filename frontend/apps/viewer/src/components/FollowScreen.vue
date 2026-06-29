<script setup lang="ts">
import { onMounted, ref } from 'vue'

import { api, type ViewerFollowedAnime } from '../api'
import FollowCard from './FollowCard.vue'
import ParticleField from './ParticleField.vue'

const emit = defineEmits<{ (event: 'open-follow', item: ViewerFollowedAnime): void }>()
const items = ref<ViewerFollowedAnime[]>([])
const loading = ref(false)
const errorMessage = ref('')

onMounted(loadFollows)

async function loadFollows() {
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await api.followedAnime()
    items.value = result.items
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '追番列表加载失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="follow-stage" aria-label="我的追番">
    <ParticleField :count="18" palette="pink" :max-size="34" />
    <div class="follow-grid-bg" aria-hidden="true" />
    <div class="follow-halo pink" aria-hidden="true" />
    <div class="follow-halo cyan" aria-hidden="true" />

    <div class="follow-wrap">
      <header class="follow-heading">
        <div><p>MY FOLLOWING <i /></p><h1>我的追番</h1><span>フォロー中・ANIME COLLECTION</span></div>
        <button type="button" :disabled="loading" @click="loadFollows">刷新列表</button>
      </header>
      <div class="result-heading">
        <div><p>FOLLOWED TITLES</p><h2>全部追番</h2></div>
        <span>共 {{ items.length }} 部</span>
      </div>

      <div v-if="loading" class="follow-cards">
        <article v-for="index in 10" :key="index" class="skeleton-card">
          <div class="skeleton-cover skeleton-block" /><div class="skeleton-line skeleton-block" />
        </article>
      </div>
      <div v-else-if="errorMessage" class="follow-state">
        <span>!</span><p>{{ errorMessage }}</p><button type="button" @click="loadFollows">重新加载</button>
      </div>
      <div v-else-if="items.length === 0" class="follow-state">
        <span>00</span><p>还没有追番</p><small>FOLLOW AN ANIME FROM ITS DETAIL PAGE</small>
      </div>
      <div v-else class="follow-cards">
        <FollowCard v-for="item in items" :key="item.bangumiId" :item="item" @open="emit('open-follow', $event)" />
      </div>
    </div>
  </section>
</template>

<style scoped>
.follow-stage { position: relative; min-height: calc(100vh - 86px); overflow: hidden; background: linear-gradient(145deg, rgba(255,249,252,.95), rgba(245,252,255,.93)); }
.follow-grid-bg { position: absolute; inset: 0; background: linear-gradient(rgba(85,119,217,.05) 1px, transparent 1px), linear-gradient(90deg, rgba(255,95,158,.045) 1px, transparent 1px); background-size: 64px 64px; mask-image: linear-gradient(to bottom, #000, transparent 88%); pointer-events: none; }
.follow-halo { position: absolute; width: 470px; height: 470px; border-radius: 50%; filter: blur(90px); pointer-events: none; }
.follow-halo.pink { top: -320px; right: 4%; background: rgba(255,159,189,.28); }
.follow-halo.cyan { top: 410px; left: -320px; background: rgba(73,214,233,.18); }
.follow-wrap { position: relative; z-index: 2; width: min(1500px, calc(100% - 72px)); margin: 0 auto; padding: 54px 0 92px; }
.follow-heading { min-height: 124px; display: flex; align-items: flex-end; justify-content: space-between; padding: 0 8px 28px; border-bottom: 1px solid var(--line-cool); }
.follow-heading p { display: flex; align-items: center; gap: 12px; color: var(--pink-500); font-family: var(--font-mono); font-size: 13px; letter-spacing: 2px; }
.follow-heading p i { width: 60px; height: 1px; background: linear-gradient(90deg, var(--pink-400), transparent); }
.follow-heading h1 { margin-top: 7px; color: var(--ink-900); font-size: 34px; line-height: 1.2; letter-spacing: 2px; }
.follow-heading div > span { display: block; margin-top: 6px; color: var(--ink-400); font-size: 13px; letter-spacing: 1.5px; }
.follow-heading > button { height: 38px; padding: 0 17px; color: var(--pink-600); font-size: 13px; border: 1px solid var(--line); background: rgba(255,255,255,.76); clip-path: polygon(var(--bevel-sm)); }
.follow-heading > button:hover:not(:disabled) { color: #fff; background: linear-gradient(135deg, var(--pink-500), var(--pink-600)); }
.result-heading { display: flex; align-items: flex-end; justify-content: space-between; margin: 42px 4px 22px; }
.result-heading p { color: var(--blue-500); font-family: var(--font-mono); font-size: 13px; letter-spacing: 1.5px; }
.result-heading h2 { margin-top: 2px; color: var(--ink-900); font-size: 22px; }
.result-heading > span { padding: 5px 13px; color: var(--ink-400); font-size: 13px; border-left: 2px solid var(--cyan-400); background: rgba(255,255,255,.65); }
.follow-cards { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 30px 20px; }
.follow-state { min-height: 280px; display: grid; place-items: center; align-content: center; gap: 7px; color: var(--ink-400); border: 1px dashed var(--line); background: rgba(255,255,255,.62); }
.follow-state > span { color: rgba(255,95,158,.24); font-family: var(--font-mono); font-size: 46px; }
.follow-state p { color: var(--ink-600); font-size: 14px; }
.follow-state small { font-family: var(--font-mono); font-size: 13px; letter-spacing: 1.2px; }
.follow-state button { margin-top: 8px; padding: 9px 17px; color: #fff; font-size: 13px; background: linear-gradient(135deg, var(--pink-500), var(--pink-600)); }
.skeleton-cover { aspect-ratio: 16 / 9; }
.skeleton-line { width: 86%; height: 13px; margin-top: 12px; }
.skeleton-block { position: relative; overflow: hidden; background: rgba(255,244,248,.75); }
.skeleton-block::after { content: ''; position: absolute; inset: 0; background: linear-gradient(100deg, transparent 20%, rgba(255,255,255,.75) 45%, transparent 70%); animation: follow-skeleton 1.2s ease-in-out infinite; }
@keyframes follow-skeleton { from { transform: translateX(-100%); } to { transform: translateX(100%); } }
</style>
