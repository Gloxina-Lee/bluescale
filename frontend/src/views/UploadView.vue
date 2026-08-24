<script setup>
import { computed, onBeforeUnmount, ref } from 'vue'
import { useRouter } from 'vue-router'
import { uploadWithProgress } from '../api'
import { hasPermission } from '../session'

const router = useRouter()
const input = ref(null)
const items = ref([])
const dragging = ref(false)
const uploading = ref(false)
const progress = ref(0)
const error = ref('')
const successCount = ref(0)

const totalSize = computed(() => items.value.reduce((sum, item) => sum + item.file.size, 0))

function acceptFiles(fileList) {
  if (uploading.value) return
  error.value = ''
  successCount.value = 0
  const additions = Array.from(fileList).filter((file) => file.type.startsWith('image/'))
  const known = new Set(items.value.map((item) => `${item.file.name}:${item.file.size}:${item.file.lastModified}`))
  for (const file of additions) {
    const key = `${file.name}:${file.size}:${file.lastModified}`
    if (!known.has(key) && items.value.length < 50) {
      items.value.push({ file, key })
      known.add(key)
    }
  }
  if (!additions.length) error.value = '请选择 JPG、PNG、GIF、WebP 或 AVIF 图片'
}

function handleDrop(event) {
  dragging.value = false
  acceptFiles(event.dataTransfer.files)
}

function removeItem(index) {
  if (uploading.value) return
  items.value.splice(index, 1)
}

function clearItems() {
  items.value = []
  if (input.value) input.value.value = ''
}

async function startUpload() {
  if (!items.value.length || uploading.value) return
  error.value = ''
  progress.value = 0
  uploading.value = true
  try {
    const payload = await uploadWithProgress(items.value.map((item) => item.file), (value) => { progress.value = value })
    successCount.value = payload.images.length
    clearItems()
    progress.value = 100
  } catch (requestError) {
    error.value = requestError.message
    if (requestError.status === 401) router.push({ name: 'login' })
  } finally {
    uploading.value = false
  }
}

function formatBytes(bytes) {
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

onBeforeUnmount(clearItems)
</script>

<template>
  <section class="content-page upload-page">
    <div class="page-heading">
      <div>
        <span class="eyebrow">UPLOAD STUDIO</span>
        <h1>上传图片</h1>
        <p>把图片拖进来，确认后点击上传图标。</p>
      </div>
      <div class="storage-note"><span class="pulse-dot"></span>图片将由 Go 从 <code>/i/</code> 直接提供</div>
    </div>

    <div class="drop-zone" :class="{ populated: items.length }">
      <input ref="input" class="visually-hidden" type="file" accept="image/jpeg,image/png,image/gif,image/webp,image/avif" multiple :disabled="uploading || items.length >= 50" @change="acceptFiles($event.target.files)" />

      <aside class="upload-sidebar">
        <div class="selection-summary">
          <span class="summary-label">待上传</span>
          <div class="summary-count"><strong>{{ items.length }}</strong><span>/ 50 张</span></div>
          <p>{{ items.length ? `共 ${formatBytes(totalSize)}` : '尚未选择图片' }}</p>
        </div>

        <div class="upload-rules">
          <span>支持格式</span>
          <strong>JPG · PNG · GIF</strong>
          <strong>WEBP · AVIF</strong>
          <small>单张不超过 25 MB</small>
        </div>

        <div class="sidebar-actions">
          <div class="sidebar-secondary-actions">
            <button class="secondary-button compact" type="button" :disabled="uploading || items.length >= 50" @click="input.click()">继续添加</button>
            <button class="ghost-button compact" :class="{ 'danger-clear-button': items.length }" type="button" :disabled="!items.length || uploading" @click="clearItems">清空</button>
          </div>
          <button class="primary-button sidebar-upload-button" type="button" :disabled="!items.length || uploading" @click="startUpload">
            <svg v-if="!uploading" viewBox="0 0 24 24"><path d="M12 16V4m0 0L7 9m5-5 5 5M5 15v3a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-3"/></svg>
            <span>{{ uploading ? `正在上传 ${progress}%` : '开始上传' }}</span>
          </button>
        </div>
      </aside>

      <div
        class="upload-canvas"
        :class="{ dragging, populated: items.length }"
        @dragenter.prevent="dragging = true"
        @dragover.prevent="dragging = true"
        @dragleave.prevent="dragging = false"
        @drop.prevent="handleDrop"
      >
        <div v-if="dragging" class="drop-overlay">
          <span>松开即可添加图片</span>
        </div>

        <div v-if="!items.length" class="drop-empty">
          <button class="upload-orb" type="button" aria-label="从文件资源管理器选择图片" :disabled="uploading" @click="input.click()">
            <svg viewBox="0 0 24 24"><path d="M12 16V4m0 0L7 9m5-5 5 5M5 15v3a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-3"/></svg>
          </button>
          <h2>拖拽图片到这里</h2>
          <p>或者 <button class="text-button" type="button" @click="input.click()">从文件资源管理器选择</button></p>
          <span class="format-note">选择后将在这里显示文件列表</span>
        </div>

        <div v-else class="preview-area">
          <div class="preview-heading">
            <div><strong>待上传列表</strong><span>拖入更多图片可继续添加</span></div>
            <span>{{ items.length }} / 50</span>
          </div>
          <div class="file-list">
            <article v-for="(item, index) in items" :key="item.key" class="file-row">
              <div class="file-details">
                <strong :title="item.file.name">{{ item.file.name }}</strong>
                <span>{{ formatBytes(item.file.size) }}</span>
              </div>
              <button class="remove-file-button" type="button" :disabled="uploading" :aria-label="`移除 ${item.file.name}`" @click="removeItem(index)">移除</button>
            </article>
          </div>
        </div>
      </div>
    </div>

    <div v-if="uploading" class="progress-track"><i :style="{ width: `${progress}%` }"></i></div>
    <p v-if="error" class="inline-message error" role="alert">{{ error }}</p>
    <div v-if="successCount" class="inline-message success">
      <span>✓</span> 已成功上传 {{ successCount }} 张图片
      <RouterLink v-if="hasPermission('manageImages')" to="/images">前往图片管理</RouterLink>
    </div>
  </section>
</template>
