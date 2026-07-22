<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'

import {
  APIError,
  api,
  clearAuthSession,
  configureAPI,
  currentAPIBaseURL,
  type AppRelease,
  type SiteSettings,
  type ViewerUser,
} from './api'
import { appDownloadURL, ignoreAppVersion, ignoredAppVersion, isNewerAppVersion } from './appUpdate'
import { loadAppConfig, normalizeAPIBaseURL, saveAPIBaseURL } from './config'
import AppUpdateDialog from './components/AppUpdateDialog.vue'
import MobileShell from './components/MobileShell.vue'
import { openExternalURL } from './native/opener'
import charaImage from '../../viewer/src/assets/chara.png'
import tauriConfig from '../../../../src-tauri/tauri.conf.json'

const appName = 'BakaVip2'
const appVersion = tauriConfig.version

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
const serverExpanded = ref(false)
const passwordVisible = ref(false)
const confirmPasswordVisible = ref(false)
const checkingAppUpdate = ref(false)
const appUpdateCheckMessage = ref('')
const appUpdateRelease = ref<AppRelease | null>(null)
const openingAppDownload = ref(false)
const appUpdateDialogError = ref('')
let appUpdateRequestID = 0

const formTitle = computed(() => (mode.value === 'login' ? '欢迎回来' : '创建你的账号'))
const formSubtitle = computed(() =>
  mode.value === 'login' ? '登录后，从上次离开的地方继续。' : '完成注册，即刻开启你的番剧时光。',
)
const serverDisplayName = computed(() => {
  try {
    const url = new URL(apiBaseUrl.value)
    return url.port ? `${url.hostname}:${url.port}` : url.hostname
  } catch {
    return apiBaseUrl.value || 'https://baka.vip/'
  }
})
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
  void checkAppUpdate(false)
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

function focusAuthInput(inputID: string) {
  document.getElementById(inputID)?.focus()
}

async function toggleServerEditor() {
  if (loading.value) {
    return
  }
  serverExpanded.value = !serverExpanded.value
  serverMessage.value = ''
  if (!serverExpanded.value) {
    return
  }
  await nextTick()
  document.querySelector<HTMLInputElement>('#auth-server-address')?.focus({ preventScroll: true })
}

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
  serverExpanded.value = false
  password.value = ''
  confirmPassword.value = ''
  inviteCode.value = ''
  passwordVisible.value = false
  confirmPasswordVisible.value = false
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
    void checkAppUpdate(false)
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
    if (showResult) {
      void checkAppUpdate(false)
    }
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

async function changeServerAddress(nextBaseURL: string) {
  const normalizedBaseURL = normalizeAPIBaseURL(nextBaseURL)
  const serverChanged = normalizedBaseURL !== currentAPIBaseURL()
  saveAPIBaseURL(normalizedBaseURL)
  configureAPI(normalizedBaseURL)
  apiBaseUrl.value = currentAPIBaseURL()
  if (!serverChanged) {
    return
  }

  appUpdateRequestID += 1
  checkingAppUpdate.value = false
  appUpdateCheckMessage.value = ''
  appUpdateRelease.value = null
  appUpdateDialogError.value = ''

  clearAuthSession()
  user.value = null
  mode.value = 'login'
  password.value = ''
  confirmPassword.value = ''
  inviteCode.value = ''
  registrationEnabled.value = true
  inviteRequired.value = false
  serverMessage.value = ''
  message.value = '服务器地址已更新，请登录新服务器'
  document.querySelector<HTMLLinkElement>('link[rel="icon"]')?.remove()
  await refreshSiteSettings(false)
  void checkAppUpdate(false)
}

async function checkAppUpdate(manual: boolean) {
  const requestID = ++appUpdateRequestID
  checkingAppUpdate.value = true
  appUpdateDialogError.value = ''
  if (manual) {
    appUpdateCheckMessage.value = ''
  }

  try {
    const result = await api.latestAppRelease()
    if (requestID !== appUpdateRequestID) {
      return
    }
    const release = result.release
    if (!release || !isNewerAppVersion(release.version, appVersion)) {
      if (manual) {
        appUpdateCheckMessage.value = `当前已是最新版本（v${appVersion}）`
      }
      return
    }
    if (!manual && ignoredAppVersion() === release.version) {
      return
    }
    appUpdateCheckMessage.value = ''
    appUpdateRelease.value = release
  } catch (error) {
    if (requestID === appUpdateRequestID && manual) {
      appUpdateCheckMessage.value = error instanceof Error ? error.message : '检查更新失败，请稍后重试'
    }
  } finally {
    if (requestID === appUpdateRequestID) {
      checkingAppUpdate.value = false
    }
  }
}

