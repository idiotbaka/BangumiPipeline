<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { APIError, api, configureAPI, currentAPIBaseURL, type SiteSettings, type ViewerUser } from './api'
import { loadAppConfig, saveAPIBaseURL } from './config'
import MobileShell from './components/MobileShell.vue'
import charaImage from '../../viewer/src/assets/chara.png'

const appName = 'BakaVip 2.0'

const ready = ref(false)
const loading = ref(false)
const checkingServer = ref(false)
const user = ref<ViewerUser | null>(null)
const mode = ref<'login' | 'register'>('login')
const apiBaseUrl = ref('https://baka.vip/')
const registrationEnabled = ref(true)
const inviteRequired = ref(false)
const username = ref('')
const password = ref('')
const confirmPassword = ref('')
const inviteCode = ref('')
const message = ref('')
const serverMessage = ref('')

const formTitle = computed(() => (mode.value === 'login' ? '欢迎回来' : '注册并进入'))
const formSubtitle = computed(() => (mode.value === 'login' ? '登录后继续观看番剧' : '创建账号后会自动登录'))
const submitLabel = computed(() => {
  if (loading.value) {
    return mode.value === 'login' ? '登录中' : '注册中'
  }
  return mode.value === 'login' ? '登录' : '注册并进入'
})
const submitDisabled = computed(() => loading.value || (mode.value === 'register' && !registrationEnabled.value))

onMounted(async () => {
  document.title = appName
  const config = await loadAppConfig()
  apiBaseUrl.value = config.apiBaseUrl
  configureAPI(config.apiBaseUrl)
  await refreshSiteSettings(false)
  try {
    const result = await api.me()
    user.value = result.user
  } catch (error) {
    if (!(error instanceof APIError) || error.status !== 401) {
      message.value = error instanceof Error ? error.message : '无法连接服务器'
    }
  } finally {
    ready.value = true
  }
})

function switchMode(nextMode: 'login' | 'register') {
  if (loading.value) {
    return
  }
  if (nextMode === 'register' && !registrationEnabled.value) {
    message.value = '当前暂未开放注册'
    return
  }
  mode.value = nextMode
  message.value = ''
  serverMessage.value = ''
  password.value = ''
  confirmPassword.value = ''
  inviteCode.value = ''
}

async function submit() {
  if (loading.value) {
    return
  }
  message.value = ''
  serverMessage.value = ''
  if (mode.value === 'register' && !registrationEnabled.value) {
    message.value = '当前暂未开放注册'
    return
  }
  if (mode.value === 'register' && password.value !== confirmPassword.value) {
    message.value = '两次输入的密码不一致'
    return
  }
  if (mode.value === 'register' && inviteRequired.value && inviteCode.value.trim() === '') {
    message.value = '请填写邀请码'
    return
  }

  loading.value = true
  try {
    saveAndApplyAPIBaseURL()
    const result =
      mode.value === 'login'
        ? await api.login(username.value, password.value)
        : await api.register(username.value, password.value, inviteCode.value)
    password.value = ''
    confirmPassword.value = ''
    inviteCode.value = ''
    user.value = result.user
  } catch (error) {
    message.value = error instanceof Error ? error.message : '请求失败'
  } finally {
    loading.value = false
  }
}

async function refreshSiteSettings(showResult = true) {
  checkingServer.value = true
  if (showResult) {
    serverMessage.value = ''
  }
  try {
    saveAndApplyAPIBaseURL()
    const result = await api.siteSettings()
    applySiteSettings(result.settings)
    if (showResult) {
      serverMessage.value = '服务器已连接'
    }
  } catch (error) {
    if (showResult) {
      serverMessage.value = error instanceof Error ? error.message : '无法连接服务器'
    }
  } finally {
    checkingServer.value = false
  }
}

async function logout() {
  if (loading.value) {
    return
  }
  loading.value = true
  message.value = ''
  try {
    await api.logout()
    user.value = null
    password.value = ''
  } catch (error) {
    message.value = error instanceof Error ? error.message : '退出失败'
  } finally {
    loading.value = false
  }
}

function applySiteSettings(settings: SiteSettings) {
  registrationEnabled.value = settings.registrationEnabled
  inviteRequired.value = settings.inviteRequired
  document.title = appName
  const existing = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!settings.hasFavicon) {
    existing?.remove()
    return
  }
  const link = existing ?? document.createElement('link')
  link.rel = 'icon'
  link.type = 'image/png'
  link.href = `${currentAPIBaseURL()}favicon.png?v=${settings.faviconUpdatedAt ?? settings.updatedAt}`
  if (!existing) {
    document.head.appendChild(link)
  }
}

