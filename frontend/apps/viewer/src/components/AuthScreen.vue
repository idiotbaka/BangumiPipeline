<script setup lang="ts">
import { computed, ref } from 'vue'

import { APIError, api, type ViewerUser } from '../api'
import charaImage from '../assets/chara.png'
import ParticleField from './ParticleField.vue'

interface Props {
  siteName: string
  registrationEnabled: boolean
  inviteRequired: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'success', user: ViewerUser): void
}>()

const loading = ref(false)
const mode = ref<'login' | 'register'>('login')
const username = ref('')
const password = ref('')
const confirmPassword = ref('')
const inviteCode = ref('')
const message = ref('')

const formTitle = computed(() => (mode.value === 'login' ? '欢迎回来' : '创建账号'))
const submitLabel = computed(() => (mode.value === 'login' ? '登录' : '注册并进入'))
const submitDisabled = computed(
  () => loading.value || (mode.value === 'register' && !props.registrationEnabled),
)

function switchMode(nextMode: 'login' | 'register') {
  mode.value = nextMode
  message.value = nextMode === 'register' && !props.registrationEnabled ? '当前暂未开放注册' : ''
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
  if (mode.value === 'register' && !props.registrationEnabled) {
    message.value = '当前暂未开放注册'
    return
  }
  if (mode.value === 'register' && props.inviteRequired && inviteCode.value.trim() === '') {
    message.value = '请填写邀请码'
    return
  }
  loading.value = true
  try {
    const result =
      mode.value === 'login'
        ? await api.login(username.value, password.value)
        : await api.register(username.value, password.value, inviteCode.value)
    password.value = ''
    confirmPassword.value = ''
    inviteCode.value = ''
    emit('success', result.user)
  } catch (error) {
    message.value = error instanceof Error ? error.message : '请求失败'
  } finally {
    loading.value = false
  }
}

// 触发两次密码不一致的局部提示也复用 submit，保留与原版一致的交互。
void APIError
</script>

<template>
  <main class="auth-screen">
    <!-- 背景光晕 + 碎片 -->
    <div class="bg-halo halo-a" aria-hidden="true" />
    <div class="bg-halo halo-b" aria-hidden="true" />

    <!-- ===== 左侧：表单区 ===== -->
    <section class="form-zone" aria-label="账号登录和注册">
      <header class="brand-row">
        <div class="brand-text">
          <p>VIEWER PORTAL</p>
          <strong>{{ siteName }}</strong>
        </div>
      </header>

      <!-- 登录/注册 切角 Tab -->
      <div class="mode-switch" role="tablist" aria-label="切换账号模式">
        <button
          role="tab"
          :aria-selected="mode === 'login'"
          :class="{ active: mode === 'login' }"
          type="button"
          @click="switchMode('login')"
        >
          登录
        </button>
        <button
          role="tab"
          :aria-selected="mode === 'register'"
          :class="{ active: mode === 'register' }"
          type="button"
          @click="switchMode('register')"
        >
          注册
        </button>
        <span class="switch-ink" :class="{ right: mode === 'register' }" aria-hidden="true" />
      </div>

      <!-- 主面板：标签贴片与表单同处一个无 clip-path 的容器，避免标签被表单的削角裁切 -->
      <div class="auth-panel-wrap">
        <span class="panel-tag">ACCESS</span>
        <form class="auth-panel" @submit.prevent="submit">
        <div class="panel-head">
          <h1>{{ formTitle }}</h1>
          <p class="panel-sub">{{ mode === 'login' ? '请输入账号信息以继续' : '填写信息以创建新账号' }}</p>
        </div>

        <label class="field">
          <span>用户名</span>
          <input
            v-model.trim="username"
            autocomplete="username"
            maxlength="32"
            minlength="3"
            required
            type="text"
          />
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
          <span class="submit-label">{{ loading ? '处理中' : submitLabel }}</span>
          <i class="submit-sweep" aria-hidden="true" />
        </button>
        </form>
      </div>

      <footer class="corner-note">
        <i class="note-bar" aria-hidden="true" />
        <span>MIN WIDTH 1200</span>
      </footer>
    </section>

    <!-- ===== 右侧：立绘视觉区 ===== -->
    <section class="visual-zone" aria-label="视觉背景">
      <!-- 几何光板 -->
      <div class="visual-plate plate-a" aria-hidden="true" />
      <div class="visual-plate plate-b" aria-hidden="true" />
      <div class="visual-plate plate-c" aria-hidden="true" />
      <div class="visual-ring" aria-hidden="true" />

      <!-- 飘动碎片 -->
      <ParticleField :count="22" palette="cool" :max-size="44" />

      <!-- 立绘 -->
      <img class="chara" :src="charaImage" alt="看板娘立绘" draggable="false" />

      <!-- 标号 caption -->
      <div class="visual-caption">
        <span class="cap-num">01</span>
        <div class="cap-text">
          <strong>ANIME LIBRARY</strong>
          <p>番剧图书馆</p>
        </div>
      </div>
    </section>
  </main>
