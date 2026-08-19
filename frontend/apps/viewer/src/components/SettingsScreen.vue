<script setup lang="ts">
import { computed, reactive, ref } from 'vue'

import { api, type ViewerInvitation, type ViewerInvitationOverview, type ViewerUser } from '../api'
import defaultAvatar from '../assets/avatar.png'
import ParticleField from './ParticleField.vue'

interface Props {
  user: ViewerUser
}

defineProps<Props>()

type SettingsTab = 'profile' | 'invitations' | 'password'

const activeTab = ref<SettingsTab>('profile')
const submitting = ref(false)
const feedback = ref('')
const feedbackType = ref<'success' | 'error'>('success')
const invitations = ref<ViewerInvitationOverview | null>(null)
const invitationsLoading = ref(false)
const invitationsError = ref('')
const invitationFeedback = ref('')
const invitationFeedbackType = ref<'success' | 'error'>('success')
const invitationCreating = ref(false)
const passwordForm = reactive({
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
})

const newPasswordLength = computed(() => Array.from(passwordForm.newPassword).length)
const confirmationMismatch = computed(() => (
  passwordForm.confirmPassword !== '' && passwordForm.newPassword !== passwordForm.confirmPassword
))
const passwordFormComplete = computed(() => (
  passwordForm.currentPassword !== ''
  && newPasswordLength.value >= 10
  && newPasswordLength.value <= 128
  && passwordForm.confirmPassword !== ''
  && !confirmationMismatch.value
))

function selectTab(tab: SettingsTab) {
  activeTab.value = tab
  feedback.value = ''
  invitationFeedback.value = ''
  if (tab === 'invitations') {
    void loadInvitations()
  }
}

function formatRegisteredAt(value: number) {
  if (!value) return '注册时间未知'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(value * 1000))
}

function registeredAtISO(value: number) {
  return value ? new Date(value * 1000).toISOString() : undefined
}

function registrationMethod(user: ViewerUser) {
  if (user.registrationSource === 'system_invite') return '邀请码注册（系统管理员邀请）'
  if (user.registrationSource === 'user_invite') {
    return `邀请码注册（用户 ${user.invitedByUsername || '未知用户'} 邀请）`
  }
  return '开放注册'
}

function formatInvitationDate(value: number | null) {
  return value ? formatRegisteredAt(value) : '-'
}

async function loadInvitations() {
  if (invitationsLoading.value) return
  invitationsLoading.value = true
  invitationsError.value = ''
  try {
    const result = await api.invitations()
    invitations.value = result.invitations
  } catch (error) {
    invitationsError.value = error instanceof Error ? error.message : '邀请码加载失败'
    if (invitations.value) {
      invitationFeedbackType.value = 'error'
      invitationFeedback.value = invitationsError.value
    }
  } finally {
    invitationsLoading.value = false
  }
}

async function createInvitation() {
  if (invitationCreating.value || !invitations.value?.allowance.canCreate) return
  invitationCreating.value = true
  invitationFeedback.value = ''
  try {
    const result = await api.generateInvitation()
    invitationFeedbackType.value = 'success'
    invitationFeedback.value = `邀请码已创建：${result.invite.code}`
    await loadInvitations()
  } catch (error) {
    invitationFeedbackType.value = 'error'
    invitationFeedback.value = error instanceof Error ? error.message : '邀请码创建失败'
  } finally {
    invitationCreating.value = false
  }
}

async function copyInvitation(invite: ViewerInvitation) {
  try {
    await navigator.clipboard.writeText(invite.code)
    invitationFeedbackType.value = 'success'
    invitationFeedback.value = '邀请码已复制'
  } catch {
    invitationFeedbackType.value = 'error'
    invitationFeedback.value = '当前浏览器不允许自动复制'
  }
}

function invitationAvailabilityText(overview: ViewerInvitationOverview) {
  const allowance = overview.allowance
  if (allowance.canCreate) return `当前可以创建 ${allowance.remainingCount} 个邀请码。`
  if (allowance.eligibleTotal >= allowance.maximumTotal) return `邀请码额度已达到最多 ${allowance.maximumTotal} 个。`
  if (allowance.nextEligibleAt) {
    return `下一份邀请码额度将在 ${formatInvitationDate(allowance.nextEligibleAt)} 获得。`
  }
  return '当前没有可创建的邀请码额度。'
}

function clearFeedback() {
  feedback.value = ''
}

