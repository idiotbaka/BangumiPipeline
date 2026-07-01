<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Clock, Refresh, Search, User, View as ViewIcon } from '@element-plus/icons-vue'
import { api, type ViewerUser, type ViewerUserActivity } from '../api'

const users = ref<ViewerUser[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const query = ref('')
const loading = ref(false)
const resetDialogVisible = ref(false)
const resetTarget = ref<ViewerUser | null>(null)
const resetPassword = ref('')
const resetting = ref(false)
const activityDialogVisible = ref(false)
const activityTarget = ref<ViewerUser | null>(null)
const activityItems = ref<ViewerUserActivity[]>([])
const activityLoading = ref(false)

onMounted(loadUsers)

async function loadUsers() {
  loading.value = true
  try {
    const result = await api.viewerUsers(page.value, pageSize.value, query.value)
    users.value = result.items
    total.value = result.total
    page.value = result.page
    pageSize.value = result.pageSize
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '加载用户失败')
  } finally {
    loading.value = false
  }
}

function searchUsers() {
  page.value = 1
  void loadUsers()
}

async function toggleUser(user: ViewerUser) {
  const disabled = !user.disabled
  const action = disabled ? '禁用' : '启用'
  try {
    await ElMessageBox.confirm(`确认${action}用户「${user.username}」？`, `${action}用户`, {
      confirmButtonText: action,
      cancelButtonText: '取消',
      type: disabled ? 'warning' : 'info',
    })
    const result = await api.updateViewerUser(user.id, { disabled })
    replaceUser(result.user)
    ElMessage.success(`${action}成功`)
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error(error instanceof Error ? error.message : `${action}失败`)
  }
}

function openResetDialog(user: ViewerUser) {
  resetTarget.value = user
  resetPassword.value = ''
  resetDialogVisible.value = true
}

async function openActivityDialog(user: ViewerUser) {
  activityTarget.value = user
  activityItems.value = []
  activityDialogVisible.value = true
  await loadUserActivities(user)
}

async function loadUserActivities(user = activityTarget.value) {
  if (!user || activityLoading.value) return
  activityLoading.value = true
  try {
    const result = await api.viewerUserActivities(user.id)
    activityItems.value = result.items
    const index = users.value.findIndex((item) => item.id === user.id)
    if (index >= 0) {
      users.value.splice(index, 1, {
        ...users.value[index],
        lastActivity: result.items[0] ?? null,
      })
    }
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '加载用户活动失败')
  } finally {
    activityLoading.value = false
  }
}

async function submitResetPassword() {
  if (!resetTarget.value || resetting.value) return
  if (resetPassword.value.length < 10 || resetPassword.value.length > 128) {
    ElMessage.warning('新密码需要 10 到 128 个字符')
    return
  }
  resetting.value = true
  try {
    const result = await api.resetViewerUserPassword(resetTarget.value.id, resetPassword.value)
    replaceUser(result.user)
    resetDialogVisible.value = false
    ElMessage.success('密码已重置')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '重置密码失败')
  } finally {
    resetting.value = false
  }
}

function replaceUser(nextUser: ViewerUser) {
  const index = users.value.findIndex((item) => item.id === nextUser.id)
  if (index >= 0) {
    users.value.splice(index, 1, {
      ...nextUser,
      lastActivity: nextUser.lastActivity ?? users.value[index].lastActivity ?? null,
    })
  }
}

