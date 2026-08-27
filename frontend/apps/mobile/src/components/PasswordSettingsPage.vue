<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { api } from '../api'

const form = reactive({ currentPassword: '', newPassword: '', confirmPassword: '' })
const submitting = ref(false)
const feedback = ref('')
const succeeded = ref(false)
const length = computed(() => Array.from(form.newPassword).length)
const mismatch = computed(() => form.confirmPassword !== '' && form.newPassword !== form.confirmPassword)
const complete = computed(() => Boolean(form.currentPassword && form.confirmPassword) && length.value >= 10 && length.value <= 128 && !mismatch.value)

async function changePassword() {
  if (submitting.value) return
  feedback.value = ''
  succeeded.value = false
  if (!form.currentPassword) { feedback.value = '请输入现在的密码'; return }
  if (length.value < 10 || length.value > 128) { feedback.value = '新密码需要 10 到 128 个字符'; return }
  if (form.newPassword !== form.confirmPassword) { feedback.value = '两次输入的新密码不一致'; return }
  submitting.value = true
  try {
    await api.changePassword(form.currentPassword, form.newPassword, form.confirmPassword)
    form.currentPassword = ''
    form.newPassword = ''
    form.confirmPassword = ''
    succeeded.value = true
    feedback.value = '密码修改成功'
  } catch (error) {
    feedback.value = error instanceof Error ? error.message : '密码修改失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="account-page">
    <form class="account-panel password-form" @submit.prevent="changePassword" @input="feedback = ''">
      <p class="account-kicker">ACCOUNT SECURITY</p>
      <h2>修改密码</h2>
      <p class="account-description">验证现在的密码后，设置新的登录密码。</p>
      <fieldset :disabled="submitting">
        <label for="mobile-current-password">现在的密码</label>
        <input id="mobile-current-password" v-model="form.currentPassword" type="password" autocomplete="current-password" placeholder="请输入现在的密码" required />
        <label for="mobile-new-password">新密码</label>
        <input id="mobile-new-password" v-model="form.newPassword" type="password" autocomplete="new-password" placeholder="10 到 128 个字符" aria-describedby="mobile-password-hint" required />
        <small id="mobile-password-hint">新密码需要 10 到 128 个字符。</small>
        <label for="mobile-confirm-password">确认新密码</label>
        <input id="mobile-confirm-password" v-model="form.confirmPassword" type="password" autocomplete="new-password" placeholder="再次输入新密码" :aria-invalid="mismatch" :aria-describedby="mismatch ? 'mobile-password-mismatch' : undefined" required />
        <small v-if="mismatch" id="mobile-password-mismatch" class="password-mismatch">两次输入的新密码不一致</small>
        <button class="account-primary" type="submit" :disabled="!complete || submitting">{{ submitting ? '正在保存…' : '保存新密码' }}</button>
      </fieldset>
      <p v-if="feedback" class="account-feedback" :class="{ error: !succeeded }" role="status">{{ feedback }}</p>
    </form>
  </div>
</template>
