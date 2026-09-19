<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { api, type ViewerSinglePageConfig } from '../api'

const config = ref<ViewerSinglePageConfig>({ items: [], activeKey: '' })
const loading = ref(false)
const savingKey = ref<string | null>(null)
const errorMessage = ref('')

onMounted(loadPages)

async function loadPages() {
  loading.value = true
  errorMessage.value = ''
  try {
    config.value = await api.viewerSinglePages()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '加载单页面失败'
  } finally {
    loading.value = false
  }
}

async function togglePage(key: string) {
  if (savingKey.value !== null) return
  const nextKey = config.value.activeKey === key ? '' : key
  savingKey.value = key
  try {
    config.value = await api.updateViewerSinglePages(nextKey)
    ElMessage.success(nextKey ? '单页面已启用' : '单页面已停用')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '更新单页面状态失败')
  } finally {
    savingKey.value = null
  }
}
</script>

<template>
  <section class="page-management">
    <header class="page-heading">
      <div>
        <p class="eyebrow">VIEWER / SINGLE PAGES</p>
        <h1>单页面管理</h1>
        <p class="description">选择在观看端导航栏展示的季度专题。每次仅有一个页面可以启用。</p>
      </div>
      <el-button :loading="loading" @click="loadPages"><el-icon><Refresh /></el-icon>刷新</el-button>
    </header>

    <div v-loading="loading" class="page-list">
      <div v-if="errorMessage" class="state error-state">
        <p>{{ errorMessage }}</p>
        <el-button type="primary" @click="loadPages">重新加载</el-button>
      </div>
      <div v-else-if="!loading && config.items.length === 0" class="state">暂无已实现的单页面</div>
      <article v-for="page in config.items" :key="page.key" class="page-card" :class="{ enabled: config.activeKey === page.key }">
        <div class="page-symbol" aria-hidden="true">SP</div>
        <div class="page-info">
          <div class="title-row">
            <h2>{{ page.title }}</h2>
            <el-tag :type="config.activeKey === page.key ? 'success' : 'info'" effect="plain">
              {{ config.activeKey === page.key ? '已启用' : '未启用' }}
            </el-tag>
          </div>
          <p>观看端路径：<code>{{ page.path }}</code></p>
          <small>静态专题内容随前端构建发布；启用后，入口显示在“番剧时间表”右侧。</small>
        </div>
        <el-button
          :type="config.activeKey === page.key ? 'default' : 'primary'"
          :loading="savingKey === page.key"
          :disabled="savingKey !== null && savingKey !== page.key"
          @click="togglePage(page.key)"
        >{{ config.activeKey === page.key ? '停用页面' : '启用页面' }}</el-button>
      </article>
    </div>
  </section>
</template>

<style scoped>
.page-management { max-width: 1120px; }
.page-heading { display: flex; justify-content: space-between; align-items: start; gap: 24px; margin-bottom: 28px; }
.eyebrow { color: #e5548a; font-size: 12px; font-weight: 700; letter-spacing: 2px; }
h1 { margin: 6px 0 8px; font-size: 28px; }
.description { color: #6e7788; line-height: 1.7; }
.page-list { min-height: 160px; display: grid; gap: 16px; }
.page-card { display: flex; align-items: center; gap: 22px; padding: 22px; border: 1px solid #e5e8ee; border-radius: 14px; background: #fff; box-shadow: 0 10px 30px rgba(39, 47, 71, .04); }
.page-card.enabled { border-color: #f4aac7; background: linear-gradient(105deg, #fff7fa, #fff); }
.page-symbol { flex: 0 0 88px; width: 88px; height: 88px; display: grid; place-items: center; color: #ed4d87; font-size: 36px; font-weight: 800; background: #ffe6ef; border-radius: 12px; }
.page-info { flex: 1; min-width: 0; }
.title-row { display: flex; align-items: center; gap: 12px; }
.title-row h2 { font-size: 19px; }
.page-info p { margin: 10px 0 5px; color: #5f6a7f; font-size: 13px; }
.page-info code { color: #bd3d70; }
.page-info small { color: #929aaa; line-height: 1.5; }
.state { padding: 50px 20px; color: #8791a2; text-align: center; background: #fff; border: 1px dashed #dde1e8; border-radius: 12px; }
.error-state p { margin-bottom: 16px; }
</style>
