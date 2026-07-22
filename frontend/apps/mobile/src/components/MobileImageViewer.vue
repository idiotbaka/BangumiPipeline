<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import {
  discardPreparedImage,
  type ImageViewerSource,
  type PreparedImage,
  prepareImage,
  savePreparedImage,
} from '../native/imageSaver'

const props = defineProps<{ source: ImageViewerSource | null }>()
const emit = defineEmits<{ (event: 'close'): void }>()

const prepared = ref<PreparedImage | null>(null)
const preparing = ref(false)
const saving = ref(false)
const prepareError = ref('')
const saveToast = ref<{ message: string; type: 'success' | 'error' } | null>(null)
const imageLoading = ref(false)
const imageError = ref(false)
let prepareRequestID = 0
let historyPushed = false
let ignoreNextPopState = false
let saveToastTimer: ReturnType<typeof window.setTimeout> | null = null

const sizeLabel = computed(() => formatByteSize(prepared.value?.byteSize ?? 0))
const saveButtonLabel = computed(() => {
  if (saving.value) return '正在保存原图...'
  if (preparing.value) return '正在读取原图...'
  if (prepareError.value) return '重新读取原图'
  return `保存原图 (${sizeLabel.value})`
})

watch(() => props.source, (source) => {
  if (!source) {
    closeViewerState()
    return
  }
  document.body.classList.add('mobile-image-viewer-open')
  imageLoading.value = true
  imageError.value = false
  clearSaveToast()
  if (!historyPushed) {
    window.history.pushState({ bpMobileImageViewer: true }, '', window.location.href)
    historyPushed = true
  }
  void prepareCurrentImage(source)
}, { immediate: true })

onMounted(() => window.addEventListener('popstate', handlePopState))
onBeforeUnmount(() => {
  window.removeEventListener('popstate', handlePopState)
  document.body.classList.remove('mobile-image-viewer-open')
  prepareRequestID++
  clearSaveToast()
  void releasePreparedImage()
})

async function prepareCurrentImage(source = props.source) {
  if (!source) return
  const requestID = ++prepareRequestID
  await releasePreparedImage()
  preparing.value = true
  prepareError.value = ''
  clearSaveToast()
  try {
    const result = await prepareImage(source)
    if (requestID !== prepareRequestID || props.source !== source) {
      await discardPreparedImage(result).catch(() => undefined)
      return
    }
    prepared.value = result
  } catch (error) {
    if (requestID === prepareRequestID) {
      prepareError.value = error instanceof Error ? error.message : '读取原图失败'
    }
  } finally {
    if (requestID === prepareRequestID) preparing.value = false
  }
}

async function saveImage() {
  if (preparing.value || saving.value) return
  if (!prepared.value) {
    await prepareCurrentImage()
    return
  }
  saving.value = true
  clearSaveToast()
  try {
    const result = await savePreparedImage(prepared.value)
    showSaveToast(`已保存至 ${result.path}`, 'success')
  } catch (error) {
    showSaveToast(error instanceof Error ? error.message : '保存原图失败', 'error')
  } finally {
    saving.value = false
  }
}

function showSaveToast(message: string, type: 'success' | 'error') {
  clearSaveToast()
  saveToast.value = { message, type }
  saveToastTimer = window.setTimeout(() => {
    saveToast.value = null
    saveToastTimer = null
  }, 2_000)
}

function clearSaveToast() {
  if (saveToastTimer !== null) {
    window.clearTimeout(saveToastTimer)
    saveToastTimer = null
  }
  saveToast.value = null
}

function requestClose() {
  if (!props.source) return
  emit('close')
  if (historyPushed) {
    ignoreNextPopState = true
    historyPushed = false
    window.history.back()
  }
}

function handlePopState() {
  if (ignoreNextPopState) {
    ignoreNextPopState = false
    return
  }
  if (!props.source) return
  historyPushed = false
  emit('close')
}

function closeViewerState() {
  document.body.classList.remove('mobile-image-viewer-open')
  prepareRequestID++
  preparing.value = false
  saving.value = false
  prepareError.value = ''
  clearSaveToast()
  imageLoading.value = false
  imageError.value = false
  void releasePreparedImage()
}

async function releasePreparedImage() {
  const current = prepared.value
  prepared.value = null
  if (current) await discardPreparedImage(current).catch(() => undefined)
}

function formatByteSize(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 KB'
  if (bytes >= 1024 * 1024) {
    const value = bytes / (1024 * 1024)
    return `${value >= 10 ? value.toFixed(1) : value.toFixed(2)} MB`
  }
  return `${Math.max(1, Math.round(bytes / 1024))} KB`
}
</script>

<template>
  <Teleport to="body">
    <Transition name="image-viewer">
      <section v-if="source" class="image-viewer" role="dialog" aria-modal="true" :aria-label="source.alt || '图片查看器'">
        <button class="image-viewer-close" type="button" aria-label="关闭图片查看器" @click="requestClose">×</button>

        <Transition name="image-save-toast">
          <div
            v-if="saveToast"
            class="image-save-toast"
            :class="saveToast.type"
            :role="saveToast.type === 'error' ? 'alert' : 'status'"
            aria-live="polite"
          >
            <i aria-hidden="true">{{ saveToast.type === 'success' ? '✓' : '!' }}</i>
            <span>{{ saveToast.message }}</span>
          </div>
        </Transition>

        <div class="image-viewer-stage" @click.self="requestClose">
          <span v-if="imageLoading" class="image-viewer-spinner" aria-label="正在加载图片" />
          <img
            v-show="!imageError"
            :src="source.src"
            :alt="source.alt"
            decoding="async"
            referrerpolicy="no-referrer"
            @load="imageLoading = false"
            @error="imageLoading = false; imageError = true"
          />
          <p v-if="imageError" class="image-viewer-error">图片显示失败</p>
        </div>

        <footer class="image-viewer-actions">
          <p v-if="prepareError" class="error">{{ prepareError }}</p>
          <button type="button" :disabled="preparing || saving" @click="saveImage">{{ saveButtonLabel }}</button>
        </footer>
      </section>
    </Transition>
  </Teleport>
