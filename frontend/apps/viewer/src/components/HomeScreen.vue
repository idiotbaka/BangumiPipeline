<script setup lang="ts">
import type { ViewerUser } from '../api'
import ParticleField from './ParticleField.vue'

interface Props {
  user: ViewerUser
  siteName: string
  loading: boolean
}

defineProps<Props>()
const emit = defineEmits<{ (e: 'logout'): void }>()
</script>

<template>
  <main class="home-shell">
    <!-- 顶栏 -->
    <header class="topbar">
      <div class="brand-row">
        <div class="brand-mark">BP</div>
        <div class="brand-text">
          <p>VIEWER PORTAL</p>
          <strong>{{ siteName }}</strong>
        </div>
      </div>

      <div class="user-chip">
        <span class="user-avatar" aria-hidden="true">{{ user.username.slice(0, 1).toUpperCase() }}</span>
        <span class="user-name">{{ user.username }}</span>
        <button class="logout-button" :disabled="loading" type="button" @click="emit('logout')">
          退出
        </button>
      </div>
    </header>

    <!-- 主留白区：氛围装饰 + 占位提示 -->
    <section class="home-stage" aria-label="首页">
      <!-- 背景光晕 -->
      <div class="stage-halo halo-1" aria-hidden="true" />
      <div class="stage-halo halo-2" aria-hidden="true" />

      <!-- 几何装饰 -->
      <div class="stage-shape shape-tri" aria-hidden="true" />
      <div class="stage-shape shape-rect" aria-hidden="true" />
      <div class="stage-shape shape-dia" aria-hidden="true" />

      <!-- 飘动碎片（低密度） -->
      <ParticleField :count="16" palette="pink" :max-size="36" />

      <!-- 中央占位提示 -->
      <div class="placeholder">
        <div class="placeholder-badge">
          <span class="badge-dot" aria-hidden="true" />
          COMING SOON
        </div>
        <h2 class="placeholder-title">内容即将上线</h2>
        <p class="placeholder-sub">
          番剧图书馆正在筹备中，敬请期待全新的观看体验。
        </p>

        <!-- 虚线占位框，暗示未来内容区 -->
        <div class="placeholder-frame" aria-hidden="true">
          <div class="frame-grid">
            <span v-for="n in 8" :key="n" class="frame-cell" />
          </div>
        </div>
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
    linear-gradient(135deg, rgba(255, 244, 248, 0.82), rgba(255, 255, 255, 0.88) 40%, rgba(228, 251, 255, 0.62)),
    repeating-linear-gradient(90deg, var(--line-soft) 0 1px, transparent 1px 46px),
    #ffffff;
}

/* ============ 顶栏 ============ */
.topbar {
  position: relative;
  z-index: 5;
  height: 84px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 48px;
  background: var(--glass-strong);
  border-bottom: 1px solid var(--line);
  backdrop-filter: blur(18px);
  animation: bp-rise 0.5s var(--ease-out) both;
}

.brand-row {
  display: flex;
  align-items: center;
  gap: 14px;
}

.brand-mark {
  display: grid;
  place-items: center;
  width: 46px;
  height: 46px;
  color: #ffffff;
  font-weight: 900;
  font-size: 14px;
  letter-spacing: 0.5px;
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600) 60%, var(--blue-500));
  box-shadow: 0 12px 28px rgba(255, 95, 158, 0.3);
  clip-path: polygon(var(--bevel-md));
}

.brand-text p {
  color: var(--ink-400);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 2px;
}

.brand-text strong {
  display: block;
  margin-top: 2px;
  overflow: hidden;
  max-width: 620px;
  font-size: 20px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 用户 chip + 切角退出按钮 */
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
  margin-right: 2px;
  color: var(--ink-900);
  font-size: 14px;
  font-weight: 800;
  max-width: 160px;
  overflow: hidden;
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
  border-radius: 0;
  clip-path: polygon(0 0, calc(100% - 8px) 0, 100% 8px, 100% 100%, 8px 100%, 0 calc(100% - 8px));
  transition: transform 180ms var(--ease-soft), box-shadow 180ms var(--ease-soft), filter 180ms var(--ease-soft);
}

.logout-button:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 12px 22px rgba(255, 95, 158, 0.34);
  filter: saturate(1.08);
}

.logout-button:disabled {
  filter: grayscale(0.3) brightness(0.95);
}