</template>

<style scoped>
.auth-screen {
  position: relative;
  min-width: 1200px;
  min-height: 100vh;
  display: grid;
  grid-template-columns: 496px minmax(704px, 1fr);
  overflow: hidden;
  background:
    linear-gradient(90deg, #ffffff 0 39%, var(--pink-50) 39%),
    repeating-linear-gradient(0deg, var(--line-soft) 0 1px, transparent 1px 38px),
    #ffffff;
}

/* 背景光晕 */
.bg-halo {
  position: absolute;
  border-radius: 50%;
  filter: blur(60px);
  pointer-events: none;
  z-index: 0;
  animation: bp-halo 7s ease-in-out infinite;
}

.halo-a {
  width: 520px;
  height: 520px;
  left: -120px;
  top: -160px;
  background: radial-gradient(circle, rgba(255, 159, 189, 0.5), transparent 70%);
}

.halo-b {
  width: 640px;
  height: 640px;
  right: -200px;
  bottom: -240px;
  background: radial-gradient(circle, rgba(73, 214, 233, 0.4), transparent 70%);
  animation-delay: 3s;
}

/* ============ 左侧表单区 ============ */
.form-zone {
  position: relative;
  z-index: 3;
  display: flex;
  flex-direction: column;
  padding: 46px 52px 36px;
  background: var(--glass-strong);
  border-right: 1px solid var(--line);
  box-shadow: 24px 0 80px rgba(255, 95, 158, 0.08);
  backdrop-filter: blur(10px);
}

/* 品牌 */
.brand-row {
  display: flex;
  align-items: center;
  gap: 14px;
  animation: bp-slide-left 0.5s var(--ease-out) both;
}

.brand-text p {
  color: var(--ink-400);
  font-size: 11px;
  letter-spacing: 2px;
}

.brand-text strong {
  display: block;
  margin-top: 3px;
  overflow: hidden;
  max-width: 330px;
  font-size: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ============ 切角 Tab ============ */
.mode-switch {
  position: relative;
  width: 244px;
  display: grid;
  grid-template-columns: 1fr 1fr;
  margin-top: 64px;
  padding: 5px;
  background: #ffffff;
  border: 1px solid var(--line);
  box-shadow: 0 16px 36px rgba(255, 95, 158, 0.12);
  clip-path: polygon(0 0, calc(100% - 10px) 0, 100% 10px, 100% 100%, 10px 100%, 0 calc(100% - 10px));
  animation: bp-rise 0.5s var(--ease-out) 0.05s both;
}

.mode-switch button {
  position: relative;
  z-index: 2;
  height: 38px;
  color: var(--ink-600);
  font-size: 14px;
  background: transparent;
  border-radius: 0;
  transition: color 200ms var(--ease-soft);
}

.mode-switch button:hover {
  color: var(--pink-600);
}

.mode-switch button.active {
  color: #ffffff;
}

/* 滑动的切角高亮块 */
.switch-ink {
  position: absolute;
  z-index: 1;
  top: 5px;
  left: 5px;
  width: calc(50% - 5px);
  height: calc(100% - 10px);
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600));
  box-shadow: 0 8px 18px rgba(255, 95, 158, 0.3);
  clip-path: polygon(0 0, calc(100% - 8px) 0, 100% 8px, 100% 100%, 8px 100%, 0 calc(100% - 8px));
  transition: transform 0.42s var(--ease-bounce);
}

.switch-ink.right {
  transform: translateX(100%);
}

/* ============ 主面板 ============
 * 注意：表单使用 clip-path 做削角，会裁掉所有溢出边界的子元素，
 * 因此把标签贴片 .panel-tag 与 .auth-panel 放在无 clip-path 的
 * .auth-panel-wrap 容器里，标签才能完整显示在面板顶部之外。 */
.auth-panel-wrap {
  position: relative;
  width: 388px;
  margin-top: 26px;
  animation: bp-rise 0.55s var(--ease-out) 0.12s both;
}

