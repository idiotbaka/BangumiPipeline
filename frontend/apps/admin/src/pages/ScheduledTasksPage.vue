<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Calendar, Setting, VideoPlay } from '@element-plus/icons-vue'
import { api, type ScheduledTask } from '../api'

const bangumiMetadataTaskKey = 'bangumi-season-metadata'

const tasks = ref<ScheduledTask[]>([])
const loading = ref(true)
const updatingKey = ref('')
const runningKey = ref('')
const intervalDrafts = reactive<Record<string, number>>({})
const customSearchVisible = ref(false)
const customSearchLoading = ref(false)
const customSearchSaving = ref(false)
const customSearchTags = ref<string[]>([])
let pollingTimer: ReturnType<typeof setInterval> | undefined

async function loadTasks(showLoading = true) {
  if (showLoading) loading.value = true
  try {
    const response = await api.scheduledTasks()
    tasks.value = response.tasks
    for (const task of response.tasks) {
      if (updatingKey.value !== task.key) intervalDrafts[task.key] = task.intervalMinutes
    }
  } catch (error) {
    if (showLoading) ElMessage.error(error instanceof Error ? error.message : '加载计划任务失败')
  } finally {
    if (showLoading) loading.value = false
  }
}

async function toggleTask(task: ScheduledTask, enabled: boolean) {
  const previous = !enabled
  updatingKey.value = task.key
  try {
    const { task: updated } = await api.updateScheduledTask(task.key, { enabled })
    Object.assign(task, updated)
    ElMessage.success(enabled ? '计划任务已启用' : '计划任务已禁用')
  } catch (error) {
    task.enabled = previous
    ElMessage.error(error instanceof Error ? error.message : '更新任务失败')
  } finally {
    updatingKey.value = ''
  }
}

async function saveInterval(task: ScheduledTask) {
  const intervalMinutes = intervalDrafts[task.key]
  if (!Number.isInteger(intervalMinutes) || intervalMinutes < 1 || intervalMinutes > 43200) {
    ElMessage.warning('执行间隔必须是 1 到 43200 之间的整数分钟')
    return
  }
  updatingKey.value = task.key
  try {
    const { task: updated } = await api.updateScheduledTask(task.key, { intervalMinutes })
    Object.assign(task, updated)
    ElMessage.success('执行间隔已保存')
  } catch (error) {
    intervalDrafts[task.key] = task.intervalMinutes
    ElMessage.error(error instanceof Error ? error.message : '保存执行间隔失败')
  } finally {
    updatingKey.value = ''
  }
}

async function runNow(task: ScheduledTask) {
  runningKey.value = task.key
  try {
    const { task: running } = await api.runScheduledTask(task.key)
    Object.assign(task, running)
    ElMessage.success('计划任务已开始执行')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '启动计划任务失败')
  } finally {
    runningKey.value = ''
    window.setTimeout(() => loadTasks(false), 500)
  }
}

async function openCustomSearchSettings() {
  customSearchVisible.value = true
  customSearchLoading.value = true
  try {
    const { settings } = await api.bangumiCustomSearchSettings()
    customSearchTags.value = [...settings.tags]
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '加载自定义抓取设置失败')
  } finally {
    customSearchLoading.value = false
  }
}

async function saveCustomSearchSettings() {
  customSearchSaving.value = true
  try {
    const { settings } = await api.updateBangumiCustomSearchSettings(customSearchTags.value)
    customSearchTags.value = [...settings.tags]
    ElMessage.success('自定义抓取设置已保存')
    customSearchVisible.value = false
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存自定义抓取设置失败')
  } finally {
    customSearchSaving.value = false
  }
}

function statusLabel(task: ScheduledTask) {
  if (task.lastStatus === 'running') return '正在执行中'
  if (task.lastStatus === 'completed') return '已完成'
  if (task.lastStatus === 'failed') return '执行失败'
  return '尚未执行'
}

function statusType(status: ScheduledTask['lastStatus']) {
  if (status === 'completed') return 'success'
  if (status === 'failed') return 'danger'
  if (status === 'running') return 'warning'
  return 'info'
}

function formatTime(timestamp: number | null) {
  return timestamp ? new Date(timestamp * 1000).toLocaleString() : '—'
}

