<script lang="ts">
const mobileCommentTimeFormatter = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
})
</script>

<script setup lang="ts">
import { computed, ref } from 'vue'

import { buildAuthenticatedMediaURL, type ViewerEpisodeComment } from '../api'
import { parseCommentContent } from '../commentContent'
import MobileCommentContentNodes from './MobileCommentContentNodes.vue'

defineOptions({ name: 'MobileEpisodeCommentItem' })
const props = withDefaults(defineProps<{
  comment: ViewerEpisodeComment
  smiles: Record<string, string>
  depth?: number
}>(), { depth: 0 })
const emit = defineEmits<{ (event: 'open-image', url: string): void }>()

const avatarFailed = ref(false)
const contentNodes = computed(() => parseCommentContent(props.comment.content, props.smiles))
const displayName = computed(() =>
  props.comment.user?.nickname.trim() || props.comment.user?.username.trim() || 'Bangumi 用户',
)
const displaySign = computed(() => props.comment.user?.sign.trim() ?? '')
const avatarInitial = computed(() => Array.from(displayName.value)[0] || 'B')
const avatarURL = computed(() => {
  const source = props.comment.user?.avatarUrl.trim() ?? ''
  return source.startsWith('/') ? buildAuthenticatedMediaURL(source) : source
})

function formatCommentTime(timestamp: number) {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '时间未知'
  return mobileCommentTimeFormatter.format(new Date(timestamp * 1000))
}

function commentDateTime(timestamp: number) {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return undefined
  return new Date(timestamp * 1000).toISOString()
}
</script>

<template>
  <article class="comment-item" :class="{ reply: depth > 0 }">
    <header class="comment-author">
      <div class="comment-avatar">
        <img
          v-if="avatarURL && !avatarFailed"
          :src="avatarURL"
          :alt="displayName"
          loading="lazy"
          decoding="async"
          referrerpolicy="no-referrer"
          @error="avatarFailed = true"
        />
        <span v-else>{{ avatarInitial }}</span>
      </div>
      <div class="comment-identity">
        <strong>{{ displayName }}</strong>
        <span v-if="displaySign" :title="displaySign">{{ displaySign }}</span>
      </div>
      <time :datetime="commentDateTime(comment.createdAt)">{{ formatCommentTime(comment.createdAt) }}</time>
    </header>

    <div class="comment-body">
      <MobileCommentContentNodes
        v-if="contentNodes.length"
        :nodes="contentNodes"
        @open-image="emit('open-image', $event)"
      />
      <span v-else class="comment-filtered">内容不可显示</span>
    </div>

    <div v-if="comment.replies.length" class="comment-replies" :class="{ compact: depth >= 1 }">
      <MobileEpisodeCommentItem
        v-for="reply in comment.replies"
        :key="reply.commentId"
        :comment="reply"
        :smiles="smiles"
        :depth="depth + 1"
        @open-image="emit('open-image', $event)"
      />
    </div>
  </article>
</template>

<style scoped>
.comment-item {
  min-width: 0;
  padding: 15px 14px 16px;
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid rgba(32, 40, 62, 0.06);
  border-radius: 9px;
  box-shadow: 0 10px 24px rgba(32, 40, 62, 0.035);
}

.comment-item:not(.reply) {
  contain: layout paint style;
  content-visibility: auto;
  contain-intrinsic-size: auto 140px;
}

.comment-item.reply {
  padding: 11px 10px 12px;
  background: rgba(246, 249, 253, 0.86);
  border-color: rgba(85, 119, 217, 0.1);
  box-shadow: none;
}

.comment-author {
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr);
  gap: 8px 9px;
  align-items: center;
}

.comment-avatar {
  grid-row: 1 / 3;
  width: 38px;
  height: 38px;
  overflow: hidden;
  display: grid;
  place-items: center;
  color: var(--pink-600);
  font-size: 14px;
  background: linear-gradient(145deg, var(--pink-50), var(--cyan-50));
  border: 1px solid rgba(255, 95, 158, 0.16);
  border-radius: 8px;
}

.comment-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.comment-identity {
  min-width: 0;
  align-self: end;
}

.comment-identity strong,
.comment-identity span {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.comment-identity strong {
  color: var(--ink-900);
  font-size: 13px;
  font-weight: 600;
}

.comment-identity span {
  margin-top: 1px;
  color: var(--ink-400);
  font-size: 11px;
}

.comment-author time {
  grid-column: 2;
  align-self: start;
  color: var(--ink-300);
  font-size: 10px;
  white-space: nowrap;
}

.comment-body {
  margin-top: 11px;
  color: var(--ink-700);
  font-size: 13px;
  line-height: 1.72;
}

.comment-filtered {
  color: var(--ink-300);
  font-style: italic;
}

.comment-replies {
  display: grid;
  gap: 8px;
  margin: 12px 0 0 18px;
}

.comment-replies.compact {
  margin-left: 0;
}

.comment-item.reply .comment-avatar {
  width: 32px;
  height: 32px;
}

.comment-item.reply .comment-author {
  grid-template-columns: 32px minmax(0, 1fr);
}
</style>
