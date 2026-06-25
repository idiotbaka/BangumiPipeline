<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Delete, Plus, Refresh, Search, View } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { api, type AnimeListItem } from '../api'
import AnimeDetailPanel from '../components/AnimeDetailPanel.vue'

const items = ref<AnimeListItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 24
const loading = ref(false)
const deletingId = ref<number | null>(null)
const refreshingId = ref<number | null>(null)
const syncingHistoryId = ref<number | null>(null)
const historySyncVisible = ref(false)
const historySyncTarget = ref<AnimeListItem | null>(null)
const historySyncForm = reactive({ rssUrl: '', excludeTitle: '', includeTitle: '' })
const historySyncRSSRequired = computed(() => historySyncTarget.value !== null && historySyncTarget.value.matchedEpisodes.length === 0)
const historySyncConfirmDisabled = computed(() => historySyncTarget.value === null || (historySyncRSSRequired.value && historySyncForm.rssUrl.trim() === ''))
const addVisible = ref(false)
const adding = ref(false)
const addBangumiId = ref('')
const detailVisible = ref(false)
const selectedBangumiId = ref<number | null>(null)
const selectedAnimeTitle = ref('')

const weekdays = ['', '周一', '周二', '周三', '周四', '周五', '周六', '周日']

async function load() {
  loading.value = true
  try {
    const result = await api.animeList(page.value, pageSize)
    items.value = result.items
    total.value = result.total
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '番剧列表加载失败')
  } finally {
    loading.value = false
  }
}

function changePage(value: number) {
  page.value = value
  void load()
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

function showDetail(anime: AnimeListItem) {
  selectedBangumiId.value = anime.bangumiId
  selectedAnimeTitle.value = anime.nameCN || anime.name
  detailVisible.value = true
}

function clearDetail() {
  selectedBangumiId.value = null
  selectedAnimeTitle.value = ''
}

function episodeTagLabel(episode: AnimeListItem['matchedEpisodes'][number]) {
  if (episode.episodeType && episode.episodeType !== 'episode') {
    return `${episode.episodeType.toUpperCase()} ${episode.episodeNumber}`
  }
  if (episode.seasonNumber > 1) {
    return `S${episode.seasonNumber}E${episode.episodeNumber}`
  }
  return episode.episodeNumber
}

function episodeTagType(episode: AnimeListItem['matchedEpisodes'][number]) {
  return episode.status === 'completed' ? 'success' : 'warning'
}

function openAddDialog() {
  addBangumiId.value = ''
  addVisible.value = true
}

function normalizedBangumiId() {
  const value = Number(addBangumiId.value)
  if (!Number.isInteger(value) || value < 1) {
    return null
  }
  return value
}

async function addAnime() {
  if (adding.value) {
    return
  }
  const bangumiId = normalizedBangumiId()
  if (bangumiId === null) {
    ElMessage.warning('请输入有效的 Bangumi Subject ID')
    return
  }
  adding.value = true
  try {
    await api.createAnime(bangumiId)
    ElMessage.success('番剧已添加')
    addVisible.value = false
    page.value = 1
    await load()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '番剧添加失败')
  } finally {
    adding.value = false
  }
}

async function refreshAnime(anime: AnimeListItem) {
  if (refreshingId.value !== null) {
    return
  }
  refreshingId.value = anime.bangumiId
  try {
    await api.refreshAnime(anime.bangumiId)
    ElMessage.success('元数据已刷新')
    await load()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '元数据刷新失败')
  } finally {
    refreshingId.value = null
  }
}

function clearHistorySyncDialog() {
  historySyncTarget.value = null
  historySyncForm.rssUrl = ''
  historySyncForm.excludeTitle = ''
  historySyncForm.includeTitle = ''
}

function openHistorySyncDialog(anime: AnimeListItem) {
  if (syncingHistoryId.value !== null) {
    return
  }
  clearHistorySyncDialog()
  historySyncTarget.value = anime
  historySyncVisible.value = true
}

function resetHistorySyncDialog() {
  if (syncingHistoryId.value === null) {
    clearHistorySyncDialog()
  }
}

async function syncHistory() {
  if (syncingHistoryId.value !== null || historySyncTarget.value === null) {
    return
  }
  if (historySyncRSSRequired.value && historySyncForm.rssUrl.trim() === '') {
    ElMessage.warning('没有已绑定话数时，请填写番剧 RSS 链接')
    return
  }
  const anime = historySyncTarget.value
  syncingHistoryId.value = anime.bangumiId
  try {
    const { result } = await api.syncAnimeHistory(anime.bangumiId, {
      rssUrl: historySyncForm.rssUrl,
      excludeTitle: historySyncForm.excludeTitle,
      includeTitle: historySyncForm.includeTitle,
    })
    if (result.bound > 0) {
      ElMessage.success(`历史话数已同步，新增绑定 ${result.bound} 话`)
    } else {
      ElMessage.info('没有发现缺失的历史话数')
    }
    historySyncVisible.value = false
    await load()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '历史话数同步失败')
  } finally {
    syncingHistoryId.value = null
    if (!historySyncVisible.value) {
      clearHistorySyncDialog()
    }
  }
}

async function deleteAnime(anime: AnimeListItem) {
  if (deletingId.value !== null) {
    return
  }
  deletingId.value = anime.bangumiId
  try {
    await api.deleteAnime(anime.bangumiId)
    ElMessage.success('番剧已删除，后续抓取会跳过该 Bangumi ID')
    if (items.value.length === 1 && page.value > 1) {
      page.value -= 1
    }
    await load()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '番剧删除失败')
  } finally {
    deletingId.value = null
  }
}

onMounted(load)
</script>

