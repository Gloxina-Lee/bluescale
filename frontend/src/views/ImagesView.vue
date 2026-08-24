<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'

const images = ref([])
const selected = ref(new Set())
const loading = ref(true)
const deleting = ref(false)
const error = ref('')
const copied = ref(null)

const allSelected = computed(() => images.value.length > 0 && selected.value.size === images.value.length)

async function loadImages() {
  loading.value = true
  error.value = ''
  try {
    const payload = await api('/api/images')
    images.value = payload.images
    selected.value = new Set()
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    loading.value = false
  }
}

function toggle(id) {
  const next = new Set(selected.value)
  next.has(id) ? next.delete(id) : next.add(id)
  selected.value = next
}

function toggleAll() {
  selected.value = allSelected.value ? new Set() : new Set(images.value.map((image) => image.id))
}

async function deleteSelected() {
  if (!selected.value.size || !window.confirm(`确定删除选中的 ${selected.value.size} 张图片吗？此操作无法撤销。`)) return
  deleting.value = true
  error.value = ''
  try {
    await api('/api/images', { method: 'DELETE', body: JSON.stringify({ ids: Array.from(selected.value) }) })
    images.value = images.value.filter((image) => !selected.value.has(image.id))
    selected.value = new Set()
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    deleting.value = false
  }
}

async function copyURL(image) {
  const url = new URL(image.url, window.location.origin).href
  await navigator.clipboard.writeText(url)
  copied.value = image.id
  window.setTimeout(() => { if (copied.value === image.id) copied.value = null }, 1600)
}

function formatBytes(bytes) {
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function formatDate(value) {
  return new Intl.DateTimeFormat('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

onMounted(loadImages)
</script>

<template>
  <section class="content-page">
    <div class="page-heading management-heading">
      <div>
        <span class="eyebrow">LIBRARY</span>
        <h1>图片管理</h1>
        <p>浏览、复制链接，或批量删除图片。</p>
      </div>
      <RouterLink class="primary-button header-action" to="/upload">
        <svg viewBox="0 0 24 24"><path d="M12 16V4m0 0L7 9m5-5 5 5M5 15v3a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-3"/></svg>
        上传图片
      </RouterLink>
    </div>

    <div class="library-toolbar">
      <label class="check-control">
        <input type="checkbox" :checked="allSelected" :disabled="!images.length" @change="toggleAll" />
        <span>全选</span>
      </label>
      <span class="image-count">{{ images.length }} 张图片</span>
      <div class="toolbar-spacer"></div>
      <span v-if="selected.size" class="selected-count">已选择 {{ selected.size }} 张</span>
      <button class="danger-button" type="button" :disabled="!selected.size || deleting" @click="deleteSelected">
        <svg viewBox="0 0 24 24"><path d="M4 7h16m-10 4v6m4-6v6M9 7l1-3h4l1 3m3 0-1 13H7L6 7"/></svg>
        {{ deleting ? '正在删除…' : '批量删除' }}
      </button>
    </div>

    <p v-if="error" class="inline-message error">{{ error }}</p>
    <div v-if="loading" class="loading-state"><span></span><p>正在读取图片…</p></div>
    <div v-else-if="!images.length" class="empty-library">
      <div class="empty-icon"><svg viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="16" rx="3"/><circle cx="9" cy="10" r="2"/><path d="m4 17 5-4 3 2 3-3 5 4"/></svg></div>
      <h2>图库还是空的</h2>
      <p>上传第一张图片，之后就可以在这里管理。</p>
      <RouterLink class="secondary-button" to="/upload">去上传</RouterLink>
    </div>
    <div v-else class="image-grid">
      <article v-for="image in images" :key="image.id" class="image-card" :class="{ selected: selected.has(image.id) }">
        <button class="card-select" type="button" :aria-label="selected.has(image.id) ? '取消选择' : '选择图片'" @click="toggle(image.id)">
          <span>{{ selected.has(image.id) ? '✓' : '' }}</span>
        </button>
        <a class="image-frame" :href="image.url" target="_blank" rel="noreferrer">
          <img :src="image.url" :alt="image.originalName" loading="lazy" />
        </a>
        <div class="image-meta">
          <div><strong :title="image.originalName">{{ image.originalName }}</strong><span>{{ formatBytes(image.size) }} · {{ formatDate(image.createdAt) }}</span></div>
          <button class="copy-button" type="button" :title="copied === image.id ? '已复制' : '复制图片链接'" @click="copyURL(image)">
            <svg v-if="copied !== image.id" viewBox="0 0 24 24"><rect x="8" y="8" width="11" height="11" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/></svg>
            <span v-else>✓</span>
          </button>
        </div>
      </article>
    </div>
  </section>
</template>

