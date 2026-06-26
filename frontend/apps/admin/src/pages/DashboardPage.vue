<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Calendar, Download, Refresh, Tickets, VideoCamera } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { api, type DashboardOverview } from '../api'
import { session } from '../session'

const loading = ref(false)
const overview = ref<DashboardOverview | null>(null)

const emptyOverview: DashboardOverview = {
  subscription: { pendingBindings: 0 },
  download: { pending: 0, downloading: 0, failed: 0 },
  media: { pending: 0, transcoding: 0, failed: 0 },
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
  </section>

  <section class="dashboard-note">
    <el-icon><Calendar /></el-icon>
    <span>计划任务运行后，概览数据会随数据库状态更新。</span>
  </section>
</template>
