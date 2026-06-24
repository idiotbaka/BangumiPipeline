<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Link } from '@element-plus/icons-vue'
import { api, type AnimeDetail } from '../api'

const props = defineProps<{
  bangumiId: number
}>()

const anime = ref<AnimeDetail | null>(null)
const loading = ref(false)
const weekdays = ['', '星期一', '星期二', '星期三', '星期四', '星期五', '星期六', '星期日']
let loadToken = 0

const score = computed(() => {
  const value = anime.value?.rating?.score
  return typeof value === 'number' ? value.toFixed(1) : '—'
})

const rank = computed(() => {
  const value = anime.value?.rating?.rank
  return typeof value === 'number' && value > 0 ? `#${value}` : '未排名'
})

function renderInfobox(value: unknown): string {
  if (typeof value === 'string' || typeof value === 'number') return String(value)
  if (Array.isArray(value)) {
    return value.map((item) => {
      if (item && typeof item === 'object' && 'v' in item) return String((item as { v: unknown }).v)
      return renderInfobox(item)
    }).filter(Boolean).join('、')
  }
  return value ? JSON.stringify(value) : '—'
}

async function load(bangumiId: number) {
  const token = ++loadToken
  anime.value = null

  if (!Number.isInteger(bangumiId) || bangumiId < 1) {
    loading.value = false
    return
  }

  loading.value = true
  try {
    const detail = (await api.animeDetail(bangumiId)).anime
    if (token === loadToken) {
      anime.value = detail
    }
  } catch (error) {
    if (token === loadToken) {
      ElMessage.error(error instanceof Error ? error.message : '番剧详情加载失败')
    }
  } finally {
    if (token === loadToken) {
      loading.value = false
    }
  }
}

watch(() => props.bangumiId, (bangumiId) => {
  void load(bangumiId)
}, { immediate: true })
</script>

<template>
  <section v-loading="loading" class="anime-detail-page">
    <template v-if="anime">
      <div class="anime-hero">
        <div class="anime-hero-bg" :style="anime.hasCover ? { backgroundImage: `url(/api/anime/${anime.bangumiId}/cover)` } : {}" />
        <div class="anime-hero-content">
          <img v-if="anime.hasCover" :src="`/api/anime/${anime.bangumiId}/cover`" :alt="anime.nameCN || anime.name" class="anime-detail-cover">
          <div v-else class="anime-detail-cover anime-cover-placeholder">NO COVER</div>
          <div class="anime-hero-copy">
            <div class="anime-hero-tags">
              <el-tag v-if="anime.platform" effect="dark">{{ anime.platform }}</el-tag>
              <el-tag effect="plain">Bangumi #{{ anime.bangumiId }}</el-tag>
            </div>
            <h1>{{ anime.nameCN || anime.name }}</h1>
            <p class="anime-original-title">{{ anime.name }}</p>
            <div class="anime-facts">
              <span>{{ anime.airDate || anime.detailDate || '首播未定' }}</span>
              <span>{{ weekdays[anime.airWeekday] || '播出星期未定' }}</span>
              <span>{{ anime.totalEpisodes || anime.eps || '未知' }} 话</span>
            </div>
            <a :href="anime.url" target="_blank" rel="noreferrer" class="bangumi-link"><el-icon><Link /></el-icon>在 Bangumi 查看</a>
          </div>
          <div class="rating-panel">
            <span>Bangumi 评分</span>
            <strong>{{ score }}</strong>
            <small>{{ rank }}</small>
          </div>
        </div>
      </div>

      <div class="detail-layout">
        <main>
          <el-card class="detail-section" shadow="never">
            <h2>剧情简介</h2>
            <p class="anime-summary">{{ anime.summary || '暂无简介。' }}</p>
          </el-card>

          <el-card class="detail-section" shadow="never">
            <div class="section-title-line"><h2>角色与声优</h2><span>最多展示前 10 个角色</span></div>
            <div v-if="anime.characters.length" class="character-grid">
              <article v-for="character in anime.characters.slice(0, 10)" :key="character.characterId" class="character-card">
                <img v-if="character.hasImage" :src="`/api/anime/${anime.bangumiId}/characters/${character.characterId}/image`" :alt="character.name">
                <div v-else class="character-image-placeholder">?</div>
                <div class="character-copy">
                  <div><strong>{{ character.name }}</strong><el-tag size="small" effect="plain">{{ character.relation || '角色' }}</el-tag></div>
                  <p>{{ character.summary || '暂无角色简介。' }}</p>
                  <div v-for="actor in character.actors" :key="actor.actorId" class="actor-line">
                    <el-avatar :size="30" :src="actor.hasImage ? `/api/actors/${actor.actorId}/image` : undefined">{{ actor.name.slice(0, 1) }}</el-avatar>
                    <span>CV {{ actor.name }}</span>
                  </div>
                </div>
              </article>
            </div>
            <el-empty v-else description="暂无角色数据" :image-size="72" />
          </el-card>
        </main>

        <aside>
          <el-card class="detail-section" shadow="never">
            <h2>标签</h2>
            <div class="tag-cloud">
              <el-tag v-for="tag in anime.tags" :key="tag.name" effect="plain">{{ tag.name }}</el-tag>
            </div>
          </el-card>

          <el-card v-if="anime.aliases.length" class="detail-section" shadow="never">
            <h2>别名</h2>
            <ul class="alias-list"><li v-for="alias in anime.aliases" :key="alias">{{ alias }}</li></ul>
          </el-card>

          <el-card class="detail-section" shadow="never">
            <h2>完整元数据</h2>
            <dl class="infobox-list">
              <div v-for="(item, index) in anime.infobox" :key="`${item.key}-${index}`">
                <dt>{{ item.key || '未命名' }}</dt><dd>{{ renderInfobox(item.value) }}</dd>
              </div>
            </dl>
          </el-card>
        </aside>
      </div>
    </template>
  </section>
</template>
