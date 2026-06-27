<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, Plus, Refresh, Tickets } from '@element-plus/icons-vue'
import { api, type ViewerInvite } from '../api'

const invites = ref<ViewerInvite[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const loading = ref(false)
const generating = ref(false)

onMounted(loadInvites)

async function loadInvites() {
  loading.value = true
  try {
    const result = await api.viewerInvites(page.value, pageSize.value)
    invites.value = result.items
    total.value = result.total
    page.value = result.page
    pageSize.value = result.pageSize
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '加载邀请码失败')
  } finally {
    loading.value = false
  }
}

async function generateInvite() {
  if (generating.value) return
  generating.value = true
  try {
    const result = await api.generateViewerInvite()
    page.value = 1
    await loadInvites()
    ElMessage.success(`邀请码已生成：${result.invite.code}`)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '生成邀请码失败')
  } finally {
    generating.value = false
  }
}

async function copyInvite(invite: ViewerInvite) {
  try {
    await navigator.clipboard.writeText(invite.code)
    ElMessage.success('邀请码已复制')
  } catch {
    ElMessage.warning('当前浏览器不允许自动复制')
  }
}

function formatDate(value: number | null) {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}
</script>

<template>
  <section>
    <header class="page-header">
      <div>
        <p class="eyebrow">FRONTEND</p>
        <h1>邀请码管理</h1>
        <p>生成并管理观看端注册邀请码，已使用的邀请码会显示使用用户。</p>
      </div>
      <div class="page-header-actions">
        <el-button :icon="Refresh" :loading="loading" @click="loadInvites">刷新</el-button>
        <el-button type="primary" :icon="Plus" :loading="generating" @click="generateInvite">生成邀请码</el-button>
      </div>
    </header>

    <el-card class="content-card management-card" shadow="never">
      <el-table v-loading="loading" :data="invites" empty-text="暂无邀请码" class="management-table">
        <el-table-column min-width="240" label="邀请码">
          <template #default="{ row }">
            <div class="invite-code-cell">
              <el-icon><Tickets /></el-icon>
              <code>{{ row.code }}</code>
            </div>
          </template>
        </el-table-column>
        <el-table-column width="120" label="状态">
          <template #default="{ row }">
            <el-tag :type="row.used ? 'info' : 'success'" effect="light">
              {{ row.used ? '已使用' : '未使用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column min-width="180" label="使用用户">
          <template #default="{ row }">
            <span>{{ row.usedByUsername || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column min-width="190" label="生成时间">
          <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column min-width="190" label="使用时间">
          <template #default="{ row }">{{ formatDate(row.usedAt) }}</template>
        </el-table-column>
        <el-table-column width="120" label="操作" fixed="right">
          <template #default="{ row }">
            <el-button size="small" :icon="CopyDocument" @click="copyInvite(row)">复制</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-pagination
      v-if="total > pageSize"
      v-model:current-page="page"
      v-model:page-size="pageSize"
      class="anime-pagination"
      layout="prev, pager, next, sizes, total"
      :page-sizes="[20, 50, 100]"
      :total="total"
      @current-change="loadInvites"
      @size-change="loadInvites"
    />
  </section>
</template>
