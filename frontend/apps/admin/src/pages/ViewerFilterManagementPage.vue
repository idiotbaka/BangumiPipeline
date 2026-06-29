<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Plus, Refresh } from '@element-plus/icons-vue'

import {
  api,
  type ViewerFilterDimension,
  type ViewerFilterDimensionInput,
} from '../api'

const items = ref<ViewerFilterDimension[]>([])
const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const editingID = ref<number | null>(null)
const tagDraft = ref('')
const form = ref<ViewerFilterDimensionInput>({ name: '', sortOrder: 0, tags: [] })

const dialogTitle = computed(() => editingID.value === null ? '新增筛选维度' : '编辑筛选维度')

onMounted(loadItems)

async function loadItems() {
  loading.value = true
  try {
    const result = await api.viewerFilterDimensions()
    items.value = result.items
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '加载筛选维度失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingID.value = null
  form.value = { name: '', sortOrder: nextSortOrder(), tags: [] }
  tagDraft.value = ''
  dialogVisible.value = true
}

function openEdit(item: ViewerFilterDimension) {
  editingID.value = item.id
  form.value = { name: item.name, sortOrder: item.sortOrder, tags: [...item.tags] }
  tagDraft.value = ''
  dialogVisible.value = true
}

function nextSortOrder() {
  return items.value.length === 0 ? 0 : Math.max(...items.value.map((item) => item.sortOrder)) + 10
}

function addTag() {
  const tag = tagDraft.value.trim()
  if (!tag) return
  if (form.value.tags.includes(tag)) {
    ElMessage.warning('该标签已经存在')
    return
  }
  if (form.value.tags.length >= 50) {
    ElMessage.warning('每个维度最多配置 50 个标签')
    return
  }
  form.value.tags.push(tag)
  tagDraft.value = ''
}

function removeTag(index: number) {
  form.value.tags.splice(index, 1)
}

async function saveItem() {
  const name = form.value.name.trim()
  if (!name) {
    ElMessage.warning('请输入维度名称')
    return
  }
  addTag()
  if (form.value.tags.length === 0) {
    ElMessage.warning('请至少添加一个标签')
    return
  }
  saving.value = true
  try {
    const input = { ...form.value, name, tags: [...form.value.tags] }
    if (editingID.value === null) {
      await api.createViewerFilterDimension(input)
      ElMessage.success('筛选维度已新增')
    } else {
      await api.updateViewerFilterDimension(editingID.value, input)
      ElMessage.success('筛选维度已更新')
    }
    dialogVisible.value = false
    await loadItems()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存筛选维度失败')
  } finally {
    saving.value = false
  }
}

async function deleteItem(item: ViewerFilterDimension) {
  try {
    await ElMessageBox.confirm(
      `确定删除筛选维度「${item.name}」吗？观看端将不再显示该维度。`,
      '删除筛选维度',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
    await api.deleteViewerFilterDimension(item.id)
    ElMessage.success('筛选维度已删除')
    await loadItems()
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error(error instanceof Error ? error.message : '删除筛选维度失败')
  }
}

function formatDate(value: number) {
  return value ? new Date(value * 1000).toLocaleString() : '-'
}
</script>

<template>
  <section>
    <header class="page-header">
      <div>
        <p class="eyebrow">FRONTEND</p>
        <h1>筛选标签管理</h1>
        <p>配置观看端番剧图书馆的筛选维度与可选 Bangumi 标签。</p>
      </div>
      <div class="page-header-actions">
        <el-button :icon="Refresh" :loading="loading" @click="loadItems">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新增维度</el-button>
      </div>
    </header>

    <el-card class="content-card management-card" shadow="never">
      <el-table v-loading="loading" :data="items" empty-text="暂无筛选维度配置" class="management-table">
        <el-table-column min-width="170" prop="name" label="维度名称" />
        <el-table-column min-width="420" label="标签列表">
          <template #default="{ row }">
            <div class="tag-list">
              <el-tag v-for="tag in row.tags" :key="tag" effect="plain">{{ tag }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column width="110" prop="sortOrder" label="排序" sortable />
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
      width="680px"
      destroy-on-close
      :close-on-click-modal="!saving"
    >
      <el-form label-position="top">
        <el-form-item label="维度名称" required>
          <el-input v-model="form.name" maxlength="40" show-word-limit placeholder="例如：年份、改编类型" />
        </el-form-item>
        <el-form-item label="排序顺序" required>
          <el-input-number v-model="form.sortOrder" :step="10" :min="-100000" :max="100000" />
          <p class="form-help">数值越小，在观看端的位置越靠前。</p>
        </el-form-item>
        <el-form-item label="标签列表" required>
          <div class="tag-editor">
            <div v-if="form.tags.length" class="tag-list editable">
              <el-tag
                v-for="(tag, index) in form.tags"
                :key="tag"
                closable
                effect="plain"
                @close="removeTag(index)"
              >
                {{ tag }}
              </el-tag>
            </div>
            <div v-else class="tag-empty">尚未添加标签</div>
            <div class="tag-input-row">
              <el-input
                v-model="tagDraft"
                maxlength="80"
                placeholder="输入番剧库中使用的完整标签名称"
                @keyup.enter="addTag"
              />
              <el-button :icon="Plus" @click="addTag">添加标签</el-button>
            </div>
            <p class="form-help standalone">同一维度可多选；标签名称需要与番剧的 Bangumi 标签完全一致。</p>
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
.tag-list { display: flex; flex-wrap: wrap; gap: 7px; }
.tag-list.editable { min-height: 34px; padding: 12px; border: 1px solid #e4e7ed; border-radius: 8px; background: #fafbfc; }
.tag-editor { width: 100%; display: grid; gap: 12px; }
.tag-input-row { display: grid; grid-template-columns: 1fr auto; gap: 10px; }
.tag-empty { display: grid; place-items: center; min-height: 58px; color: #98a2b3; font-size: 13px; border: 1px dashed #d8dde6; border-radius: 8px; background: #fafbfc; }
.form-help { margin: 7px 0 0 12px; color: #98a2b3; font-size: 12px; }
.form-help.standalone { margin: -3px 0 0; }
</style>
