<script setup lang="ts">
import { onMounted, ref } from 'vue'

import { APIError, api, type SiteSettings, type ViewerUser } from './api'
import AuthScreen from './components/AuthScreen.vue'
import HomeScreen from './components/HomeScreen.vue'

const defaultSiteName = 'BangumiPipeline Viewer'
const user = ref<ViewerUser | null>(null)
const ready = ref(false)
const loading = ref(false)
const message = ref('')
const siteName = ref(defaultSiteName)
const registrationEnabled = ref(true)
const inviteRequired = ref(false)

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

function onAuthSuccess(nextUser: ViewerUser) {
  user.value = nextUser
  message.value = ''
}

async function logout() {
  if (loading.value) {
    return
  }
  loading.value = true
  try {
    await api.logout()
    user.value = null
  } catch (error) {
    message.value = error instanceof Error ? error.message : '退出失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <!-- 启动屏 -->
  <main v-if="!ready" class="boot-screen">
    <div class="boot-halo halo-a" aria-hidden="true" />
    <div class="boot-halo halo-b" aria-hidden="true" />
    <div class="boot-mark">BP</div>
    <div class="boot-line"><span /></div>
    <p class="boot-tip">LOADING</p>
  </main>

  <!-- 登录 / 注册 -->
  <AuthScreen
    v-else-if="!user"
    :site-name="siteName"
    :registration-enabled="registrationEnabled"
    :invite-required="inviteRequired"
    @success="onAuthSuccess"
  />

  <!-- 已登录首页 -->
  <HomeScreen v-else :user="user" :site-name="siteName" :loading="loading" @logout="logout" />
</template>

<style scoped>
/* ============ 启动屏 ============ */
.boot-screen {
  position: relative;
  min-width: 1200px;
  min-height: 100vh;
  display: grid;
  place-items: center;
  gap: 18px;
  align-content: center;
  overflow: hidden;
  background:
    linear-gradient(135deg, rgba(255, 244, 248, 0.94), rgba(228, 251, 255, 0.8)),
    repeating-linear-gradient(90deg, var(--line-soft) 0 1px, transparent 1px 42px),
    #ffffff;
}

.boot-halo {
  position: absolute;
  border-radius: 50%;
  filter: blur(70px);
  pointer-events: none;
  animation: bp-halo 6s ease-in-out infinite;
}

.halo-a {
  width: 460px;
  height: 460px;
  left: 50%;
  top: 50%;
  transform: translate(-130%, -120%);
  background: radial-gradient(circle, rgba(255, 159, 189, 0.5), transparent 70%);
}

.halo-b {
  width: 460px;
  height: 460px;
  left: 50%;
  top: 50%;
  transform: translate(30%, 20%);
  background: radial-gradient(circle, rgba(73, 214, 233, 0.4), transparent 70%);
  animation-delay: 3s;
}

.boot-mark {
  position: relative;
  z-index: 2;
  display: grid;
  place-items: center;
  width: 76px;
  height: 76px;
  color: #ffffff;
  font-size: 25px;
  letter-spacing: 1px;
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600) 62%, var(--blue-500));
  box-shadow: 0 16px 34px rgba(255, 95, 158, 0.34);
  clip-path: polygon(0 0, calc(100% - 16px) 0, 100% 16px, 100% 100%, 16px 100%, 0 calc(100% - 16px));
  animation: bp-rise 0.5s var(--ease-out) both;
}

.boot-line {
  position: relative;
  z-index: 2;
  width: 220px;
  height: 8px;
  padding: 2px;
  background: #ffffff;
  border: 1px solid var(--line);
  clip-path: polygon(0 0, calc(100% - 6px) 0, 100% 6px, 100% 100%, 6px 100%, 0 calc(100% - 6px));
  box-shadow: 0 14px 30px rgba(255, 95, 158, 0.16);
}

.boot-line span {
  display: block;
  width: 44%;
  height: 100%;
  background: linear-gradient(90deg, var(--pink-500), var(--cyan-400));
  animation: bp-load 1.1s ease-in-out infinite alternate;
}

.boot-tip {
  position: relative;
  z-index: 2;
  color: var(--ink-400);
  font-size: 11px;
  letter-spacing: 3px;
  animation: bp-halo 1.4s ease-in-out infinite;
}
</style>