<template>
  <section>
    <header class="page-header">
      <div>
        <p class="eyebrow">ANIME CATALOG</p>
        <h1>番剧管理</h1>
        <p>按抓取时间倒序展示数据库中的 Bangumi 动画元数据。</p>
      </div>
      <div class="page-header-actions">
        <el-button type="primary" :icon="Plus" @click="openAddDialog">新增番剧</el-button>
        <el-tag size="large" effect="plain">共 {{ total }} 部</el-tag>
      </div>
    </header>

    <div v-loading="loading" class="anime-grid">
      <article v-for="anime in items" :key="anime.bangumiId" class="anime-card">
        <div class="anime-cover-wrap">
          <img v-if="anime.hasCover" :src="`/api/anime/${anime.bangumiId}/cover`" :alt="anime.nameCN || anime.name" class="anime-cover">
          <div v-else class="anime-cover-placeholder">NO COVER</div>
          <span v-if="anime.platform" class="anime-platform">{{ anime.platform }}</span>
        </div>
        <div class="anime-card-body">
          <div class="anime-card-main">
            <div class="anime-title-block">
              <h2>{{ anime.nameCN || anime.name }}</h2>
              <p v-if="anime.nameCN">{{ anime.name }}</p>
            </div>
            <dl class="anime-meta-row">
              <div><dt>首播</dt><dd>{{ anime.airDate || '未定' }} {{ weekdays[anime.airWeekday] || '' }}</dd></div>
              <div><dt>话数</dt><dd>{{ anime.episodes > 0 ? `${anime.episodes} 话` : '未知' }}</dd></div>
            </dl>
            <div v-if="anime.matchedEpisodes.length > 0" class="anime-episode-tags">
              <el-tag
                v-for="episode in anime.matchedEpisodes"
                :key="`${episode.seasonNumber}-${episode.episodeType}-${episode.episodeNumber}`"
                :type="episodeTagType(episode)"
                effect="light"
                size="small"
                round
              >{{ episodeTagLabel(episode) }}</el-tag>
            </div>
          </div>
          <div class="anime-actions">
            <el-button size="small" :icon="View" type="primary" plain @click="showDetail(anime)">详情</el-button>
            <el-button
              size="small"
              :icon="Refresh"
              plain
              :loading="refreshingId === anime.bangumiId"
              @click="refreshAnime(anime)"
            >刷新元数据</el-button>
            <el-button
              size="small"
              :icon="Search"
              plain
              :loading="syncingHistoryId === anime.bangumiId"
              @click="openHistorySyncDialog(anime)"
            >同步历史话数</el-button>
            <el-popconfirm
              width="240"
              confirm-button-text="删除"
              cancel-button-text="取消"
              confirm-button-type="danger"
              :title="`确认删除「${anime.nameCN || anime.name}」？`"
              @confirm="deleteAnime(anime)"
            >
              <template #reference>
                <el-button size="small" :icon="Delete" type="danger" plain :loading="deletingId === anime.bangumiId">删除</el-button>
              </template>
            </el-popconfirm>
          </div>
        </div>
      </article>
    </div>

    <el-dialog
      v-model="addVisible"
      title="新增番剧"
      width="min(460px, calc(100vw - 32px))"
      destroy-on-close
      append-to-body
    >
      <el-form class="manual-anime-dialog" label-position="top" @submit.prevent>
        <el-form-item label="Bangumi Subject ID">
          <el-input v-model.trim="addBangumiId" placeholder="456079" clearable @keyup.enter="addAnime">
            <template #prepend>#</template>
          </el-input>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">取消</el-button>
        <el-button type="primary" :loading="adding" @click="addAnime">添加</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="historySyncVisible"
      title="同步历史话数"
      width="min(500px, calc(100vw - 32px))"
      destroy-on-close
      append-to-body
      @closed="resetHistorySyncDialog"
    >
      <el-form class="history-sync-dialog" label-position="top" @submit.prevent>
        <el-form-item label="番剧RSS链接" :required="historySyncRSSRequired">
          <el-input v-model.trim="historySyncForm.rssUrl" :placeholder="historySyncRSSRequired ? '请输入番剧 RSS 链接' : '留空自动搜索'" clearable @keyup.enter="syncHistory" />
        </el-form-item>
        <el-form-item label="过滤字段">
          <el-input v-model.trim="historySyncForm.excludeTitle" placeholder="标题包含该字段时跳过" clearable @keyup.enter="syncHistory" />
        </el-form-item>
        <el-form-item label="包含字段">
          <el-input v-model.trim="historySyncForm.includeTitle" placeholder="标题必须包含该字段" clearable @keyup.enter="syncHistory" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="historySyncVisible = false">取消</el-button>
        <el-button
          type="primary"
          :disabled="historySyncConfirmDisabled"
          :loading="historySyncTarget !== null && syncingHistoryId === historySyncTarget.bangumiId"
          @click="syncHistory"
        >确认</el-button>
      </template>
    </el-dialog>
    <el-empty v-if="!loading && items.length === 0" description="尚未抓取到番剧数据" />
    <div v-if="total > pageSize" class="anime-pagination">
      <el-pagination background layout="prev, pager, next" :current-page="page" :page-size="pageSize" :total="total" @current-change="changePage" />
    </div>

    <el-dialog
      v-model="detailVisible"
      class="anime-detail-dialog"
      :title="selectedAnimeTitle || '番剧详情'"
      width="min(1120px, calc(100vw - 32px))"
      top="4vh"
      body-class="anime-detail-dialog-body"
      destroy-on-close
      append-to-body
      @closed="clearDetail"
    >
      <AnimeDetailPanel v-if="selectedBangumiId !== null" :bangumi-id="selectedBangumiId" />
    </el-dialog>
  </section>
</template>
