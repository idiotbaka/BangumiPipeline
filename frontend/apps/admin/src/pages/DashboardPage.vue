<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Calendar, Download, FolderOpened, Refresh, Tickets, VideoCamera } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { api, type DashboardOverview } from '../api'
import { session } from '../session'

const loading = ref(false)
const overview = ref<DashboardOverview | null>(null)

const emptyOverview: DashboardOverview = {
  subscription: { pendingBindings: 0 },
  download: { pending: 0, downloading: 0, failed: 0 },
  media: { pending: 0, transcoding: 0, failed: 0 },
  storage: { roots: [] },
}

const current = computed(() => overview.value ?? emptyOverview)

const statusGroups = computed(() => [
  {
    title: '订阅匹配管理',
    icon: Tickets,
    route: '/subscriptions',
    total: current.value.subscription.pendingBindings,
    items: [
      { label: '待绑定', value: current.value.subscription.pendingBindings, tone: 'warning' },
    ],
  },
  {
    title: '下载管理',
    icon: Download,
    route: '/downloads',
    total: current.value.download.pending + current.value.download.downloading + current.value.download.failed,
    items: [
      { label: '下载中', value: current.value.download.downloading, tone: 'primary' },
      { label: '待下载', value: current.value.download.pending, tone: 'warning' },
      { label: '下载失败', value: current.value.download.failed, tone: 'danger' },
    ],
  },
  {
    title: '转码管理',
    icon: VideoCamera,
    route: '/transcodes',
    total: current.value.media.pending + current.value.media.transcoding + current.value.media.failed,
    items: [
      { label: '待处理', value: current.value.media.pending, tone: 'warning' },
      { label: '转码中', value: current.value.media.transcoding, tone: 'primary' },
      { label: '处理失败', value: current.value.media.failed, tone: 'danger' },
    ],
  },
])

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let size = value
  let unitIndex = 0
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex++
  }
  return `${size >= 10 || unitIndex === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[unitIndex]}`
}

function storageProgressPercentage(root: DashboardOverview['storage']['roots'][number]) {
  if (!root.available || !Number.isFinite(root.usedPercent)) return 0
  return Math.min(100, Math.max(0, Number(root.usedPercent.toFixed(1))))
}

function storageProgressStatus(root: DashboardOverview['storage']['roots'][number]) {
  if (!root.available) return 'exception'
  if (root.usedPercent >= 95 || root.freeBytes < 10 * 1024 * 1024 * 1024) return 'exception'
  if (root.usedPercent >= 85) return 'warning'
  return undefined
}

async function loadOverview() {
  loading.value = true
  try {
    overview.value = (await api.dashboardOverview()).overview
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '系统概览加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(loadOverview)
</script>

<template>
  <header class="page-header dashboard-header">
    <div>
      <p class="eyebrow">MANAGEMENT CONSOLE</p>
      <h1>系统概览</h1>
      <p>当前队列和待处理工作状态。</p>
    </div>
    <div class="page-header-actions">
      <el-button :icon="Refresh" :loading="loading" @click="loadOverview">刷新</el-button>
      <el-tag size="large" type="success" effect="light">服务正常</el-tag>
    </div>
  </header>

  <el-alert title="管理员登录与 SQLite 会话已启用" type="success" :closable="false" show-icon>
    当前用户：{{ session.user?.username }}
  </el-alert>

  <section v-loading="loading" class="overview-grid">
    <article v-for="group in statusGroups" :key="group.title" class="overview-card">
      <header class="overview-card-header">
        <el-icon class="overview-icon"><component :is="group.icon" /></el-icon>
        <div>
          <h2>{{ group.title }}</h2>
          <span>{{ group.total }} 项需要关注</span>
        </div>
      </header>
      <div class="overview-stat-list">
        <div v-for="item in group.items" :key="item.label" class="overview-stat-row" :class="`overview-${item.tone}`">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
        </div>
      </div>
      <router-link :to="group.route" class="overview-link">查看详情</router-link>
    </article>

    <article class="overview-card storage-overview-card">
      <header class="overview-card-header">
        <el-icon class="overview-icon"><FolderOpened /></el-icon>
        <div>
          <h2>媒体存储空间</h2>
          <span>{{ current.storage.roots.length }} 个存储路径</span>
        </div>
      </header>
      <div v-if="current.storage.roots.length" class="storage-overview-list">
        <div
          v-for="root in current.storage.roots"
          :key="root.path"
          class="storage-overview-row"
          :class="{ 'storage-overview-row-error': !root.available }"
        >
          <div class="storage-root-copy">
            <div>
              <span>{{ root.label }}</span>
              <el-tag size="small" effect="plain" :type="root.isDefault ? undefined : 'success'">{{ root.isDefault ? '默认' : '额外' }}</el-tag>
            </div>
            <strong :title="root.path">{{ root.path }}</strong>
          </div>
          <template v-if="root.available">
            <div class="storage-space-copy">
              <span>{{ formatBytes(root.freeBytes) }} 可用</span>
              <small>共 {{ formatBytes(root.totalBytes) }}</small>
            </div>
            <el-progress
              class="storage-progress"
              :percentage="storageProgressPercentage(root)"
              :status="storageProgressStatus(root)"
              :show-text="false"
            />
          </template>
          <el-alert v-else class="storage-error" type="warning" :title="root.errorMessage || '路径不可用'" :closable="false" />
        </div>
      </div>
      <el-empty v-else description="尚未配置媒体存储路径" :image-size="64" />
      <router-link to="/settings" class="overview-link">管理存储路径</router-link>
    </article>
  </section>

  <section class="dashboard-note">
    <el-icon><Calendar /></el-icon>
    <span>计划任务状态随数据库更新；磁盘空间会在打开或刷新概览时读取。</span>
  </section>
</template>
