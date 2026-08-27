<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import ErrorToast from '../components/ErrorToast.vue'

const tokens = ref([])
const selected = ref(new Set())
const loading = ref(true)
const working = ref(false)
const error = ref('')
const createOpen = ref(false)
const detailOpen = ref(false)
const tokenName = ref('')
const generated = ref(null)
const detail = ref(null)
const copied = ref(false)

const allSelected = computed(() => tokens.value.length > 0 && selected.value.size === tokens.value.length)

async function loadTokens() {
  loading.value = true
  error.value = ''
  try {
    const payload = await api('/api/tokens')
    tokens.value = payload.tokens
    selected.value = new Set(Array.from(selected.value).filter((id) => tokens.value.some((token) => token.id === id)))
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
  selected.value = allSelected.value ? new Set() : new Set(tokens.value.map((token) => token.id))
}

function openCreate() {
  tokenName.value = ''
  generated.value = null
  copied.value = false
  createOpen.value = true
}

function closeCreate() {
  if (working.value) return
  createOpen.value = false
  generated.value = null
  tokenName.value = ''
}

async function createToken() {
  if (working.value) return
  working.value = true
  error.value = ''
  try {
    generated.value = await api('/api/tokens', { method: 'POST', body: JSON.stringify({ name: tokenName.value }) })
    await loadTokens()
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    working.value = false
  }
}

async function copyToken() {
  if (!generated.value?.token) return
  await navigator.clipboard.writeText(generated.value.token)
  copied.value = true
}

async function showDetail(id) {
  working.value = true
  error.value = ''
  try {
    detail.value = await api(`/api/tokens/${id}`)
    detailOpen.value = true
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    working.value = false
  }
}

async function deleteTokens(ids = Array.from(selected.value)) {
  if (!ids.length || working.value || !window.confirm(`确定删除选中的 ${ids.length} 个 API Token 吗？使用它们的客户端将立即失去写入权限。`)) return
  working.value = true
  error.value = ''
  try {
    await api('/api/tokens', { method: 'DELETE', body: JSON.stringify({ ids }) })
    selected.value = new Set()
    detailOpen.value = false
    await loadTokens()
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    working.value = false
  }
}

function formatDate(value) {
  if (!value) return '尚未使用'
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value))
}

onMounted(loadTokens)
</script>