function saveAndApplyAPIBaseURL() {
  saveAPIBaseURL(apiBaseUrl.value)
  configureAPI(apiBaseUrl.value)
  apiBaseUrl.value = currentAPIBaseURL()
}
</script>

<template>
  <main v-if="!ready" class="boot-screen">
    <div class="boot-grid" aria-hidden="true" />
    <div class="boot-float-card card-one" aria-hidden="true">
      <span />
      <i />
    </div>
    <div class="boot-float-card card-two" aria-hidden="true">
      <span />
      <i />
      <em />
    </div>
    <section class="boot-status" aria-label="应用加载中">
      <span class="boot-kicker">MOBILE VIEWER</span>
      <p class="boot-title">{{ appName }}</p>
      <div class="boot-line"><span /></div>
      <p>LOADING</p>
    </section>
    <img class="boot-chara" :src="charaImage" alt="" draggable="false" />
  </main>

  <MobileShell v-else-if="user" :user="user" :loading="loading" @logout="logout" />

  <main v-else class="auth-screen">
    <div class="grid-layer" aria-hidden="true" />
    <section class="hero-panel" aria-label="站点视觉">
      <div class="brand-block">
        <span class="brand-kicker">MOBILE VIEWER</span>
        <p class="brand-title">{{ appName }}</p>
      </div>
      <div class="visual-card" aria-hidden="true">
        <div class="plate plate-a" />
        <div class="plate plate-b" />
        <div class="float-card float-card-a">
          <span />
          <i />
          <em />
        </div>
        <div class="float-card float-card-b">
          <span />
          <i />
        </div>
        <div class="float-card float-card-c">
          <span />
          <i />
          <em />
        </div>
        <div class="dash dash-a" />
        <div class="dash dash-b" />
      </div>
    </section>

    <section class="auth-card" :class="{ 'register-card': mode === 'register' }" aria-label="账号登录和注册">
      <form :key="mode" class="auth-form" @submit.prevent="submit">
        <div class="form-head">
          <button v-if="mode === 'register'" class="back-button" :disabled="loading" type="button" @click="switchMode('login')">
            返回登录
          </button>
          <span class="panel-tag">{{ mode === 'login' ? 'LOGIN' : 'REGISTER' }}</span>
          <p class="form-title">{{ formTitle }}</p>
          <p class="form-subtitle">{{ formSubtitle }}</p>
        </div>

        <label class="field server-field">
          <span>服务器</span>
          <div class="server-row">
            <input v-model.trim="apiBaseUrl" autocomplete="url" inputmode="url" required type="url" />
            <button :disabled="checkingServer || loading" type="button" @click="refreshSiteSettings(true)">
              {{ checkingServer ? '连接中' : '连接' }}
            </button>
          </div>
          <small v-if="serverMessage">{{ serverMessage }}</small>
        </label>

        <label class="field">
          <span>用户名</span>
          <input v-model.trim="username" autocomplete="username" maxlength="32" minlength="3" required type="text" />
        </label>

        <label class="field">
          <span>密码</span>
          <input
            v-model="password"
            :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
            maxlength="128"
            minlength="10"
            required
            type="password"
          />
        </label>

        <label v-if="mode === 'register'" class="field">
          <span>确认密码</span>
          <input v-model="confirmPassword" autocomplete="new-password" maxlength="128" minlength="10" required type="password" />
        </label>

        <label v-if="mode === 'register' && inviteRequired" class="field">
          <span>邀请码</span>
          <input v-model.trim="inviteCode" autocomplete="off" maxlength="32" required type="text" />
        </label>

        <p class="form-message" :class="{ show: message }">{{ message }}</p>

        <button class="submit-button" :class="{ loading }" :disabled="submitDisabled" type="submit">
          <span>{{ submitLabel }}</span>
          <i aria-hidden="true" />
        </button>

        <div class="auth-switcher">
          <span>{{ mode === 'login' ? '没有账号？' : '已有账号？' }}</span>
          <button
            :disabled="loading || (mode === 'login' && !registrationEnabled)"
            type="button"
            @click="switchMode(mode === 'login' ? 'register' : 'login')"
          >
            {{ mode === 'login' ? (registrationEnabled ? '前往注册' : '暂未开放注册') : '返回登录' }}
          </button>
        </div>
      </form>
    </section>
  </main>
</template>