async function changePassword() {
  if (submitting.value) return
  feedback.value = ''
  if (!passwordForm.currentPassword) {
    feedbackType.value = 'error'
    feedback.value = '请输入现在的密码'
    return
  }
  if (newPasswordLength.value < 10 || newPasswordLength.value > 128) {
    feedbackType.value = 'error'
    feedback.value = '新密码需要 10 到 128 个字符'
    return
  }
  if (passwordForm.newPassword !== passwordForm.confirmPassword) {
    feedbackType.value = 'error'
    feedback.value = '两次输入的新密码不一致'
    return
  }

  submitting.value = true
  try {
    await api.changePassword(
      passwordForm.currentPassword,
      passwordForm.newPassword,
      passwordForm.confirmPassword,
    )
    passwordForm.currentPassword = ''
    passwordForm.newPassword = ''
    passwordForm.confirmPassword = ''
    feedbackType.value = 'success'
    feedback.value = '密码修改成功'
  } catch (error) {
    feedbackType.value = 'error'
    feedback.value = error instanceof Error ? error.message : '密码修改失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <section class="settings-stage" aria-label="用户设置">
    <ParticleField :count="16" palette="cool" :max-size="32" />
    <div class="settings-grid" aria-hidden="true" />
    <div class="settings-halo pink" aria-hidden="true" />
    <div class="settings-halo cyan" aria-hidden="true" />

    <div class="settings-wrap">
      <header class="settings-heading">
        <p>USER CONFIGURATION <i /></p>
        <h1>设置</h1>
        <span>ACCOUNT CENTER・个人中心</span>
      </header>

      <div class="settings-layout">
        <aside class="settings-sidebar" aria-label="设置导航">
          <div class="settings-nav-group">
            <p>账户</p>
            <button
              type="button"
              :class="{ active: activeTab === 'profile' }"
              :aria-current="activeTab === 'profile' ? 'page' : undefined"
              @click="selectTab('profile')"
            >
              <svg class="settings-tab-icon" viewBox="0 0 1024 1024" aria-hidden="true">
                <path d="M441.37931 600.275862m-388.413793 0a388.413793 388.413793 0 1 0 776.827586 0 388.413793 388.413793 0 1 0-776.827586 0Z" fill="#D8D8D8" />
                <path d="M512 0c282.765241 0 512 229.234759 512 512S794.765241 1024 512 1024 0 794.765241 0 512 229.234759 0 512 0z m0 88.275862C277.98069 88.275862 88.275862 277.98069 88.275862 512s189.704828 423.724138 423.724138 423.724138 423.724138-189.704828 423.724138-423.724138S746.01931 88.275862 512 88.275862z" fill="#464646" />
                <path d="M660.126897 654.795034a44.137931 44.137931 0 0 1 56.849655 67.51338C653.594483 775.68 587.987862 803.310345 520.827586 803.310345c-67.160276 0-132.766897-27.630345-196.148965-81.001931a44.137931 44.137931 0 0 1 56.849655-67.51338c48.445793 40.783448 94.631724 60.239448 139.29931 60.239449 44.667586 0 90.853517-19.456 139.299311-60.239449z" fill="#6A6A6A" />
              </svg>
              <strong>个人信息</strong>
              <i class="nav-arrow" aria-hidden="true" />
            </button>
            <button
              type="button"
              :class="{ active: activeTab === 'invitations' }"
              :aria-current="activeTab === 'invitations' ? 'page' : undefined"
              @click="selectTab('invitations')"
            >
              <svg class="settings-tab-icon" viewBox="0 0 1082 1024" aria-hidden="true">
                <path d="M411.983448 635.586207m-388.413793 0a388.413793 388.413793 0 1 0 776.827586 0 388.413793 388.413793 0 1 0-776.827586 0Z" fill="#D8D8D8" />
                <path d="M918.863448 16.172138a114.758621 114.758621 0 0 1 148.621242 147.950345L773.808552 933.570207a114.758621 114.758621 0 0 1-215.781518-3.813517c-43.078621-125.969655-85.680552-211.614897-125.704827-255.664552-41.277793-45.391448-133.420138-95.744-274.855724-147.950345a114.758621 114.758621 0 0 1-1.606621-214.686896l41.154207-15.907311L918.863448 16.172138z m66.100966 97.456552a26.482759 26.482759 0 0 0-34.251035-15.130483L187.727448 393.833931l-0.723862 0.264828-1.889103 0.882758a26.482759 26.482759 0 0 0-11.581793 11.74069l-1.129931 2.59531a26.482759 26.482759 0 0 0 15.660138 34.039173l-0.229518-0.105931 0.194207 0.088275c150.245517 55.437241 251.692138 110.291862 305.893517 167.38869l3.707587 3.990069c49.981793 54.995862 97.28 150.068966 143.924965 286.490483a26.482759 26.482759 0 0 0 49.787586 0.882758L985.017379 132.64331a26.482759 26.482759 0 0 0-0.052965-18.996965z" fill="#464646" />
                <path d="M808.862267 240.906858m41.206308 15.81762l0 0q41.206308 15.81762 25.388689 57.023928l-120.213911 313.167945q-15.81762 41.206308-57.023928 25.388688l0 0q-41.206308-15.81762-25.388689-57.023928l120.213911-313.167944q15.81762-41.206308 57.023928-25.388689Z" fill="#6A6A6A" />
              </svg>
              <strong>邀请码</strong>
              <i class="nav-arrow" aria-hidden="true" />
            </button>
          </div>
          <div class="settings-nav-group">
            <p>安全</p>
            <button
              type="button"
              :class="{ active: activeTab === 'password' }"
              :aria-current="activeTab === 'password' ? 'page' : undefined"
              @click="selectTab('password')"
            >
              <svg class="settings-tab-icon" viewBox="0 0 1085 1024" aria-hidden="true">
                <path d="M467.897379 617.931034m-388.413793 0a388.413793 388.413793 0 1 0 776.827586 0 388.413793 388.413793 0 1 0-776.827586 0Z" fill="#D8D8D8" />
                <path d="M550.700138 32.944552a61.793103 61.793103 0 0 1 48.180965 0.441379c77.965241 33.82731 139.440552 58.91531 184.231725 75.211035 35.663448 12.958897 84.691862 20.850759 146.661517 23.128275a79.448276 79.448276 0 0 1 76.605793 79.448276v412.336552a114.758621 114.758621 0 0 1-34.568828 82.096552l-249.820689 243.994482a203.034483 203.034483 0 0 1-280.328828 3.283863L177.822897 706.877793a114.758621 114.758621 0 0 1-36.510897-83.93269V217.900138A79.448276 79.448276 0 0 1 221.854897 138.593103l1.341793 0.017656 0.988689 0.03531c33.509517 1.465379 80.648828-8.315586 140.358621-30.031448 80.578207-29.307586 141.594483-54.077793 182.695724-74.063449l3.478069-1.588965z m24.134621 86.210207l-0.459035 0.229517C530.749793 139.828966 470.987034 163.84 394.699034 191.558621c-63.558621 23.128276-117.248 35.133793-161.844965 35.486896l-3.266207-0.03531v395.934896c0 6.426483 2.348138 12.605793 6.532414 17.408l1.906758 1.959725 263.838897 245.989517a114.758621 114.758621 0 0 0 158.437517-1.836138l249.82069-244.012138a26.482759 26.482759 0 0 0 7.980138-18.944V219.577379l-2.56-0.088276c-61.863724-2.966069-113.540414-11.29931-155.330207-25.388137l-7.256276-2.542345c-42.707862-15.536552-98.921931-38.311724-168.854069-68.396138l-9.268965-4.007724z" fill="#464646" />
                <path d="M375.596138 541.254621a44.137931 44.137931 0 1 1 62.42869-62.42869l93.625379 93.643035 230.94731-230.964966a44.137931 44.137931 0 1 1 62.42869 62.42869l-262.17931 262.161655a44.137931 44.137931 0 0 1-60.292414 1.977379l-2.118621-1.977379-124.822069-124.839724z" fill="#6A6A6A" />
              </svg>
              <strong>修改密码</strong>
              <i class="nav-arrow" aria-hidden="true" />
            </button>
          </div>
        </aside>

        <main class="settings-panel">
          <section v-if="activeTab === 'profile'" class="settings-content" aria-labelledby="profile-title">
            <header class="panel-heading">
              <div>
                <p>ACCOUNT PROFILE</p>
                <h2 id="profile-title">个人信息</h2>
              </div>
              <span>READ ONLY</span>
            </header>

            <div class="profile-summary">
              <img class="profile-avatar" :src="defaultAvatar" alt="" />
              <div>
                <strong>{{ user.username }}</strong>
                <p>观看端账户</p>
              </div>
            </div>

            <dl class="profile-fields">
              <div>
                <dt>用户名</dt>
                <dd>{{ user.username }}</dd>
              </div>
              <div>
                <dt>注册时间</dt>
                <dd><time :datetime="registeredAtISO(user.createdAt)">{{ formatRegisteredAt(user.createdAt) }}</time></dd>
              </div>
              <div>
                <dt>注册方式</dt>
                <dd>{{ registrationMethod(user) }}</dd>
              </div>
            </dl>

            <p class="readonly-note"><i aria-hidden="true" />当前页面仅用于查看账户资料，个人信息暂不支持修改。</p>
          </section>

          <section v-else-if="activeTab === 'password'" class="settings-content" aria-labelledby="password-title">
            <header class="panel-heading">
              <div>
                <p>SECURITY CREDENTIALS</p>
                <h2 id="password-title">修改密码</h2>
              </div>
              <span>SECURE</span>
            </header>

            <p class="password-intro">验证现在的密码后，为账户设置一个 10 到 128 个字符的新密码。</p>

            <form class="password-form" @submit.prevent="changePassword">
              <label>
                <span>现在的密码</span>
                <input
                  v-model="passwordForm.currentPassword"
                  type="password"
                  autocomplete="current-password"
                  maxlength="128"
                  placeholder="请输入现在的密码"
                  @input="clearFeedback"
                />
              </label>
              <label>
                <span>新的密码</span>
                <input
                  v-model="passwordForm.newPassword"
                  type="password"
                  autocomplete="new-password"
                  maxlength="128"
                  placeholder="请输入新的密码"
                  @input="clearFeedback"
                />
                <small :class="{ invalid: passwordForm.newPassword !== '' && (newPasswordLength < 10 || newPasswordLength > 128) }">
                  需要 10 到 128 个字符
                </small>
              </label>
              <label>
                <span>再输入一次</span>
                <input
                  v-model="passwordForm.confirmPassword"
                  type="password"
                  autocomplete="new-password"
                  maxlength="128"
                  placeholder="请再次输入新的密码"
                  :aria-invalid="confirmationMismatch"
                  @input="clearFeedback"
                />
                <small v-if="confirmationMismatch" class="invalid">两次输入的新密码不一致</small>
              </label>

              <p
                v-if="feedback"
                class="form-feedback"
                :class="feedbackType"
                :role="feedbackType === 'error' ? 'alert' : 'status'"
              >
                <i aria-hidden="true" />{{ feedback }}
              </p>

              <button type="submit" :disabled="submitting || !passwordFormComplete">
                <span v-if="submitting" class="button-spinner" aria-hidden="true" />
                {{ submitting ? '正在提交' : '确认修改' }}
              </button>
            </form>
          </section>

          <section v-else class="settings-content" aria-labelledby="invitations-title">
            <header class="panel-heading">
              <div>
                <p>INVITATION ACCESS</p>
                <h2 id="invitations-title">邀请码</h2>
              </div>
              <span>COMMUNITY</span>
            </header>

            <div v-if="invitationsLoading && !invitations" class="invitation-state" aria-live="polite">
              <span class="invitation-loader" aria-hidden="true" />
              <p>正在计算邀请码额度</p>
            </div>
            <div v-else-if="invitationsError && !invitations" class="invitation-state error" role="alert">
              <strong>!</strong>
              <p>{{ invitationsError }}</p>
              <button type="button" @click="loadInvitations">重新加载</button>
            </div>
            <template v-else-if="invitations">
              <div class="invitation-actions">
                <div>
                  <strong>创建邀请码</strong>
                  <p>{{ invitationAvailabilityText(invitations) }}</p>
                </div>
                <button
                  type="button"
                  :disabled="invitationCreating || invitationsLoading || !invitations.allowance.canCreate"
                  @click="createInvitation"
                >
                  <span v-if="invitationCreating" class="button-spinner" aria-hidden="true" />
                  {{ invitationCreating ? '正在创建' : '创建邀请码' }}
                </button>
              </div>
              <p class="invitation-rule">
                注册满一周后即可创建邀请码。邀请码数量由账号使用时长决定。
              </p>

              <p
                v-if="invitationFeedback"
                class="form-feedback invitation-feedback"
                :class="invitationFeedbackType"
                :role="invitationFeedbackType === 'error' ? 'alert' : 'status'"
              >
                <i aria-hidden="true" />{{ invitationFeedback }}
              </p>

              <div class="invitation-table-wrap">
                <table class="invitation-table">
                  <thead>
                    <tr>
                      <th>邀请码</th>
                      <th>创建时间</th>
                      <th>使用时间</th>
                      <th>使用用户</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-if="invitations.items.length === 0">
                      <td colspan="4" class="invitation-empty">尚未创建邀请码</td>
                    </tr>
                    <tr v-for="invite in invitations.items" :key="invite.id">
                      <td>
                        <button class="invitation-code" type="button" title="点击复制" @click="copyInvitation(invite)">
                          {{ invite.code }}
                        </button>
                      </td>
                      <td>{{ formatInvitationDate(invite.createdAt) }}</td>
                      <td>{{ formatInvitationDate(invite.usedAt) }}</td>
                      <td>
                        <span :class="{ unused: !invite.used }">{{ invite.usedByUsername || '未使用' }}</span>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </template>
          </section>
        </main>
      </div>
    </div>
  </section>
</template>

<style scoped>
.settings-stage { position: relative; min-height: calc(100vh - 86px); overflow: hidden; background: linear-gradient(145deg, rgba(255,249,252,.96), rgba(244,252,255,.94)); }
.settings-grid { position: absolute; inset: 0; pointer-events: none; background: linear-gradient(rgba(85,119,217,.05) 1px, transparent 1px), linear-gradient(90deg, rgba(255,95,158,.045) 1px, transparent 1px); background-size: 64px 64px; mask-image: linear-gradient(to bottom, #000, transparent 88%); }
.settings-halo { position: absolute; width: 480px; height: 480px; border-radius: 50%; filter: blur(94px); pointer-events: none; }
.settings-halo.pink { top: -330px; right: 7%; background: rgba(255,159,189,.3); }
.settings-halo.cyan { bottom: -340px; left: -170px; background: rgba(73,214,233,.2); }
.settings-wrap { position: relative; z-index: 2; width: min(1320px, calc(100% - 96px)); margin: 0 auto; padding: 54px 0 96px; }
.settings-heading { min-height: 124px; padding: 0 8px 28px; border-bottom: 1px solid var(--line-cool); }
.settings-heading > p { display: flex; align-items: center; gap: 12px; color: var(--pink-500); font-family: var(--font-mono); font-size: 13px; letter-spacing: 2px; }
.settings-heading > p i { width: 60px; height: 1px; background: linear-gradient(90deg, var(--pink-400), transparent); }
.settings-heading h1 { margin-top: 7px; color: var(--ink-900); font-size: 34px; line-height: 1.2; letter-spacing: 2px; }
.settings-heading > span { display: block; margin-top: 6px; color: var(--ink-400); font-size: 13px; letter-spacing: 1.5px; }
.settings-layout { display: grid; grid-template-columns: 248px minmax(0, 1fr); gap: 34px; margin-top: 36px; align-items: start; }
.settings-sidebar { padding: 13px; border: 1px solid var(--line-cool); background: rgba(255,255,255,.78); box-shadow: 0 18px 42px rgba(85,119,217,.1); backdrop-filter: blur(15px); clip-path: polygon(0 0, calc(100% - 16px) 0, 100% 16px, 100% 100%, 16px 100%, 0 calc(100% - 16px)); }
.settings-nav-group + .settings-nav-group { margin-top: 18px; padding-top: 17px; border-top: 1px solid rgba(85,119,217,.1); }
.settings-nav-group > p { padding: 0 11px 8px; color: var(--ink-400); font-family: var(--font-mono); font-size: 11px; letter-spacing: 2px; }
.settings-nav-group > button { position: relative; width: 100%; height: 46px; display: grid; grid-template-columns: 24px minmax(0, 1fr) 8px; align-items: center; gap: 10px; padding: 0 13px; color: var(--ink-600); text-align: left; clip-path: polygon(var(--bevel-sm)); transition: color 160ms ease, background 160ms ease, transform 160ms ease; }
.settings-nav-group > button:hover { color: var(--pink-600); background: var(--pink-50); }
.settings-nav-group > button.active { color: #fff; background: linear-gradient(135deg, var(--pink-500), var(--pink-600)); box-shadow: 0 9px 20px rgba(255,95,158,.24); }
.settings-nav-group > button + button { margin-top: 4px; }
.settings-nav-group strong { overflow: hidden; font-size: 14px; text-overflow: ellipsis; white-space: nowrap; }
.settings-tab-icon { width: 22px; height: 22px; display: block; }
.nav-arrow { width: 6px; height: 6px; border-top: 1px solid currentColor; border-right: 1px solid currentColor; transform: rotate(45deg); }
.settings-panel { min-height: 520px; border: 1px solid var(--line-cool); background: rgba(255,255,255,.86); box-shadow: 0 22px 52px rgba(85,119,217,.11); backdrop-filter: blur(18px); clip-path: polygon(0 0, calc(100% - 22px) 0, 100% 22px, 100% 100%, 22px 100%, 0 calc(100% - 22px)); }
.settings-content { padding: 36px 42px 44px; animation: bp-rise .36s var(--ease-out) both; }
.panel-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 24px; padding-bottom: 24px; border-bottom: 1px solid rgba(85,119,217,.12); }
.panel-heading p { color: var(--blue-500); font-family: var(--font-mono); font-size: 12px; letter-spacing: 1.7px; }
.panel-heading h2 { margin-top: 4px; color: var(--ink-900); font-size: 25px; letter-spacing: 1px; }
.panel-heading > span { padding: 5px 11px; color: var(--pink-500); font-family: var(--font-mono); font-size: 10px; letter-spacing: 1.3px; border: 1px solid var(--line); background: var(--pink-50); clip-path: polygon(var(--bevel-sm)); }
.profile-summary { display: flex; align-items: center; gap: 20px; margin-top: 34px; padding: 24px 26px; background: linear-gradient(120deg, rgba(255,244,248,.88), rgba(236,253,255,.72)); border-left: 3px solid var(--pink-400); }
.profile-avatar { width: 62px; height: 62px; flex: 0 0 auto; object-fit: cover; background: linear-gradient(135deg, var(--cyan-400), var(--blue-500)); box-shadow: 0 13px 26px rgba(85,119,217,.2); clip-path: polygon(14px 0, 100% 0, 100% calc(100% - 14px), calc(100% - 14px) 100%, 0 100%, 0 14px); }
.profile-summary strong { color: var(--ink-900); font-size: 20px; }
.profile-summary p { margin-top: 4px; color: var(--ink-400); font-size: 13px; }
.profile-fields { margin-top: 26px; border-top: 1px solid rgba(85,119,217,.11); }
.profile-fields > div { min-height: 68px; display: grid; grid-template-columns: 150px minmax(0, 1fr); align-items: center; gap: 24px; border-bottom: 1px solid rgba(85,119,217,.11); }
.profile-fields dt { color: var(--ink-400); font-size: 13px; }
.profile-fields dd { color: var(--ink-700); font-size: 14px; }
.readonly-note { display: flex; align-items: center; gap: 9px; margin-top: 24px; color: var(--ink-400); font-size: 12px; }
.readonly-note i { width: 7px; height: 7px; flex: 0 0 auto; background: var(--cyan-400); transform: rotate(45deg); }
.invitation-state { min-height: 310px; display: grid; place-items: center; align-content: center; gap: 12px; color: var(--ink-400); }
.invitation-state.error strong { color: rgba(255,95,158,.3); font-family: var(--font-mono); font-size: 44px; }
.invitation-state p { font-size: 13px; }
.invitation-state button { padding: 8px 15px; color: var(--pink-600); font-size: 12px; border: 1px solid var(--line); background: var(--pink-50); clip-path: polygon(var(--bevel-sm)); }
.invitation-loader { width: 28px; height: 28px; border: 3px solid rgba(255,95,158,.16); border-top-color: var(--pink-500); border-radius: 50%; animation: settings-spin .8s linear infinite; }
.invitation-actions { display: flex; align-items: center; justify-content: space-between; gap: 24px; margin-top: 22px; padding: 18px 20px; border-left: 3px solid var(--cyan-400); background: rgba(247,252,255,.82); }
.invitation-actions > div > strong { color: var(--ink-700); font-size: 14px; }
.invitation-actions > div > p { margin-top: 3px; color: var(--ink-400); font-size: 12px; }
.invitation-actions > button { min-width: 132px; height: 40px; display: inline-flex; align-items: center; justify-content: center; gap: 8px; padding: 0 16px; color: #fff; font-size: 13px; background: linear-gradient(135deg, var(--pink-500), var(--pink-600)); box-shadow: 0 10px 22px rgba(255,95,158,.22); clip-path: polygon(var(--bevel-sm)); }
.invitation-actions > button:disabled { cursor: not-allowed; opacity: .48; box-shadow: none; }
.invitation-rule { margin-top: 10px; color: var(--ink-400); font-size: 11px; line-height: 1.7; }
.invitation-feedback { margin-top: 18px; }
.invitation-table-wrap { margin-top: 22px; overflow: hidden; border: 1px solid rgba(85,119,217,.14); clip-path: polygon(0 0, calc(100% - 10px) 0, 100% 10px, 100% 100%, 10px 100%, 0 calc(100% - 10px)); }
.invitation-table { width: 100%; border-collapse: collapse; table-layout: fixed; background: rgba(255,255,255,.78); }
.invitation-table th { height: 42px; padding: 0 13px; color: var(--ink-400); font-size: 11px; text-align: left; background: rgba(244,248,253,.92); border-bottom: 1px solid rgba(85,119,217,.13); }
.invitation-table th:first-child { width: 25%; }
.invitation-table th:nth-child(2), .invitation-table th:nth-child(3) { width: 26%; }
.invitation-table td { height: 54px; padding: 8px 13px; overflow: hidden; color: var(--ink-600); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; border-bottom: 1px solid rgba(85,119,217,.09); }
.invitation-table tr:last-child td { border-bottom: 0; }
.invitation-code { max-width: 100%; overflow: hidden; color: var(--pink-600); font-family: var(--font-mono); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.invitation-code:hover { color: var(--blue-500); }
.invitation-table .unused { color: var(--ink-300); }
.invitation-empty { height: 110px !important; color: var(--ink-400) !important; text-align: center; }
.password-intro { margin-top: 28px; color: var(--ink-600); font-size: 14px; }
.password-form { width: min(560px, 100%); display: grid; gap: 20px; margin-top: 27px; }
.password-form label { display: grid; gap: 8px; }
.password-form label > span { color: var(--ink-700); font-size: 13px; }
.password-form input { width: 100%; height: 46px; padding: 0 14px; color: var(--ink-900); border: 1px solid rgba(85,119,217,.2); background: rgba(250,252,255,.92); clip-path: polygon(var(--bevel-sm)); transition: border-color 160ms ease, box-shadow 160ms ease, background 160ms ease; }
.password-form input:focus { border-color: var(--pink-400); background: #fff; box-shadow: 0 0 0 3px rgba(255,95,158,.09); }
.password-form input[aria-invalid='true'] { border-color: var(--pink-500); }
.password-form input::placeholder { color: var(--ink-300); }
.password-form small { margin-top: -3px; color: var(--ink-400); font-size: 11px; }
.password-form small.invalid { color: var(--pink-600); }
.form-feedback { min-height: 42px; display: flex; align-items: center; gap: 10px; padding: 9px 13px; font-size: 13px; border: 1px solid; clip-path: polygon(var(--bevel-sm)); }
.form-feedback i { width: 7px; height: 7px; flex: 0 0 auto; transform: rotate(45deg); }
.form-feedback.success { color: #247c76; border-color: rgba(30,195,216,.28); background: rgba(236,253,255,.82); }
.form-feedback.success i { background: var(--cyan-500); }
.form-feedback.error { color: var(--pink-700); border-color: var(--line); background: var(--pink-50); }
.form-feedback.error i { background: var(--pink-500); }
.password-form > button { width: 152px; height: 44px; display: inline-flex; align-items: center; justify-content: center; gap: 9px; margin-top: 3px; color: #fff; font-size: 14px; letter-spacing: .5px; background: linear-gradient(135deg, var(--pink-500), var(--pink-600)); box-shadow: 0 12px 24px rgba(255,95,158,.24); clip-path: polygon(var(--bevel-sm)); transition: transform 160ms ease, box-shadow 160ms ease, opacity 160ms ease; }
.password-form > button:hover:not(:disabled) { transform: translateY(-2px); box-shadow: 0 17px 30px rgba(255,95,158,.3); }
.password-form > button:disabled { cursor: not-allowed; opacity: .5; box-shadow: none; }
.button-spinner { width: 14px; height: 14px; border: 2px solid rgba(255,255,255,.45); border-top-color: #fff; border-radius: 50%; animation: settings-spin .7s linear infinite; }
@keyframes settings-spin { to { transform: rotate(360deg); } }
</style>
