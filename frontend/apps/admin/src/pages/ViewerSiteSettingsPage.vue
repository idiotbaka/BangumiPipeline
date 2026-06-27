<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, UploadFilled } from '@element-plus/icons-vue'
import { api, type ViewerSiteSettings } from '../api'

const settings = ref<ViewerSiteSettings | null>(null)
const siteName = ref('')
const registrationEnabled = ref(true)
const inviteRequired = ref(false)
const loading = ref(false)
const saving = ref(false)
const savingRegistration = ref(false)
const uploading = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const faviconURL = computed(() => {
  if (!settings.value?.hasFavicon) return ''
  const version = settings.value.faviconUpdatedAt ?? settings.value.updatedAt
  return `/api/viewer/site-settings/favicon?v=${version}`
})

onMounted(loadSettings)

async function loadSettings() {
  loading.value = true
  try {
    const result = await api.viewerSiteSettings()
    settings.value = result.settings
    syncForm(result.settings)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '加载网站设置失败')
  } finally {
    loading.value = false
  }
}

async function saveSiteName() {
  if (saving.value) return
  saving.value = true
  try {
    const result = await api.updateViewerSiteSettings({
      siteName: siteName.value,
      registrationEnabled: registrationEnabled.value,
      inviteRequired: inviteRequired.value,
    })
    settings.value = result.settings
    syncForm(result.settings)
    ElMessage.success('网站名称已保存')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存网站名称失败')
  } finally {
    saving.value = false
  }
}

async function saveRegistrationSettings() {
  if (savingRegistration.value) return
  savingRegistration.value = true
  try {
    const result = await api.updateViewerSiteSettings({
      siteName: siteName.value,
      registrationEnabled: registrationEnabled.value,
      inviteRequired: inviteRequired.value,
    })
    settings.value = result.settings
    syncForm(result.settings)
    ElMessage.success('注册设置已保存')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存注册设置失败')
  } finally {
    savingRegistration.value = false
  }
}

function syncForm(nextSettings: ViewerSiteSettings) {
  siteName.value = nextSettings.siteName
  registrationEnabled.value = nextSettings.registrationEnabled
  inviteRequired.value = nextSettings.inviteRequired
}

function chooseFavicon() {
  fileInput.value?.click()
}

async function uploadFavicon(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (file.type && file.type !== 'image/png') {
    ElMessage.warning('请上传 PNG 文件')
    return
  }
  if (!file.name.toLowerCase().endsWith('.png')) {
    ElMessage.warning('favicon 文件需要是 .png')
    return
  }
  if (file.size > 1024 * 1024) {
    ElMessage.warning('favicon 不能超过 1MiB')
    return
  }
  uploading.value = true
  try {
    const result = await api.uploadViewerFavicon(file)
    settings.value = result.settings
    ElMessage.success('favicon 已上传')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '上传 favicon 失败')
  } finally {
    uploading.value = false
  }
}

function formatDate(value: number | null | undefined) {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}
</script>

<template>
  <section>
    <header class="page-header">
      <div>
        <p class="eyebrow">FRONTEND</p>
        <h1>网站设置</h1>
        <p>调整观看端展示名称和浏览器小图标。</p>
      </div>
      <div class="page-header-actions">
        <el-button :icon="Refresh" :loading="loading" @click="loadSettings">刷新</el-button>
      </div>
    </header>

    <div v-loading="loading" class="site-settings-grid">
      <el-card class="settings-card" shadow="never">
        <template #header>
          <div class="settings-title">
            <el-icon class="module-icon"><UploadFilled /></el-icon>
            <div>
              <h2>网站名称</h2>
              <p>用于观看端 title、登录页和顶部品牌展示。</p>
            </div>
          </div>
        </template>
        <el-form label-position="top">
          <el-form-item label="网站名称">
            <el-input v-model="siteName" maxlength="80" show-word-limit placeholder="BangumiPipeline Viewer" />
            <p class="form-help">名称需要 1 到 80 个可显示字符。</p>
          </el-form-item>
          <div class="settings-actions">
            <span>最后更新：{{ formatDate(settings?.updatedAt) }}</span>
            <el-button type="primary" :loading="saving" @click="saveSiteName">保存名称</el-button>
          </div>
        </el-form>
      </el-card>

      <el-card class="settings-card" shadow="never">
        <template #header>
          <div class="settings-title">
            <el-icon class="module-icon"><UploadFilled /></el-icon>
            <div>
              <h2>注册设置</h2>
              <p>控制观看端是否开放新用户注册，以及是否需要邀请码。</p>
            </div>
          </div>
        </template>
        <div class="registration-settings-form">
          <div class="settings-switch-row">
            <div>
              <strong>开放注册</strong>
              <p>关闭后，观看端注册接口会拒绝创建新账号。</p>
            </div>
            <el-switch v-model="registrationEnabled" />
          </div>
          <div class="settings-switch-row">
            <div>
              <strong>需要邀请码才能注册</strong>
              <p>开启后，注册表单会要求填写未使用的邀请码。</p>
            </div>
            <el-switch v-model="inviteRequired" />
          </div>
          <div class="settings-actions">
            <span>最后更新：{{ formatDate(settings?.updatedAt) }}</span>
            <el-button type="primary" :loading="savingRegistration" @click="saveRegistrationSettings">保存注册设置</el-button>
          </div>
        </div>
      </el-card>

      <el-card class="settings-card" shadow="never">
        <template #header>
          <div class="settings-title">
            <el-icon class="module-icon"><UploadFilled /></el-icon>
            <div>
              <h2>网站小图标</h2>
              <p>上传 favicon.png 后，观看端会从本地服务读取。</p>
            </div>
          </div>
        </template>
        <div class="favicon-settings">
          <div class="favicon-preview">
            <img v-if="faviconURL" :src="faviconURL" alt="favicon preview" />
            <span v-else>PNG</span>
          </div>
          <div class="favicon-copy">
            <strong>{{ settings?.hasFavicon ? '已设置 favicon.png' : '尚未设置 favicon.png' }}</strong>
            <p>只接受 PNG 文件，大小不超过 1MiB。</p>
            <small>最后更新：{{ formatDate(settings?.faviconUpdatedAt) }}</small>
          </div>
          <el-button type="primary" :loading="uploading" @click="chooseFavicon">上传 favicon.png</el-button>
          <input ref="fileInput" class="hidden-file-input" accept="image/png,.png" type="file" @change="uploadFavicon" />
        </div>
      </el-card>
    </div>
  </section>
</template>
