<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Calendar, Clock, Download, FolderOpened, Refresh, Tickets, User, VideoCamera } from '@element-plus/icons-vue'
import { api, type DashboardBindingDay, type DashboardStorageRoot, type ScheduledTask } from '../api'
import { useDashboardSection } from '../dashboard'

const pipeline = useDashboardSection(async () => (await api.dashboardPipeline()).overview)
const storage = useDashboardSection(async () => (await api.dashboardStorage()).storage)
const tasks = useDashboardSection(async () => (await api.scheduledTasks()).tasks)
const activities = useDashboardSection(api.dashboardActivities)
const loading = computed(() => pipeline.loading.value || storage.loading.value || tasks.loading.value || activities.loading.value)
const current = pipeline.data
const clock = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
const loadActivities = activities.load
function refresh() {
  void pipeline.load()
  void tasks.load()
  void loadActivities()
  void storage.load()
}

const statusGroups = computed(() => [
  {
    title: '订阅匹配管理', icon: Tickets, route: '/subscriptions',
    total: current.value?.subscription.pendingBindings,
    items: [{ label: '待绑定', value: current.value?.subscription.pendingBindings, tone: 'warning' }],
    dailyLabel: '已绑定',
    daily: [
      { label: '昨日', date: current.value?.yesterday, count: current.value?.subscription.yesterday.total, breakdown: current.value?.subscription.yesterday },
      { label: '今日', date: current.value?.today, count: current.value?.subscription.today.total, breakdown: current.value?.subscription.today },
    ],
  },
  {
    title: '下载管理', icon: Download, route: '/downloads',
    total: current.value ? current.value.download.pending + current.value.download.downloading + current.value.download.failed : undefined,
    items: [
      { label: '下载中', value: current.value?.download.downloading, tone: 'primary' },
      { label: '待下载', value: current.value?.download.pending, tone: 'warning' },
      { label: '下载失败', value: current.value?.download.failed, tone: 'danger' },
    ],
    dailyLabel: '下载完成',
    daily: [
      { label: '昨日', date: current.value?.yesterday, count: current.value?.download.yesterday, breakdown: undefined },
      { label: '今日', date: current.value?.today, count: current.value?.download.today, breakdown: undefined },
    ],
  },
  {
    title: '转码管理', icon: VideoCamera, route: '/transcodes',
    total: current.value ? current.value.media.pending + current.value.media.transcoding + current.value.media.failed : undefined,
    items: [
      { label: '待处理', value: current.value?.media.pending, tone: 'warning' },
      { label: '转码中', value: current.value?.media.transcoding, tone: 'primary' },
      { label: '处理失败', value: current.value?.media.failed, tone: 'danger' },
    ],
    dailyLabel: '处理完成',
    daily: [
      { label: '昨日', date: current.value?.yesterday, count: current.value?.media.yesterday, breakdown: undefined },
      { label: '今日', date: current.value?.today, count: current.value?.media.today, breakdown: undefined },
    ],
  },
])

