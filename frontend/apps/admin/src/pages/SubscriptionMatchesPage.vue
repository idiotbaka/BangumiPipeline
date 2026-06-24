<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, type AnimeSearchItem, type SubscriptionBindingStatus, type SubscriptionItem } from '../api'

const items = ref<SubscriptionItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 50
const loading = ref(true)
const bindingStatus = ref<SubscriptionBindingStatus>('pending')
const operatingId = ref<number | null>(null)

const manualVisible = ref(false)
const manualItem = ref<SubscriptionItem | null>(null)
const animeOptions = ref<AnimeSearchItem[]>([])
const animeLoading = ref(false)
const animeQuery = ref('')
const selectedBangumiId = ref<number | null>(null)
const manualForm = reactive({
  seasonNumber: 1,
  episodeType: 'episode',
  episodeNumber: '',
})

const bindingFilters: Array<{ label: string; value: SubscriptionBindingStatus }> = [
  { label: '待绑定', value: 'pending' },
  { label: '已绑定', value: 'bound' },
  { label: '已忽略', value: 'ignored' },
]

async function load(showLoading = true) {
  if (showLoading) loading.value = true
  try {
    const result = await api.subscriptionItems(page.value, pageSize, bindingStatus.value)
    items.value = result.items
    total.value = result.total
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '加载订阅条目失败')
  } finally {
    if (showLoading) loading.value = false
  }
}

function changeStatus(value: string | number | boolean) {
  bindingStatus.value = value as SubscriptionBindingStatus
  page.value = 1
  void load()
}

function changePage(value: number) {
  page.value = value
  void load()
}

async function confirmBinding(item: SubscriptionItem) {
  operatingId.value = item.id
  try {
    await api.confirmSubscriptionBinding(item.id)
    ElMessage.success('订阅条目已绑定')
    await load(false)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '确认绑定失败')
  } finally {
    operatingId.value = null
  }
}

async function ignoreItem(item: SubscriptionItem) {
  operatingId.value = item.id
  try {
    await api.ignoreSubscriptionItem(item.id)
    ElMessage.success('订阅条目已忽略')
    await load(false)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '忽略条目失败')
  } finally {
    operatingId.value = null
  }
}

async function openManualDialog(item: SubscriptionItem) {
  manualItem.value = item
  selectedBangumiId.value = item.boundBangumiId ?? item.bangumiId
  manualForm.seasonNumber = item.boundSeasonNumber ?? item.seasonNumber ?? 1
  manualForm.episodeType = item.boundEpisodeType || item.episodeType || 'episode'
  manualForm.episodeNumber = item.boundEpisodeNumber || item.episodeNumber || ''
  animeQuery.value = item.boundAnimeName || item.matchedName || item.parsedName
  manualVisible.value = true
  await searchAnime()
}

async function searchAnime() {
  animeLoading.value = true
  try {
    const result = await api.animeSearch(animeQuery.value, 500)
    animeOptions.value = result.items
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '搜索番剧失败')
  } finally {
    animeLoading.value = false
  }
}

async function submitManualBinding() {
  if (!manualItem.value || !selectedBangumiId.value) {
    ElMessage.warning('请选择要绑定的番剧')
    return
  }
  if (!Number.isInteger(manualForm.seasonNumber) || manualForm.seasonNumber < 1 || !manualForm.episodeNumber.trim()) {
    ElMessage.warning('请填写有效的季数和集数')
    return
  }
  operatingId.value = manualItem.value.id
  try {
    await api.bindSubscriptionItem(manualItem.value.id, {
      bangumiId: selectedBangumiId.value,
      seasonNumber: manualForm.seasonNumber,
      episodeType: manualForm.episodeType,
      episodeNumber: manualForm.episodeNumber.trim(),
    })
    ElMessage.success('订阅条目已绑定')
    manualVisible.value = false
    await load(false)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '手动绑定失败')
  } finally {
    operatingId.value = null
  }
}

