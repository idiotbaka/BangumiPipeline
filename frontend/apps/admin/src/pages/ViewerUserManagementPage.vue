<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Search, User } from '@element-plus/icons-vue'
import { api, type ViewerUser } from '../api'

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
    users.value.splice(index, 1, nextUser)
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
        <el-table-column min-width="190" label="禁用时间">
          <template #default="{ row }">{{ formatDate(row.disabledAt) }}</template>
        </el-table-column>
        <el-table-column width="220" label="操作" fixed="right">
          <template #default="{ row }">
            <div class="table-actions">
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
  </section>
</template>
