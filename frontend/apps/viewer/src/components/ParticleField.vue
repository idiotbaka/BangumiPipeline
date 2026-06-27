<script setup lang="ts">
import { computed } from 'vue'

/**
 * 碧蓝档案风格的「卡片碎片飘动」背景层。
 * 纯 CSS keyframes 驱动，所有碎片参数在 setup 里一次性随机生成并写入内联样式，
 * 无运行时 JS 物理计算，性能开销极低。
 */

type Shape = 'diamond' | 'triangle' | 'square' | 'circle' | 'ribbon'

interface Shard {
  shape: Shape
  size: number
  left: number
  top: number
  driftX: number
  driftY: number
  rotate: number
  duration: number
  delay: number
  opacity: number
  color: string
}

const props = withDefaults(
  defineProps<{
    /** 碎片数量 */
    count?: number
    /** 主题色调：pink 偏粉，cool 粉青蓝混合 */
    palette?: 'pink' | 'cool'
    /** 单个碎片最大尺寸（px） */
    maxSize?: number
  }>(),
  {
    count: 26,
    palette: 'pink',
    maxSize: 46,
  },
)

const SHAPES: Shape[] = ['diamond', 'triangle', 'square', 'circle', 'ribbon']

const PINK_PALETTE = [
  'rgba(255, 95, 158, 0.5)', // pink-500
  'rgba(255, 159, 189, 0.55)', // pink-300
  'rgba(255, 196, 217, 0.6)', // pink-200
  'rgba(255, 229, 122, 0.5)', // yellow-300
  'rgba(255, 255, 255, 0.7)',
]
const COOL_PALETTE = [
  'rgba(255, 95, 158, 0.45)',
  'rgba(73, 214, 233, 0.45)', // cyan-400
  'rgba(85, 119, 217, 0.4)', // blue-500
  'rgba(255, 229, 122, 0.45)',
  'rgba(255, 255, 255, 0.65)',
]

function rand(min: number, max: number): number {
  return Math.random() * (max - min) + min
}

const shards = computed<Shard[]>(() => {
  const palette = props.palette === 'cool' ? COOL_PALETTE : PINK_PALETTE
  const list: Shard[] = []
  for (let i = 0; i < props.count; i++) {
    const shape = SHAPES[Math.floor(Math.random() * SHAPES.length)]
    const size = rand(props.maxSize * 0.32, props.maxSize)
    const goUp = Math.random() > 0.32 // 大多数向上飘，营造轻盈感
    list.push({
      shape,
      size,
      left: rand(-2, 100),
      top: rand(-2, 104),
      driftX: rand(-60, 60),
      driftY: goUp ? rand(-180, -70) : rand(70, 180),
      rotate: rand(180, 540) * (Math.random() > 0.5 ? 1 : -1),
      duration: rand(9, 20),
      delay: rand(-12, 0),
      opacity: rand(0.55, 1),
      color: palette[Math.floor(Math.random() * palette.length)],
    })
  }
  return list
})

function shardStyle(s: Shard): Record<string, string> {
  return {
    left: `${s.left}%`,
    top: `${s.top}%`,
    width: `${s.size}px`,
    height: `${s.size}px`,
    opacity: String(s.opacity),
    background: s.color,
    animationDuration: `${s.duration}s`,
    animationDelay: `${s.delay}s`,
    ['--dx' as string]: `${s.driftX}px`,
    ['--dy' as string]: `${s.driftY}px`,
    ['--rot' as string]: `${s.rotate}deg`,
  }
}
</script>

<template>
  <div class="bp-particles" aria-hidden="true">
    <span
      v-for="(s, i) in shards"
      :key="i"
      class="bp-shard"
      :class="`shape-${s.shape}`"
      :style="shardStyle(s)"
    />
  </div>
</template>

<style scoped>
.bp-particles {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
  z-index: 1;
}

.bp-shard {
  position: absolute;
  display: block;
  border: 1px solid rgba(255, 255, 255, 0.7);
  box-shadow: 0 6px 18px rgba(85, 119, 217, 0.08);
  backdrop-filter: blur(0.5px);
  animation-name: bp-shard;
  animation-timing-function: ease-in-out;
  animation-iteration-count: infinite;
  will-change: transform, opacity;
}

/* 形状用 clip-path 实现，零圆角，几何感强 */
.shape-diamond {
  clip-path: polygon(50% 0, 100% 50%, 50% 100%, 0 50%);
  border-radius: 0;
}

.shape-triangle {
  clip-path: polygon(50% 0, 100% 100%, 0 100%);
  border-radius: 0;
}

.shape-square {
  border-radius: 2px;
}

.shape-circle {
  border-radius: 50%;
}

.shape-ribbon {
  clip-path: polygon(0 30%, 100% 0, 100% 70%, 0 100%);
  border-radius: 0;
}

@media (prefers-reduced-motion: reduce) {
  .bp-shard {
    animation: none;
    opacity: 0.5;
  }
}
</style>