const sortedTasks = computed(() => [...(tasks.data.value ?? [])].sort((a, b) => {
  const priority: Record<string, number> = { failed: 0, running: 1, completed: 2, idle: 3 }
  return (priority[a.lastStatus] ?? 3) - (priority[b.lastStatus] ?? 3) || a.name.localeCompare(b.name, 'zh-CN')
}))
const failedTasks = computed(() => tasks.data.value?.filter((task) => task.lastStatus === 'failed').length ?? 0)
function taskStatus(task: ScheduledTask) {
  return ({ failed: '执行失败', running: '正在执行', completed: '已完成', idle: '尚未执行' } as Record<string, string>)[task.lastStatus] ?? '尚未执行'
}
function taskTone(task: ScheduledTask) {
  if (task.lastStatus === 'failed') return 'danger'
  if (task.lastStatus === 'running') return 'warning'
  if (task.lastStatus === 'completed') return 'success'
  return 'info'
}
function bindingBreakdown(day: DashboardBindingDay) {
  return `自动 ${day.automatic} · 手动 ${day.manual}${day.unknown ? ` · 来源未知 ${day.unknown}` : ''}`
}
function relativeTime(timestamp: number) {
  const serverNow = (activities.data.value?.generatedAt ?? clock.value / 1000) + Math.max(0, clock.value - activities.loadedAt.value) / 1000
  const seconds = Math.max(0, serverNow - timestamp)
  if (seconds < 60) return '刚刚'
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前`
  return `${Math.floor(seconds / 86400)} 天前`
}
function formatTime(value: number | null) {
  return value ? new Date(value * 1000).toLocaleString() : '尚未执行'
}
function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index++ }
  return `${size >= 10 || index === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`
}
function storagePercentage(root: DashboardStorageRoot) {
  return root.available && Number.isFinite(root.usedPercent) ? Math.min(100, Math.max(0, root.usedPercent)) : 0
}
function storageStatus(root: DashboardStorageRoot) {
  if (!root.available || root.usedPercent >= 95 || root.freeBytes < 10 * 1024 ** 3) return 'exception'
  if (root.usedPercent >= 85) return 'warning'
  return undefined
}
onMounted(() => { refresh(); timer = setInterval(() => { clock.value = Date.now() }, 60000) })
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
</script>

<template>
  <header class="page-header dashboard-header">
    <div>
      <p class="eyebrow">MANAGEMENT CONSOLE</p>
      <h1>系统概览</h1>
      <p>关注待处理工作，查看近期产出与用户活动。</p>
    </div>
    <el-button :icon="Refresh" :loading="loading" @click="refresh">刷新</el-button>
  </header>

  <div class="dashboard-period">
    <el-icon><Calendar /></el-icon>
    <span v-if="current">服务器时区 {{ current.timezone }} · 今日 {{ current.today }} · 昨日 {{ current.yesterday }}</span>
    <span v-else>按服务器时区统计昨日与今日的自然日数据</span>
    <span v-if="current" class="dashboard-updated">数据更新于 {{ formatTime(current.generatedAt) }}</span>
  </div>
  <el-alert v-if="pipeline.error.value" type="error" :closable="false" show-icon class="dashboard-section-error">
    {{ current ? '流水线刷新失败，显示上次数据：' : '流水线数据加载失败：' }}{{ pipeline.error.value }}
    <el-button link type="primary" @click="pipeline.load">重试</el-button>
  </el-alert>

  <section class="overview-grid">
    <article v-for="group in statusGroups" :key="group.title" class="overview-card pipeline-overview-card" v-loading="pipeline.loading.value">
      <header class="overview-card-header">
        <el-icon class="overview-icon"><component :is="group.icon" /></el-icon>
        <div><h2>{{ group.title }}</h2><span>{{ group.total === undefined ? '等待数据' : `${group.total} 项需要关注` }}</span></div>
      </header>
      <div class="overview-stat-list">
        <div v-for="item in group.items" :key="item.label" class="overview-stat-row" :class="`overview-${item.tone}`">
          <span>{{ item.label }}</span><strong>{{ item.value ?? '—' }}</strong>
        </div>
      </div>
      <div class="overview-daily">
        <span class="overview-daily-heading">{{ group.dailyLabel }}概览</span>
        <div class="overview-daily-columns">
          <div v-for="day in group.daily" :key="day.label">
            <span :title="day.date">{{ day.label }} <small>{{ day.date?.slice(5) }}</small></span>
            <strong>{{ day.count ?? '—' }}</strong>
            <small v-if="day.breakdown">{{ bindingBreakdown(day.breakdown) }}</small>
          </div>
        </div>
      </div>
      <router-link :to="group.route" class="overview-link">查看详情</router-link>
    </article>

    <article class="overview-card dashboard-tasks-card" v-loading="tasks.loading.value">
      <header class="overview-card-header">
        <el-icon class="overview-icon"><Clock /></el-icon>
        <div><h2>计划任务执行概览</h2><span>每个任务最近一次运行状态</span></div>
        <el-tag v-if="tasks.data.value" class="dashboard-header-tag" :type="failedTasks ? 'danger' : 'success'" effect="light">{{ failedTasks ? `${failedTasks} 项执行失败` : '暂无执行失败' }}</el-tag>
      </header>
      <el-alert v-if="tasks.error.value" type="error" :closable="false" show-icon>
        {{ tasks.data.value ? '刷新失败，显示上次数据：' : '' }}{{ tasks.error.value }} <el-button link type="primary" @click="tasks.load">重试</el-button>
      </el-alert>
      <div v-if="tasks.data.value?.length" class="dashboard-task-list">
        <div v-for="task in sortedTasks" :key="task.key" class="dashboard-task-item" :class="{ 'dashboard-task-failed': task.lastStatus === 'failed' }">
          <div class="dashboard-task-title"><strong>{{ task.name }}</strong><span v-if="!task.enabled">未启用</span><el-tag :type="taskTone(task)" effect="plain" size="small">{{ taskStatus(task) }}</el-tag></div>
          <p class="dashboard-task-time">{{ task.lastStatus === 'running' ? '开始于' : '最近运行' }} {{ formatTime(task.lastStatus === 'running' ? task.lastStartedAt : task.lastFinishedAt) }}</p>
          <p v-if="task.lastStatus === 'failed' && task.lastError" class="dashboard-task-error" :title="task.lastError">{{ task.lastError }}</p>
        </div>
      </div>
      <el-empty v-else-if="!tasks.loading.value && !tasks.error.value" description="暂无计划任务" :image-size="48" />
      <router-link to="/scheduled-tasks" class="overview-link">管理计划任务</router-link>
    </article>

    <article class="overview-card dashboard-activities-card" v-loading="activities.loading.value">
      <header class="overview-card-header"><el-icon class="overview-icon"><User /></el-icon><div><h2>用户活动</h2><span>最近 5 条观看记录</span></div></header>
      <el-alert v-if="activities.error.value" type="error" :closable="false" show-icon>
        {{ activities.data.value ? '刷新失败，显示上次数据：' : '' }}{{ activities.error.value }} <el-button link type="primary" @click="loadActivities">重试</el-button>
      </el-alert>
      <ol v-if="activities.data.value?.items.length" class="dashboard-activity-list">
        <li v-for="item in activities.data.value.items" :key="item.id">
          <time :datetime="new Date(item.lastWatchedAt * 1000).toISOString()" :title="formatTime(item.lastWatchedAt)">{{ relativeTime(item.lastWatchedAt) }}</time>
          <p><strong>{{ item.username }}</strong> 观看<router-link :to="`/anime/${item.bangumiId}`">「{{ item.animeTitle }}」</router-link>的 {{ item.episodeLabel }}<template v-if="item.episodeTitle">「{{ item.episodeTitle }}」</template><span :class="{ 'dashboard-activity-completed': item.completed }">（{{ item.completed ? '已看完' : `看到 ${item.progressPercent}%` }}）</span></p>
        </li>
      </ol>
      <el-empty v-else-if="!activities.loading.value && !activities.error.value" description="暂无观看记录" :image-size="48" />
      <router-link to="/viewer-users" class="overview-link">查看用户管理</router-link>
    </article>

    <article class="overview-card storage-overview-card" v-loading="storage.loading.value">
      <header class="overview-card-header"><el-icon class="overview-icon"><FolderOpened /></el-icon><div><h2>媒体存储空间</h2><span>{{ storage.data.value ? `${storage.data.value.roots.length} 个存储路径` : '等待数据' }}</span></div></header>
      <el-alert v-if="storage.error.value" type="error" :closable="false" show-icon>
        {{ storage.data.value ? '刷新失败，显示上次数据：' : '' }}{{ storage.error.value }} <el-button link type="primary" @click="storage.load">重试</el-button>
      </el-alert>
      <div v-if="storage.data.value?.roots.length" class="storage-overview-list">
        <div v-for="root in storage.data.value.roots" :key="root.path" class="storage-overview-row" :class="{ 'storage-overview-row-error': !root.available }">
          <div class="storage-root-copy"><div><span>{{ root.label }}</span><el-tag size="small" effect="plain" :type="root.isDefault ? undefined : 'success'">{{ root.isDefault ? '默认' : '额外' }}</el-tag></div><strong :title="root.path">{{ root.path }}</strong></div>
          <template v-if="root.available">
            <div class="storage-space-copy"><span>{{ formatBytes(root.freeBytes) }} 可用</span><small>共 {{ formatBytes(root.totalBytes) }}</small></div>
            <el-progress class="storage-progress" :percentage="storagePercentage(root)" :status="storageStatus(root)" :show-text="false" />
          </template>
          <el-alert v-else class="storage-error" type="warning" :title="root.errorMessage || '路径不可用'" :closable="false" />
        </div>
      </div>
      <el-empty v-else-if="!storage.loading.value && !storage.error.value" description="尚未配置媒体存储路径" :image-size="64" />
      <router-link to="/settings" class="overview-link">管理存储路径</router-link>
    </article>
  </section>
  <p class="dashboard-note">每日数据按服务器时区及当前记录统计；媒体完成数包含复制与转码等处理方式。用户活动展示每位用户每话的最近观看记录。</p>
</template>
