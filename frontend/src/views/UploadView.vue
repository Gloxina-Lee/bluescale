<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import AlbumPickerModal from '../components/AlbumPickerModal.vue'
import ErrorToast from '../components/ErrorToast.vue'
import { api, uploadWithProgress } from '../api'

const router = useRouter()
const input = ref(null)
const items = ref([])
const dragging = ref(false)
const uploading = ref(false)
const progress = ref(0)
const error = ref('')
const successCount = ref(0)
const uploadSettings = ref({ maxImageSizeMB: 50, maxImagesPerUpload: 50 })
const albums = ref([])
const selectedAlbumIDs = ref([])
const albumPickerOpen = ref(false)
const isPublic = ref(false)
const uploadSettingsOpen = ref(false)
const uploadSettingsArea = ref(null)

const totalSize = computed(() => items.value.reduce((sum, item) => sum + item.file.size, 0))

function acceptFiles(fileList) {
  if (uploading.value) return
  error.value = ''
  successCount.value = 0
  const selected = Array.from(fileList)
  const additions = selected.filter((file) => /^(image\/(jpeg|png|gif|webp|avif))$/.test(file.type))
  const accepted = additions.filter((file) => file.size <= uploadSettings.value.maxImageSizeMB * 1024 * 1024)
  const known = new Set(items.value.map((item) => `${item.file.name}:${item.file.size}:${item.file.lastModified}`))
  let capacityExceeded = false
  for (const file of accepted) {
    const key = `${file.name}:${file.size}:${file.lastModified}`
    if (known.has(key)) continue
    if (items.value.length >= uploadSettings.value.maxImagesPerUpload) {
      capacityExceeded = true
      continue
    }
    items.value.push({ file, key })
    known.add(key)
  }
  if (!additions.length) error.value = '请选择 JPG、PNG、GIF、WebP 或 AVIF 图片'
  else if (accepted.length < additions.length) error.value = `已忽略超过 ${uploadSettings.value.maxImageSizeMB} MB 的图片`
  else if (capacityExceeded) error.value = `单次最多选择 ${uploadSettings.value.maxImagesPerUpload} 张图片`
}

async function loadUploadSettings() {
  try {
    const [settings, albumPayload] = await Promise.all([api('/api/settings'), api('/api/albums')])
    uploadSettings.value = settings.upload
    albums.value = albumPayload.albums
    if (items.value.length > settings.upload.maxImagesPerUpload) {
      items.value.splice(settings.upload.maxImagesPerUpload)
    }
  } catch (requestError) {
    if (requestError.status === 401) router.push({ name: 'login' })
    else error.value = requestError.message
  }
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
    const payload = await uploadWithProgress(items.value.map((item) => item.file), (value) => { progress.value = value }, selectedAlbumIDs.value, isPublic.value)
    successCount.value = payload.images.length
    clearItems()
    selectedAlbumIDs.value = []
    isPublic.value = false
    uploadSettingsOpen.value = false
    progress.value = 100
  } catch (requestError) {
    error.value = requestError.message
    if (requestError.status === 401) router.push({ name: 'login' })
  } finally {
    uploading.value = false
  }
}

function openAlbumPicker() {
  uploadSettingsOpen.value = false
  albumPickerOpen.value = true
}

function handleSettingsPointerDown(event) {
  if (uploadSettingsOpen.value && !uploadSettingsArea.value?.contains(event.target)) uploadSettingsOpen.value = false
}

function handleSettingsKeydown(event) {
  if (event.key === 'Escape') uploadSettingsOpen.value = false
}

