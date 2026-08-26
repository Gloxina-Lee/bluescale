<script setup>
import { computed, onBeforeUnmount, onMounted } from 'vue'

const props = defineProps({ images: { type: Array, required: true }, index: { type: Number, required: true } })
const emit = defineEmits(['close', 'change', 'copy'])
const image = computed(() => props.images[props.index])

function previous() {
  if (props.index > 0) emit('change', props.index - 1)
}

function next() {
  if (props.index < props.images.length - 1) emit('change', props.index + 1)
}

function handleKeydown(event) {
  if (event.key === 'Escape') emit('close')
  if (event.key === 'ArrowLeft') previous()
  if (event.key === 'ArrowRight') next()
}

function formatBytes(bytes) {
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function formatDate(value) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

onMounted(() => {
  document.body.classList.add('preview-open')
  window.addEventListener('keydown', handleKeydown)
})
onBeforeUnmount(() => {
  document.body.classList.remove('preview-open')
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <div class="image-preview-layer" role="dialog" aria-modal="true" aria-label="图片预览" @mousedown.self="emit('close')">
    <button class="preview-close" type="button" aria-label="关闭预览" @click="emit('close')">×</button>
    <button class="preview-arrow previous" type="button" aria-label="上一张" :disabled="index === 0" @click="previous">‹</button>
    <figure v-if="image" class="image-preview-figure">
      <div class="preview-image-stage"><img :src="image.url" :alt="image.originalName" /></div>
      <figcaption>
        <div><strong>{{ image.originalName }}</strong><span>{{ image.mimeType }} · {{ formatBytes(image.size) }} · {{ formatDate(image.createdAt) }}</span></div>
        <span class="preview-position">{{ index + 1 }} / {{ images.length }}</span>
        <button class="secondary-button compact" type="button" @click="emit('copy', image)">复制链接</button>
        <a class="secondary-button compact" :href="image.url" target="_blank" rel="noreferrer">打开原图</a>
      </figcaption>
    </figure>
    <button class="preview-arrow next" type="button" aria-label="下一张" :disabled="index >= images.length - 1" @click="next">›</button>
  </div>
</template>