function canConfirm(item: SubscriptionItem) {
  return item.matchStatus === 'matched' && item.bangumiId !== null && item.seasonNumber !== null && item.episodeNumber !== ''
}

function matchStatusType(status: SubscriptionItem['matchStatus']) {
  return status === 'matched' ? 'success' : 'danger'
}

function matchStatusLabel(status: SubscriptionItem['matchStatus']) {
  return status === 'matched' ? '匹配成功' : '匹配失败'
}

function bindingStatusType(status: SubscriptionBindingStatus) {
  if (status === 'bound') return 'success'
  if (status === 'ignored') return 'info'
  return 'warning'
}

function bindingStatusLabel(status: SubscriptionBindingStatus) {
  if (status === 'bound') return '已绑定'
  if (status === 'ignored') return '已忽略'
  return '待绑定'
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

function formatEpisode(seasonNumber: number | null, episodeType: string, episodeNumber: string) {
  const season = seasonNumber ? `S${String(seasonNumber).padStart(2, '0')}` : '季数未定'
  const episode = episodeNumber || '集数未定'
  if (episodeType && episodeType !== 'episode') {
    return `${season} ${episodeTypeLabel(episodeType)} ${episode}`
  }
  return `${season} E${episode}`
}

function formatMatchedEpisode(item: SubscriptionItem) {
  return formatEpisode(item.seasonNumber, item.episodeType, item.episodeNumber)
}

function formatBoundEpisode(item: SubscriptionItem) {
  return formatEpisode(item.boundSeasonNumber, item.boundEpisodeType, item.boundEpisodeNumber)
}

function formatScore(score: number) {
  return score > 0 ? score.toFixed(2) : '—'
}

function formatSize(bytes: number) {
  if (!bytes) return '—'
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

function formatTime(timestamp: number | null) {
  return timestamp ? new Date(timestamp * 1000).toLocaleString() : '—'
}

function animeTitle(anime: AnimeSearchItem) {
  return anime.nameCN || anime.name
}

onMounted(load)
</script>

<template>
  <header class="page-header">
    <div>
      <p class="eyebrow">SUBSCRIPTION MATCHES</p>
      <h1>订阅匹配管理</h1>
      <p>确认、修正或忽略 RSS 条目的番剧季集绑定结果。</p>
    </div>
    <el-button :loading="loading" @click="load()">刷新</el-button>
  </header>

  <div class="subscription-toolbar">
    <el-radio-group :model-value="bindingStatus" @change="changeStatus">
      <el-radio-button v-for="filter in bindingFilters" :key="filter.value" :label="filter.value">{{ filter.label }}</el-radio-button>
    </el-radio-group>
  </div>

  <el-card class="content-card subscription-card" shadow="never" v-loading="loading">
    <el-empty v-if="!loading && items.length === 0" description="当前状态下暂无订阅条目" />
    <el-table v-else :data="items" class="subscription-table">
      <el-table-column label="RSS 条目" min-width="320" show-overflow-tooltip>
        <template #default="{ row }">
          <div class="subscription-title-cell">
            <strong>{{ row.title }}</strong>
            <span>{{ formatSize(row.contentLength) }} · {{ formatTime(row.publishedAt) }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="96">
        <template #default="{ row }">
          <el-tag :type="bindingStatusType(row.bindingStatus)" effect="plain">{{ bindingStatusLabel(row.bindingStatus) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="规则匹配" width="110">
        <template #default="{ row }">
          <el-tag :type="matchStatusType(row.matchStatus)" effect="plain">{{ matchStatusLabel(row.matchStatus) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="匹配番剧" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          <span v-if="row.bangumiId">{{ row.matchedName }} #{{ row.bangumiId }}</span>
          <span v-else class="muted-text">—</span>
        </template>
      </el-table-column>
      <el-table-column label="匹配季集" width="150">
        <template #default="{ row }">{{ formatMatchedEpisode(row) }}</template>
      </el-table-column>
      <el-table-column label="绑定目标" min-width="190" show-overflow-tooltip>
        <template #default="{ row }">
          <span v-if="row.boundBangumiId">{{ row.boundAnimeName }} {{ formatBoundEpisode(row) }}</span>
          <span v-else class="muted-text">—</span>
        </template>
      </el-table-column>
      <el-table-column label="分数" width="80">
        <template #default="{ row }">{{ formatScore(row.matchScore) }}</template>
      </el-table-column>
      <el-table-column label="说明" min-width="220" show-overflow-tooltip>
        <template #default="{ row }">{{ row.bindingNote || row.matchReason }}</template>
      </el-table-column>
      <el-table-column label="操作" width="230" fixed="right">
        <template #default="{ row }">
          <div class="subscription-actions">
            <el-button
              size="small"
              type="primary"
              plain
              :disabled="!canConfirm(row)"
              :loading="operatingId === row.id"
              @click="confirmBinding(row)"
            >确认绑定</el-button>
            <el-button size="small" plain :loading="operatingId === row.id" @click="openManualDialog(row)">手动选择番剧</el-button>
            <el-popconfirm
              width="220"
              title="确认忽略该订阅条目？"
              confirm-button-text="忽略"
              cancel-button-text="取消"
              @confirm="ignoreItem(row)"
            >
              <template #reference>
                <el-button size="small" type="info" plain :loading="operatingId === row.id">忽略</el-button>
              </template>
            </el-popconfirm>
          </div>
        </template>
      </el-table-column>
    </el-table>
  </el-card>

  <div v-if="total > pageSize" class="anime-pagination">
    <el-pagination background layout="prev, pager, next" :current-page="page" :page-size="pageSize" :total="total" @current-change="changePage" />
  </div>

  <el-dialog v-model="manualVisible" title="手动选择番剧" width="min(760px, calc(100vw - 32px))">
    <div v-if="manualItem" class="manual-binding-dialog">
      <div class="manual-source-title">{{ manualItem.title }}</div>
      <el-input v-model.trim="animeQuery" placeholder="搜索番剧名称、中文名或别名" clearable @keyup.enter="searchAnime">
        <template #append><el-button :loading="animeLoading" @click="searchAnime">搜索</el-button></template>
      </el-input>
      <el-scrollbar class="anime-picker-list" v-loading="animeLoading">
        <el-empty v-if="!animeLoading && animeOptions.length === 0" description="未找到番剧" :image-size="72" />
        <label
          v-for="anime in animeOptions"
          :key="anime.bangumiId"
          class="anime-picker-item"
          :class="{ selected: selectedBangumiId === anime.bangumiId }"
        >
          <input v-model="selectedBangumiId" type="radio" :value="anime.bangumiId">
          <span>
            <strong>{{ animeTitle(anime) }}</strong>
            <small v-if="anime.nameCN">{{ anime.name }}</small>
            <small>Bangumi #{{ anime.bangumiId }}</small>
          </span>
        </label>
      </el-scrollbar>
      <div class="manual-episode-form">
        <el-form label-position="top">
          <el-form-item label="季数">
            <el-input-number v-model="manualForm.seasonNumber" :min="1" :max="99" controls-position="right" />
          </el-form-item>
          <el-form-item label="集类型">
            <el-select v-model="manualForm.episodeType">
              <el-option label="正片" value="episode" />
              <el-option label="OVA" value="ova" />
              <el-option label="OAD" value="oad" />
              <el-option label="SP" value="sp" />
            </el-select>
          </el-form-item>
          <el-form-item label="集数">
            <el-input v-model.trim="manualForm.episodeNumber" placeholder="例如 0、1、13.5、OVA1" />
          </el-form-item>
        </el-form>
      </div>
    </div>
    <template #footer>
      <el-button @click="manualVisible = false">取消</el-button>
      <el-button type="primary" :loading="operatingId === manualItem?.id" @click="submitManualBinding">确认</el-button>
    </template>
  </el-dialog>
</template>