/* ============ 主留白区 ============ */
.home-stage {
  position: relative;
  min-height: calc(100vh - 84px);
  overflow: hidden;
}

.stage-halo {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  pointer-events: none;
  z-index: 0;
  animation: bp-halo 9s ease-in-out infinite;
}

.halo-1 {
  width: 720px;
  height: 720px;
  left: -180px;
  top: -200px;
  background: radial-gradient(circle, rgba(255, 159, 189, 0.45), transparent 70%);
}

.halo-2 {
  width: 820px;
  height: 820px;
  right: -240px;
  bottom: -280px;
  background: radial-gradient(circle, rgba(73, 214, 233, 0.38), transparent 70%);
  animation-delay: 4s;
}

/* 几何装饰块 */
.stage-shape {
  position: absolute;
  z-index: 1;
  border: 1px solid rgba(255, 255, 255, 0.8);
  animation: bp-sway 6s ease-in-out infinite alternate;
}

.shape-tri {
  left: 7%;
  top: 22%;
  width: 0;
  height: 0;
  border: 0;
  border-left: 80px solid transparent;
  border-right: 80px solid transparent;
  border-bottom: 130px solid rgba(255, 229, 122, 0.28);
  filter: drop-shadow(0 18px 40px rgba(255, 229, 122, 0.18));
}

.shape-rect {
  right: 9%;
  top: 18%;
  width: 200px;
  height: 200px;
  background: rgba(255, 159, 189, 0.2);
  clip-path: polygon(0 0, 100% 0, 86% 100%, 0 100%);
  animation-delay: 1s;
}

.shape-dia {
  right: 16%;
  bottom: 16%;
  width: 140px;
  height: 140px;
  background: rgba(73, 214, 233, 0.22);
  clip-path: polygon(50% 0, 100% 50%, 50% 100%, 0 50%);
  animation-delay: 2s;
}

/* ============ 中央占位提示 ============ */
.placeholder {
  position: relative;
  z-index: 4;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: calc(100vh - 84px);
  padding: 40px;
  text-align: center;
  animation: bp-rise 0.7s var(--ease-out) 0.1s both;
}

.placeholder-badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 7px 14px;
  color: var(--pink-600);
  font-size: 12px;
  font-weight: 900;
  letter-spacing: 2px;
  background: var(--pink-50);
  border: 1px solid var(--pink-100);
  clip-path: polygon(8px 0, 100% 0, calc(100% - 8px) 100%, 0 100%);
}

.badge-dot {
  width: 7px;
  height: 7px;
  background: var(--pink-500);
  border-radius: 50%;
  animation: bp-halo 1.6s ease-in-out infinite;
}

.placeholder-title {
  margin-top: 22px;
  font-size: 40px;
  font-weight: 700;
  letter-spacing: 4px;
  color: var(--ink-900);
}

.placeholder-sub {
  margin-top: 14px;
  max-width: 460px;
  color: var(--ink-400);
  font-size: 15px;
  line-height: 1.7;
}

/* 虚线占位框：暗示未来内容区 */
.placeholder-frame {
  margin-top: 44px;
  width: min(960px, 84%);
  padding: 28px;
  background: var(--glass);
  border: 1.5px dashed var(--line);
  backdrop-filter: blur(8px);
  clip-path: polygon(0 0, calc(100% - 18px) 0, 100% 18px, 100% 100%, 18px 100%, 0 calc(100% - 18px));
}

.frame-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 18px;
}

.frame-cell {
  height: 150px;
  background:
    linear-gradient(135deg, rgba(255, 244, 248, 0.8), rgba(228, 251, 255, 0.6));
  border: 1px dashed var(--line-soft);
  clip-path: polygon(0 0, calc(100% - 10px) 0, 100% 10px, 100% 100%, 10px 100%, 0 calc(100% - 10px));
  animation: bp-halo 5s ease-in-out infinite;
}

.frame-cell:nth-child(2) {
  animation-delay: 0.6s;
}
.frame-cell:nth-child(3) {
  animation-delay: 1.2s;
}
.frame-cell:nth-child(4) {
  animation-delay: 1.8s;
}
.frame-cell:nth-child(5) {
  animation-delay: 2.4s;
}
.frame-cell:nth-child(6) {
  animation-delay: 3s;
}
.frame-cell:nth-child(7) {
  animation-delay: 3.6s;
}
.frame-cell:nth-child(8) {
  animation-delay: 4.2s;
}
</style>
