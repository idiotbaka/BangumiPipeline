<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Calendar, Collection, Document, Setting, SwitchButton, Tickets, VideoCamera } from '@element-plus/icons-vue'
import { api } from '../api'
import { session } from '../session'

const route = useRoute()
const router = useRouter()
const activeMenu = computed(() => route.path.startsWith('/anime/') ? '/anime' : route.path)

async function logout() {
  try {
    await api.logout()
  } catch {
    ElMessage.warning('服务端退出失败，本地会话已清除')
  }
  session.user = null
  await router.replace('/login')
}
</script>

<template>
  <el-container class="dashboard-shell">
    <el-aside width="248px" class="sidebar">
      <div class="sidebar-brand"><span>BP</span><strong>BangumiPipeline</strong></div>
      <el-menu :default-active="activeMenu" router class="sidebar-menu">
        <el-menu-item index="/dashboard">
          <el-icon><Collection /></el-icon>
          <span>系统概览</span>
        </el-menu-item>
        <el-menu-item index="/scheduled-tasks">
          <el-icon><Calendar /></el-icon>
          <span>计划任务</span>
        </el-menu-item>
        <el-menu-item index="/anime">
          <el-icon><VideoCamera /></el-icon>
          <span>番剧管理</span>
        </el-menu-item>
        <el-menu-item index="/subscriptions">
          <el-icon><Tickets /></el-icon>
          <span>订阅匹配管理</span>
        </el-menu-item>
        <el-menu-item index="/system-logs">
          <el-icon><Document /></el-icon>
          <span>系统日志</span>
        </el-menu-item>
        <el-menu-item index="/settings">
          <el-icon><Setting /></el-icon>
          <span>系统设置</span>
        </el-menu-item>
      </el-menu>
      <button class="logout-button" type="button" @click="logout">
        <el-icon><SwitchButton /></el-icon>
        退出登录
      </button>
    </el-aside>

    <el-main class="dashboard-main">
      <router-view />
    </el-main>
  </el-container>
</template>
