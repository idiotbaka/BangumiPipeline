<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, type ViewerInvitation, type ViewerInvitationOverview } from '../api'
import { formatAccountDate } from '../account'
import { copyText } from '../native/clipboard'

const overview = ref<ViewerInvitationOverview | null>(null)
const loading = ref(false)
const creating = ref(false)
const copying = ref<number | null>(null)
const copied = ref<number | null>(null)
const loadError = ref('')
const feedback = ref('')
const feedbackError = ref(false)
const canCreate = computed(() => Boolean(overview.value?.allowance.canCreate) && !loading.value && !creating.value && !loadError.value)
const availability = computed(() => {
  const allowance = overview.value?.allowance
  if (!allowance) return ''
  if (allowance.canCreate) return `当前可以创建 ${allowance.remainingCount} 个邀请码。`
  if (allowance.eligibleTotal >= allowance.maximumTotal) return `已达到最多 ${allowance.maximumTotal} 个邀请码的额度。`
  if (allowance.nextEligibleAt) return `下一份额度将在 ${formatAccountDate(allowance.nextEligibleAt)} 获得。`
  return '当前没有可创建的邀请码额度。'
})

onMounted(loadInvitations)

async function loadInvitations() {
  if (loading.value) return
  loading.value = true
  loadError.value = ''
  try {
    overview.value = (await api.invitations()).invitations
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '邀请码加载失败'
  } finally {
    loading.value = false
  }
}

async function createInvitation() {
  if (!canCreate.value) return
  creating.value = true
  feedback.value = ''
  try {
    const { invite } = await api.generateInvitation()
    // 保留成功创建的码，即使随后的额度刷新失败，也可以立即复制。
    if (overview.value && !overview.value.items.some(item => item.id === invite.id)) {
      overview.value.items.unshift(invite)
      const allowance = overview.value.allowance
      allowance.createdCount += 1
      allowance.remainingCount = Math.max(0, allowance.remainingCount - 1)
      allowance.canCreate = allowance.remainingCount > 0
    }
    feedbackError.value = false
    feedback.value = '邀请码已创建，点击复制即可分享。'
    await loadInvitations()
  } catch (error) {
    feedbackError.value = true
    feedback.value = error instanceof Error ? error.message : '邀请码创建失败'
    // 另一端可能已消耗额度，重新读取服务端状态。
    await loadInvitations()
  } finally {
    creating.value = false
  }
}

async function copyInvitation(invite: ViewerInvitation) {
  if (copying.value !== null) return
  copying.value = invite.id
  copied.value = null
  try {
    await copyText(invite.code)
    copied.value = invite.id
    feedbackError.value = false
    feedback.value = '邀请码已复制'
  } catch {
    feedbackError.value = true
    feedback.value = '复制失败，可长按邀请码手动复制。'
  } finally {
    copying.value = null
  }
}
</script>

<template>
  <div class="account-page">
    <section class="account-panel invitation-quota">
      <p class="account-kicker">INVITATIONS</p>
      <h2>邀请好友，一起追番</h2>
      <p class="account-description">注册满一周后即可创建邀请码，数量由账号使用时长决定。</p>
      <div v-if="overview" class="invitation-allowance">
        <strong>{{ overview.allowance.remainingCount }}<small> 个可创建</small></strong>
        <span>已创建 {{ overview.allowance.createdCount }} / 最多 {{ overview.allowance.maximumTotal }} 个</span>
      </div>
      <p v-if="overview" class="account-description">{{ availability }}</p>
      <p v-if="loading && !overview" class="account-description" role="status">正在加载邀请码与额度…</p>
      <button class="account-primary" type="button" :disabled="!canCreate" @click="createInvitation">
        {{ creating ? '正在创建…' : '创建邀请码' }}
      </button>
    </section>
    <div v-if="loadError" class="account-feedback error" role="alert">
      <p>{{ loadError }}</p><button type="button" :disabled="loading" @click="loadInvitations">{{ loading ? '重试中…' : '重新加载' }}</button>
    </div>
    <p v-if="feedback" class="account-feedback" :class="{ error: feedbackError }" role="status">{{ feedback }}</p>
    <section v-if="overview" class="invitation-list" aria-label="已创建的邀请码">
      <header class="account-section-heading"><h3>我的邀请码</h3><span>{{ overview.items.length }} 个</span></header>
      <p v-if="!overview.items.length" class="account-panel account-description">还没有创建邀请码，获得额度后就可以邀请好友了。</p>
      <article v-for="invite in overview.items" :key="invite.id" class="account-panel invitation-card">
        <header><span class="invitation-status" :class="{ used: invite.used }">{{ invite.used ? '已使用' : '未使用' }}</span><small>{{ formatAccountDate(invite.createdAt) }}</small></header>
        <div class="invitation-code-row"><code>{{ invite.code }}</code><button type="button" :aria-label="`复制邀请码 ${invite.code}`" :disabled="copying !== null" @click="copyInvitation(invite)">{{ copying === invite.id ? '复制中…' : copied === invite.id ? '已复制' : '复制' }}</button></div>
        <p v-if="invite.used" class="account-description">由 {{ invite.usedByUsername || '未知用户' }} 使用 · {{ formatAccountDate(invite.usedAt) }}</p>
        <p v-else class="account-description">可以分享给好友，供一个账号注册使用。</p>
      </article>
    </section>
  </div>
</template>
