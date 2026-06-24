<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { api, systemLogStreamURL, type LogLevel, type SystemLog } from '../api'

const levels = ref<LogLevel[]>(['INFO', 'WARNING', 'ERROR'])
const logs = ref<SystemLog[]>([])
const loading = ref(false)
const following = ref(true)
const terminal = ref<HTMLElement>()
let source: EventSource | null = null
let generation = 0

function formatTime(timestamp: number) {
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
    second: '2-digit', fractionalSecondDigits: 3, hour12: false,
  }).format(new Date(timestamp))
}

function formatFields(fields: Record<string, unknown>) {
  return Object.entries(fields)
    .map(([key, value]) => `${key}=${typeof value === 'string' ? value : JSON.stringify(value)}`)
    .join('  ')
}

async function scrollToBottom() {
  if (!following.value) return
  await nextTick()
  if (terminal.value) terminal.value.scrollTop = terminal.value.scrollHeight
}

function append(entry: SystemLog) {
  if (logs.value.some((item) => item.id === entry.id)) return
  logs.value.push(entry)
  if (logs.value.length > 1000) logs.value.splice(0, logs.value.length - 1000)
  void scrollToBottom()
}

function connect(afterId: number, currentGeneration: number) {
  source?.close()
  source = new EventSource(systemLogStreamURL(levels.value, afterId))
  source.onmessage = (event) => {
    if (currentGeneration !== generation) return
    try {
      append(JSON.parse(event.data) as SystemLog)
    } catch {
      // Ignore malformed events; EventSource remains connected.
    }
  }
}

async function reload() {
  const currentGeneration = ++generation
  source?.close()
  source = null
  loading.value = true
  try {
    const result = await api.systemLogs(levels.value)
    if (currentGeneration !== generation) return
    logs.value = result.logs
    await scrollToBottom()
    connect(logs.value.at(-1)?.id ?? 0, currentGeneration)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '系统日志加载失败')
  } finally {
    if (currentGeneration === generation) loading.value = false
  }
}

function handleScroll() {
  if (!terminal.value) return
  const distance = terminal.value.scrollHeight - terminal.value.scrollTop - terminal.value.clientHeight
  following.value = distance < 48
}

watch(levels, reload, { deep: true })
onMounted(reload)
onBeforeUnmount(() => {
  generation++
  source?.close()
})
</script>

<template>
  <section>
    <header class="page-header log-page-header">
      <div>
        <p class="eyebrow">OBSERVABILITY</p>
        <h1>系统日志</h1>
        <p>最近最多保留在视图中的 1000 行，并实时追加服务端新日志。</p>
      </div>
      <div class="log-filters">
        <el-checkbox-group v-model="levels">
          <el-checkbox-button label="INFO" value="INFO" />
          <el-checkbox-button label="WARNING" value="WARNING" />
          <el-checkbox-button label="ERROR" value="ERROR" />
        </el-checkbox-group>
        <el-tag :type="following ? 'success' : 'info'" effect="plain">
          {{ following ? '实时跟随' : '已暂停滚动' }}
        </el-tag>
      </div>
    </header>

    <el-card class="content-card log-card" shadow="never" v-loading="loading">
      <div ref="terminal" class="log-terminal" @scroll="handleScroll">
        <div v-if="logs.length === 0" class="log-empty">当前筛选条件下暂无日志</div>
        <div v-for="entry in logs" :key="entry.id" class="log-line" :class="`log-${entry.level.toLowerCase()}`">
          <time>{{ formatTime(entry.createdAt) }}</time>
          <span class="log-level">{{ entry.level.padEnd(7, ' ') }}</span>
          <span class="log-source">[{{ entry.source }}]</span>
          <span class="log-message">{{ entry.message }}</span>
          <span v-if="Object.keys(entry.fields).length" class="log-fields">{{ formatFields(entry.fields) }}</span>
        </div>
      </div>
    </el-card>
  </section>
</template>
