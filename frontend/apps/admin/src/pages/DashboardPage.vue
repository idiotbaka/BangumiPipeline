<script setup lang="ts">
import { Calendar, Collection, Download, Film, Setting, VideoCamera } from '@element-plus/icons-vue'
import { session } from '../session'

const modules = [
  { title: '资源采集', description: 'RSS 与 Bangumi 元数据采集已接入', icon: Collection, status: '已接入' },
  { title: '下载管理', description: 'qBittorrent 创建、同步和失败重试已接入', icon: Download, status: '已接入' },
  { title: '媒体整理', description: '下载产物识别、命名和入库已接入', icon: Film, status: '已接入' },
  { title: '转码任务', description: 'FFmpeg 探测、转码和字幕压制已接入', icon: VideoCamera, status: '已接入' },
  { title: '计划任务', description: '固定间隔调度、手动执行与运行状态已接入', icon: Calendar, status: '已接入' },
  { title: '系统设置', description: '网络、订阅和下载配置已接入', icon: Setting, status: '已接入' },
]
</script>

<template>
  <header class="page-header">
    <div>
      <p class="eyebrow">MANAGEMENT CONSOLE</p>
      <h1>系统概览</h1>
      <p>基础脚手架已启动，业务模块将在后续迭代中接入。</p>
    </div>
    <el-tag size="large" type="success" effect="light">服务正常</el-tag>
  </header>

  <el-alert title="管理员登录与 SQLite 会话已启用" type="success" :closable="false" show-icon>
    当前用户：{{ session.user?.username }}
  </el-alert>

  <section class="module-grid">
    <el-card v-for="module in modules" :key="module.title" shadow="hover" class="module-card">
      <el-icon class="module-icon"><component :is="module.icon" /></el-icon>
      <h2>{{ module.title }}</h2>
      <p>{{ module.description }}</p>
      <el-tag :type="module.status === '已接入' ? 'success' : 'info'" effect="plain">{{ module.status }}</el-tag>
    </el-card>
  </section>
</template>
