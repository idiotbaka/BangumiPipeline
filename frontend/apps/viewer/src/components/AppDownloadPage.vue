<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import appIcon from '../../../../../src-tauri/icons/icon.png'
import { api, type AppRelease } from '../api'

const release = ref<AppRelease | null>(null)
const loading = ref(true)
const message = ref('')

const downloadURL = computed(() =>
  release.value ? `/api/app/releases/${release.value.id}/download` : '',
)

onMounted(() => {
  document.title = 'BakaVip2 - Android APP 下载'
  void loadRelease()
})

async function loadRelease() {
  loading.value = true
  message.value = ''
  try {
    const result = await api.latestAppRelease()
    release.value = result.release
  } catch (error) {
    message.value = error instanceof Error ? error.message : '暂时无法获取最新版本'
  } finally {
    loading.value = false
  }
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '-'
  const units = ['B', 'KiB', 'MiB', 'GiB']
  let size = value
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  return `${size.toFixed(unit === 0 ? 0 : 2)} ${units[unit]}`
}
</script>

<template>
  <main class="app-download-page">
    <div class="ambient-grid" aria-hidden="true" />
    <div class="ambient-orb orb-pink" aria-hidden="true" />
    <div class="ambient-orb orb-cyan" aria-hidden="true" />

    <section class="download-shell">
      <header class="app-identity">
        <div class="logo-frame">
          <img :src="appIcon" alt="BakaVip2 Logo" />
          <span class="logo-shine" aria-hidden="true" />
        </div>
        <div>
          <p class="platform-label"><span /> ANDROID APPLICATION</p>
          <h1>BakaVip2</h1>
          <p class="app-subtitle">随时随地，继续你的番剧时光</p>
        </div>
      </header>

      <div v-if="loading" class="release-panel loading-panel" aria-live="polite">
        <div class="skeleton-line skeleton-short" />
        <div class="skeleton-line skeleton-title" />
        <div class="skeleton-line" />
        <div class="skeleton-line skeleton-wide" />
        <div class="skeleton-button" />
      </div>

      <div v-else-if="message" class="release-panel state-panel" role="alert">
        <span class="state-icon">!</span>
        <h2>获取版本信息失败</h2>
        <p>{{ message }}</p>
        <button type="button" @click="loadRelease">重新获取</button>
      </div>

      <div v-else-if="!release" class="release-panel state-panel">
        <span class="state-icon empty">·</span>
        <h2>安装包准备中</h2>
        <p>目前还没有已发布的 APP 版本，请稍后再来。</p>
      </div>

      <article v-else class="release-panel">
        <div class="version-row">
          <div>
            <span class="latest-chip">LATEST RELEASE</span>
            <h2>版本 {{ release.version }}</h2>
          </div>
          <div class="file-size">
            <span>APK SIZE</span>
            <strong>{{ formatBytes(release.apkSize) }}</strong>
          </div>
        </div>

        <section class="release-notes">
          <div class="section-heading">
            <span class="heading-mark" />
            <h3>本次更新</h3>
          </div>
          <p>{{ release.releaseNotes }}</p>
        </section>

        <a class="download-button" :href="downloadURL" download>
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M12 3v11m0 0 4-4m-4 4-4-4M5 17v2h14v-2" />
          </svg>
          <span>
            <strong>下载 Android APK</strong>
            <small>BakaVip2-{{ release.version }}.apk</small>
          </span>
          <i aria-hidden="true">›</i>
        </a>

        <p class="compatibility-note">
          <span aria-hidden="true">◆</span>
          适用于 Android 7.0 及以上的 arm64-v8a 设备
        </p>
      </article>
    </section>

    <footer>
      <span>BakaVip2</span>
      <i />
      <span>Powered by BangumiPipeline</span>
    </footer>
  </main>
</template>

<style scoped>
.app-download-page {
  position: relative;
  min-height: 100vh;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 28px;
  overflow: hidden;
  padding: 56px 20px 28px;
  color: #26314b;
  background:
    linear-gradient(145deg, rgba(255, 247, 250, 0.96), rgba(241, 252, 255, 0.96)),
    #fff;
}

