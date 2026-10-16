<script setup lang="ts">
import { computed, ref } from 'vue'
import { api, type TranslationKind, type TranslationProgressTarget } from '../api'
import { translationKinds, useProgressDetails } from '../progress'

const props = defineProps<{ bangumiId: number }>()
const { items, loading, error, load } = useProgressDetails(() => props.bangumiId, api.translationProgressDetails)
const kind = ref<TranslationKind | ''>('')
const status = ref<TranslationProgressTarget['status'] | ''>('')
const filtered = computed(() => items.value.filter((item) => (!kind.value || item.kind === kind.value) && (!status.value || item.status === status.value)))
const labels = { pending: '待翻译', completed: '已完成', no_source: '无原文' }
function kindLabel(value: TranslationKind) {
  return translationKinds.find((item) => item.value === value)?.label ?? value
}
</script>

<template>
  <div class="progress-details" v-loading="loading">
    <div class="progress-detail-toolbar">
      <el-select v-model="kind" placeholder="全部翻译项目" clearable aria-label="翻译项目" style="width: 170px">
        <el-option v-for="option in translationKinds" :key="option.value" :value="option.value" :label="option.label" />
      </el-select>
      <el-select v-model="status" placeholder="全部明细状态" clearable aria-label="翻译明细状态" style="width: 170px">
        <el-option v-for="(label, value) in labels" :key="value" :value="value" :label="label" />
      </el-select>
      <span class="progress-hint">{{ filtered.length }} 项 · 仅统计当前已入库资料，声优按人去重。</span>
    </div>
    <div v-if="error" class="progress-task-line">
      <span class="progress-error-text">{{ error }}</span>
      <el-button link type="primary" @click="load">重试加载明细</el-button>
    </div>
    <el-table v-else :data="filtered" max-height="420" empty-text="当前筛选下没有翻译明细" size="small">
      <el-table-column label="翻译项目" width="125">
        <template #default="{ row }">{{ kindLabel(row.kind) }}</template>
      </el-table-column>
      <el-table-column prop="label" label="番剧 / 分集 / 角色 / 声优" min-width="260" show-overflow-tooltip />
      <el-table-column prop="id" label="Bangumi ID" width="120" />
      <el-table-column label="状态" width="110">
        <template #default="{ row }: { row: TranslationProgressTarget }">
          <el-tag :type="row.status === 'completed' ? 'success' : row.status === 'pending' ? 'warning' : 'info'" effect="plain" size="small">{{ labels[row.status] }}</el-tag>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>
