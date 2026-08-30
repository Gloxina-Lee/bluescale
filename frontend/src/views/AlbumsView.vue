<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { session } from '../session'
import BaseSelect from '../components/BaseSelect.vue'
import ErrorToast from '../components/ErrorToast.vue'

const albums = ref([])
const selected = ref(new Set())
const loading = ref(true)
const working = ref(false)
const error = ref('')
const createOpen = ref(false)
const mergeOpen = ref(false)
const newAlbumName = ref('')
const mergeTargetID = ref(null)

const allSelected = computed(() => albums.value.length > 0 && selected.value.size === albums.value.length)
const isAdmin = computed(() => Boolean(session.user))
const selectedAlbums = computed(() => albums.value.filter((album) => selected.value.has(album.id)))
const mergeTargetOptions = computed(() => selectedAlbums.value.map((album) => ({ value: album.id, label: album.name })))

async function loadAlbums() {
  loading.value = true
  error.value = ''
  try {
    const payload = await api('/api/albums')
    albums.value = payload.albums
    selected.value = new Set(Array.from(selected.value).filter((id) => albums.value.some((album) => album.id === id)))
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
  selected.value = allSelected.value ? new Set() : new Set(albums.value.map((album) => album.id))
}

async function createAlbum() {
  if (!newAlbumName.value.trim() || working.value) return
  working.value = true
  error.value = ''
  try {
    await api('/api/albums', { method: 'POST', body: JSON.stringify({ name: newAlbumName.value }) })
    newAlbumName.value = ''
    createOpen.value = false
    await loadAlbums()
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    working.value = false
  }
}

async function deleteAlbums(ids = Array.from(selected.value)) {
  if (!ids.length || working.value || !window.confirm(`确定删除选中的 ${ids.length} 个相册吗？图片文件不会被删除。`)) return
  working.value = true
  error.value = ''
  try {
    await api('/api/albums', { method: 'DELETE', body: JSON.stringify({ ids }) })
    selected.value = new Set()
    await loadAlbums()
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    working.value = false
  }
}

function openMerge() {
  if (selectedAlbums.value.length < 2) return
  mergeTargetID.value = selectedAlbums.value[0].id
  mergeOpen.value = true
}

async function mergeAlbums() {
  if (!mergeTargetID.value || selected.value.size < 2 || working.value) return
  working.value = true
  error.value = ''
  try {
    await api('/api/albums/merge', {
      method: 'POST',
      body: JSON.stringify({ ids: Array.from(selected.value), targetId: mergeTargetID.value }),
    })
    mergeOpen.value = false
    selected.value = new Set()
    await loadAlbums()
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    working.value = false
  }
}

function formatDate(value) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

onMounted(loadAlbums)
</script>

<template>
  <section class="content-page albums-page" :class="{ 'public-library': !isAdmin }">
    <ErrorToast :message="error" @close="error = ''" />
    <div class="page-heading management-heading">
      <div><span class="eyebrow">ALBUMS</span><h1>{{ isAdmin ? '相册管理' : '公开相册' }}</h1><p>{{ isAdmin ? '以标签方式组织图片，不会复制或移动原文件。' : '无需登录即可浏览每个相册中公开分享的图片。' }}</p></div>
      <button v-if="isAdmin" class="primary-button header-action" type="button" @click="createOpen = true">
        <svg viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>创建相册
      </button>
    </div>

    <div v-if="isAdmin" class="library-toolbar album-toolbar">
      <label class="check-control"><input type="checkbox" :checked="allSelected" @change="toggleAll" /><span>全选</span></label>
      <span class="image-count">共 {{ albums.length }} 个相册</span>
      <div class="toolbar-spacer"></div>
      <span v-if="selected.size" class="selected-count">已选择 {{ selected.size }} 个</span>
      <button class="secondary-button compact" type="button" :disabled="selected.size < 2 || working" @click="openMerge">合并相册</button>
      <button class="danger-button" type="button" :disabled="!selected.size || working" @click="deleteAlbums()">
        <svg viewBox="0 0 24 24"><path d="M4 7h16m-10 4v6m4-6v6M9 7l1-3h4l1 3m3 0-1 13H7L6 7"/></svg>批量删除
      </button>
    </div>

    <div v-if="loading" class="loading-state"><span></span><p>正在读取相册…</p></div>
    <div v-else-if="!albums.length" class="empty-library">
      <div class="empty-icon"><svg viewBox="0 0 24 24"><path d="M3 7h7l2 2h9v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/><path d="M3 7V5a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v2"/></svg></div>
      <h2>{{ isAdmin ? '还没有相册' : '暂无公开相册' }}</h2><p>{{ isAdmin ? '创建相册后，可以在上传或图片管理时把图片加入其中。' : '管理员尚未创建任何相册。' }}</p>
      <button v-if="isAdmin" class="secondary-button" type="button" @click="createOpen = true">创建第一个相册</button>
    </div>
    <div v-else class="album-list" role="list">
      <div class="album-list-heading" :class="{ guest: !isAdmin }" aria-hidden="true"><span v-if="isAdmin"></span><span>相册名称</span><span>{{ isAdmin ? '图片数量' : '公开图片' }}</span><span>创建时间</span><span></span></div>
      <article v-for="album in albums" :key="album.id" class="album-list-row" :class="{ selected: selected.has(album.id), guest: !isAdmin }" role="listitem">
        <label v-if="isAdmin" class="list-select-control" :aria-label="selected.has(album.id) ? '取消选择' : '选择相册'"><input type="checkbox" :checked="selected.has(album.id)" @change="toggle(album.id)" /></label>
        <div class="album-identity">
          <span><svg viewBox="0 0 24 24"><path d="M3 7h7l2 2h9v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/></svg></span>
          <div><strong>{{ album.name }}</strong><small aria-hidden="true">{{ album.imageCount }} 张 · {{ formatDate(album.createdAt) }}</small></div>
        </div>
        <span>{{ album.imageCount }} 张</span>
        <time :datetime="album.createdAt">{{ formatDate(album.createdAt) }}</time>
        <div class="album-row-actions">
          <RouterLink class="secondary-button compact" :to="{ name: 'images', query: { album: album.id } }">查看图片</RouterLink>
          <button v-if="isAdmin" class="row-delete-button" type="button" :aria-label="`删除相册 ${album.name}`" @click="deleteAlbums([album.id])">×</button>
        </div>
      </article>
    </div>

    <div v-if="isAdmin && createOpen" class="modal-layer" @mousedown.self="!working && (createOpen = false)">
      <section class="management-modal compact-modal" role="dialog" aria-modal="true" aria-labelledby="create-album-title">
        <header><div><span class="eyebrow">NEW ALBUM</span><h2 id="create-album-title">创建相册</h2></div><button type="button" aria-label="关闭" :disabled="working" @click="createOpen = false">×</button></header>
        <form @submit.prevent="createAlbum">
          <label class="field"><span>相册名称</span><input v-model="newAlbumName" maxlength="80" required autofocus placeholder="例如：旅行照片" /></label>
          <footer><button class="secondary-button" type="button" :disabled="working" @click="createOpen = false">取消</button><button class="primary-button" type="submit" :disabled="working || !newAlbumName.trim()">{{ working ? '正在创建…' : '创建相册' }}</button></footer>
        </form>
      </section>
    </div>

    <div v-if="isAdmin && mergeOpen" class="modal-layer" @mousedown.self="!working && (mergeOpen = false)">
      <section class="management-modal compact-modal" role="dialog" aria-modal="true" aria-labelledby="merge-album-title">
        <header><div><span class="eyebrow">MERGE</span><h2 id="merge-album-title">合并 {{ selected.size }} 个相册</h2></div><button type="button" aria-label="关闭" :disabled="working" @click="mergeOpen = false">×</button></header>
        <form @submit.prevent="mergeAlbums">
          <div class="field"><span>合并后保留</span><BaseSelect v-model="mergeTargetID" :options="mergeTargetOptions" aria-label="合并后保留的相册" /><small>其余相册会被删除，所有图片关系会合并到这个相册中</small></div>
          <footer><button class="secondary-button" type="button" :disabled="working" @click="mergeOpen = false">取消</button><button class="primary-button" type="submit" :disabled="working">{{ working ? '正在合并…' : '确认合并' }}</button></footer>
        </form>
      </section>
    </div>
  </section>
</template>