.ambient-grid {
  position: absolute;
  inset: 0;
  opacity: 0.45;
  pointer-events: none;
  background-image:
    linear-gradient(rgba(85, 119, 217, 0.055) 1px, transparent 1px),
    linear-gradient(90deg, rgba(85, 119, 217, 0.055) 1px, transparent 1px);
  background-size: 42px 42px;
  mask-image: linear-gradient(to bottom, #000, transparent 82%);
}

.ambient-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(12px);
  pointer-events: none;
}

.orb-pink {
  width: 420px;
  height: 420px;
  top: -210px;
  right: -120px;
  background: radial-gradient(circle, rgba(255, 159, 189, 0.36), transparent 70%);
}

.orb-cyan {
  width: 380px;
  height: 380px;
  left: -180px;
  bottom: -170px;
  background: radial-gradient(circle, rgba(73, 214, 233, 0.28), transparent 70%);
}

.download-shell {
  position: relative;
  z-index: 1;
  width: min(100%, 620px);
}

.app-identity {
  display: flex;
  align-items: center;
  gap: 22px;
  margin: 0 10px 26px;
}

.logo-frame {
  position: relative;
  flex: 0 0 auto;
  width: 96px;
  height: 96px;
  overflow: hidden;
  padding: 4px;
  border: 1px solid rgba(255, 95, 158, 0.24);
  border-radius: 25px;
  background: rgba(255, 255, 255, 0.84);
  box-shadow: 0 18px 42px rgba(225, 77, 137, 0.18);
}

.logo-frame img {
  width: 100%;
  height: 100%;
  border-radius: 21px;
  object-fit: cover;
}

.logo-shine {
  position: absolute;
  width: 72px;
  height: 24px;
  top: 4px;
  left: 10px;
  opacity: 0.5;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.5);
  filter: blur(8px);
  transform: rotate(-18deg);
}

.platform-label {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 3px;
  color: #71809f;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 2px;
}

.platform-label span {
  width: 17px;
  height: 3px;
  background: linear-gradient(90deg, #ff5f9e, #49d6e9);
}

.app-identity h1 {
  color: #20283e;
  font-size: clamp(32px, 8vw, 44px);
  line-height: 1.08;
  letter-spacing: -1px;
}

.app-subtitle {
  margin-top: 7px;
  color: #71809f;
  font-size: 14px;
}

.release-panel {
  position: relative;
  overflow: hidden;
  padding: 34px;
  border: 1px solid rgba(255, 95, 158, 0.18);
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.88);
  box-shadow:
    0 28px 80px rgba(54, 74, 126, 0.12),
    0 8px 26px rgba(255, 95, 158, 0.08);
  backdrop-filter: blur(18px);
}

.release-panel::before {
  position: absolute;
  width: 150px;
  height: 6px;
  top: 0;
  left: 34px;
  content: '';
  background: linear-gradient(90deg, #ff5f9e, #49d6e9, transparent);
}

.version-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
}

.latest-chip {
  display: inline-flex;
  padding: 4px 10px;
  color: #e23d80;
  font-size: 9px;
  font-weight: 800;
  letter-spacing: 1.5px;
  border: 1px solid rgba(255, 95, 158, 0.2);
  border-radius: 999px;
  background: #fff4f8;
}

.version-row h2 {
  margin-top: 9px;
  color: #26314b;
  font-size: 26px;
}

.file-size {
  display: grid;
  gap: 3px;
  padding-left: 18px;
  text-align: right;
  border-left: 1px solid rgba(85, 119, 217, 0.14);
}

.file-size span {
  color: #a0a9bb;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 1.5px;
}

.file-size strong {
  color: #526078;
  font-size: 15px;
  white-space: nowrap;
}

.release-notes {
  margin: 30px 0;
  padding: 22px 24px;
  border: 1px solid rgba(85, 119, 217, 0.1);
  border-radius: 15px;
  background: linear-gradient(135deg, rgba(255, 244, 248, 0.62), rgba(236, 253, 255, 0.56));
}

.section-heading {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 13px;
}