function formatBytes(bytes) {
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

onMounted(() => {
  document.addEventListener('pointerdown', handleSettingsPointerDown)
  window.addEventListener('keydown', handleSettingsKeydown)
  loadUploadSettings()
})
onBeforeUnmount(() => {
  clearItems()
  document.removeEventListener('pointerdown', handleSettingsPointerDown)
  window.removeEventListener('keydown', handleSettingsKeydown)
})
</script>

<template>
  <section class="content-page upload-page">
    <ErrorToast :message="error" @close="error = ''" />
    <div class="page-heading">
      <div>
        <span class="eyebrow">UPLOAD STUDIO</span>
        <h1>上传图片</h1>
        <p>把图片拖进来，确认后点击上传图标。</p>
      </div>
      <div class="storage-note"><span class="pulse-dot"></span>图片将由 Go 从 <code>/i/</code> 直接提供</div>
    </div>

    <div class="drop-zone" :class="{ populated: items.length }">
      <input ref="input" class="visually-hidden" type="file" accept="image/jpeg,image/png,image/gif,image/webp,image/avif" multiple :disabled="uploading || items.length >= uploadSettings.maxImagesPerUpload" @change="acceptFiles($event.target.files)" />

      <aside class="upload-sidebar">
        <div class="selection-summary">
          <span class="summary-label">待上传</span>
          <div class="summary-count"><strong>{{ items.length }}</strong><span>/ {{ uploadSettings.maxImagesPerUpload }} 张</span></div>
          <p>{{ items.length ? `共 ${formatBytes(totalSize)}` : '尚未选择图片' }}</p>
        </div>

        <div class="upload-rules">
          <span>支持格式</span>
          <strong>JPG · PNG · GIF</strong>
          <strong>WEBP · AVIF</strong>
          <small>单张不超过 {{ uploadSettings.maxImageSizeMB }} MB</small>
        </div>

        <div class="sidebar-actions">
          <div class="sidebar-secondary-actions">
            <button class="secondary-button compact" type="button" :disabled="uploading || items.length >= uploadSettings.maxImagesPerUpload" @click="input.click()">继续添加</button>
            <button class="ghost-button compact" :class="{ 'danger-clear-button': items.length }" type="button" :disabled="!items.length || uploading" @click="clearItems">清空</button>
          </div>
          <div ref="uploadSettingsArea" class="upload-settings-menu">
            <button class="secondary-button compact upload-settings-trigger" type="button" :disabled="uploading" :aria-expanded="uploadSettingsOpen" @click="uploadSettingsOpen = !uploadSettingsOpen">
              <svg viewBox="0 0 24 24"><path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.86 2.86-.06-.06A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 .6l-.04.08V21h-4v-.92l-.04-.08a1.7 1.7 0 0 0-1-.6 1.7 1.7 0 0 0-1.88.34l-.06.06-2.86-2.86.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-.6-1L3.92 14H3v-4h.92L4 9.96a1.7 1.7 0 0 0 .6-1 1.7 1.7 0 0 0-.34-1.88l-.06-.06 2.86-2.86.06.06A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-.6l.04-.08V3h4v.92l.04.08a1.7 1.7 0 0 0 1 .6 1.7 1.7 0 0 0 1.88-.34l.06-.06 2.86 2.86-.06.06A1.7 1.7 0 0 0 19.4 9c.1.4.3.75.6 1l.08.04H21v4h-.92L20 14a1.7 1.7 0 0 0-.6 1Z"/></svg>
              上传设置<span v-if="selectedAlbumIDs.length" class="settings-count">{{ selectedAlbumIDs.length }}</span><svg class="menu-chevron" viewBox="0 0 24 24"><path d="m8 10 4 4 4-4"/></svg>
            </button>
            <div v-if="uploadSettingsOpen" class="upload-settings-popover">
              <button class="upload-visibility-control" type="button" :disabled="uploading" @click="isPublic = !isPublic">
                <span class="upload-setting-icon"><svg v-if="isPublic" viewBox="0 0 24 24"><path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6S2 12 2 12Z"/><circle cx="12" cy="12" r="2.5"/></svg><svg v-else viewBox="0 0 24 24"><rect x="5" y="10" width="14" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></svg></span>
                <span><strong>{{ isPublic ? '公开图片' : '私密图片' }}</strong><small>{{ isPublic ? '访客可查看并复制链接' : '仅登录或使用 Token 后可查看' }}</small></span>
                <span class="upload-setting-state">点击切换</span>
              </button>
              <button class="secondary-button compact album-upload-button" type="button" :disabled="uploading || !albums.length" @click="openAlbumPicker">
                <svg viewBox="0 0 24 24"><path d="M3 7h7l2 2h9v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/><path d="M12 12v5m-2.5-2.5h5"/></svg>
                {{ selectedAlbumIDs.length ? `上传到相册 · 已选 ${selectedAlbumIDs.length} 个` : '上传到相册' }}
              </button>
            </div>
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
            <span>{{ items.length }} / {{ uploadSettings.maxImagesPerUpload }}</span>
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
    <div v-if="successCount" class="inline-message success">
      <span>✓</span> 已成功上传 {{ successCount }} 张图片
      <RouterLink to="/images">前往图片管理</RouterLink>
    </div>
    <AlbumPickerModal v-if="albumPickerOpen" :albums="albums" :initial-selected="selectedAlbumIDs" title="上传到相册" description="本次选择的全部图片将同时加入所选相册。" confirm-text="确认相册" @close="albumPickerOpen = false" @confirm="selectedAlbumIDs = $event; albumPickerOpen = false" />
  </section>
</template>
