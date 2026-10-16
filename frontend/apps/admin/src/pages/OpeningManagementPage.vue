<script setup lang="ts">
import { Refresh, Search } from '@element-plus/icons-vue'
import { api, type OpeningAnimeProgress, type OpeningProgressFilter } from '../api'
import { formatProgressTime, useProgressList } from '../progress'
import ProgressTaskStatus from '../components/ProgressTaskStatus.vue'
import OpeningProgressDetails from '../components/OpeningProgressDetails.vue'

const { items, total, page, pageSize, query, status, loading, error, revision, load, search, changePage, refresh } =
  useProgressList(api.openingProgress, 'all')

const filters: Array<{ label: string; value: OpeningProgressFilter }> = [
  { label: '全部', value: 'all' }, { label: '已完成', value: 'completed' },
  { label: '待完成', value: 'pending' }, { label: '有失败', value: 'failed' },
  { label: '等待足够话数', value: 'waiting_episodes' }, { label: '含未识别到结果', value: 'not_found' },
]
const labels = { completed: '已完成', pending: '待完成', failed: '有失败', waiting_episodes: '等待足够话数' }
const columns: Array<{ label: string; prop: keyof OpeningAnimeProgress }> = [
  { label: '正片成品', prop: 'total' }, { label: '已检测到', prop: 'detected' },
  { label: '未识别到', prop: 'notFound' }, { label: '待识别', prop: 'pending' },
  { label: '等待足够话数', prop: 'waitingEpisodes' }, { label: '失败', prop: 'failed' },
]
</script>

<template>
  <header class="page-header">
    <div>
      <p class="eyebrow">OPENING DETECTION</p>
      <h1>片头曲识别管理</h1>
      <p>按番剧查看正片成品的最近识别结果，展开行可检查每季、每话的片头范围与错误。</p>
    </div>
    <el-button :icon="Refresh" :loading="loading" @click="refresh">刷新</el-button>
  </header>

  <ProgressTaskStatus task-key="detect-media-openings" :revision="revision" />

  <div class="progress-toolbar">
    <el-radio-group v-model="status" @change="search" aria-label="片头识别进度筛选">
      <el-radio-button v-for="filter in filters" :key="filter.value" :value="filter.value">{{ filter.label }}</el-radio-button>
    </el-radio-group>
    <form class="progress-search" @submit.prevent="search">
      <el-input v-model="query" placeholder="搜索番剧标题或别名" clearable :prefix-icon="Search" aria-label="搜索番剧" @clear="search" />
      <el-button native-type="submit" :disabled="loading">搜索</el-button>
    </form>
  </div>

  <p class="progress-hint">仅展示有已完成正片成品的番剧，按视频记录计数。“已完成”包含未识别到 OP；“待完成”包含失败和等待足够话数。任务按番剧与季复用未变化的完整结果，新增或变更成品时重新分析；页面展示已保存的最近结果。</p>

  <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon class="progress-list-error">
    <el-button link type="primary" @click="load">重试加载</el-button>
  </el-alert>
  <el-card class="content-card" shadow="never" v-loading="loading">
    <el-table :data="items" row-key="bangumiId" empty-text="当前筛选条件下暂无正片成品番剧" class="progress-table">
      <el-table-column type="expand" width="42">
        <template #default="{ row }"><OpeningProgressDetails :bangumi-id="row.bangumiId" /></template>
      </el-table-column>
      <el-table-column label="番剧" min-width="250">
        <template #default="{ row }">
          <div class="progress-title-cell">
            <router-link :to="`/anime/${row.bangumiId}`">{{ row.nameCN || row.name || '未命名番剧' }}</router-link>
            <span class="progress-hint">#{{ row.bangumiId }}<template v-if="row.nameCN && row.name !== row.nameCN"> · {{ row.name }}</template></span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="总体状态" min-width="155">
        <template #default="{ row }: { row: OpeningAnimeProgress }">
          <div class="progress-title-cell">
            <el-tag :type="row.status === 'completed' ? 'success' : row.status === 'failed' ? 'danger' : 'warning'" effect="plain">{{ labels[row.status] }}</el-tag>
            <span class="progress-hint">已完成 {{ row.detected + row.notFound }} / {{ row.total }}</span>
            <el-tag v-if="row.detected === row.total" type="success" size="small" effect="plain">全部检测到 OP</el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column v-for="column in columns" :key="column.prop" :label="column.label" :prop="column.prop" :min-width="column.prop === 'waitingEpisodes' ? 120 : 95" align="center" />
      <el-table-column label="最近分析" min-width="180">
        <template #default="{ row }">{{ formatProgressTime(row.lastAnalyzedAt) }}</template>
      </el-table-column>
    </el-table>
  </el-card>
  <div class="anime-pagination" v-if="total > 0">
    <el-pagination background layout="total, prev, pager, next" :current-page="page" :page-size="pageSize" :total="total" @current-change="changePage" />
  </div>
</template>