.auth-panel {
  position: relative;
  padding: 30px 28px 28px;
  background:
    linear-gradient(#ffffff, #ffffff) padding-box,
    linear-gradient(135deg, rgba(255, 95, 158, 0.7), rgba(73, 214, 233, 0.5)) border-box;
  border: 1.5px solid transparent;
  box-shadow: 0 26px 60px rgba(255, 95, 158, 0.16);
  clip-path: polygon(0 0, calc(100% - 16px) 0, 100% 16px, 100% 100%, 16px 100%, 0 calc(100% - 16px));
}

/* 顶部斜切标签贴片 */
.panel-tag {
  position: absolute;
  top: -13px;
  left: 24px;
  z-index: 2;
  display: grid;
  place-items: center;
  min-width: 86px;
  height: 26px;
  padding: 0 12px;
  color: var(--ink-900);
  font-size: 12px;
  letter-spacing: 1.5px;
  background: var(--yellow-300);
  box-shadow: 0 8px 18px rgba(255, 229, 122, 0.4);
  clip-path: polygon(0 0, 100% 0, calc(100% - 12px) 100%, 0 100%);
  transform: rotate(-3deg);
  animation: bp-tag-in 0.5s var(--ease-bounce) 0.3s both;
}

/* 右侧渐变描边装饰条 */
.auth-panel::after {
  content: '';
  position: absolute;
  right: -1px;
  bottom: 38px;
  width: 4px;
  height: 76px;
  background: linear-gradient(var(--cyan-400), var(--pink-500));
}

.panel-head {
  margin-bottom: 24px;
}

.panel-head h1 {
  font-size: 32px;
  line-height: 1.16;
  letter-spacing: 1px;
}

.panel-sub {
  margin-top: 6px;
  color: var(--ink-400);
  font-size: 13px;
}

/* ============ 输入框 ============ */
.field {
  display: block;
  margin-top: 16px;
  animation: bp-rise 0.5s var(--ease-out) both;
}

.field:nth-of-type(1) {
  animation-delay: 0.18s;
}
.field:nth-of-type(2) {
  animation-delay: 0.24s;
}
.field:nth-of-type(3) {
  animation-delay: 0.3s;
}
.field:nth-of-type(4) {
  animation-delay: 0.36s;
}

.field span {
  display: block;
  margin-bottom: 7px;
  color: var(--ink-600);
  font-size: 12px;
  letter-spacing: 0.5px;
}

.field input {
  width: 100%;
  height: 48px;
  padding: 0 14px;
  color: var(--ink-900);
  font-size: 14px;
  background:
    linear-gradient(var(--pink-50), var(--pink-50)) padding-box,
    linear-gradient(135deg, rgba(255, 95, 158, 0.28), rgba(73, 214, 233, 0.24)) border-box;
  border: 1.5px solid transparent;
  border-radius: 3px;
  transition: background 160ms var(--ease-soft), box-shadow 160ms var(--ease-soft),
    transform 160ms var(--ease-soft);
}

.field input::placeholder {
  color: var(--ink-300);
}

.field input:focus {
  background:
    linear-gradient(#ffffff, #ffffff) padding-box,
    linear-gradient(135deg, var(--pink-500), var(--cyan-400)) border-box;
  box-shadow: 0 10px 22px rgba(255, 95, 158, 0.18);
  transform: translateY(-1px);
}

.form-message {
  min-height: 22px;
  margin: 14px 0 0;
  color: var(--pink-600);
  font-size: 13px;
  line-height: 1.55;
}

/* ============ 提交按钮 ============ */
.submit-button {
  position: relative;
  width: 100%;
  height: 52px;
  margin-top: 20px;
  overflow: hidden;
  color: #ffffff;
  font-size: 15px;
  letter-spacing: 1px;
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600) 58%, var(--blue-500));
  box-shadow: 0 18px 36px rgba(255, 95, 158, 0.32);
  border-radius: 3px;
  clip-path: polygon(0 0, calc(100% - 12px) 0, 100% 12px, 100% 100%, 12px 100%, 0 calc(100% - 12px));
  transition: transform 200ms var(--ease-soft), box-shadow 200ms var(--ease-soft),
    filter 200ms var(--ease-soft);
  animation: bp-rise 0.5s var(--ease-out) 0.42s both;
}

.submit-label {
  position: relative;
  z-index: 2;
}

