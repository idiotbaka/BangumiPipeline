<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { Connection, Delete, Download, FolderOpened, Link, Plus } from '@element-plus/icons-vue'
import { api } from '../api'

const formRef = ref<FormInstance>()
const llmFormRef = ref<FormInstance>()
const subscriptionFormRef = ref<FormInstance>()
const downloadFormRef = ref<FormInstance>()
const loading = ref(true)
const saving = ref(false)
const savingLLM = ref(false)
const savingSubscription = ref(false)
const savingDownload = ref(false)
const savingStorage = ref(false)
const testingDownload = ref(false)
const testingLLM = ref(false)
const updatedAt = ref(0)
const llmUpdatedAt = ref(0)
const subscriptionUpdatedAt = ref(0)
const downloadUpdatedAt = ref(0)
const storageUpdatedAt = ref(0)
const defaultMediaRoot = ref('')
const form = reactive({ httpProxy: '', httpsProxy: '' })
const llmForm = reactive({ baseUrl: '', apiKey: '', model: '' })
const subscriptionForm = reactive({ rssUrl: '' })
const downloadForm = reactive({
  host: '127.0.0.1',
  port: 8080,
  username: '',
  password: '',
  maxConcurrentDownloads: 2,
})
const storageForm = reactive({
  extraRoots: [] as string[],
})

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

function validateHost(_rule: unknown, value: string, callback: (error?: Error) => void) {
  if (!value.trim()) {
    callback(new Error('请输入 qBittorrent WebUI IP 地址'))
    return
  }
  if (/[\\/]/.test(value)) {
    callback(new Error('只填写 IP 或主机名，不包含 http:// 和路径'))
    return
  }
  callback()
}

const rules: FormRules<typeof form> = {
  httpProxy: [{ validator: validateProxy, trigger: 'blur' }],
  httpsProxy: [{ validator: validateProxy, trigger: 'blur' }],
}

const subscriptionRules: FormRules<typeof subscriptionForm> = {
  rssUrl: [{ validator: validateHTTPURL, trigger: 'blur' }],
}

const llmRules: FormRules<typeof llmForm> = {
  baseUrl: [{ validator: validateHTTPURL, trigger: 'blur' }],
}

const downloadRules: FormRules<typeof downloadForm> = {
  host: [{ validator: validateHost, trigger: 'blur' }],
  port: [{ type: 'number', min: 1, max: 65535, message: '端口号必须在 1 到 65535 之间', trigger: 'change' }],
  maxConcurrentDownloads: [{ type: 'number', min: 1, max: 50, message: '并发下载数必须在 1 到 50 之间', trigger: 'change' }],
}

async function loadSettings() {
  loading.value = true
  try {
    const [network, llm, subscription, download, storage] = await Promise.all([
      api.networkSettings(),
      api.llmSettings(),
      api.subscriptionSettings(),
      api.downloadSettings(),
      api.mediaStorageSettings(),
    ])
    form.httpProxy = network.settings.httpProxy
    form.httpsProxy = network.settings.httpsProxy
    updatedAt.value = network.settings.updatedAt
    llmForm.baseUrl = llm.settings.baseUrl
    llmForm.apiKey = llm.settings.apiKey
    llmForm.model = llm.settings.model
    llmUpdatedAt.value = llm.settings.updatedAt
    subscriptionForm.rssUrl = subscription.settings.rssUrl
    subscriptionUpdatedAt.value = subscription.settings.updatedAt
    downloadForm.host = download.settings.host
    downloadForm.port = download.settings.port
    downloadForm.username = download.settings.username
    downloadForm.password = download.settings.password
    downloadForm.maxConcurrentDownloads = download.settings.maxConcurrentDownloads
    downloadUpdatedAt.value = download.settings.updatedAt
    defaultMediaRoot.value = storage.settings.defaultRoot
    storageForm.extraRoots = [...storage.settings.extraRoots]
    storageUpdatedAt.value = storage.settings.updatedAt
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '加载设置失败')
  } finally {
    loading.value = false
  }
}