function ignoreCurrentAppRelease() {
  if (!appUpdateRelease.value || openingAppDownload.value) {
    return
  }
  ignoreAppVersion(appUpdateRelease.value.version)
  appUpdateRelease.value = null
  appUpdateDialogError.value = ''
}

async function openAppDownloadPage() {
  if (!appUpdateRelease.value || openingAppDownload.value) {
    return
  }
  openingAppDownload.value = true
  appUpdateDialogError.value = ''
  try {
    await openExternalURL(appDownloadURL())
    appUpdateRelease.value = null
  } catch (error) {
    appUpdateDialogError.value = error instanceof Error ? error.message : '无法打开系统浏览器'
  } finally {
    openingAppDownload.value = false
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
  <main v-if="!ready" class="boot-screen" aria-label="应用加载中">
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
    <img class="boot-chara" :src="charaImage" alt="" draggable="false" />
  </main>

  <MobileShell
    v-else-if="user"
    :user="user"
    :loading="loading"
    :api-base-url="apiBaseUrl"
    :checking-app-update="checkingAppUpdate"
    :app-update-check-message="appUpdateCheckMessage"
    @logout="logout"
    @server-address-change="changeServerAddress"
    @check-app-update="checkAppUpdate(true)"
  />

  <main v-else class="auth-screen">
    <div class="grid-layer" aria-hidden="true" />
    <div class="auth-glow auth-glow-primary" aria-hidden="true" />
    <div class="auth-glow auth-glow-secondary" aria-hidden="true" />
    <img class="auth-chara-background" :src="charaImage" alt="" aria-hidden="true" draggable="false" />

    <section class="auth-layout" aria-label="账号登录和注册">
      <header class="auth-header">
        <div class="auth-brand">
          <span class="auth-brand-copy">
            <strong>{{ appName }}</strong>
            <small>你的私人番剧放映室</small>
          </span>
        </div>

        <div class="auth-heading">
          <button
            v-if="mode === 'register'"
            class="auth-back-button"
            :disabled="loading"
            type="button"
            aria-label="返回登录"
            @click="switchMode('login')"
          >
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="m14.5 6-6 6 6 6" />
            </svg>
          </button>
          <div>
            <h1>{{ formTitle }}</h1>
            <p>{{ formSubtitle }}</p>
          </div>
        </div>
      </header>

      <form :key="mode" class="auth-form" @submit.prevent="submit">
        <button
          class="server-disclosure"
          :class="{ expanded: serverExpanded }"
          :aria-expanded="serverExpanded"
          :disabled="loading"
          type="button"
          @click="toggleServerEditor"
        >
          <span class="server-status-dot" aria-hidden="true" />
          <span class="server-disclosure-copy">
            <small>当前服务器</small>
            <strong>{{ serverDisplayName }}</strong>
          </span>
          <span class="server-edit-label">{{ serverExpanded ? '收起' : '更改' }}</span>
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="m8 10 4 4 4-4" />
          </svg>
        </button>

        <Transition name="server-editor">
          <div v-if="serverExpanded" class="server-editor">
            <label for="auth-server-address">服务器地址</label>
            <div class="server-editor-row">
              <input
                id="auth-server-address"
                v-model.trim="apiBaseUrl"
                autocomplete="url"
                enterkeyhint="go"
                inputmode="url"
                required
                type="url"
                @keydown.enter.prevent="refreshSiteSettings(true)"
              />
              <button :disabled="checkingServer || loading" type="button" @click="refreshSiteSettings(true)">
                {{ checkingServer ? '连接中' : '连接' }}
              </button>
            </div>
            <small v-if="serverMessage" :class="{ success: serverMessage === '服务器已连接' }" role="status">
              {{ serverMessage }}
            </small>
          </div>
        </Transition>

        <div class="auth-fields">
          <div class="auth-field">
            <label for="auth-username">用户名</label>
            <span class="auth-input-shell">
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <circle cx="12" cy="8" r="3.5" />
                <path d="M5.5 19c.5-3.5 2.7-5.3 6.5-5.3s6 1.8 6.5 5.3" />
              </svg>
              <input
                id="auth-username"
                v-model.trim="username"
                autocapitalize="none"
                autocomplete="username"
                enterkeyhint="next"
                maxlength="32"
                minlength="3"
                placeholder="请输入用户名"
                required
                spellcheck="false"
                type="text"
                @keydown.enter.prevent="focusAuthInput('auth-password')"
              />
            </span>
          </div>

          <div class="auth-field">
            <label for="auth-password">密码</label>
            <span class="auth-input-shell">
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <rect x="5.5" y="10" width="13" height="9.5" rx="3" />
                <path d="M8.5 10V7.5a3.5 3.5 0 0 1 7 0V10" />
              </svg>
              <input
                id="auth-password"
                v-model="password"
                :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
                :enterkeyhint="mode === 'login' ? 'done' : 'next'"
                maxlength="128"
                minlength="10"
                placeholder="请输入密码"
                required
                :type="passwordVisible ? 'text' : 'password'"
                @keydown.enter.prevent="mode === 'login' ? submit() : focusAuthInput('auth-confirm-password')"
              />
              <button
                class="password-toggle"
                type="button"
                :aria-label="passwordVisible ? '隐藏密码' : '显示密码'"
                @click="passwordVisible = !passwordVisible"
              >
                <svg v-if="passwordVisible" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M3.5 12s3-5 8.5-5 8.5 5 8.5 5-3 5-8.5 5-8.5-5-8.5-5Z" />
                  <circle cx="12" cy="12" r="2.5" />
                </svg>
                <svg v-else viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M4 4 20 20M9.5 7.4A9 9 0 0 1 12 7c5.5 0 8.5 5 8.5 5a13.6 13.6 0 0 1-2.3 2.8M14.7 16.5c-.8.3-1.7.5-2.7.5-5.5 0-8.5-5-8.5-5a13 13 0 0 1 3-3.3" />
                </svg>
              </button>
            </span>
          </div>

          <div v-if="mode === 'register'" class="auth-field">
            <label for="auth-confirm-password">确认密码</label>
            <span class="auth-input-shell">
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <rect x="5.5" y="10" width="13" height="9.5" rx="3" />
                <path d="M8.5 10V7.5a3.5 3.5 0 0 1 7 0V10" />
              </svg>
              <input
                id="auth-confirm-password"
                v-model="confirmPassword"
                autocomplete="new-password"
                :enterkeyhint="inviteRequired ? 'next' : 'done'"
                maxlength="128"
                minlength="10"
                placeholder="请再次输入密码"
                required
                :type="confirmPasswordVisible ? 'text' : 'password'"
                @keydown.enter.prevent="inviteRequired ? focusAuthInput('auth-invite-code') : submit()"
              />
              <button
                class="password-toggle"
                type="button"
                :aria-label="confirmPasswordVisible ? '隐藏确认密码' : '显示确认密码'"
                @click="confirmPasswordVisible = !confirmPasswordVisible"
              >
                <svg v-if="confirmPasswordVisible" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M3.5 12s3-5 8.5-5 8.5 5 8.5 5-3 5-8.5 5-8.5-5-8.5-5Z" />
                  <circle cx="12" cy="12" r="2.5" />
                </svg>
                <svg v-else viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M4 4 20 20M9.5 7.4A9 9 0 0 1 12 7c5.5 0 8.5 5 8.5 5a13.6 13.6 0 0 1-2.3 2.8M14.7 16.5c-.8.3-1.7.5-2.7.5-5.5 0-8.5-5-8.5-5a13 13 0 0 1 3-3.3" />
                </svg>
              </button>
            </span>
          </div>

          <div v-if="mode === 'register' && inviteRequired" class="auth-field">
            <label for="auth-invite-code">邀请码</label>
            <span class="auth-input-shell">
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path d="M4.5 7.5h15v10h-15zM8 7.5v10M16 7.5v10M4.5 11h15" />
              </svg>
              <input
                id="auth-invite-code"
                v-model.trim="inviteCode"
                autocapitalize="characters"
                autocomplete="off"
                enterkeyhint="done"
                maxlength="32"
                placeholder="请输入邀请码"
                required
                spellcheck="false"
                type="text"
                @keydown.enter.prevent="submit"
              />
            </span>
          </div>
        </div>

        <Transition name="form-message">
          <p v-if="message" class="form-message" role="alert">
            <span aria-hidden="true">!</span>
            {{ message }}
          </p>
        </Transition>

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

  <AppUpdateDialog
    v-if="ready && appUpdateRelease"
    :release="appUpdateRelease"
    :current-version="appVersion"
    :opening-download="openingAppDownload"
    :error-message="appUpdateDialogError"
    @download="openAppDownloadPage"
    @ignore="ignoreCurrentAppRelease"
  />
</template>
