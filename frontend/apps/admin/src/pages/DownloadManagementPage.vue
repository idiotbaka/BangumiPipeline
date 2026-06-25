<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { api, type DownloadJob, type DownloadRetryAction, type DownloadStatus } from '../api'

const items = ref<DownloadJob[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 50
const loading = ref(true)
const status = ref<DownloadStatus>('downloading')
const retryingId = ref<number | null>(null)
let pollingTimer: ReturnType<typeof setInterval> | undefined

const filters: Array<{ label: string; value: DownloadStatus }> = [
  { label: '下载中', value: 'downloading' },
  { label: '待下载', value: 'pending' },
  { label: '已完成', value: 'completed' },
  { label: '下载失败', value: 'failed' },
]

async function load(showLoading = true) {
  if (showLoading) loading.value = true
  try {
    const result = await api.downloadJobs(page.value, pageSize, status.value)
    items.value = result.items
    total.value = result.total
  } catch (error) {
    if (showLoading) ElMessage.error(error instanceof Error ? error.message : '加载下载任务失败')
  } finally {
    if (showLoading) loading.value = false
  }
}

function changeStatus(value: string | number | boolean) {
  status.value = value as DownloadStatus
  page.value = 1
  void load()
}

function changePage(value: number) {
  page.value = value
  void load()
}

function statusType(value: DownloadStatus) {
  if (value === 'completed') return 'success'
  if (value === 'failed') return 'danger'
  if (value === 'downloading') return 'warning'
  return 'info'
}

function statusLabel(value: DownloadStatus) {
  if (value === 'completed') return '已完成'
  if (value === 'failed') return '下载失败'
  if (value === 'downloading') return '下载中'
  return '待下载'
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

function formatEpisode(job: DownloadJob) {
  const season = `S${String(job.seasonNumber).padStart(2, '0')}`
  if (job.episodeType && job.episodeType !== 'episode') {
    return `${season} ${episodeTypeLabel(job.episodeType)} ${job.episodeNumber}`
  }
  return `${season} E${job.episodeNumber}`
}

function formatBytes(bytes: number) {
  if (!bytes) return '—'
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(2)} MB`
  return `${(bytes / 1024).toFixed(1)} KB`
}

function formatSpeed(bytesPerSecond: number) {
  if (!bytesPerSecond) return '—'
  return `${formatBytes(bytesPerSecond)}/s`
}

function formatTime(timestamp: number | null) {
  return timestamp ? new Date(timestamp * 1000).toLocaleString() : '—'
}

function progressPercentage(job: DownloadJob) {
  return Math.max(0, Math.min(100, Math.round(job.progress * 100)))
}

function retryMessage(action: DownloadRetryAction) {
  if (action === 'corrected') return '已根据 qBittorrent 状态纠正本地记录'
  if (action === 'deleted_reset') return '已删除 qBittorrent 失败任务并重置为待下载'
  return 'qBittorrent 中未找到任务，已重置为待下载'
}

async function retryJob(job: DownloadJob) {
  retryingId.value = job.id
  try {
    const { result } = await api.retryDownloadJob(job.id)
    ElMessage.success(retryMessage(result.action))
    await load(false)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '重试下载任务失败')
  } finally {
    retryingId.value = null
  }
}

onMounted(async () => {
  await load()
  pollingTimer = window.setInterval(() => load(false), 5000)
})

onBeforeUnmount(() => {
  if (pollingTimer) window.clearInterval(pollingTimer)
})
</script>

<template>
  <header class="page-header">
    <div>
      <p class="eyebrow">DOWNLOADS</p>
      <h1>下载管理</h1>
      <p>查看已绑定话数的 qBittorrent 下载状态和本地记录。</p>
    </div>
    <el-button :icon="Refresh" :loading="loading" @click="load()">刷新</el-button>
  </header>

  <div class="subscription-toolbar">
    <el-radio-group :model-value="status" @change="changeStatus">
      <el-radio-button v-for="filter in filters" :key="filter.value" :label="filter.value">{{ filter.label }}</el-radio-button>
    </el-radio-group>
  </div>

  <el-card class="content-card download-card" shadow="never" v-loading="loading">
    <el-empty v-if="!loading && items.length === 0" description="当前状态下暂无下载任务" />
    <el-table v-else :data="items" class="download-table">
      <el-table-column label="番剧话数" min-width="240" show-overflow-tooltip>
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
      <el-table-column label="进度" min-width="180">
        <template #default="{ row }">
          <el-progress :percentage="progressPercentage(row)" :stroke-width="8" :show-text="true" />
        </template>
      </el-table-column>
      <el-table-column label="大小" width="130">
        <template #default="{ row }">
          <span>{{ formatBytes(row.downloadedSize) }} / {{ formatBytes(row.totalSize) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="速度" width="110">
        <template #default="{ row }">{{ formatSpeed(row.downloadSpeed) }}</template>
      </el-table-column>
      <el-table-column label="保存目录" min-width="190" show-overflow-tooltip>
        <template #default="{ row }">
          <span v-if="row.folderName">{{ row.folderName }}</span>
          <span v-else class="muted-text">计划任务创建后生成</span>
        </template>
      </el-table-column>
      <el-table-column label="qBittorrent 任务" min-width="220" show-overflow-tooltip>
        <template #default="{ row }">
          <span v-if="row.qbitName">{{ row.qbitName }}</span>
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
