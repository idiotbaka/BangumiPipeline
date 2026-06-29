<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Plus, Refresh, UploadFilled } from '@element-plus/icons-vue'
import {
  api,
  type AnimeSearchItem,
  type ViewerCarouselInput,
  type ViewerCarouselItem,
} from '../api'

const items = ref<ViewerCarouselItem[]>([])
const animeOptions = ref<AnimeSearchItem[]>([])
const loading = ref(false)
const animeLoading = ref(false)
const dialogVisible = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const previewURL = ref('')
const form = ref({
  bangumiId: null as number | null,
  sortOrder: 0,
})
let animeRequestID = 0
let objectPreviewURL = ''

const dialogTitle = computed(() => (editingId.value === null ? '新增轮播图' : '编辑轮播图'))
const imageRequired = computed(() => editingId.value === null)

onMounted(async () => {
  await Promise.all([loadItems(), searchAnime('')])
})

onBeforeUnmount(revokeObjectPreview)

async function loadItems() {
  loading.value = true
  try {
    const result = await api.viewerCarousels()
    items.value = result.items
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '加载轮播图失败')
  } finally {
    loading.value = false
  }
}

async function searchAnime(query: string) {
  const requestID = ++animeRequestID
  animeLoading.value = true
  try {
    const result = await api.animeSearch(query, 50)
    if (requestID === animeRequestID) animeOptions.value = result.items
  } catch (error) {
    if (requestID === animeRequestID) {
      ElMessage.error(error instanceof Error ? error.message : '搜索番剧失败')
    }
  } finally {
    if (requestID === animeRequestID) animeLoading.value = false
  }
}

function openCreate() {
  editingId.value = null
  form.value = { bangumiId: null, sortOrder: nextSortOrder() }
  selectedFile.value = null
  setPreview('')
  dialogVisible.value = true
}

function openEdit(item: ViewerCarouselItem) {
  editingId.value = item.id
  form.value = { bangumiId: item.bangumiId, sortOrder: item.sortOrder }
  selectedFile.value = null
  const currentOption = animeOptions.value.find((anime) => anime.bangumiId === item.bangumiId)
  if (!currentOption) {
    animeOptions.value = [
      { bangumiId: item.bangumiId, name: item.title, nameCN: item.title },
      ...animeOptions.value,
    ]
  }
  setPreview(imageURL(item))
  dialogVisible.value = true
}

function nextSortOrder() {
  if (items.value.length === 0) return 0
  return Math.max(...items.value.map((item) => item.sortOrder)) + 10
}

function imageURL(item: ViewerCarouselItem) {
  return `/api/viewer/carousels/${item.id}/image?v=${item.imageUpdatedAt}`
}

function chooseImage() {
  fileInput.value?.click()
}

async function selectImage(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  const extension = file.name.toLowerCase()
  if ((file.type && !['image/jpeg', 'image/png'].includes(file.type)) || !/\.(jpe?g|png)$/.test(extension)) {
    ElMessage.warning('请上传 JPG 或 PNG 图片')
    return
  }
  if (file.size > 10 * 1024 * 1024) {
    ElMessage.warning('轮播图不能超过 10MiB')
    return
  }
  const url = URL.createObjectURL(file)
  try {
    const size = await imageSize(url)
    if (size.width <= size.height) {
      ElMessage.warning('轮播图必须是横向宽图')
      URL.revokeObjectURL(url)
      return
    }
  } catch {
    ElMessage.warning('无法读取图片，请更换文件')
    URL.revokeObjectURL(url)
    return
  }
  selectedFile.value = file
  revokeObjectPreview()
  objectPreviewURL = url
  previewURL.value = url
}

function imageSize(url: string) {
  return new Promise<{ width: number; height: number }>((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve({ width: image.naturalWidth, height: image.naturalHeight })
    image.onerror = reject
    image.src = url
  })
}

async function saveItem() {
  if (!form.value.bangumiId) {
    ElMessage.warning('请选择要绑定的番剧')
    return
  }
  if (imageRequired.value && !selectedFile.value) {
    ElMessage.warning('请上传轮播宽图')
    return
  }
  const input: ViewerCarouselInput = {
    bangumiId: form.value.bangumiId,
    sortOrder: form.value.sortOrder,
    file: selectedFile.value,
  }
  saving.value = true
  try {
    if (editingId.value === null) {
      await api.createViewerCarousel(input)
      ElMessage.success('轮播图已新增')
    } else {
      await api.updateViewerCarousel(editingId.value, input)
      ElMessage.success('轮播图已更新')
    }
    dialogVisible.value = false
    await loadItems()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存轮播图失败')
  } finally {
    saving.value = false
  }
}

