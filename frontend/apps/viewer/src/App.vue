<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { APIError, api, type SiteSettings, type ViewerUser } from './api'
import charaImage from './assets/chara.png'

const defaultSiteName = 'BangumiPipeline Viewer'
const user = ref<ViewerUser | null>(null)
const ready = ref(false)
const loading = ref(false)
const mode = ref<'login' | 'register'>('login')
const username = ref('')
const password = ref('')
const confirmPassword = ref('')
const inviteCode = ref('')
const message = ref('')
const siteName = ref(defaultSiteName)
const registrationEnabled = ref(true)
const inviteRequired = ref(false)
const formTitle = computed(() => (mode.value === 'login' ? '欢迎回来' : '创建账号'))
const submitLabel = computed(() => (mode.value === 'login' ? '登录' : '注册并进入'))
const submitDisabled = computed(() => loading.value || (mode.value === 'register' && !registrationEnabled.value))

onMounted(async () => {
  try {
    const result = await api.siteSettings()
    applySiteSettings(result.settings)
  } catch {
    document.title = siteName.value
  }
  try {
    const result = await api.me()
    user.value = result.user
  } catch (error) {
    if (!(error instanceof APIError) || error.status !== 401) {
      message.value = error instanceof Error ? error.message : '无法连接观看端'
    }
  } finally {
    ready.value = true
  }
})

function applySiteSettings(settings: SiteSettings) {
  siteName.value = settings.siteName || defaultSiteName
  registrationEnabled.value = settings.registrationEnabled
  inviteRequired.value = settings.inviteRequired
  document.title = siteName.value
  const existing = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!settings.hasFavicon) {
    existing?.remove()
    return
  }
  const link = existing ?? document.createElement('link')
  link.rel = 'icon'
  link.type = 'image/png'
  link.href = `/favicon.png?v=${settings.faviconUpdatedAt ?? settings.updatedAt}`
  if (!existing) {
    document.head.appendChild(link)
  }
}

function switchMode(nextMode: 'login' | 'register') {
  mode.value = nextMode
  message.value = nextMode === 'register' && !registrationEnabled.value ? '当前暂未开放注册' : ''
  confirmPassword.value = ''
  inviteCode.value = ''
}

async function submit() {
  message.value = ''
  if (loading.value) {
    return
  }
  if (mode.value === 'register' && password.value !== confirmPassword.value) {
    message.value = '两次输入的密码不一致'
    return
  }
  if (mode.value === 'register' && !registrationEnabled.value) {
    message.value = '当前暂未开放注册'
    return
  }
  if (mode.value === 'register' && inviteRequired.value && inviteCode.value.trim() === '') {
    message.value = '请填写邀请码'
    return
  }
  loading.value = true
  try {
    const result =
      mode.value === 'login'
        ? await api.login(username.value, password.value)
        : await api.register(username.value, password.value, inviteCode.value)
    user.value = result.user
    password.value = ''
    confirmPassword.value = ''
    inviteCode.value = ''
  } catch (error) {
    message.value = error instanceof Error ? error.message : '请求失败'
  } finally {
    loading.value = false
  }
}

async function logout() {
  if (loading.value) {
    return
  }
  loading.value = true
  try {
    await api.logout()
    user.value = null
    mode.value = 'login'
  } catch (error) {
    message.value = error instanceof Error ? error.message : '退出失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main v-if="!ready" class="boot-screen">
    <div class="boot-mark">BP</div>
    <div class="boot-line"><span /></div>
  </main>

  <main v-else-if="!user" class="auth-screen">
    <section class="auth-form-zone" aria-label="账号登录和注册">
      <div class="brand-row">
        <div class="brand-mark">BP</div>
        <div>
          <p>Viewer Portal</p>
          <strong>{{ siteName }}</strong>
        </div>
      </div>

      <div class="mode-switch" aria-label="切换账号模式">
        <button :class="{ active: mode === 'login' }" type="button" @click="switchMode('login')">登录</button>
        <button :class="{ active: mode === 'register' }" type="button" @click="switchMode('register')">注册</button>
      </div>

      <form class="auth-panel" @submit.prevent="submit">
        <div class="panel-head">
          <span>ACCESS</span>
          <h1>{{ formTitle }}</h1>
        </div>

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
          <input
            v-model="confirmPassword"
            autocomplete="new-password"
            maxlength="128"
            minlength="10"
            required
            type="password"
          />
        </label>

        <label v-if="mode === 'register' && inviteRequired" class="field">
          <span>邀请码</span>
          <input
            v-model.trim="inviteCode"
            autocomplete="off"
            maxlength="32"
            placeholder="请输入邀请码"
            required
            type="text"
          />
        </label>

        <p v-if="message" class="form-message">{{ message }}</p>
        <button class="submit-button" :disabled="submitDisabled" type="submit">
          <span>{{ loading ? '处理中' : submitLabel }}</span>
        </button>
      </form>

      <div class="corner-note">
        <span>MIN WIDTH 1200</span>
        <i />
      </div>
    </section>

    <section class="auth-visual-zone" aria-label="视觉背景">
      <div class="visual-plate plate-a" />
      <div class="visual-plate plate-b" />
      <div class="visual-plate plate-c" />
      <img class="chara" :src="charaImage" alt="" />
      <div class="visual-caption">
        <span>01</span>
        <strong>ANIME LIBRARY</strong>
      </div>
    </section>
  </main>

  <main v-else class="viewer-shell">
    <header class="topbar">
      <div class="brand-row compact">
        <div class="brand-mark">BP</div>
        <div>
          <p>Viewer Portal</p>
          <strong>{{ siteName }}</strong>
        </div>
      </div>
      <div class="user-chip">
        <span>{{ user.username }}</span>
        <button :disabled="loading" type="button" @click="logout">退出</button>
      </div>
    </header>
    <section class="home-blank" aria-label="首页" />
  </main>
</template>
