<script setup lang="ts">
import { computed, ref } from 'vue'
import { api, type OpeningEpisodeProgress } from '../api'
import { formatProgressTime, useProgressDetails } from '../progress'

const props = defineProps<{ bangumiId: number }>()
const { items, loading, error, load } = useProgressDetails(() => props.bangumiId, api.openingProgressDetails)
const status = ref<OpeningEpisodeProgress['status'] | ''>('')
const filtered = computed(() => items.value.filter((item) => !status.value || item.status === status.value))
const labels = { detected: '已检测到', not_found: '未识别到', pending: '待识别', waiting_episodes: '等待足够话数', failed: '识别失败' }

function formatSeconds(value: number) {
  const total = Math.max(0, Math.floor(value))
  return `${String(Math.floor(total / 60)).padStart(2, '0')}:${String(total % 60).padStart(2, '0')}`
}
</script>

<template>
  <div class="progress-details" v-loading="loading">
    <div class="progress-detail-toolbar">
      <el-select v-model="status" placeholder="全部识别状态" clearable aria-label="识别明细状态" style="width: 190px">
        <el-option v-for="(label, value) in labels" :key="value" :value="value" :label="label" />
      </el-select>
      <span class="progress-hint">{{ filtered.length }} 个成品 · 按季分组，同组至少 3 个正片成品才分析。</span>
    </div>
    <div v-if="error" class="progress-task-line">
      <span class="progress-error-text">{{ error }}</span>
      <el-button link type="primary" @click="load">重试加载明细</el-button>
    </div>
    <el-table v-else :data="filtered" max-height="420" empty-text="当前筛选下没有正片成品" size="small">
      <el-table-column label="话数" width="125">
        <template #default="{ row }">S{{ String(row.seasonNumber).padStart(2, '0') }} E{{ row.episodeNumber }}</template>
      </el-table-column>
      <el-table-column prop="mediaId" label="成品 ID" width="95" />
      <el-table-column label="状态" min-width="150">
        <template #default="{ row }: { row: OpeningEpisodeProgress }">
          <el-tag :type="row.status === 'detected' ? 'success' : row.status === 'failed' ? 'danger' : row.status === 'pending' || row.status === 'waiting_episodes' ? 'warning' : 'info'" effect="plain" size="small">{{ labels[row.status] }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="片头范围" width="145">
        <template #default="{ row }">{{ row.status === 'detected' ? `${formatSeconds(row.startSeconds)} – ${formatSeconds(row.endSeconds)}` : '—' }}</template>
      </el-table-column>
      <el-table-column label="最近分析" width="180">
        <template #default="{ row }">{{ formatProgressTime(row.updatedAt) }}</template>
      </el-table-column>
      <el-table-column label="说明 / 错误" min-width="270">
        <template #default="{ row }">
          <span v-if="row.status === 'waiting_episodes'">本季已有 {{ row.groupSize }} / {{ row.minimumGroupSize }} 个正片成品，等待补齐</span>
          <span v-else-if="row.status === 'failed'" class="progress-error-text">{{ row.errorMessage || '识别失败，请查看系统日志' }}</span>
          <span v-else-if="row.status === 'not_found'">已分析，未找到符合规则的 OP 片段</span>
          <span v-else-if="row.status === 'pending'">等待计划任务分析</span>
          <span v-else>分析时同组 {{ row.analyzedGroupSize }} 个成品</span>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>