.submit-sweep {
  position: absolute;
  inset: 0;
  z-index: 1;
  background: linear-gradient(110deg, transparent 0 30%, rgba(255, 255, 255, 0.5) 42%, transparent 56%);
  transform: translateX(-130%);
}

.submit-button:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 24px 44px rgba(255, 95, 158, 0.38);
  filter: saturate(1.08);
}

.submit-button:hover:not(:disabled) .submit-sweep {
  animation: bp-sweep 0.9s var(--ease-soft);
}

.submit-button:active:not(:disabled) {
  transform: translateY(0);
}

.submit-button:disabled {
  filter: grayscale(0.3) brightness(0.95);
}

/* ============ 角注 ============ */
.corner-note {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: auto;
  color: rgba(82, 96, 120, 0.6);
  font-size: 11px;
  letter-spacing: 1.5px;
  animation: bp-rise 0.5s var(--ease-out) 0.5s both;
}

.note-bar {
  width: 44px;
  height: 4px;
  background: linear-gradient(90deg, var(--pink-500), var(--cyan-400));
}

/* ============ 右侧视觉区 ============ */
.visual-zone {
  position: relative;
  overflow: hidden;
  background:
    linear-gradient(135deg, rgba(255, 244, 248, 0.86), rgba(255, 255, 255, 0.5) 42%, rgba(228, 251, 255, 0.86)),
    repeating-linear-gradient(90deg, var(--line-cool) 0 1px, transparent 1px 56px);
}

/* 几何光板（削角，缓慢摇摆） */
.visual-plate {
  position: absolute;
  border: 1px solid rgba(255, 255, 255, 0.8);
  box-shadow: 0 22px 60px rgba(85, 119, 217, 0.12);
  animation: bp-sway 5s ease-in-out infinite alternate;
  z-index: 1;
}

.plate-a {
  left: 64px;
  top: 122px;
  width: 270px;
  height: 130px;
  background: rgba(255, 159, 189, 0.32);
  clip-path: polygon(0 0, 100% 0, 84% 100%, 0 100%);
}

.plate-b {
  right: 60px;
  top: 72px;
  width: 340px;
  height: 174px;
  background: rgba(73, 214, 233, 0.24);
  clip-path: polygon(14% 0, 100% 0, 100% 80%, 86% 100%, 0 100%, 0 16%);
  animation-delay: 420ms;
}

.plate-c {
  right: 128px;
  bottom: 80px;
  width: 430px;
  height: 150px;
  background: rgba(255, 229, 122, 0.32);
  clip-path: polygon(0 0, 88% 0, 100% 100%, 16% 100%);
  animation-delay: 780ms;
}

/* 旋转装饰环 */
.visual-ring {
  position: absolute;
  z-index: 1;
  left: 90px;
  bottom: 200px;
  width: 120px;
  height: 120px;
  border: 2px dashed rgba(255, 95, 158, 0.4);
  border-radius: 50%;
  animation: bp-spin 24s linear infinite;
}

/* 立绘 */
.chara {
  position: absolute;
  z-index: 3;
  right: 6%;
  bottom: -46px;
  height: min(92vh, 960px);
  max-height: 960px;
  object-fit: contain;
  pointer-events: none;
  filter: drop-shadow(0 28px 50px rgba(42, 54, 102, 0.24));
  animation: bp-float 4.2s ease-in-out infinite alternate;
}

/* 标号 caption */
.visual-caption {
  position: absolute;
  z-index: 4;
  left: 56px;
  bottom: 54px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 18px 12px 12px;
  color: var(--ink-900);
  background: var(--glass-strong);
  border: 1px solid rgba(255, 255, 255, 0.9);
  box-shadow: 0 22px 48px rgba(255, 95, 158, 0.16);
  backdrop-filter: blur(16px);
  clip-path: polygon(0 0, calc(100% - 14px) 0, 100% 14px, 100% 100%, 14px 100%, 0 calc(100% - 14px));
  animation: bp-rise 0.6s var(--ease-out) 0.4s both;
}

.cap-num {
  display: grid;
  place-items: center;
  width: 44px;
  height: 36px;
  color: #ffffff;
  font-size: 13px;
  background: linear-gradient(135deg, var(--pink-500), var(--pink-600));
  clip-path: polygon(10px 0, 100% 0, 100% 100%, 0 100%, 0 10px);
}

.cap-text strong {
  display: block;
  font-size: 13px;
  letter-spacing: 1px;
}

.cap-text p {
  margin-top: 2px;
  color: var(--ink-400);
  font-size: 11px;
}
</style>
