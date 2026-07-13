<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Refresh, UploadFilled } from '@element-plus/icons-vue'

import { api, type AppRelease } from '../api'

const maxAPKBytes = 256 * 1024 * 1024
const versionPattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/

const items = ref<AppRelease[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const publishing = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const form = ref({
  version: '',
  releaseNotes: '',
})

onMounted(loadItems)

async function loadItems() {
  loading.value = true
  try {
    const result = await api.appReleases()
    items.value = result.items
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '加载 APP 版本失败')
  } finally {
    loading.value = false
  }
}

function openPublish() {
  form.value = { version: '', releaseNotes: '' }
  selectedFile.value = null
  dialogVisible.value = true
}

function chooseAPK() {
  fileInput.value?.click()
}

function selectAPK(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!file.name.toLowerCase().endsWith('.apk')) {
    ElMessage.warning('请选择 .apk 文件')
    return
  }
  if (file.size < 4 || file.size > maxAPKBytes) {
    ElMessage.warning('APK 文件必须小于 256MiB')
    return
  }
  selectedFile.value = file
}

async function publishRelease() {
  const version = form.value.version.trim()
  const releaseNotes = form.value.releaseNotes.trim()
  if (!versionPattern.test(version)) {
    ElMessage.warning('版本号格式应为 major.minor.patch，例如 1.1.0')
    return
  }
  if (!selectedFile.value) {
    ElMessage.warning('请上传 arm64-v8a 的 APK 文件')
    return
  }
  if (!releaseNotes) {
    ElMessage.warning('请填写更新日志')
    return
  }

  publishing.value = true
  try {
    await api.publishAppRelease({
      version,
      releaseNotes,
      file: selectedFile.value,
    })
    ElMessage.success(`BakaVip2 v${version} 发布成功`)
    dialogVisible.value = false
    await loadItems()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '发布 APP 版本失败')
  } finally {
    publishing.value = false
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

function formatDate(value: number) {
  return value ? new Date(value * 1000).toLocaleString() : '-'
}
</script>

<template>
  <section>
    <header class="page-header">
      <div>
        <p class="eyebrow">FRONTEND</p>
        <h1>APP版本管理</h1>
        <p>发布 BakaVip2 Android arm64-v8a 安装包与更新日志。</p>
      </div>
      <div class="page-header-actions">
        <el-button :icon="Refresh" :loading="loading" @click="loadItems">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openPublish">发布新版本</el-button>
      </div>
    </header>

    <el-card class="content-card management-card" shadow="never">
      <el-table v-loading="loading" :data="items" empty-text="暂无已发布版本" class="management-table">
        <el-table-column width="150" label="版本号">
          <template #default="{ row }">
            <el-tag type="success" effect="light">v{{ row.version }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column min-width="360" label="更新日志">
          <template #default="{ row }">
            <p class="release-notes-cell">{{ row.releaseNotes }}</p>
          </template>
        </el-table-column>
        <el-table-column width="140" label="APK 大小">
          <template #default="{ row }">{{ formatBytes(row.apkSize) }}</template>
        </el-table-column>
        <el-table-column width="190" label="发布时间">
          <template #default="{ row }">{{ formatDate(row.publishedAt) }}</template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="dialogVisible"
      title="发布新版本"
      width="680px"
      destroy-on-close
      :close-on-click-modal="!publishing"
      :close-on-press-escape="!publishing"
    >
      <el-form label-position="top">
        <el-form-item label="版本号" required>
          <el-input
            v-model="form.version"
            maxlength="32"
            placeholder="例如 1.1.0"
            :disabled="publishing"
          />
          <p class="form-help">使用 major.minor.patch 格式；相同版本号不可重复发布。</p>
        </el-form-item>

        <el-form-item label="arm64-v8a APK" required>
          <input ref="fileInput" class="native-file-input" type="file" accept=".apk,application/vnd.android.package-archive" @change="selectAPK" />
          <button class="apk-picker" type="button" :disabled="publishing" @click="chooseAPK">
            <el-icon><UploadFilled /></el-icon>
            <span>
              <strong>{{ selectedFile?.name || '选择 APK 文件' }}</strong>
              <small>{{ selectedFile ? formatBytes(selectedFile.size) : '仅支持 .apk，最大 256MiB' }}</small>
            </span>
          </button>
        </el-form-item>

        <el-form-item label="更新日志" required>
          <el-input
            v-model="form.releaseNotes"
            type="textarea"
            :rows="8"
            maxlength="10000"
            show-word-limit
            resize="vertical"
            placeholder="例如：&#10;1. 新增 APP 更新提醒；&#10;2. 优化播放器体验；"
            :disabled="publishing"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button :disabled="publishing" @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="publishing" @click="publishRelease">确认发布</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.release-notes-cell {
  max-width: 760px;
  color: #526078;
  line-height: 1.65;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.form-help {
  margin-top: 6px;
  color: #8b95ad;
  font-size: 12px;
}

.native-file-input {
  display: none;
}

.apk-picker {
  width: 100%;
  min-height: 88px;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 18px 20px;
  color: #526078;
  text-align: left;
  border: 1px dashed #aebbd3;
  border-radius: 10px;
  background: #f8faff;
  transition: border-color 0.2s ease, background 0.2s ease;
}

.apk-picker:hover:not(:disabled) {
  border-color: #409eff;
  background: #f1f7ff;
}

.apk-picker:disabled {
  cursor: wait;
  opacity: 0.68;
}

.apk-picker .el-icon {
  flex: 0 0 auto;
  color: #409eff;
  font-size: 34px;
}

.apk-picker span {
  min-width: 0;
  display: grid;
  gap: 5px;
}

.apk-picker strong {
  overflow: hidden;
  color: #303b52;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.apk-picker small {
  color: #8b95ad;
}
</style>