onMounted(async () => {
  await loadTasks()
  pollingTimer = window.setInterval(() => loadTasks(false), 3000)
})
onBeforeUnmount(() => {
  if (pollingTimer) window.clearInterval(pollingTimer)
})
</script>

<template>
  <header class="page-header">
    <div>
      <p class="eyebrow">AUTOMATION</p>
      <h1>计划任务</h1>
      <p>配置固定执行间隔，查看运行结果，或手动立即执行。</p>
    </div>
    <el-button :loading="loading" @click="loadTasks()">刷新</el-button>
  </header>

  <el-alert
    title="任务执行状态会实时更新"
    description="启用后，任务会按照设置的分钟间隔自动执行；禁用不会中断已经开始的任务。手动执行不受启用状态限制。"
    type="info"
    :closable="false"
    show-icon
  />

  <el-card class="content-card task-list-card" shadow="never" v-loading="loading">
    <el-empty v-if="!loading && tasks.length === 0" description="暂无计划任务" />
    <article v-for="task in tasks" :key="task.key" class="task-row task-row-detailed">
      <div class="task-icon"><el-icon><Calendar /></el-icon></div>
      <div class="task-content">
        <div class="task-title-line">
          <h2>{{ task.name }}</h2>
          <el-tag :type="statusType(task.lastStatus)" effect="plain">{{ statusLabel(task) }}</el-tag>
        </div>
        <p>{{ task.description }}</p>
        <div class="task-runtime-grid">
          <div><span>上次开始</span><strong>{{ formatTime(task.lastStartedAt) }}</strong></div>
          <div><span>上次完成</span><strong>{{ formatTime(task.lastFinishedAt) }}</strong></div>
          <div><span>下次执行</span><strong>{{ task.enabled ? formatTime(task.nextRunAt) : '任务已禁用' }}</strong></div>
        </div>
        <el-alert
          v-if="task.lastStatus === 'failed' && task.lastError"
          class="task-error"
          :title="`执行失败：${task.lastError}`"
          type="error"
          :closable="false"
          show-icon
        />
      </div>
      <div class="task-controls">
        <div class="interval-editor">
          <span>每隔</span>
          <el-input-number
            v-model="intervalDrafts[task.key]"
            :min="1"
            :max="43200"
            :step="1"
            controls-position="right"
            :disabled="updatingKey !== ''"
          />
          <span>分钟</span>
          <el-button
            :disabled="intervalDrafts[task.key] === task.intervalMinutes"
            :loading="updatingKey === task.key"
            @click="saveInterval(task)"
          >保存</el-button>
        </div>
        <div class="task-actions">
          <div class="task-toggle-inline">
            <span>{{ task.enabled ? '已启用' : '已禁用' }}</span>
            <el-switch
              v-model="task.enabled"
              :loading="updatingKey === task.key"
              :disabled="updatingKey !== ''"
              @change="toggleTask(task, Boolean($event))"
            />
          </div>
          <el-button
            v-if="task.key === bangumiMetadataTaskKey"
            :icon="Setting"
            :disabled="customSearchLoading || customSearchSaving"
            @click="openCustomSearchSettings"
          >自定义抓取设置</el-button>
          <el-button
            type="primary"
            :icon="VideoPlay"
            :loading="runningKey === task.key || task.lastStatus === 'running'"
            :disabled="task.lastStatus === 'running'"
            @click="runNow(task)"
          >立即执行</el-button>
        </div>
      </div>
    </article>
  </el-card>

  <el-dialog
    v-model="customSearchVisible"
    title="自定义抓取设置"
    width="min(560px, calc(100vw - 32px))"
    destroy-on-close
    append-to-body
  >
    <el-form class="bangumi-custom-search-dialog" label-position="top" @submit.prevent>
      <el-form-item label="抓取标签名称">
        <el-select
          v-model="customSearchTags"
          multiple
          filterable
          allow-create
          default-first-option
          :reserve-keyword="false"
          placeholder="输入后回车，例如 2026年7月"
          :loading="customSearchLoading"
        >
          <el-option v-for="tag in customSearchTags" :key="tag" :label="tag" :value="tag" />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="customSearchVisible = false">取消</el-button>
      <el-button type="primary" :loading="customSearchSaving" :disabled="customSearchLoading" @click="saveCustomSearchSettings">保存</el-button>
    </template>
  </el-dialog>
</template>
