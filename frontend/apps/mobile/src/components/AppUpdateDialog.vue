<script setup lang="ts">
import type { AppRelease } from '../api'

interface Props {
  release: AppRelease
  currentVersion: string
  openingDownload: boolean
  errorMessage: string
}

defineProps<Props>()
defineEmits<{
  (event: 'download'): void
  (event: 'ignore'): void
}>()
</script>

<template>
  <Teleport to="body">
    <div class="update-dialog-layer" role="presentation">
      <section
        class="update-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="app-update-title"
        aria-describedby="app-update-notes"
      >
        <div class="update-dialog-accent" aria-hidden="true" />
        <header>
          <span class="update-badge">NEW RELEASE</span>
          <h2 id="app-update-title">发现新版本</h2>
          <p>可从 v{{ currentVersion }} 更新至 <strong>v{{ release.version }}</strong></p>
        </header>

        <div class="version-track" aria-hidden="true">
          <span>v{{ currentVersion }}</span>
          <i><b /></i>
          <span class="latest">v{{ release.version }}</span>
        </div>

        <section id="app-update-notes" class="update-notes">
          <h3><span /> 更新日志</h3>
          <p>{{ release.releaseNotes }}</p>
        </section>

        <p v-if="errorMessage" class="update-error" role="alert">{{ errorMessage }}</p>

        <div class="update-actions">
          <button class="ignore-button" type="button" :disabled="openingDownload" @click="$emit('ignore')">
            忽略该版本
          </button>
          <button class="download-button" type="button" :disabled="openingDownload" @click="$emit('download')">
            <span>{{ openingDownload ? '正在打开' : '前往下载' }}</span>
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M12 3v11m0 0 4-4m-4 4-4-4M5 18v2h14v-2" />
            </svg>
          </button>
        </div>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.update-dialog-layer {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: grid;
  place-items: center;
  padding: 22px;
  background: rgba(21, 27, 43, 0.48);
  backdrop-filter: blur(7px);
  animation: update-layer-in 180ms ease-out both;
}

.update-dialog {
  position: relative;
  width: min(100%, 390px);
  max-height: min(680px, calc(100vh - 44px));
  overflow: auto;
  padding: 28px 24px 22px;
  color: #20283e;
  border: 1px solid rgba(255, 255, 255, 0.92);
  border-radius: 20px;
  background:
    radial-gradient(circle at 100% 0, rgba(142, 232, 242, 0.25), transparent 35%),
    linear-gradient(145deg, #fff, #fff8fb);
  box-shadow: 0 28px 72px rgba(21, 27, 43, 0.28);
  animation: update-dialog-in 220ms cubic-bezier(0.16, 1, 0.3, 1) both;
}

.update-dialog-accent {
  position: absolute;
  width: 118px;
  height: 5px;
  top: 0;
  left: 24px;
  border-radius: 0 0 5px 5px;
  background: linear-gradient(90deg, #e55282, #49c8da, transparent);
}

.update-badge {
  display: inline-flex;
  padding: 4px 9px;
  color: #d64274;
  font-size: 9px;
  font-weight: 800;
  letter-spacing: 0.14em;
  border: 1px solid rgba(229, 82, 130, 0.16);
  border-radius: 999px;
  background: rgba(255, 239, 245, 0.9);
}

.update-dialog h2 {
  margin-top: 12px;
  font-size: 25px;
  line-height: 1.2;
}

.update-dialog header p {
  margin-top: 7px;
  color: #8b95ad;
  font-size: 13px;
}

.update-dialog header strong {
  color: #d64274;
}

.version-track {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 10px;
  margin-top: 22px;
  color: #9ba4b7;
  font-size: 11px;
  font-weight: 600;
}

.version-track i {
  position: relative;
  height: 2px;
  overflow: visible;
  background: rgba(229, 82, 130, 0.14);
}

.version-track i::before,
.version-track i::after {
  position: absolute;
  width: 6px;
  height: 6px;
  top: 50%;
  content: '';
  border-radius: 50%;
  transform: translateY(-50%);
}

.version-track i::before {
  left: 0;
  background: #cbd1dd;
}

.version-track i::after {
  right: 0;
  background: #e55282;
  box-shadow: 0 0 0 4px rgba(229, 82, 130, 0.1);
}

.version-track i b {
  display: block;
  width: 100%;
  height: 100%;
  background: linear-gradient(90deg, #cbd1dd, #e55282);
}

.version-track .latest {
  color: #d64274;
}

.update-notes {
  margin-top: 22px;
  padding: 18px;
  border: 1px solid rgba(72, 92, 137, 0.08);
  border-radius: 13px;
  background: linear-gradient(135deg, rgba(255, 239, 245, 0.7), rgba(238, 252, 255, 0.72));
}

.update-notes h3 {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.update-notes h3 span {
  width: 5px;
  height: 15px;
  border-radius: 3px;
  background: #e55282;
}

.update-notes p {
  margin-top: 10px;
  max-height: 220px;
  overflow: auto;
  color: #68748e;
  font-size: 13px;
  line-height: 1.75;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.update-error {
  margin-top: 14px;
  padding: 9px 11px;
  color: #c13d5c;
  font-size: 12px;
  line-height: 1.5;
  border-radius: 8px;
  background: #fff0f3;
}

.update-actions {
  display: grid;
  grid-template-columns: 1fr 1.35fr;
  gap: 10px;
  margin-top: 22px;
}

.update-actions button {
  min-height: 48px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 600;
}

.update-actions button:disabled {
  cursor: wait;
  opacity: 0.65;
}

.ignore-button {
  color: #69758f;
  border: 1px solid rgba(72, 92, 137, 0.12);
  background: #f5f7fa;
}

.download-button {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: #fff;
  background: linear-gradient(120deg, #e55282, #d64274);
  box-shadow: 0 10px 22px rgba(229, 82, 130, 0.24);
}

.download-button svg {
  width: 18px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

@keyframes update-layer-in {
  from { opacity: 0; }
  to { opacity: 1; }
}

@keyframes update-dialog-in {
  from { opacity: 0; transform: translateY(18px) scale(0.97); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}

@media (max-width: 360px) {
  .update-dialog-layer { padding: 15px; }
  .update-dialog { padding-inline: 19px; }
  .update-actions { grid-template-columns: 1fr; }
}

@media (prefers-reduced-motion: reduce) {
  .update-dialog-layer,
  .update-dialog {
    animation: none;
  }
}
</style>