function addStorageRoot() {
  storageForm.extraRoots.push('')
}

function removeStorageRoot(index: number) {
  storageForm.extraRoots.splice(index, 1)
}

function storagePayload() {
  return storageForm.extraRoots.map((path) => path.trim()).filter(Boolean)
}

async function saveStorage() {
  savingStorage.value = true
  try {
    const { settings } = await api.updateMediaStorageSettings(storagePayload())
    defaultMediaRoot.value = settings.defaultRoot
    storageForm.extraRoots = [...settings.extraRoots]
    storageUpdatedAt.value = settings.updatedAt
    ElMessage.success('额外磁盘存储路径已保存')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存存储路径失败')
  } finally {
    savingStorage.value = false
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

function llmPayload() {
  return {
    baseUrl: llmForm.baseUrl.trim(),
    apiKey: llmForm.apiKey.trim(),
    model: llmForm.model.trim(),
  }
}

async function saveLLM() {
  if (!(await llmFormRef.value?.validate().catch(() => false))) return
  savingLLM.value = true
  try {
    const { settings } = await api.updateLLMSettings(llmPayload())
    llmForm.baseUrl = settings.baseUrl
    llmForm.apiKey = settings.apiKey
    llmForm.model = settings.model
    llmUpdatedAt.value = settings.updatedAt
    ElMessage.success('LLM 设置已保存')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存 LLM 设置失败')
  } finally {
    savingLLM.value = false
  }
}

async function testLLM() {
  if (!(await llmFormRef.value?.validate().catch(() => false))) return
  testingLLM.value = true
  try {
    await api.testLLMSettings(llmPayload())
    ElMessage.success('LLM 连接正常：OK')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '测试 LLM 连接失败')
  } finally {
    testingLLM.value = false
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

function downloadPayload() {
  return {
    host: downloadForm.host.trim(),
    port: downloadForm.port,
    username: downloadForm.username.trim(),
    password: downloadForm.password,
    maxConcurrentDownloads: downloadForm.maxConcurrentDownloads,
  }
}

async function saveDownload() {
  if (!(await downloadFormRef.value?.validate().catch(() => false))) return
  savingDownload.value = true
  try {
    const { settings } = await api.updateDownloadSettings(downloadPayload())
    downloadForm.host = settings.host
    downloadForm.port = settings.port
    downloadForm.username = settings.username
    downloadForm.password = settings.password
    downloadForm.maxConcurrentDownloads = settings.maxConcurrentDownloads
    downloadUpdatedAt.value = settings.updatedAt
    ElMessage.success('下载设置已保存')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存下载设置失败')
  } finally {
    savingDownload.value = false
  }
}

async function testDownload() {
  if (!(await downloadFormRef.value?.validate().catch(() => false))) return
  testingDownload.value = true
  try {
    const { result } = await api.testDownloadSettings(downloadPayload())
    ElMessage.success(`qBittorrent 连接正常：${result.version}`)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '测试连接失败')
  } finally {
    testingDownload.value = false
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
      <p>配置外部请求使用的网络代理、下载器和番剧 RSS 订阅源。</p>
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
        <div class="task-icon"><el-icon><Connection /></el-icon></div>
        <div>
          <h2>LLM 翻译设置</h2>
          <p>用于翻译番剧简介、分集信息、角色和声优简介。</p>
        </div>
      </div>
    </template>

    <el-form ref="llmFormRef" :model="llmForm" :rules="llmRules" label-position="top" @submit.prevent="saveLLM">
      <el-form-item label="Base URL" prop="baseUrl">
        <el-input v-model.trim="llmForm.baseUrl" size="large" placeholder="https://api.openai.com/v1" clearable />
        <div class="form-help">填写 OpenAI Chat 兼容接口的 v1 根地址，系统会调用 /chat/completions。</div>
      </el-form-item>
      <el-form-item label="API KEY">
        <el-input v-model.trim="llmForm.apiKey" size="large" type="password" show-password placeholder="sk-..." clearable />
      </el-form-item>
      <el-form-item label="模型名称">
        <el-input v-model.trim="llmForm.model" size="large" placeholder="gpt-4o-mini" clearable />
        <div class="form-help">计划任务需要 Base URL 和模型名称有效；API KEY 可按服务要求填写。</div>
      </el-form-item>
      <div class="settings-actions">
        <span>最后更新：{{ formatTime(llmUpdatedAt) }}</span>
        <div class="settings-button-row">
          <el-button size="large" :loading="testingLLM" @click="testLLM">测试连接</el-button>
          <el-button type="primary" size="large" native-type="submit" :loading="savingLLM">保存 LLM 设置</el-button>
        </div>
      </div>
    </el-form>
  </el-card>

  <el-card class="content-card settings-card" shadow="never" v-loading="loading">
    <template #header>
      <div class="settings-title">
        <div class="task-icon"><el-icon><FolderOpened /></el-icon></div>
        <div>
          <h2>额外磁盘存储路径</h2>
          <p>配置可用于存放成品视频的其他磁盘根目录。</p>
        </div>
      </div>
    </template>

    <div class="storage-settings-form">
      <el-form-item label="默认媒体目录">
        <el-input :model-value="defaultMediaRoot" size="large" disabled />
        <div class="form-help">未移动存储路径的番剧会继续使用该目录。</div>
      </el-form-item>

      <div class="storage-root-list">
        <div v-for="(_root, index) in storageForm.extraRoots" :key="index" class="storage-root-row">
          <el-input v-model.trim="storageForm.extraRoots[index]" size="large" placeholder="E:\media 或 /opt/drive2" clearable />
          <el-button :icon="Delete" type="danger" plain @click="removeStorageRoot(index)" />
        </div>
      </div>
      <el-button :icon="Plus" plain @click="addStorageRoot">添加路径</el-button>
      <div class="form-help">路径必须是运行服务器上的绝对路径；移动番剧时只能选择默认目录或这里配置的目录。</div>
      <div class="settings-actions">
        <span>最后更新：{{ formatTime(storageUpdatedAt) }}</span>
        <el-button type="primary" size="large" :loading="savingStorage" @click="saveStorage">保存存储路径</el-button>
      </div>
    </div>
  </el-card>

  <el-card class="content-card settings-card" shadow="never" v-loading="loading">
    <template #header>
      <div class="settings-title">
        <div class="task-icon"><el-icon><Download /></el-icon></div>
        <div>
          <h2>下载设置</h2>
          <p>连接本机 qBittorrent WebUI，并控制自动下载并发数。</p>
        </div>
      </div>
    </template>

    <el-form ref="downloadFormRef" :model="downloadForm" :rules="downloadRules" label-position="top" @submit.prevent="saveDownload">
      <div class="download-settings-grid">
        <el-form-item label="qBittorrent IP 地址" prop="host">
          <el-input v-model.trim="downloadForm.host" size="large" placeholder="127.0.0.1" clearable />
        </el-form-item>
        <el-form-item label="端口号" prop="port">
          <el-input-number v-model="downloadForm.port" :min="1" :max="65535" size="large" controls-position="right" />
        </el-form-item>
      </div>
      <div class="download-settings-grid">
        <el-form-item label="用户名">
          <el-input v-model.trim="downloadForm.username" size="large" placeholder="admin" clearable />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="downloadForm.password" size="large" type="password" show-password placeholder="123456" clearable />
        </el-form-item>
      </div>
      <el-form-item label="并发下载数" prop="maxConcurrentDownloads">
        <el-input-number v-model="downloadForm.maxConcurrentDownloads" :min="1" :max="50" size="large" controls-position="right" />
        <div class="form-help">计划任务只会在当前下载中任务少于该数量时创建新的 qBittorrent 下载。</div>
      </el-form-item>
      <div class="settings-actions">
        <span>最后更新：{{ formatTime(downloadUpdatedAt) }}</span>
        <div class="settings-button-row">
          <el-button size="large" :loading="testingDownload" @click="testDownload">测试连接</el-button>
          <el-button type="primary" size="large" native-type="submit" :loading="savingDownload">保存下载设置</el-button>
        </div>
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
