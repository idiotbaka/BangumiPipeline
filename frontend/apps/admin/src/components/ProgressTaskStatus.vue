<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { api, type ScheduledTask } from '../api'
import { formatProgressTime } from '../progress'

const props = defineProps<{ taskKey: string; revision: number }>()
const task = ref<ScheduledTask | null>(null)
const loading = ref(false)
const error = ref('')
let generation = 0

const labels: Record<string, string> = { idle: '尚未运行', running: '运行中', completed: '上次运行完成', failed: '上次运行失败' }

async function load() {
  const current = ++generation
  loading.value = true
  error.value = ''
  task.value = null
  try {
    const result = await api.scheduledTasks()
    if (current !== generation) return
    task.value = result.tasks.find((item) => item.key === props.taskKey) ?? null
    if (!task.value) error.value = '未找到对应计划任务'
  } catch (cause) {
    if (current === generation) error.value = cause instanceof Error ? cause.message : '加载计划任务状态失败'
  } finally {
    if (current === generation) loading.value = false
  }
}

watch(() => [props.taskKey, props.revision], load, { immediate: true })
onBeforeUnmount(() => { generation++ })
</script>

<template>
  <el-card class="content-card progress-task-card" shadow="never" v-loading="loading">
    <div v-if="error" class="progress-task-line">
      <span class="progress-error-text">{{ error }}</span>
      <el-button link type="primary" @click="load">重试</el-button>
    </div>
    <template v-else-if="task">
      <div class="progress-task-line">
        <strong>{{ task.name }}</strong>
        <el-tag :type="task.enabled ? 'success' : 'info'" effect="plain">{{ task.enabled ? '已启用' : '未启用' }}</el-tag>
        <el-tag :type="task.lastStatus === 'failed' ? 'danger' : task.lastStatus === 'running' ? 'warning' : 'info'" effect="plain">
          {{ labels[task.lastStatus] }}
        </el-tag>
        <router-link to="/scheduled-tasks">查看计划任务</router-link>
      </div>
      <p class="progress-hint">上次开始：{{ formatProgressTime(task.lastStartedAt) }} · 上次结束：{{ formatProgressTime(task.lastFinishedAt) }} · 下次运行：{{ formatProgressTime(task.nextRunAt) }}</p>
      <el-alert v-if="task.lastError" :title="task.lastError" type="error" :closable="false" show-icon />
    </template>
  </el-card>
</template>
