<script setup lang="ts">
import { computed, ref } from 'vue'

import type { ViewerEpisodeComment } from '../api'
import { parseCommentContent } from '../commentContent'
import CommentContentNodes from './CommentContentNodes.vue'

defineOptions({ name: 'EpisodeCommentItem' })
const props = withDefaults(defineProps<{
  comment: ViewerEpisodeComment
  smiles: Record<string, string>
  depth?: number
}>(), { depth: 0 })

const avatarFailed = ref(false)
const contentNodes = computed(() => parseCommentContent(props.comment.content, props.smiles))
const displayName = computed(() =>
  props.comment.user?.nickname.trim() || props.comment.user?.username.trim() || 'Bangumi 用户',
)
const displaySign = computed(() => props.comment.user?.sign.trim() ?? '')
const avatarInitial = computed(() => Array.from(displayName.value)[0] || 'B')

function formatCommentTime(timestamp: number) {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '时间未知'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
    hour12: false,
  }).format(new Date(timestamp * 1000))
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
          v-if="comment.user?.avatarUrl && !avatarFailed"
          :src="comment.user.avatarUrl"
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
      <time :datetime="commentDateTime(comment.createdAt)">
        {{ formatCommentTime(comment.createdAt) }}
      </time>
    </header>
    <div class="comment-body">
      <CommentContentNodes v-if="contentNodes.length" :nodes="contentNodes" />
      <span v-else class="comment-filtered">内容不可显示</span>
    </div>
    <div
      v-if="comment.replies.length"
      class="comment-replies"
      :class="{ compact: depth >= 2 }"
    >
      <EpisodeCommentItem
        v-for="reply in comment.replies"
        :key="reply.commentId"
        :comment="reply"
        :smiles="smiles"
        :depth="depth + 1"
      />
    </div>
  </article>
</template>

<style scoped>
.comment-item { min-width: 0; padding: 14px 12px 15px; border-bottom: 1px dashed rgba(85,119,217,.16); background: rgba(255,255,255,.5); }
.comment-item.reply { padding: 11px 10px 12px; border: 1px solid rgba(85,119,217,.1); background: rgba(246,249,253,.74); }
.comment-author { display: grid; grid-template-columns: 38px minmax(0, 1fr) auto; align-items: center; gap: 9px; }
.comment-avatar { width: 38px; height: 38px; overflow: hidden; display: grid; place-items: center; flex: 0 0 auto; color: var(--pink-600); font-size: 14px; background: linear-gradient(145deg, var(--pink-50), var(--cyan-50)); border: 1px solid rgba(255,95,158,.16); clip-path: polygon(0 0, calc(100% - 7px) 0, 100% 7px, 100% 100%, 7px 100%, 0 calc(100% - 7px)); }
.comment-avatar img { width: 100%; height: 100%; object-fit: cover; }
.comment-identity { min-width: 0; }
.comment-identity strong, .comment-identity span { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.comment-identity strong { color: var(--ink-900); font-size: 13px; font-weight: 600; }
.comment-identity span { margin-top: 2px; color: var(--ink-400); font-size: 11px; }
.comment-author time { align-self: start; padding-top: 2px; color: var(--ink-300); font-family: var(--font-mono); font-size: 10px; white-space: nowrap; }
.comment-body { margin: 10px 0 0 47px; color: var(--ink-700); font-size: 13px; line-height: 1.72; }
.comment-filtered { color: var(--ink-300); font-style: italic; }
.comment-replies { display: grid; gap: 7px; margin: 11px 0 0 28px; }
.comment-replies.compact { margin-left: 0; }
.comment-item.reply .comment-avatar { width: 32px; height: 32px; }
.comment-item.reply .comment-author { grid-template-columns: 32px minmax(0, 1fr) auto; }
.comment-item.reply .comment-body { margin-left: 41px; }
</style>