<template>
  <section class="content-page api-page">
    <ErrorToast :message="error" @close="error = ''" />
    <div class="page-heading management-heading">
      <div><span class="eyebrow">DEVELOPER API</span><h1>API</h1><p>生成和管理用于敏感操作的 Bearer Token。</p></div>
      <div class="header-action-group">
        <RouterLink class="secondary-button header-action" to="/docs"><svg viewBox="0 0 24 24"><path d="M5 4h10a2 2 0 0 1 2 2v14H7a2 2 0 0 1-2-2V4Z"/><path d="M9 8h4m-4 4h4m4-6h2v14h-2"/></svg>查看文档</RouterLink>
        <button class="primary-button header-action" type="button" @click="openCreate">
          <svg viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>生成 Token
        </button>
      </div>
    </div>

    <div class="api-guide-card">
      <div><span class="api-guide-icon"><svg viewBox="0 0 24 24"><path d="M8 9 5 12l3 3m8-6 3 3-3 3M14 5l-4 14"/></svg></span><div><strong>读取公开内容无需鉴权</strong><p><code>GET /api/images</code>、<code>GET /api/albums</code> 和公开的 <code>/i/...</code> 可直接访问。</p></div></div>
      <div><span class="api-guide-icon secure"><svg viewBox="0 0 24 24"><rect x="5" y="10" width="14" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></svg></span><div><strong>写入和私密内容需要鉴权</strong><p>请求头使用 <code>Authorization: Bearer &lt;token&gt;</code>。Token 只在创建后显示一次。</p></div></div>
    </div>

    <div class="library-toolbar token-toolbar">
      <label class="check-control"><input type="checkbox" :checked="allSelected" @change="toggleAll" /><span>全选</span></label>
      <span class="image-count">共 {{ tokens.length }} 个 Token</span>
      <div class="toolbar-spacer"></div>
      <span v-if="selected.size" class="selected-count">已选择 {{ selected.size }} 个</span>
      <button class="danger-button" type="button" :disabled="!selected.size || working" @click="deleteTokens()">
        <svg viewBox="0 0 24 24"><path d="M4 7h16m-10 4v6m4-6v6M9 7l1-3h4l1 3m3 0-1 13H7L6 7"/></svg>批量删除
      </button>
    </div>

    <div v-if="loading" class="loading-state"><span></span><p>正在读取 API Token…</p></div>
    <div v-else-if="!tokens.length" class="empty-library">
      <div class="empty-icon"><svg viewBox="0 0 24 24"><path d="M14 7a5 5 0 1 0-4.6 7H11l2 2h2v2h2v2h4v-4l-6.4-6.4A5 5 0 0 0 14 7Z"/></svg></div>
      <h2>还没有 API Token</h2><p>创建后即可用于上传、删除和读取私密图片。</p>
      <button class="secondary-button" type="button" @click="openCreate">生成第一个 Token</button>
    </div>
    <div v-else class="token-list" role="list">
      <div class="token-list-heading" aria-hidden="true"><span></span><span>名称</span><span>Token 标识</span><span></span></div>
      <article v-for="token in tokens" :key="token.id" class="token-list-row" :class="{ selected: selected.has(token.id) }" role="listitem">
        <label class="list-select-control" :aria-label="selected.has(token.id) ? '取消选择' : '选择 Token'"><input type="checkbox" :checked="selected.has(token.id)" @change="toggle(token.id)" /></label>
        <div class="token-identity"><span><svg viewBox="0 0 24 24"><path d="M14 7a5 5 0 1 0-4.6 7H11l2 2h2v2h2v2h4v-4l-6.4-6.4A5 5 0 0 0 14 7Z"/></svg></span><strong>{{ token.name }}</strong></div>
        <code>{{ token.prefix }}</code>
        <div class="token-row-actions"><button class="secondary-button compact" type="button" :disabled="working" @click="showDetail(token.id)">详细信息</button><button class="row-delete-button" type="button" :aria-label="`删除 ${token.name}`" @click="deleteTokens([token.id])">×</button></div>
      </article>
    </div>

    <div v-if="createOpen" class="modal-layer" @mousedown.self="closeCreate">
      <section class="management-modal token-create-modal" role="dialog" aria-modal="true" aria-labelledby="create-token-title">
        <header><div><span class="eyebrow">NEW TOKEN</span><h2 id="create-token-title">{{ generated ? '保存 API Token' : '生成 API Token' }}</h2><p>{{ generated ? '关闭后将无法再次查看完整 Token。' : '名称用于区分不同客户端或用途。' }}</p></div><button type="button" aria-label="关闭" :disabled="working" @click="closeCreate">×</button></header>
        <form v-if="!generated" @submit.prevent="createToken">
          <label class="field"><span>Token 名称</span><input v-model="tokenName" maxlength="64" autofocus placeholder="例如：自动上传脚本" /></label>
          <footer><button class="secondary-button" type="button" :disabled="working" @click="closeCreate">取消</button><button class="primary-button" type="submit" :disabled="working">{{ working ? '正在生成…' : '生成 Token' }}</button></footer>
        </form>
        <div v-else class="generated-token-panel">
          <p class="token-warning">请立即复制并安全保存。服务器只保存摘要，无法恢复此 Token。</p>
          <code>{{ generated.token }}</code>
          <footer><button class="secondary-button" type="button" @click="copyToken">{{ copied ? '已复制' : '复制 Token' }}</button><button class="primary-button" type="button" @click="closeCreate">我已保存</button></footer>
        </div>
      </section>
    </div>

    <div v-if="detailOpen && detail" class="modal-layer" @mousedown.self="detailOpen = false">
      <section class="management-modal compact-modal token-detail-modal" role="dialog" aria-modal="true" aria-labelledby="token-detail-title">
        <header><div><span class="eyebrow">TOKEN DETAILS</span><h2 id="token-detail-title">{{ detail.name }}</h2></div><button type="button" aria-label="关闭" @click="detailOpen = false">×</button></header>
        <dl><div><dt>Token 标识</dt><dd><code>{{ detail.prefix }}</code></dd></div><div><dt>创建时间</dt><dd>{{ formatDate(detail.createdAt) }}</dd></div><div><dt>最后使用时间</dt><dd>{{ formatDate(detail.lastUsedAt) }}</dd></div></dl>
        <footer><button class="danger-button" type="button" @click="deleteTokens([detail.id])">删除 Token</button><button class="secondary-button" type="button" @click="detailOpen = false">关闭</button></footer>
      </section>
    </div>
  </section>
</template>
