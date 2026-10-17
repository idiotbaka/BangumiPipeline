<script setup lang="ts">
import { Refresh, Search } from '@element-plus/icons-vue'
import { ref } from 'vue'
import { api, type TranslationAnimeProgress, type TranslationKind, type TranslationProgressStatus } from '../api'
import { translationKinds, useProgressList } from '../progress'
import ProgressTaskStatus from '../components/ProgressTaskStatus.vue'
import TranslationProgressDetails from '../components/TranslationProgressDetails.vue'

const { items, total, page, pageSize, query, status, loading, error, revision, load, reload, search, changePage, refresh } =
  useProgressList(api.translationProgress, 'all')
const detailsRevision = ref(0)
function translationSaved() {
  detailsRevision.value++
  void reload()
}

const filters: Array<{ label: string; value: TranslationProgressStatus }> = [
  { label: '全部', value: 'all' }, { label: '待翻译', value: 'pending' },
  { label: '已完成', value: 'completed' }, { label: '元数据未就绪', value: 'metadata_pending' },
  { label: '无可翻译内容', value: 'no_content' },
]
const labels: Record<TranslationAnimeProgress['status'], string> = {
  pending: '待翻译', completed: '已完成', metadata_pending: '元数据未就绪', no_content: '无可翻译内容',
}
const stageLabels: Record<string, string> = { detail: '详情', characters: '角色 / 声优', episodes: '分集' }

function countLabel(row: TranslationAnimeProgress, kind: TranslationKind) {
  const count = row.counts.find((item) => item.kind === kind)
  if (!count || (count.total === 0 && count.missingSource === 0)) return '暂无资料'
  if (count.total === 0) return count.missingSource > 1 ? `无原文（${count.missingSource} 项）` : '无原文'
  if (kind === 'anime_title' || kind === 'anime_summary') return count.completed ? '已完成' : '待翻译'
  return `${count.completed} / ${count.total}`
}

function missingSources(row: TranslationAnimeProgress, kind: TranslationKind) {
  const count = row.counts.find((item) => item.kind === kind)
  return count && count.total > 0 ? count.missingSource : 0
}
</script>

<template>
  <header class="page-header">
    <div>
      <p class="eyebrow">TRANSLATION</p>
      <h1>元数据翻译管理</h1>
      <p>按番剧查看当前入库资料的中文内容完成情况，展开行可检查具体分集、角色与声优。</p>
    </div>
    <el-button :icon="Refresh" :loading="loading" @click="refresh">刷新</el-button>
  </header>

  <ProgressTaskStatus task-key="translate-anime-metadata" :revision="revision" />

  <div class="progress-toolbar">
    <el-radio-group v-model="status" @change="search" aria-label="翻译进度筛选">
      <el-radio-button v-for="filter in filters" :key="filter.value" :value="filter.value">{{ filter.label }}</el-radio-button>
    </el-radio-group>
    <form class="progress-search" @submit.prevent="search">
      <el-input v-model="query" placeholder="搜索番剧标题或别名" clearable :prefix-icon="Search" aria-label="搜索番剧" @clear="search" />
      <el-button native-type="submit" :disabled="loading">搜索</el-button>
    </form>
  </div>

  <p class="progress-hint">完成数 / 有原文的项目数；已有中文、中文直拷和 LLM 译文均计为完成。无原文不算待翻译，翻译失败仍保留为待翻译，请结合任务错误排查。</p>

  <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon class="progress-list-error">
    <el-button link type="primary" @click="load">重试加载</el-button>
  </el-alert>
  <el-card class="content-card" shadow="never" v-loading="loading">
    <el-table :data="items" row-key="bangumiId" empty-text="当前筛选条件下暂无番剧" class="progress-table">
      <el-table-column type="expand" width="42">
        <template #default="{ row }: { row: TranslationAnimeProgress }">
          <div class="progress-metadata-warnings" v-if="row.metadata.some((stage) => stage.status !== 'completed')">
            <p v-for="stage in row.metadata.filter((item) => item.status !== 'completed')" :key="stage.kind" :class="{ 'progress-error-text': stage.status === 'failed' }">
              {{ stageLabels[stage.kind] }}{{ stage.status === 'failed' ? '抓取失败' : '尚未抓取完成' }}{{ stage.errorMessage ? `：${stage.errorMessage}` : '' }}
            </p>
          </div>
          <TranslationProgressDetails :bangumi-id="row.bangumiId" :revision="detailsRevision" @saved="translationSaved" />
        </template>
      </el-table-column>
      <el-table-column label="番剧" min-width="250">
        <template #default="{ row }: { row: TranslationAnimeProgress }">
          <div class="progress-title-cell">
            <router-link :to="`/anime/${row.bangumiId}`">{{ row.nameCN || row.name || '未命名番剧' }}</router-link>
            <span class="progress-hint">#{{ row.bangumiId }}<template v-if="row.nameCN && row.name !== row.nameCN"> · {{ row.name }}</template></span>
            <div class="progress-stage-tags">
              <el-tooltip v-for="stage in row.metadata.filter((item) => item.status !== 'completed')" :key="stage.kind" :content="stage.errorMessage || '元数据尚未抓取完成'">
                <el-tag :type="stage.status === 'failed' ? 'danger' : 'info'" size="small" effect="plain">{{ stageLabels[stage.kind] }}{{ stage.status === 'failed' ? '抓取失败' : '待补全' }}</el-tag>
              </el-tooltip>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="总体状态" min-width="155">
        <template #default="{ row }: { row: TranslationAnimeProgress }">
          <div class="progress-title-cell">
            <el-tag :type="row.status === 'completed' ? 'success' : row.status === 'pending' ? 'warning' : 'info'" effect="plain">{{ labels[row.status] }}</el-tag>
            <span class="progress-hint">{{ row.completed }} / {{ row.total }} 项</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column v-for="kind in translationKinds" :key="kind.value" :label="kind.label" min-width="105">
        <template #default="{ row }">
          <div class="progress-title-cell">
            <span>{{ countLabel(row, kind.value) }}</span>
            <span v-if="missingSources(row, kind.value)" class="progress-hint">无原文 {{ missingSources(row, kind.value) }} 项</span>
          </div>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
  <div class="anime-pagination" v-if="total > 0">
    <el-pagination background layout="total, prev, pager, next" :current-page="page" :page-size="pageSize" :total="total" @current-change="changePage" />
  </div>
</template>
