<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { api } from '../api'
import { session } from '../session'

const router = useRouter()
const formRef = ref<FormInstance>()
const submitting = ref(false)
const form = reactive({ username: '', password: '' })
const rules: FormRules<typeof form> = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

async function submit() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  submitting.value = true
  try {
    const { user } = await api.login(form.username, form.password)
    session.user = user
    await router.replace('/dashboard')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '登录失败')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="auth-shell">
    <section class="auth-intro">
      <div class="brand-mark">AB</div>
      <p class="eyebrow">AUTOBANGUMI</p>
      <h1>欢迎回来</h1>
      <p>登录后查看资源采集、下载、媒体整理和转码任务的统一状态。</p>
    </section>

    <el-card class="auth-card" shadow="never">
      <template #header>
        <div>
          <h2>管理员登录</h2>
          <p>使用初始化时创建的账号。</p>
        </div>
      </template>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="submit">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" autocomplete="username" size="large" autofocus />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" show-password autocomplete="current-password" size="large" />
        </el-form-item>
        <el-button type="primary" size="large" native-type="submit" :loading="submitting" class="submit-button">
          登录
        </el-button>
      </el-form>
    </el-card>
  </main>
</template>
