<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AlbumPickerModal from '../components/AlbumPickerModal.vue'
import BaseSelect from '../components/BaseSelect.vue'
import ErrorToast from '../components/ErrorToast.vue'
import ImagePreviewModal from '../components/ImagePreviewModal.vue'
import { api } from '../api'
import { session } from '../session'

const route = useRoute()
const router = useRouter()
const images = ref([])
const albums = ref([])
const selected = ref(new Set())
const loading = ref(true)
const deleting = ref(false)
const albumWorking = ref(false)
const error = ref('')
const copied = ref(null)
const previewIndex = ref(-1)
const albumPickerOpen = ref(false)
const filtersOpen = ref(false)
const batchMenuOpen = ref(false)
const batchArea = ref(null)
const viewMode = ref(localStorage.getItem('bluescale-image-view') === 'list' ? 'list' : 'grid')
const formatFilter = ref(typeof route.query.format === 'string' ? route.query.format : 'all')
const albumFilter = ref(typeof route.query.album === 'string' ? route.query.album : 'all')
const visibilityFilter = ref(typeof route.query.visibility === 'string' ? route.query.visibility : 'all')
const page = ref(Math.max(1, Number.parseInt(route.query.page, 10) || 1))
const storedPageSize = Number.parseInt(localStorage.getItem('bluescale-image-page-size'), 10)
const pageSize = ref([12, 24, 48, 96].includes(storedPageSize) ? storedPageSize : 24)
const total = ref(0)
const totalPages = ref(0)
let loadGeneration = 0

const allSelected = computed(() => images.value.length > 0 && selected.value.size === images.value.length)
const isAdmin = computed(() => Boolean(session.user))
const activeAlbum = computed(() => albums.value.find((album) => String(album.id) === albumFilter.value) || null)
const hasFilters = computed(() => formatFilter.value !== 'all' || albumFilter.value !== 'all' || (isAdmin.value && visibilityFilter.value !== 'all'))
const pageSizeOptions = [12, 24, 48, 96].map((value) => ({ value, label: `${value} 张` }))
const formatOptions = [
  { value: 'all', label: '全部格式' },
  { value: 'jpeg', label: 'JPEG' },
  { value: 'png', label: 'PNG' },
  { value: 'gif', label: 'GIF' },
  { value: 'webp', label: 'WebP' },
  { value: 'avif', label: 'AVIF' },
]
const albumOptions = computed(() => [
  { value: 'all', label: '全部' },
  { value: 'none', label: '无相册' },
  ...albums.value.map((album) => ({ value: String(album.id), label: `${album.name}（${album.imageCount}）` })),
])
const visibilityOptions = [
  { value: 'all', label: '全部' },
  { value: 'public', label: '公开' },
  { value: 'private', label: '私密' },
]

async function loadAlbums() {
  try {
    const payload = await api('/api/albums')
    albums.value = payload.albums
    if (!['all', 'none'].includes(albumFilter.value) && !albums.value.some((album) => String(album.id) === albumFilter.value)) albumFilter.value = 'all'
  } catch (requestError) {
    error.value = requestError.message
  }
}

async function loadImages() {
  const generation = ++loadGeneration
  loading.value = true
  error.value = ''
  const parameters = new URLSearchParams({ page: String(page.value), pageSize: String(pageSize.value) })
  if (formatFilter.value !== 'all') parameters.set('format', formatFilter.value)
  if (albumFilter.value !== 'all') parameters.set('album', albumFilter.value)
  if (isAdmin.value && visibilityFilter.value !== 'all') parameters.set('visibility', visibilityFilter.value)
  try {
    const payload = await api(`/api/images?${parameters}`)
    if (generation !== loadGeneration) return
    images.value = payload.images
    total.value = payload.total
    totalPages.value = payload.totalPages
    selected.value = new Set()
    if (payload.page !== page.value) page.value = payload.page
  } catch (requestError) {
    if (generation === loadGeneration) error.value = requestError.message
  } finally {
    if (generation === loadGeneration) loading.value = false
  }
}