async function deleteItem(item: ViewerCarouselItem) {
  try {
    await ElMessageBox.confirm(`确定删除「${item.title}」的轮播图吗？`, '删除轮播图', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await api.deleteViewerCarousel(item.id)
    ElMessage.success('轮播图已删除')
    await loadItems()
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error(error instanceof Error ? error.message : '删除轮播图失败')
  }
}

function animeLabel(anime: AnimeSearchItem) {
  return anime.nameCN || anime.name || `Bangumi ${anime.bangumiId}`
}

function formatDate(value: number) {
  return value ? new Date(value * 1000).toLocaleString() : '-'
}

function setPreview(url: string) {
  revokeObjectPreview()
  previewURL.value = url
}

function revokeObjectPreview() {
  if (objectPreviewURL) {
    URL.revokeObjectURL(objectPreviewURL)
    objectPreviewURL = ''
  }
}
</script>

<template>
  <section>
    <header class="page-header">
      <div>
        <p class="eyebrow">FRONTEND</p>
        <h1>轮播图管理</h1>
        <p>配置观看端首页轮播宽图、绑定番剧与展示顺序。</p>
      </div>
      <div class="page-header-actions">
        <el-button :icon="Refresh" :loading="loading" @click="loadItems">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新增轮播图</el-button>
      </div>
    </header>

    <el-card class="content-card management-card" shadow="never">
      <el-table v-loading="loading" :data="items" empty-text="暂无轮播图配置" class="management-table">
        <el-table-column width="220" label="轮播宽图">
          <template #default="{ row }">
            <img class="carousel-table-image" :src="imageURL(row)" :alt="row.title" />
          </template>
        </el-table-column>
        <el-table-column min-width="260" label="绑定番剧">
          <template #default="{ row }">
            <div class="carousel-anime-cell">
              <strong>{{ row.title }}</strong>
              <span>Bangumi ID：{{ row.bangumiId }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column width="120" prop="sortOrder" label="排序" sortable />
        <el-table-column width="190" label="更新时间">
          <template #default="{ row }">{{ formatDate(row.updatedAt) }}</template>
        </el-table-column>
        <el-table-column width="170" label="操作" fixed="right">
          <template #default="{ row }">
            <el-button size="small" :icon="Edit" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" plain :icon="Delete" @click="deleteItem(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="720px"
      destroy-on-close
      :close-on-click-modal="!saving"
    >
      <el-form label-position="top">
        <el-form-item label="绑定番剧" required>
          <el-select
            v-model="form.bangumiId"
            filterable
            remote
            clearable
            :remote-method="searchAnime"
            :loading="animeLoading"
            placeholder="输入番剧名称或别名搜索"
            style="width: 100%"
          >
            <el-option
              v-for="anime in animeOptions"
              :key="anime.bangumiId"
              :label="animeLabel(anime)"
              :value="anime.bangumiId"
            >
              <div class="carousel-anime-option">
                <span>{{ animeLabel(anime) }}</span>
                <small>Bangumi ID {{ anime.bangumiId }}</small>
              </div>
            </el-option>
          </el-select>
        </el-form-item>
        <el-form-item label="排序顺序" required>
          <el-input-number v-model="form.sortOrder" :step="10" :min="-100000" :max="100000" />
          <p class="carousel-form-help">数值越小越靠前；相同数值按创建顺序排列。</p>
        </el-form-item>
        <el-form-item :label="imageRequired ? '轮播宽图' : '轮播宽图（不上传则保留原图）'" :required="imageRequired">
          <div class="carousel-upload">
            <div class="carousel-preview">
              <img v-if="previewURL" :src="previewURL" alt="轮播图预览" />
              <div v-else>
                <el-icon><UploadFilled /></el-icon>
                <span>请选择横向宽图</span>
              </div>
            </div>
            <div class="carousel-upload-actions">
              <el-button :icon="UploadFilled" @click="chooseImage">
                {{ selectedFile ? '重新选择' : '选择图片' }}
              </el-button>
              <span>{{ selectedFile?.name || '支持 JPG、PNG，最大 10MiB' }}</span>
            </div>
            <input
              ref="fileInput"
              class="hidden-file-input"
              accept="image/jpeg,image/png,.jpg,.jpeg,.png"
              type="file"
              @change="selectImage"
            />
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="saving" @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveItem">保存</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.carousel-table-image {
  display: block;
  width: 188px;
  height: 76px;
  object-fit: cover;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  background: #f5f7fa;
}

.carousel-anime-cell {
  min-width: 0;
  display: grid;
  gap: 5px;
}

.carousel-anime-cell strong {
  overflow: hidden;
  color: #101828;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.carousel-anime-cell span,
.carousel-form-help,
.carousel-upload-actions span {
  color: #98a2b3;
  font-size: 12px;
}

.carousel-anime-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.carousel-anime-option small {
  color: #98a2b3;
}

.carousel-form-help {
  margin: 7px 0 0 12px;
}

.carousel-upload {
  width: 100%;
  display: grid;
  gap: 12px;
}

.carousel-preview {
  overflow: hidden;
  width: 100%;
  aspect-ratio: 16 / 6;
  border: 1px dashed #cfd5df;
  border-radius: 10px;
  background: #f7f9fc;
}

.carousel-preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.carousel-preview > div {
  height: 100%;
  display: grid;
  place-content: center;
  justify-items: center;
  gap: 9px;
  color: #98a2b3;
}

.carousel-preview .el-icon {
  font-size: 30px;
}

.carousel-upload-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
</style>
