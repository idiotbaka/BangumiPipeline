<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import { APIError, api, type TranslationContent, type TranslationProgressTarget } from '../api'
import { translationKinds } from '../progress'

const props = defineProps<{ bangumiId: number; target: TranslationProgressTarget }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const content = shallowRef<TranslationContent | null>(null)
const translatedText = ref('')
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const conflicted = ref(false)
let active = true
const kindLabel = translationKinds.find((item) => item.value === props.target.kind)?.label ?? props.target.kind

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await api.translationContent(props.bangumiId, props.target.kind, props.target.id)
    if (!active) return
    content.value = result.item
    translatedText.value = result.item.translatedText
    conflicted.value = false
  } catch (cause) {
    if (active) error.value = cause instanceof Error ? cause.message : '加载翻译内容失败'
  } finally {
    if (active) loading.value = false
  }
}

function close() {
  if (!saving.value) emit('close')
}

async function save() {
  const current = content.value
  if (!current || loading.value || saving.value || conflicted.value) return
  if (!translatedText.value.trim()) {
    error.value = '中文译文不能为空'
    return
  }
  saving.value = true
  error.value = ''
  try {
    await api.updateTranslation(props.bangumiId, current.kind, current.id, {
      originalText: current.originalText,
      expectedTranslatedText: current.translatedText,
      translatedText: translatedText.value,
    })
    if (!active) return
    ElMessage.success('翻译已保存')
    emit('close')
    emit('saved')
  } catch (cause) {
    if (!active) return
    error.value = cause instanceof Error ? cause.message : '保存翻译失败'
    conflicted.value = cause instanceof APIError && (cause.code === 'translation_conflict' || cause.code === 'translation_not_found')
  } finally {
    if (active) saving.value = false
  }
}

onMounted(load)
onBeforeUnmount(() => { active = false })
</script>

<template>
  <el-dialog :model-value="true" title="编辑翻译" width="min(820px, 92vw)" top="6vh" append-to-body :before-close="close" :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving">
    <div v-loading="loading" class="translation-editor-body">
      <p>{{ kindLabel }} · {{ target.label }}（Bangumi ID：{{ target.id }}）</p>
      <el-alert v-if="content?.shared" title="该声优的简介译文由所有关联番剧共用，保存后会同步影响这些番剧。" type="info" :closable="false" show-icon />
      <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon style="margin: 12px 0" />
      <el-button v-if="!content || conflicted" link type="primary" :disabled="loading || saving" @click="load">{{ conflicted ? '重新加载内容（会替换当前输入）' : '重新加载内容' }}</el-button>
      <el-form v-if="content" label-position="top" @submit.prevent="save">
        <el-form-item label="原文（只读）">
          <el-input :model-value="content.originalText" type="textarea" :autosize="{ minRows: 4, maxRows: 7 }" readonly aria-label="原文（只读）" />
        </el-form-item>
        <el-form-item label="中文译文" required>
          <el-input v-model="translatedText" type="textarea" :autosize="{ minRows: 5, maxRows: 9 }" :disabled="saving || loading || !content.originalText" placeholder="填写中文译文，保存后该项目将计为已完成" aria-label="中文译文" />
        </el-form-item>
        <p class="progress-hint">原文发生变化时，元数据刷新仍会清空对应译文。保存不会清除计划任务上一次的失败记录。</p>
      </el-form>
    </div>
    <template #footer>
      <el-button :disabled="saving" @click="close">取消</el-button>
      <el-button type="primary" :loading="saving" :disabled="loading || !content?.originalText || !translatedText.trim() || conflicted" @click="save">保存翻译</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.translation-editor-body {
  max-height: calc(88vh - 150px);
  overflow-y: auto;
  padding: 0 4px;
}
</style>