function syncQuery() {
  const query = {}
  if (formatFilter.value !== 'all') query.format = formatFilter.value
  if (albumFilter.value !== 'all') query.album = albumFilter.value
  if (isAdmin.value && visibilityFilter.value !== 'all') query.visibility = visibilityFilter.value
  if (page.value > 1) query.page = String(page.value)
  router.replace({ name: 'images', query })
}

function applyFilters() {
  page.value = 1
  syncQuery()
  loadImages()
}

function changePage(nextPage) {
  if (nextPage < 1 || nextPage > totalPages.value || nextPage === page.value) return
  page.value = nextPage
  syncQuery()
  loadImages()
}

function changePageSize() {
  localStorage.setItem('bluescale-image-page-size', String(pageSize.value))
  page.value = 1
  syncQuery()
  loadImages()
}

function clearFilters() {
  formatFilter.value = 'all'
  albumFilter.value = 'all'
  visibilityFilter.value = 'all'
  applyFilters()
}

function setViewMode(nextMode) {
  viewMode.value = nextMode
  localStorage.setItem('bluescale-image-view', nextMode)
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
	batchMenuOpen.value = false
  if (!selected.value.size || !window.confirm(`确定删除选中的 ${selected.value.size} 张图片吗？此操作无法撤销。`)) return
  deleting.value = true
  error.value = ''
  try {
    await api('/api/images', { method: 'DELETE', body: JSON.stringify({ ids: Array.from(selected.value) }) })
    await Promise.all([loadImages(), loadAlbums()])
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    deleting.value = false
  }
}

async function addSelectedToAlbums(albumIDs) {
  if (!selected.value.size || !albumIDs.length) return
  albumWorking.value = true
  error.value = ''
  try {
    await api('/api/images/albums', { method: 'POST', body: JSON.stringify({ imageIds: Array.from(selected.value), albumIds: albumIDs }) })
    albumPickerOpen.value = false
    await loadAlbums()
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    albumWorking.value = false
  }
}

async function removeSelectedFromAlbum() {
  if (!activeAlbum.value || !selected.value.size || !window.confirm(`从“${activeAlbum.value.name}”移出选中的 ${selected.value.size} 张图片？图片文件不会被删除。`)) return
  albumWorking.value = true
  error.value = ''
  try {
    await api('/api/images/albums', { method: 'DELETE', body: JSON.stringify({ imageIds: Array.from(selected.value), albumIds: [activeAlbum.value.id] }) })
    await Promise.all([loadImages(), loadAlbums()])
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    albumWorking.value = false
  }
}

async function updateVisibility(isPublic) {
	batchMenuOpen.value = false
  if (!selected.value.size || albumWorking.value) return
  albumWorking.value = true
  error.value = ''
  try {
    await api('/api/images/visibility', { method: 'PUT', body: JSON.stringify({ ids: Array.from(selected.value), isPublic }) })
    await Promise.all([loadImages(), loadAlbums()])
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    albumWorking.value = false
  }
}

function openAlbumPicker() {
  batchMenuOpen.value = false
  albumPickerOpen.value = true
}

function handleMenuPointerDown(event) {
  if (batchMenuOpen.value && !batchArea.value?.contains(event.target)) batchMenuOpen.value = false
}

function handleMenuKeydown(event) {
  if (event.key === 'Escape') batchMenuOpen.value = false
}

async function copyURL(image) {
  const url = new URL(image.url, window.location.origin).href
  await navigator.clipboard.writeText(url)
  copied.value = image.id
  window.setTimeout(() => { if (copied.value === image.id) copied.value = null }, 1600)
}

function openPreview(image) {
  previewIndex.value = images.value.findIndex((candidate) => candidate.id === image.id)
}

