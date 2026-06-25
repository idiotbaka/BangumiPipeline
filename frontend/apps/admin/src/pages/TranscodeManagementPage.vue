<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { api, type MediaJob, type MediaJobStatus } from '../api'

const items = ref<MediaJob[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 50
const loading = ref(true)
const status = ref<MediaJobStatus>('pending')
const retryingId = ref<number | null>(null)
let pollingTimer: ReturnType<typeof setInterval> | undefined

const filters: Array<{ label: string; value: MediaJobStatus }> = [
  { label: '待处理', value: 'pending' },
  { label: '转码中', value: 'transcoding' },
  { label: '已完成', value: 'completed' },
  { label: '处理失败', value: 'failed' },
]

async function load(showLoading = true) {
  if (showLoading) loading.value = true
  try {
    const result = await api.mediaJobs(page.value, pageSize, status.value)
    items.value = result.items
    total.value = result.total
  } catch (error) {
    if (showLoading) ElMessage.error(error instanceof Error ? error.message : '加载转码任务失败')
  } finally {
    if (showLoading) loading.value = false
  }
}

function refreshPolling() {
  if (pollingTimer) {
    window.clearInterval(pollingTimer)
    pollingTimer = undefined
  }
  if (status.value === 'transcoding') {
    pollingTimer = window.setInterval(() => load(false), 3000)
  }
}

function changeStatus(value: string | number | boolean) {
  status.value = value as MediaJobStatus
  page.value = 1
  refreshPolling()
  void load()
}

function changePage(value: number) {
  page.value = value
  void load()
}

function statusType(value: MediaJobStatus) {
  if (value === 'completed') return 'success'
  if (value === 'failed') return 'danger'
  if (value === 'transcoding') return 'warning'
  return 'info'
}

function statusLabel(value: MediaJobStatus) {
  if (value === 'completed') return '已完成'
  if (value === 'failed') return '处理失败'
  if (value === 'transcoding') return '转码中'
  return '待处理'
}

function actionLabel(value: string) {
  if (value === 'copy' || value === 'move') return '直接复制'
  if (value === 'remux') return '封装转换'
  if (value === 'transcode') return '转码'
  if (value === 'burn_subtitles') return '字幕压制'
  return value || '待判断'
}

function episodeTypeLabel(type: string) {
  const labels: Record<string, string> = {
    episode: '正片',
    ova: 'OVA',
    oad: 'OAD',
    sp: 'SP',
    special: 'SP',
  }
  return labels[type] || type || '未知'
}

function formatEpisode(job: MediaJob) {
  const season = `S${String(job.seasonNumber).padStart(2, '0')}`
  if (job.episodeType && job.episodeType !== 'episode') {
    return `${season} ${episodeTypeLabel(job.episodeType)} ${job.episodeNumber}`
  }
  return `${season} E${job.episodeNumber}`
}

function formatTime(timestamp: number | null) {
  return timestamp ? new Date(timestamp * 1000).toLocaleString() : '—'
}

function formatDurationMs(value: number) {
  if (!value || value < 0) return '—'
  const totalSeconds = Math.floor(value / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const minuteText = String(minutes).padStart(hours > 0 ? 2 : 1, '0')
  const secondText = String(seconds).padStart(2, '0')
  if (hours > 0) return `${hours}:${minuteText}:${secondText}`
  return `${minuteText}:${secondText}`
}

function progressPercent(job: MediaJob) {
  if (!Number.isFinite(job.progress)) return 0
  return Math.max(0, Math.min(100, Math.round(job.progress * 100)))
}

function progressLabel(job: MediaJob) {
  return `已处理 ${formatDurationMs(job.processedDurationMs)} / 总时长 ${formatDurationMs(job.totalDurationMs)}`
}

function subtitleLabel(job: MediaJob) {
  const values: string[] = []
  if (job.hasExternalSubtitles) values.push('外挂')
  if (job.hasInternalSubtitles) values.push('内封')
  return values.length > 0 ? values.join(' + ') : '无'
}

async function retryJob(job: MediaJob) {
  retryingId.value = job.id
  try {
    await api.retryMediaJob(job.id)
    ElMessage.success('已重置为待处理，等待计划任务重新处理')
    await load(false)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '重试转码任务失败')
  } finally {
    retryingId.value = null
  }
}

onMounted(async () => {
  await load()
  refreshPolling()
})

onBeforeUnmount(() => {
  if (pollingTimer) window.clearInterval(pollingTimer)
})
</script>

<template>
  <header class="page-header">
    <div>
      <p class="eyebrow">TRANSCODE</p>
      <h1>转码管理</h1>
      <p>查看下载完成视频的判断、转码、字幕压制和最终产物状态。</p>
    </div>
    <el-button :icon="Refresh" :loading="loading" @click="load()">刷新</el-button>
  </header>

  <div class="subscription-toolbar">
    <el-radio-group :model-value="status" @change="changeStatus">
      <el-radio-button v-for="filter in filters" :key="filter.value" :label="filter.value">{{ filter.label }}</el-radio-button>
    </el-radio-group>
  </div>

  <el-card class="content-card media-card" shadow="never" v-loading="loading">
    <el-empty v-if="!loading && items.length === 0" description="当前状态下暂无转码任务" />
    <el-table v-else :data="items" class="media-table">
      <el-table-column label="番剧话数" min-width="250" show-overflow-tooltip>
        <template #default="{ row }">
          <div class="download-title-cell">
            <strong>{{ row.animeName }} {{ formatEpisode(row) }}</strong>
            <span>{{ row.title }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="96">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)" effect="plain">{{ statusLabel(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column v-if="status === 'transcoding'" label="进度" min-width="230">
        <template #default="{ row }">
          <div class="transcode-progress-cell">
            <div class="transcode-progress-head">
              <strong>{{ progressPercent(row) }}%</strong>
              <span>{{ progressLabel(row) }}</span>
            </div>
            <el-progress :percentage="progressPercent(row)" :stroke-width="8" :show-text="false" />
          </div>
        </template>
      </el-table-column>
      <el-table-column label="处理方式" width="110">
        <template #default="{ row }">{{ actionLabel(row.action) }}</template>
      </el-table-column>
      <el-table-column label="编码" width="130">
        <template #default="{ row }">
          <span>{{ row.videoCodec || '—' }} / {{ row.audioCodec || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="字幕" width="90">
        <template #default="{ row }">{{ subtitleLabel(row) }}</template>
      </el-table-column>
      <el-table-column label="源文件" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          <span v-if="row.sourceFile">{{ row.sourceFile }}</span>
          <span v-else class="muted-text">等待判断</span>
        </template>
      </el-table-column>
      <el-table-column label="产物文件" min-width="190" show-overflow-tooltip>
        <template #default="{ row }">
          <span v-if="row.outputFile">{{ row.outputFile }}</span>
          <span v-else class="muted-text">—</span>
        </template>
      </el-table-column>
      <el-table-column label="时间" width="180">
        <template #default="{ row }">
          <div class="download-time-cell">
            <span>开始 {{ formatTime(row.startedAt) }}</span>
            <span v-if="row.completedAt">完成 {{ formatTime(row.completedAt) }}</span>
            <span v-else-if="row.failedAt">失败 {{ formatTime(row.failedAt) }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="错误" min-width="220" show-overflow-tooltip>
        <template #default="{ row }">
          <span v-if="row.errorMessage">{{ row.errorMessage }}</span>
          <span v-else class="muted-text">—</span>
        </template>
      </el-table-column>
      <el-table-column v-if="status === 'failed'" label="操作" width="104" fixed="right">
        <template #default="{ row }">
          <el-button size="small" type="primary" plain :icon="Refresh" :loading="retryingId === row.id" @click="retryJob(row)">重试</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-card>

  <div v-if="total > pageSize" class="anime-pagination">
    <el-pagination background layout="prev, pager, next" :current-page="page" :page-size="pageSize" :total="total" @current-change="changePage" />
  </div>
</template>

<style scoped>
.transcode-progress-cell {
  display: grid;
  gap: 6px;
  min-width: 0;
}

.transcode-progress-head {
  align-items: center;
  display: flex;
  gap: 8px;
  min-width: 0;
}

.transcode-progress-head strong {
  color: #344054;
  flex: 0 0 auto;
  font-size: 12px;
  font-weight: 700;
}

.transcode-progress-head span {
  color: #667085;
  flex: 1 1 auto;
  font-size: 12px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