.heading-mark {
  width: 7px;
  height: 18px;
  border-radius: 4px;
  background: linear-gradient(#ff5f9e, #ff9fbd);
}

.section-heading h3 {
  color: #3a4767;
  font-size: 15px;
}

.release-notes > p {
  color: #65718d;
  font-size: 14px;
  line-height: 1.85;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.download-button {
  position: relative;
  min-height: 70px;
  display: flex;
  align-items: center;
  gap: 15px;
  padding: 12px 19px;
  overflow: hidden;
  color: #fff;
  border-radius: 16px;
  background: linear-gradient(120deg, #ff5f9e, #ee3f86 55%, #d83679);
  box-shadow: 0 16px 30px rgba(238, 63, 134, 0.27);
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.download-button::after {
  position: absolute;
  width: 130px;
  height: 130px;
  right: -48px;
  top: -70px;
  content: '';
  border: 1px solid rgba(255, 255, 255, 0.24);
  border-radius: 50%;
}

.download-button:hover {
  transform: translateY(-2px);
  box-shadow: 0 20px 36px rgba(238, 63, 134, 0.34);
}

.download-button svg {
  z-index: 1;
  width: 25px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.download-button > span {
  z-index: 1;
  display: grid;
  gap: 2px;
}

.download-button strong {
  font-size: 16px;
}

.download-button small {
  color: rgba(255, 255, 255, 0.75);
  font-size: 11px;
}

.download-button i {
  z-index: 1;
  margin-left: auto;
  font-size: 30px;
  font-style: normal;
}

.compatibility-note {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 8px;
  margin-top: 18px;
  color: #929bad;
  font-size: 11px;
  text-align: center;
}

.compatibility-note span {
  color: #49c8da;
  font-size: 7px;
}

.state-panel {
  min-height: 330px;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 10px;
  text-align: center;
}

.state-icon {
  width: 52px;
  height: 52px;
  display: grid;
  place-items: center;
  margin-bottom: 4px;
  color: #fff;
  font-size: 24px;
  font-weight: 800;
  border-radius: 50%;
  background: linear-gradient(135deg, #ff7fa6, #ee3f86);
}

.state-icon.empty {
  background: linear-gradient(135deg, #8ee8f2, #5577d9);
}

.state-panel h2 {
  font-size: 21px;
}

.state-panel p {
  color: #7f8aa3;
  font-size: 13px;
}

.state-panel button {
  margin-top: 10px;
  padding: 9px 20px;
  color: #fff;
  border-radius: 999px;
  background: #ff5f9e;
}

.loading-panel {
  min-height: 330px;
  display: grid;
  align-content: center;
  gap: 14px;
}

.skeleton-line,
.skeleton-button {
  height: 15px;
  border-radius: 8px;
  background: linear-gradient(100deg, #f2f4f8 25%, #fff 42%, #f2f4f8 60%);
  background-size: 240% 100%;
  animation: skeleton-flow 1.25s ease-in-out infinite;
}

.skeleton-short { width: 24%; }
.skeleton-title { width: 48%; height: 30px; margin-bottom: 14px; }
.skeleton-wide { width: 82%; }
.skeleton-button { height: 70px; margin-top: 22px; }

.app-download-page > footer {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  gap: 10px;
  color: #a0a9bb;
  font-size: 10px;
  letter-spacing: 0.7px;
}

.app-download-page > footer i {
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: #ff9fbd;
}

@keyframes skeleton-flow {
  from { background-position: 100% 0; }
  to { background-position: -100% 0; }
}

@media (max-width: 600px) {
  .app-download-page {
    place-items: start center;
    padding: 38px 16px 24px;
  }

  .app-identity {
    gap: 16px;
    margin: 0 4px 22px;
  }

  .logo-frame {
    width: 78px;
    height: 78px;
    border-radius: 21px;
  }

  .logo-frame img { border-radius: 17px; }
  .app-subtitle { font-size: 12px; }
  .platform-label { font-size: 8px; letter-spacing: 1.4px; }
  .release-panel { padding: 27px 22px; border-radius: 20px; }
  .release-panel::before { left: 22px; }
  .version-row h2 { font-size: 23px; }
  .release-notes { margin: 24px 0; padding: 18px; }
  .download-button { min-height: 66px; }
}

@media (max-width: 380px) {
  .file-size { padding-left: 10px; }
  .release-panel { padding-inline: 18px; }
  .compatibility-note { line-height: 1.5; }
}

@media (prefers-reduced-motion: reduce) {
  .skeleton-line,
  .skeleton-button {
    animation: none;
  }

  .download-button {
    transition: none;
  }
}
</style>
