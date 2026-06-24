<script setup lang="ts">
import { computed, watch } from 'vue'
import { ArrowLeft } from '@element-plus/icons-vue'
import { useRoute, useRouter } from 'vue-router'
import AnimeDetailPanel from '../components/AnimeDetailPanel.vue'

const route = useRoute()
const router = useRouter()

const bangumiId = computed(() => {
  const value = Number(route.params.bangumiId)
  return Number.isInteger(value) && value > 0 ? value : null
})

watch(bangumiId, (value) => {
  if (value === null) {
    void router.replace('/anime')
  }
}, { immediate: true })
</script>

<template>
  <section>
    <el-button :icon="ArrowLeft" text class="back-button" @click="router.push('/anime')">返回番剧列表</el-button>
    <AnimeDetailPanel v-if="bangumiId !== null" :bangumi-id="bangumiId" />
  </section>
</template>