</template>

<style scoped>
:global(body.mobile-image-viewer-open) {
  overflow: hidden;
  background: #000000;
}

.image-viewer {
  position: fixed;
  inset: 0;
  z-index: 1800;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  color: #ffffff;
  background: #000000;
}

.image-viewer-close {
  position: absolute;
  top: calc(12px + env(safe-area-inset-top));
  right: 14px;
  z-index: 2;
  width: 40px;
  height: 40px;
  display: grid;
  place-items: center;
  padding: 0 0 3px;
  color: #ffffff;
  font-size: 30px;
  line-height: 1;
  background: rgba(24, 24, 24, 0.76);
  border: 1px solid rgba(255, 255, 255, 0.2);
  border-radius: 50%;
  backdrop-filter: blur(8px);
}

.image-save-toast {
  position: fixed;
  top: calc(18px + env(safe-area-inset-top));
  left: 50%;
  z-index: 4;
  width: max-content;
  max-width: min(calc(100vw - 112px), 420px);
  min-height: 42px;
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 9px 13px 9px 10px;
  color: #ffffff;
  font-size: 12px;
  line-height: 1.45;
  background: rgba(43, 23, 34, 0.96);
  border: 1px solid rgba(255, 95, 158, 0.42);
  border-radius: 12px;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.32);
  pointer-events: none;
  transform: translateX(-50%);
}

.image-save-toast.error {
  background: rgba(42, 23, 28, 0.96);
  border-color: rgba(253, 164, 175, 0.38);
}

.image-save-toast i {
  width: 24px;
  height: 24px;
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  color: #ffffff;
  font-size: 14px;
  font-style: normal;
  font-weight: 700;
  background: var(--pink-500);
  border-radius: 50%;
}

.image-save-toast.error i {
  color: #4c0519;
  background: #fda4af;
}

.image-save-toast span {
  min-width: 0;
  overflow-wrap: anywhere;
}

.image-save-toast-enter-active,
.image-save-toast-leave-active {
  transition: opacity 160ms ease, transform 180ms cubic-bezier(0.22, 1, 0.36, 1);
}

.image-save-toast-enter-from,
.image-save-toast-leave-to {
  opacity: 0;
  transform: translate(-50%, -8px);
}

.image-viewer-stage {
  grid-row: 1 / 3;
  min-width: 0;
  min-height: 0;
  display: grid;
  place-items: center;
  padding: calc(64px + env(safe-area-inset-top)) 10px 16px;
  overflow: hidden;
  touch-action: pinch-zoom;
}

.image-viewer-stage img {
  display: block;
  width: auto;
  height: auto;
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  user-select: none;
  -webkit-user-drag: none;
}

.image-viewer-spinner {
  position: absolute;
  width: 38px;
  height: 38px;
  border: 2px solid rgba(255, 255, 255, 0.2);
  border-top-color: #ffffff;
  border-radius: 50%;
  animation: image-viewer-spin 0.8s linear infinite;
}

@keyframes image-viewer-spin {
  to { transform: rotate(360deg); }
}

.image-viewer-error {
  color: rgba(255, 255, 255, 0.64);
  font-size: 13px;
}

.image-viewer-actions {
  position: relative;
  z-index: 2;
  grid-row: 3;
  display: grid;
  gap: 9px;
  padding: 12px 18px max(18px, env(safe-area-inset-bottom));
  background: linear-gradient(transparent, rgba(0, 0, 0, 0.92) 28%);
}

.image-viewer-actions p {
  overflow-wrap: anywhere;
  color: #86efac;
  font-size: 11px;
  text-align: center;
}

.image-viewer-actions p.error {
  color: #fda4af;
}

.image-viewer-actions button {
  width: min(100%, 420px);
  min-height: 46px;
  justify-self: center;
  color: #ffffff;
  font-size: 14px;
  font-weight: 600;
  background: rgba(255, 255, 255, 0.14);
  border: 1px solid rgba(255, 255, 255, 0.34);
  border-radius: 999px;
  backdrop-filter: blur(12px);
}

.image-viewer-actions button:active:not(:disabled) {
  background: rgba(238, 63, 134, 0.78);
  transform: scale(0.98);
}

.image-viewer-actions button:disabled {
  opacity: 0.56;
}

.image-viewer-enter-active,
.image-viewer-leave-active {
  transition: opacity 180ms ease;
}

.image-viewer-enter-active .image-viewer-stage img,
.image-viewer-leave-active .image-viewer-stage img {
  transition: transform 220ms cubic-bezier(0.22, 1, 0.36, 1);
}

.image-viewer-enter-from,
.image-viewer-leave-to {
  opacity: 0;
}

.image-viewer-enter-from .image-viewer-stage img,
.image-viewer-leave-to .image-viewer-stage img {
  transform: scale(0.94);
}
</style>