function formatBytes(bytes) {
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function formatDate(value) {
  return new Intl.DateTimeFormat('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

watch(() => route.query.album, (value) => {
  const next = typeof value === 'string' ? value : 'all'
  if (next !== albumFilter.value) {
    albumFilter.value = next
    page.value = 1
    loadImages()
  }
})

watch(() => route.query.visibility, (value) => {
  if (!isAdmin.value) return
  const next = typeof value === 'string' ? value : 'all'
  if (next !== visibilityFilter.value) {
    visibilityFilter.value = next
    page.value = 1
    loadImages()
  }
})

onMounted(async () => {
  document.addEventListener('pointerdown', handleMenuPointerDown)
  window.addEventListener('keydown', handleMenuKeydown)
  await loadAlbums()
  await loadImages()
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', handleMenuPointerDown)
  window.removeEventListener('keydown', handleMenuKeydown)
})
</script>

<template>
  <section class="content-page images-page" :class="{ 'public-library': !isAdmin }">
    <ErrorToast :message="error" @close="error = ''" />
    <div class="page-heading management-heading">
      <div><span class="eyebrow">LIBRARY</span><h1>{{ activeAlbum ? activeAlbum.name : (isAdmin ? '图片管理' : '公开图片') }}</h1><p>{{ activeAlbum ? (isAdmin ? '正在查看该相册中的图片；移出相册不会删除原图。' : '浏览该相册中公开分享的图片。') : (isAdmin ? '筛选、预览并管理已上传的图片。' : '无需登录即可浏览、预览并复制公开图片链接。') }}</p></div>
      <div class="header-library-controls">
        <div class="header-page-size"><span>每页</span><BaseSelect v-model="pageSize" :options="pageSizeOptions" aria-label="每页显示数量" @change="changePageSize" /></div>
        <div class="header-pagination" aria-label="分页"><button type="button" aria-label="上一页" :disabled="page <= 1 || loading" @click="changePage(page - 1)">‹</button><span><strong>{{ totalPages ? page : 0 }}</strong><i>/</i>{{ totalPages }}</span><button type="button" aria-label="下一页" :disabled="page >= totalPages || loading" @click="changePage(page + 1)">›</button></div>
      </div>
    </div>

    <div v-if="filtersOpen" class="image-filter-bar">
      <div class="filter-field"><span>图片格式</span><BaseSelect v-model="formatFilter" :options="formatOptions" aria-label="图片格式" @change="applyFilters" /></div>
      <div class="filter-field"><span>所在相册</span><BaseSelect v-model="albumFilter" :options="albumOptions" aria-label="所在相册" @change="applyFilters" /></div>
      <div v-if="isAdmin" class="filter-field"><span>可见范围</span><BaseSelect v-model="visibilityFilter" :options="visibilityOptions" aria-label="可见范围" @change="applyFilters" /></div>
      <button v-if="hasFilters" class="ghost-button compact filter-reset" type="button" @click="clearFilters">清除筛选</button>
    </div>

    <div class="library-toolbar">
      <label v-if="isAdmin" class="check-control"><input type="checkbox" :checked="allSelected" @change="toggleAll" /><span>全选本页</span></label>
      <button class="secondary-button compact filter-toggle-button" :class="{ active: filtersOpen || hasFilters }" type="button" :aria-expanded="filtersOpen" @click="filtersOpen = !filtersOpen">
        <svg viewBox="0 0 24 24"><path d="M4 6h16M7 12h10m-7 6h4"/></svg>筛选<span v-if="hasFilters" class="filter-active-dot" aria-label="已应用筛选"></span>
      </button>
      <span class="image-count">共 {{ total }} 张<span v-if="totalPages"> · 第 {{ page }} / {{ totalPages }} 页</span></span>
      <div class="toolbar-spacer"></div>
      <div class="view-mode-switch" role="group" aria-label="显示模式">
        <button type="button" title="网格视图" :class="{ active: viewMode === 'grid' }" :aria-pressed="viewMode === 'grid'" @click="setViewMode('grid')"><svg viewBox="0 0 24 24"><rect x="4" y="4" width="6" height="6" rx="1"/><rect x="14" y="4" width="6" height="6" rx="1"/><rect x="4" y="14" width="6" height="6" rx="1"/><rect x="14" y="14" width="6" height="6" rx="1"/></svg><span>网格</span></button>
        <button type="button" title="列表视图" :class="{ active: viewMode === 'list' }" :aria-pressed="viewMode === 'list'" @click="setViewMode('list')"><svg viewBox="0 0 24 24"><path d="M9 6h11M9 12h11M9 18h11"/><rect x="4" y="5" width="2" height="2" rx=".5"/><rect x="4" y="11" width="2" height="2" rx=".5"/><rect x="4" y="17" width="2" height="2" rx=".5"/></svg><span>列表</span></button>
      </div>
      <template v-if="isAdmin">
        <span v-if="selected.size" class="selected-count">已选择 {{ selected.size }} 张</span>
        <button v-if="activeAlbum" class="secondary-button compact remove-album-button" type="button" :disabled="!selected.size || albumWorking" @click="removeSelectedFromAlbum">移出相册</button>
        <div ref="batchArea" class="toolbar-menu-wrapper">
          <button class="secondary-button compact batch-menu-trigger" type="button" :disabled="!selected.size || albumWorking || deleting" :aria-expanded="batchMenuOpen" @click="batchMenuOpen = !batchMenuOpen">批量操作<svg viewBox="0 0 24 24"><path d="m8 10 4 4 4-4"/></svg></button>
          <div v-if="batchMenuOpen" class="toolbar-menu-popover" role="menu">
            <button type="button" role="menuitem" @click="updateVisibility(true)"><svg viewBox="0 0 24 24"><path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6S2 12 2 12Z"/><circle cx="12" cy="12" r="2.5"/></svg><span><strong>设为公开</strong><small>访客可查看和复制链接</small></span></button>
            <button type="button" role="menuitem" @click="updateVisibility(false)"><svg viewBox="0 0 24 24"><rect x="5" y="10" width="14" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></svg><span><strong>设为私密</strong><small>仅凭据有效时可查看</small></span></button>
            <button type="button" role="menuitem" :disabled="!albums.length" @click="openAlbumPicker"><svg viewBox="0 0 24 24"><path d="M3 7h7l2 2h9v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/><path d="M12 12v5m-2.5-2.5h5"/></svg><span><strong>加入相册</strong><small>可同时选择多个相册</small></span></button>
            <div class="toolbar-menu-divider"></div>
            <button class="danger" type="button" role="menuitem" @click="deleteSelected"><svg viewBox="0 0 24 24"><path d="M4 7h16m-10 4v6m4-6v6M9 7l1-3h4l1 3m3 0-1 13H7L6 7"/></svg><span><strong>{{ deleting ? '正在删除…' : '批量删除' }}</strong><small>永久删除图片文件</small></span></button>
          </div>
        </div>
      </template>
    </div>

    <div v-if="loading" class="loading-state"><span></span><p>正在读取图片…</p></div>
    <div v-else-if="!images.length" class="empty-library">
      <div class="empty-icon"><svg viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="16" rx="3"/><circle cx="9" cy="10" r="2"/><path d="m4 17 5-4 3 2 3-3 5 4"/></svg></div>
      <h2>{{ hasFilters ? '没有符合条件的图片' : (isAdmin ? '这里还没有图片' : '暂无公开图片') }}</h2><p>{{ hasFilters ? '尝试调整格式或相册筛选条件。' : (isAdmin ? '上传第一张图片，之后就可以在这里管理。' : '管理员尚未公开任何图片。') }}</p>
      <button v-if="hasFilters" class="secondary-button" type="button" @click="clearFilters">清除筛选</button><RouterLink v-else-if="isAdmin" class="secondary-button" to="/upload">去上传</RouterLink>
    </div>

    <div v-else-if="viewMode === 'grid'" class="image-grid">
      <article v-for="image in images" :key="image.id" class="image-card" :class="{ selected: selected.has(image.id) }">
        <button v-if="isAdmin" class="card-select" type="button" :aria-label="selected.has(image.id) ? '取消选择' : '选择图片'" @click="toggle(image.id)"><span>{{ selected.has(image.id) ? '✓' : '' }}</span></button>
        <button class="image-frame preview-trigger" type="button" :aria-label="`预览 ${image.originalName}`" @click="openPreview(image)"><img :src="image.url" :alt="image.originalName" loading="lazy" /></button>
        <div class="image-meta"><div><strong :title="image.originalName">{{ image.originalName }}</strong><span>{{ formatBytes(image.size) }} · {{ formatDate(image.createdAt) }}</span><small class="visibility-badge" :class="image.isPublic ? 'public' : 'private'">{{ image.isPublic ? '公开' : '私密' }}</small></div><button class="copy-button" type="button" :title="copied === image.id ? '已复制' : '复制图片链接'" @click="copyURL(image)"><svg v-if="copied !== image.id" viewBox="0 0 24 24"><rect x="8" y="8" width="11" height="11" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/></svg><span v-else>✓</span></button></div>
      </article>
    </div>

    <div v-else class="image-list" role="list">
      <div class="image-list-heading" :class="{ guest: !isAdmin }" aria-hidden="true"><span v-if="isAdmin"></span><span>名称</span><span>大小</span><span>上传时间</span><span></span></div>
      <article v-for="image in images" :key="image.id" class="image-list-row" :class="{ selected: selected.has(image.id), guest: !isAdmin }" role="listitem">
        <label v-if="isAdmin" class="list-select-control" :aria-label="selected.has(image.id) ? '取消选择' : '选择图片'"><input type="checkbox" :checked="selected.has(image.id)" @change="toggle(image.id)" /></label>
        <div class="list-image-identity"><button class="list-thumbnail preview-trigger" type="button" :aria-label="`预览 ${image.originalName}`" @click="openPreview(image)"><img :src="image.url" :alt="image.originalName" loading="lazy" /></button><div><strong :title="image.originalName">{{ image.originalName }}</strong><span>{{ image.mimeType }} · {{ image.isPublic ? '公开' : '私密' }}</span></div></div>
        <span class="list-file-size">{{ formatBytes(image.size) }}</span><time :datetime="image.createdAt">{{ formatDate(image.createdAt) }}</time>
        <button class="copy-button" type="button" :title="copied === image.id ? '已复制' : '复制图片链接'" @click="copyURL(image)"><svg v-if="copied !== image.id" viewBox="0 0 24 24"><rect x="8" y="8" width="11" height="11" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/></svg><span v-else>✓</span></button>
      </article>
    </div>

    <div v-if="totalPages > 1" class="bottom-pagination" aria-label="分页"><button type="button" :disabled="page <= 1" @click="changePage(page - 1)">上一页</button><span>第 {{ page }} / {{ totalPages }} 页</span><button type="button" :disabled="page >= totalPages" @click="changePage(page + 1)">下一页</button></div>

    <AlbumPickerModal v-if="isAdmin && albumPickerOpen" :albums="albums" title="把图片加入相册" :description="`已选择 ${selected.size} 张图片，可以同时加入多个相册。`" confirm-text="加入相册" :busy="albumWorking" @close="albumPickerOpen = false" @confirm="addSelectedToAlbums" />
    <ImagePreviewModal v-if="previewIndex >= 0" :images="images" :index="previewIndex" @close="previewIndex = -1" @change="previewIndex = $event" @copy="copyURL" />
  </section>
</template>
