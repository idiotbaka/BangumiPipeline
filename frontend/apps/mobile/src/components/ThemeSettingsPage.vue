<script setup lang="ts">
import type { MobileTheme } from '../theme'

defineProps<{ theme: MobileTheme; saving: boolean; message: string }>()
const emit = defineEmits<{ (event: 'change', value: MobileTheme): void }>()
const options: Array<{ value: MobileTheme; label: string; note: string }> = [
  { value: 'light', label: '浅色模式', note: '明亮清透 · 默认' },
  { value: 'dark', label: '深色模式', note: '柔和暗色 · 专注观看' },
]
</script>

<template>
  <div class="theme-page">
    <section class="theme-settings-card">
      <p class="theme-kicker">APPEARANCE</p>
      <fieldset :disabled="saving">
        <legend>选择界面外观</legend>
        <p class="theme-description">让每一次打开，都合你的心意。</p>
        <div class="theme-options">
          <label v-for="option in options" :key="option.value" class="theme-option" :class="{ selected: theme === option.value }">
            <span class="theme-preview" :class="option.value" aria-hidden="true">
              <span class="preview-top"><i /><b /></span>
              <span class="preview-banner" />
              <span class="preview-title" />
              <span class="preview-posters"><i /><i /><i /></span>
              <span class="preview-nav"><i /><i /><i /><i /></span>
            </span>
            <span class="theme-option-label">
              <span>{{ option.label }}</span>
              <input
                type="radio"
                name="mobile-theme"
                :value="option.value"
                :checked="theme === option.value"
                :aria-label="option.label"
                @change="emit('change', option.value)"
              />
            </span>
            <small>{{ option.note }}</small>
          </label>
        </div>
      </fieldset>
    </section>
    <p class="theme-note">立即应用于所有页面和开屏画面，选择会保存在此设备。</p>
    <p v-if="message" class="theme-message" role="status">{{ message }}</p>
  </div>
</template>

<style scoped>
.theme-page { padding: 14px; display: grid; gap: 14px; }
.theme-settings-card { padding: 22px 16px; border: 1px solid var(--line-soft); border-radius: 20px; background: var(--glass-strong); }
.theme-kicker { margin-bottom: 8px; color: var(--pink-600); font-size: 10px; letter-spacing: 2px; }
fieldset { min-width: 0; margin: 0; padding: 0; border: 0; }
legend { padding: 0; color: var(--ink-900); font-size: 19px; }
.theme-description { margin-top: 6px; color: var(--ink-400); font-size: 12px; }
.theme-options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-top: 24px; }
.theme-option { min-width: 0; padding: 7px 7px 12px; border: 2px solid transparent; border-radius: 16px; cursor: pointer; }
.theme-option.selected { border-color: var(--pink-500); background: var(--pink-50); }
.theme-option:focus-within { outline: 2px solid var(--pink-300); outline-offset: 3px; }
fieldset:disabled .theme-option { cursor: wait; }
.theme-option-label { display: flex; justify-content: space-between; align-items: center; gap: 6px; margin: 13px 2px 5px; font-size: 14px; }
.theme-option input { width: 18px; height: 18px; flex: 0 0 auto; margin: 0; accent-color: #cb437c; }
.theme-option small { display: block; margin: 0 2px; color: var(--ink-400); font-size: 10px; }
.theme-preview { display: grid; gap: 10px; padding: 12px 9px 9px; border: 1px solid rgba(130,145,174,.18); border-radius: 11px; background: #f5f6fa; }
.theme-preview.dark { background: #141923; }
.preview-top { display: flex; align-items: center; justify-content: space-between; height: 8px; }
.preview-top i { width: 23px; height: 5px; border-radius: 3px; background: #e375a3; }
.preview-top b { width: 34px; height: 7px; border-radius: 5px; background: #e1e6ef; }
.dark .preview-top b { background: #30394b; }
.preview-banner { display: block; height: 49px; border-radius: 7px; background: linear-gradient(130deg, #f5cbdc, #d8e8f5); }
.dark .preview-banner { background: linear-gradient(130deg, #583550, #2c4b60); }
.preview-title { width: 40%; height: 4px; border-radius: 3px; background: #9ba6ba; }
.preview-posters { display: grid; grid-template-columns: repeat(3, 1fr); gap: 5px; }
.preview-posters i { height: 46px; border-radius: 4px; background: #e4e8f0; }
.preview-posters i:nth-child(2) { background: #f0e0e9; }
.dark .preview-posters i { background: #263344; }
.dark .preview-posters i:nth-child(2) { background: #3d2b3e; }
.preview-nav { display: flex; justify-content: space-around; padding: 7px 0; border-radius: 6px; background: #fff; }
.dark .preview-nav { background: #222a39; }
.preview-nav i { width: 6px; height: 6px; border-radius: 2px; background: #9ba6ba; }
.preview-nav i:first-child { background: #e375a3; }
.theme-note, .theme-message { padding: 0 6px; color: var(--ink-400); font-size: 12px; line-height: 1.8; }
.theme-message { color: var(--pink-600); }
@media (min-width: 760px) { .theme-page { width: min(100%, 680px); margin: 0 auto; padding: 24px; } }
</style>
