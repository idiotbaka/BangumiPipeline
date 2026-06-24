<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { Connection, Link } from '@element-plus/icons-vue'
import { api } from '../api'

const formRef = ref<FormInstance>()
const subscriptionFormRef = ref<FormInstance>()
const loading = ref(true)
const saving = ref(false)
const savingSubscription = ref(false)
const updatedAt = ref(0)
const subscriptionUpdatedAt = ref(0)
const form = reactive({ httpProxy: '', httpsProxy: '' })
const subscriptionForm = reactive({ rssUrl: '' })

function validateProxy(_rule: unknown, value: string, callback: (error?: Error) => void) {
  if (!value) {
    callback()
    return
  }
  try {
    const parsed = new URL(value)
    callback(parsed.protocol === 'http:' || parsed.protocol === 'https:' ? undefined : new Error('仅支持 HTTP/HTTPS 代理'))
  } catch {
    callback(new Error('请输入完整地址，例如 http://127.0.0.1:10808'))
  }
}

function validateHTTPURL(_rule: unknown, value: string, callback: (error?: Error) => void) {
  if (!value) {
    callback()
    return
  }
  try {
    const parsed = new URL(value)
    callback(parsed.protocol === 'http:' || parsed.protocol === 'https:' ? undefined : new Error('仅支持 HTTP/HTTPS 地址'))
  } catch {
    callback(new Error('请输入完整地址，例如 https://mikanani.me/RSS/MyBangumi?token=...'))
  }
}

const rules: FormRules<typeof form> = {
  httpProxy: [{ validator: validateProxy, trigger: 'blur' }],
  httpsProxy: [{ validator: validateProxy, trigger: 'blur' }],
}

const subscriptionRules: FormRules<typeof subscriptionForm> = {
  rssUrl: [{ validator: validateHTTPURL, trigger: 'blur' }],
}

async function loadSettings() {
  loading.value = true
  try {
    const [network, subscription] = await Promise.all([
      api.networkSettings(),
      api.subscriptionSettings(),
    ])
    form.httpProxy = network.settings.httpProxy
    form.httpsProxy = network.settings.httpsProxy
    updatedAt.value = network.settings.updatedAt
    subscriptionForm.rssUrl = subscription.settings.rssUrl
    subscriptionUpdatedAt.value = subscription.settings.updatedAt
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '加载设置失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  saving.value = true
  try {
    const { settings } = await api.updateNetworkSettings(form.httpProxy, form.httpsProxy)
    form.httpProxy = settings.httpProxy
    form.httpsProxy = settings.httpsProxy
    updatedAt.value = settings.updatedAt
    ElMessage.success('代理设置已保存')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存设置失败')
  } finally {
    saving.value = false
  }
}

async function saveSubscription() {
  if (!(await subscriptionFormRef.value?.validate().catch(() => false))) return
  savingSubscription.value = true
  try {
    const { settings } = await api.updateSubscriptionSettings(subscriptionForm.rssUrl)
    subscriptionForm.rssUrl = settings.rssUrl
    subscriptionUpdatedAt.value = settings.updatedAt
    ElMessage.success('订阅配置已保存')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存订阅配置失败')
  } finally {
    savingSubscription.value = false
  }
}

function formatTime(timestamp: number) {
  return timestamp ? new Date(timestamp * 1000).toLocaleString() : '尚未保存'
}

onMounted(loadSettings)
</script>

<template>
  <header class="page-header">
    <div>
      <p class="eyebrow">SYSTEM SETTINGS</p>
      <h1>系统设置</h1>
      <p>配置外部请求使用的网络代理和番剧 RSS 订阅源。</p>
    </div>
  </header>

  <el-card class="content-card settings-card" shadow="never" v-loading="loading">
    <template #header>
      <div class="settings-title">
        <div class="task-icon"><el-icon><Connection /></el-icon></div>
        <div>
          <h2>网络代理</h2>
          <p>留空表示对应协议直接连接。</p>
        </div>
      </div>
    </template>

    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="save">
      <el-form-item label="HTTP 代理" prop="httpProxy">
        <el-input v-model.trim="form.httpProxy" size="large" placeholder="http://127.0.0.1:10808" clearable />
        <div class="form-help">用于目标地址为 HTTP 的外部请求。</div>
      </el-form-item>
      <el-form-item label="HTTPS 代理" prop="httpsProxy">
        <el-input v-model.trim="form.httpsProxy" size="large" placeholder="http://127.0.0.1:10808" clearable />
        <div class="form-help">多数 HTTP 代理也使用 http:// 前缀处理 HTTPS CONNECT 请求。</div>
      </el-form-item>
      <div class="settings-actions">
        <span>最后更新：{{ formatTime(updatedAt) }}</span>
        <el-button type="primary" size="large" native-type="submit" :loading="saving">保存设置</el-button>
      </div>
    </el-form>
  </el-card>

  <el-card class="content-card settings-card" shadow="never" v-loading="loading">
    <template #header>
      <div class="settings-title">
        <div class="task-icon"><el-icon><Link /></el-icon></div>
        <div>
          <h2>订阅配置</h2>
          <p>填入外部 RSS 番剧订阅 URL，计划任务会抓取新条目并尝试匹配本地番剧。</p>
        </div>
      </div>
    </template>

    <el-form ref="subscriptionFormRef" :model="subscriptionForm" :rules="subscriptionRules" label-position="top" @submit.prevent="saveSubscription">
      <el-form-item label="RSS 番剧订阅 URL" prop="rssUrl">
        <el-input v-model.trim="subscriptionForm.rssUrl" size="large" placeholder="https://mikanani.me/RSS/MyBangumi?token=..." clearable />
        <div class="form-help">URL 中可能包含私密 token，只会保存在本地数据库；系统日志会自动隐藏查询参数。</div>
      </el-form-item>
      <div class="settings-actions">
        <span>最后更新：{{ formatTime(subscriptionUpdatedAt) }}</span>
        <el-button type="primary" size="large" native-type="submit" :loading="savingSubscription">保存订阅配置</el-button>
      </div>
    </el-form>
  </el-card>
</template>
