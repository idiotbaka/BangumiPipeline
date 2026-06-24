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
const form = reactive({ username: '', password: '', confirmation: '' })

const validateConfirmation = (_rule: unknown, value: string, callback: (error?: Error) => void) => {
  callback(value === form.password ? undefined : new Error('两次输入的密码不一致'))
}

const rules: FormRules<typeof form> = {
  username: [
    { required: true, message: '请输入管理员用户名', trigger: 'blur' },
    { min: 3, max: 32, message: '用户名长度应为 3–32 个字符', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 10, max: 128, message: '密码长度应为 10–128 个字符', trigger: 'blur' },
  ],
  confirmation: [
    { required: true, message: '请再次输入密码', trigger: 'blur' },
    { validator: validateConfirmation, trigger: 'blur' },
  ],
}

async function submit() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  submitting.value = true
  try {
    const { user } = await api.setup(form.username, form.password)
    session.user = user
    ElMessage.success('管理员账号已创建')
    await router.replace('/dashboard')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '初始化失败')
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
      <h1>初始化管理后台</h1>
      <p>创建系统中的首个管理员账号。完成后即可登录统一管理采集、下载和媒体处理任务。</p>
    </section>

    <el-card class="auth-card" shadow="never">
      <template #header>
        <div>
          <h2>创建管理员</h2>
          <p>该页面只会在首次启动时显示。</p>
        </div>
      </template>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="submit">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" autocomplete="username" size="large" placeholder="至少 3 个字符" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" show-password autocomplete="new-password" size="large" placeholder="至少 10 个字符" />
        </el-form-item>
        <el-form-item label="确认密码" prop="confirmation">
          <el-input v-model="form.confirmation" type="password" show-password autocomplete="new-password" size="large" />
        </el-form-item>
        <el-button type="primary" size="large" native-type="submit" :loading="submitting" class="submit-button">
          创建账号并登录
        </el-button>
      </el-form>
    </el-card>
  </main>
</template>