function formatDate(value: number | null | undefined) {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

function activityEpisodeText(item: ViewerUserActivity) {
  return item.episodeTitle ? `${item.episodeLabel}「${item.episodeTitle}」` : item.episodeLabel
}

function activitySummary(item: ViewerUserActivity | null | undefined) {
  if (!item) return '暂无观看记录'
  return `于 ${formatDate(item.lastWatchedAt)} 观看「${item.animeTitle}」的 ${activityEpisodeText(item)}`
}

function activityProgressText(item: ViewerUserActivity) {
  if (item.completed) return '已看完'
  if (item.durationSeconds <= 0) return `看到 ${item.progressPercent}%`
  return `看到 ${formatDuration(item.positionSeconds)} / ${formatDuration(item.durationSeconds)}`
}

function activityTotalText(item: ViewerUserActivity) {
  const total = item.totalEpisodes > 0 ? `全 ${item.totalEpisodes} 话` : '话数未定'
  const latest = item.latestEpisodeLabel ? `更新至 ${item.latestEpisodeLabel}` : '更新话数未知'
  return `${total} · ${latest}`
}

function formatDuration(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '00:00'
  const totalSeconds = Math.floor(value)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const paddedMinutes = String(minutes).padStart(2, '0')
  const paddedSeconds = String(seconds).padStart(2, '0')
  if (hours > 0) return `${hours}:${paddedMinutes}:${paddedSeconds}`
  return `${paddedMinutes}:${paddedSeconds}`
}
</script>

<template>
  <section>
    <header class="page-header">
      <div>
        <p class="eyebrow">FRONTEND</p>
        <h1>用户管理</h1>
        <p>管理观看端注册用户，支持用户名搜索、禁用账号和重置密码。</p>
      </div>
      <div class="page-header-actions">
        <el-button :icon="Refresh" :loading="loading" @click="loadUsers">刷新</el-button>
      </div>
    </header>

    <div class="management-toolbar">
      <el-input
        v-model="query"
        clearable
        placeholder="搜索用户名"
        size="large"
        @clear="searchUsers"
        @keyup.enter="searchUsers"
      >
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-button type="primary" size="large" :icon="Search" @click="searchUsers">搜索</el-button>
    </div>

    <el-card class="content-card management-card" shadow="never">
      <el-table v-loading="loading" :data="users" empty-text="暂无注册用户" class="management-table">
        <el-table-column width="72" label="ID" prop="id" />
        <el-table-column min-width="220" label="用户名">
          <template #default="{ row }">
            <div class="viewer-user-cell">
              <el-icon><User /></el-icon>
              <strong>{{ row.username }}</strong>
            </div>
          </template>
        </el-table-column>
        <el-table-column width="120" label="状态">
          <template #default="{ row }">
            <el-tag :type="row.disabled ? 'danger' : 'success'" effect="light">
              {{ row.disabled ? '已禁用' : '正常' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column min-width="190" label="注册时间">
          <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column min-width="360" label="上次活跃时间">
          <template #default="{ row }">
            <div class="last-activity-cell" :class="{ empty: !row.lastActivity }">
              <el-icon><Clock /></el-icon>
              <span>{{ activitySummary(row.lastActivity) }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column min-width="190" label="禁用时间">
          <template #default="{ row }">{{ formatDate(row.disabledAt) }}</template>
        </el-table-column>
        <el-table-column width="310" label="操作" fixed="right">
          <template #default="{ row }">
            <div class="table-actions">
              <el-button size="small" :icon="ViewIcon" @click="openActivityDialog(row)">查看活动</el-button>
              <el-button size="small" :type="row.disabled ? 'success' : 'warning'" @click="toggleUser(row)">
                {{ row.disabled ? '启用' : '禁用' }}
              </el-button>
              <el-button size="small" @click="openResetDialog(row)">重置密码</el-button>
            </div>
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
      @current-change="loadUsers"
      @size-change="searchUsers"
    />

    <el-dialog v-model="resetDialogVisible" title="重置密码" width="440px">
      <div class="reset-password-dialog">
        <p>为用户「{{ resetTarget?.username }}」设置一个新密码。</p>
        <el-input
          v-model="resetPassword"
          autofocus
          maxlength="128"
          minlength="10"
          placeholder="输入 10 到 128 个字符的新密码"
          show-password
          type="password"
          @keyup.enter="submitResetPassword"
        />
      </div>
      <template #footer>
        <el-button @click="resetDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="resetting" @click="submitResetPassword">确认重置</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="activityDialogVisible"
      :title="activityTarget ? `${activityTarget.username} 的活动记录` : '查看活动'"
      width="760px"
    >
      <div v-loading="activityLoading" class="activity-dialog">
        <el-empty v-if="!activityLoading && activityItems.length === 0" description="暂无观看记录" />
        <el-timeline v-else>
          <el-timeline-item
            v-for="item in activityItems"
            :key="`${item.bangumiId}-${item.mediaId}`"
            :timestamp="formatDate(item.lastWatchedAt)"
            placement="top"
          >
            <article class="activity-item">
              <div class="activity-title-line">
                <strong :title="item.animeTitle">{{ item.animeTitle }}</strong>
                <el-tag :type="item.completed ? 'success' : 'primary'" effect="light" size="small">
                  {{ item.completed ? '已看完' : `${item.progressPercent}%` }}
                </el-tag>
              </div>
              <p>{{ activityEpisodeText(item) }}</p>
              <div class="activity-meta">
                <span>{{ activityTotalText(item) }}</span>
                <span>{{ activityProgressText(item) }}</span>
              </div>
              <el-progress :percentage="item.progressPercent" :stroke-width="7" :show-text="false" />
            </article>
          </el-timeline-item>
        </el-timeline>
      </div>
      <template #footer>
        <el-button :loading="activityLoading" @click="loadUserActivities()">刷新</el-button>
        <el-button type="primary" @click="activityDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.last-activity-cell {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  min-width: 0;
  color: #344054;
  line-height: 1.5;
}

.last-activity-cell.empty {
  color: #98a2b3;
}

.last-activity-cell .el-icon {
  flex: none;
  margin-top: 2px;
  color: #3867ff;
}

.last-activity-cell.empty .el-icon {
  color: #c8ced8;
}

.last-activity-cell span {
  min-width: 0;
  overflow-wrap: anywhere;
}

.activity-dialog {
  min-height: 220px;
  max-height: 62vh;
  overflow: auto;
  padding: 4px 8px 4px 0;
}

.activity-dialog :deep(.el-timeline) {
  padding-left: 8px;
}

.activity-item {
  display: grid;
  gap: 9px;
  padding: 14px 16px;
  border: 1px solid #e4e8ef;
  border-radius: 8px;
  background: #f8fafc;
}

.activity-title-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}

.activity-title-line strong {
  min-width: 0;
  overflow: hidden;
  color: #101828;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.activity-item p {
  margin: 0;
  color: #475467;
  font-size: 13px;
  line-height: 1.5;
}

.activity-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  color: #98a2b3;
  font-size: 12px;
}

.activity-meta span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
