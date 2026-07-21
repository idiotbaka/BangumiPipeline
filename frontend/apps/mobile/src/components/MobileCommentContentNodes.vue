<script setup lang="ts">
import type { CommentContentNode } from '../commentContent'

defineOptions({ name: 'MobileCommentContentNodes' })
defineProps<{ nodes: CommentContentNode[] }>()

function hideBrokenImage(event: Event) {
  const image = event.currentTarget
  if (image instanceof HTMLImageElement) image.hidden = true
}

function externalImageStyle(node: Extract<CommentContentNode, { type: 'image' }>) {
  if (!node.width || !node.height) return undefined
  return {
    width: `${node.width}px`,
    aspectRatio: `${node.width} / ${node.height}`,
  }
}
</script>

<template>
  <span class="comment-content-fragment">
    <template v-for="(node, index) in nodes" :key="index">
      <span v-if="node.type === 'text'">{{ node.value }}</span>
      <s v-else-if="node.type === 'strike'" class="comment-strike">
        <MobileCommentContentNodes :nodes="node.children" />
      </s>
      <span
        v-else-if="node.type === 'mask'"
        class="comment-mask"
        tabindex="0"
        title="点击查看隐藏内容"
      >
        <MobileCommentContentNodes :nodes="node.children" />
      </span>
      <img
        v-else-if="node.type === 'image'"
        class="comment-external-image"
        :class="{ 'comment-external-image--sized': Boolean(node.width && node.height) }"
        :src="node.url"
        :style="externalImageStyle(node)"
        alt="评论图片"
        loading="lazy"
        decoding="async"
        referrerpolicy="no-referrer"
        @error="hideBrokenImage"
      />
      <img
        v-else
        class="comment-smile"
        :src="node.url"
        :alt="node.code"
        :title="node.code"
        loading="lazy"
        decoding="async"
        @error="hideBrokenImage"
      />
    </template>
  </span>
</template>

<style scoped>
.comment-content-fragment {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.comment-strike {
  text-decoration-color: rgba(82, 96, 120, 0.8);
  text-decoration-thickness: 1.5px;
}

.comment-mask {
  padding: 1px 3px;
  color: transparent;
  text-shadow: 0 0 8px rgba(32, 40, 62, 0.92);
  background: rgba(32, 40, 62, 0.82);
  border-radius: 3px;
  outline: none;
  transition: color 150ms ease, text-shadow 150ms ease, background 150ms ease;
}

.comment-mask:focus,
.comment-mask:active {
  color: inherit;
  text-shadow: none;
  background: rgba(255, 225, 236, 0.72);
}

.comment-smile {
  display: inline-block;
  width: auto;
  height: auto;
  max-width: 70px;
  max-height: 70px;
  margin: 0 2px;
  object-fit: contain;
  vertical-align: middle;
}

.comment-external-image {
  box-sizing: border-box;
  display: block;
  width: auto;
  height: auto;
  max-width: min(100%, 360px);
  max-height: 280px;
  margin: 8px 0 3px;
  object-fit: contain;
  object-position: left center;
  background: rgba(246, 249, 253, 0.82);
  border: 1px solid rgba(85, 119, 217, 0.14);
  border-radius: 6px;
}

.comment-external-image--sized {
  display: inline-block;
  max-width: min(calc(100% - 12px), 360px);
  margin-right: 12px;
  vertical-align: top;
}
</style>
