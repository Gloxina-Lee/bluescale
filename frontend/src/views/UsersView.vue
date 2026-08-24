<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { defaultRouteName, hasPermission, loadUser, session } from '../session'

const router = useRouter()
const users = ref([])
const groups = ref([])
const activeTab = ref('users')
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const modal = ref(null)

const userForm = reactive({ displayName: '', username: '', password: '', groupId: 0 })
const groupForm = reactive({ name: '', permissions: { upload: true, manageImages: false, manageUsers: false } })

const defaultGroup = computed(() => groups.value.find((group) => group.isDefault) || groups.value[0])
const permissionOptions = [
  { key: 'upload', title: '上传图片', description: '可以进入上传页面并上传新图片' },
  { key: 'manageImages', title: '管理图片', description: '可以查看图库、复制链接和删除图片' },
  { key: 'manageUsers', title: '管理用户', description: '可以创建、编辑和删除用户及用户组' },
]

async function loadAll() {
  loading.value = true
  error.value = ''
  try {
    const [userPayload, groupPayload] = await Promise.all([api('/api/users'), api('/api/user-groups')])
    users.value = userPayload.users
    groups.value = groupPayload.groups
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    loading.value = false
  }
}

function openCreateUser() {
  error.value = ''
  Object.assign(userForm, { displayName: '', username: '', password: '', groupId: defaultGroup.value?.id || 0 })
  modal.value = { type: 'user', mode: 'create', title: '创建用户' }
}

function openEditUser(user) {
  error.value = ''
  Object.assign(userForm, { displayName: user.displayName, username: user.username, password: '', groupId: user.group.id })
  modal.value = { type: 'user', mode: 'edit', id: user.id, title: '编辑用户' }
}

function openCreateGroup() {
  error.value = ''
  Object.assign(groupForm, { name: '', permissions: { upload: true, manageImages: false, manageUsers: false } })
  modal.value = { type: 'group', mode: 'create', title: '创建用户组' }
}

function openEditGroup(group) {
  error.value = ''
  Object.assign(groupForm, { name: group.name, permissions: { ...group.permissions } })
  modal.value = { type: 'group', mode: 'edit', id: group.id, title: '编辑用户组' }
}

function closeModal() {
  if (!busy.value) modal.value = null
}

async function refreshAfterMutation() {
  await loadUser()
  if (!hasPermission('manageUsers')) {
    router.replace({ name: defaultRouteName() })
    return
  }
  await loadAll()
}

async function saveUser() {
  busy.value = true
  error.value = ''
  try {
    const editing = modal.value.mode === 'edit'
    const path = editing ? `/api/users/${modal.value.id}` : '/api/users'
    await api(path, {
      method: editing ? 'PUT' : 'POST',
      body: JSON.stringify(userForm),
    })
    modal.value = null
    await refreshAfterMutation()
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    busy.value = false
  }
}

async function saveGroup() {
  busy.value = true
  error.value = ''
  try {
    const editing = modal.value.mode === 'edit'
    const path = editing ? `/api/user-groups/${modal.value.id}` : '/api/user-groups'
    await api(path, {
      method: editing ? 'PUT' : 'POST',
      body: JSON.stringify(groupForm),
    })
    modal.value = null
    await refreshAfterMutation()
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    busy.value = false
  }
}

async function deleteUser(user) {
  if (!window.confirm(`确定删除用户“${user.displayName}”吗？`)) return
  error.value = ''
  try {
    await api(`/api/users/${user.id}`, { method: 'DELETE' })
    await loadAll()
  } catch (requestError) {
    error.value = requestError.message
  }
}

async function deleteGroup(group) {
  if (!window.confirm(`确定删除用户组“${group.name}”吗？`)) return
  error.value = ''
  try {
    await api(`/api/user-groups/${group.id}`, { method: 'DELETE' })
    await loadAll()
  } catch (requestError) {
    error.value = requestError.message
  }
}

function permissionNames(groupPermissions) {
  return permissionOptions.filter((option) => groupPermissions[option.key]).map((option) => option.title)
}

function formatDate(value) {
  return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'short', day: 'numeric' }).format(new Date(value))
}

onMounted(loadAll)
</script>

<template>
  <section class="content-page users-page">
    <div class="page-heading management-heading">
      <div>
        <span class="eyebrow">ACCESS CONTROL</span>
        <h1>用户管理</h1>
        <p>管理登录账号、用户组和每个角色能够访问的功能。</p>
      </div>
    </div>

    <div class="access-overview">
      <article><span>用户</span><strong>{{ users.length }}</strong><small>个登录账号</small></article>
      <article><span>用户组</span><strong>{{ groups.length }}</strong><small>组权限方案</small></article>
      <article><span>当前身份</span><strong class="overview-role">{{ session.user?.group?.name }}</strong><small>{{ session.user?.username }}</small></article>
    </div>

    <div class="access-toolbar">
      <div class="access-tabs" role="tablist" aria-label="用户管理分类">
        <button type="button" role="tab" :aria-selected="activeTab === 'users'" :class="{ active: activeTab === 'users' }" @click="activeTab = 'users'">用户</button>
        <button type="button" role="tab" :aria-selected="activeTab === 'groups'" :class="{ active: activeTab === 'groups' }" @click="activeTab = 'groups'">用户组</button>
      </div>
      <button v-if="activeTab === 'users'" class="primary-button access-add-button" type="button" @click="openCreateUser">
        <svg viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>创建用户
      </button>
      <button v-else class="primary-button access-add-button" type="button" @click="openCreateGroup">
        <svg viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>创建用户组
      </button>
    </div>

    <p v-if="error && !modal" class="inline-message error" role="alert">{{ error }}</p>
    <div v-if="loading" class="loading-state"><span></span><p>正在读取用户信息…</p></div>

    <div v-else-if="activeTab === 'users'" class="access-list user-directory">
      <article v-for="user in users" :key="user.id" class="access-row user-access-row">
        <div class="directory-identity">
          <span class="avatar">{{ user.displayName.slice(0, 1) }}</span>
          <div><strong>{{ user.displayName }}</strong><span>@{{ user.username }}</span></div>
        </div>
        <div class="directory-group"><span>{{ user.group.name }}</span><small>{{ permissionNames(user.permissions).join(' · ') }}</small></div>
        <span class="directory-date">{{ formatDate(user.createdAt) }}</span>
        <div class="row-actions">
          <button type="button" @click="openEditUser(user)">编辑</button>
          <button class="destructive" type="button" :disabled="user.id === session.user?.id" :title="user.id === session.user?.id ? '不能删除当前登录账号' : '删除用户'" @click="deleteUser(user)">删除</button>
        </div>
      </article>
    </div>

    <div v-else class="access-list group-directory">
      <article v-for="group in groups" :key="group.id" class="access-row group-access-row">
        <div class="group-title">
          <div><strong>{{ group.name }}</strong><span v-if="group.isSystem" class="system-badge">内置</span></div>
          <small>{{ group.userCount }} 位用户</small>
        </div>
        <div class="permission-chips">
          <span v-for="name in permissionNames(group.permissions)" :key="name">{{ name }}</span>
        </div>
        <div class="row-actions">
          <button type="button" :disabled="group.isSystem" :title="group.isSystem ? '内置用户组不可编辑' : '编辑用户组'" @click="openEditGroup(group)">编辑</button>
          <button class="destructive" type="button" :disabled="group.isSystem || group.userCount > 0" :title="group.isSystem ? '内置用户组不可删除' : group.userCount ? '请先移动组内用户' : '删除用户组'" @click="deleteGroup(group)">删除</button>
        </div>
      </article>
    </div>

    <Teleport to="body">
      <div v-if="modal" class="modal-layer" @mousedown.self="closeModal">
        <section class="management-modal" role="dialog" aria-modal="true" :aria-labelledby="`${modal.type}-modal-title`">
          <header>
            <div><span class="eyebrow">{{ modal.mode === 'create' ? 'NEW' : 'EDIT' }}</span><h2 :id="`${modal.type}-modal-title`">{{ modal.title }}</h2></div>
            <button type="button" aria-label="关闭" :disabled="busy" @click="closeModal">×</button>
          </header>

          <form v-if="modal.type === 'user'" @submit.prevent="saveUser">
            <label class="field"><span>用户名称</span><input v-model.trim="userForm.displayName" required maxlength="64" autocomplete="off" placeholder="例如：设计组成员" /></label>
            <label class="field"><span>登录账号</span><input v-model.trim="userForm.username" required maxlength="64" autocomplete="off" placeholder="输入唯一登录账号" /></label>
            <label class="field">
              <span>{{ modal.mode === 'create' ? '登录密码' : '新密码（可选）' }}</span>
              <input v-model="userForm.password" :required="modal.mode === 'create'" minlength="8" maxlength="128" type="password" autocomplete="new-password" :placeholder="modal.mode === 'create' ? '至少 8 个字符' : '留空则保持原密码'" />
            </label>
            <label class="field"><span>归属用户组</span><span class="select-wrap"><select v-model.number="userForm.groupId" required><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}{{ group.isDefault ? '（默认）' : '' }}</option></select></span></label>
            <p v-if="error" class="form-error" role="alert">{{ error }}</p>
            <footer><button class="ghost-button" type="button" :disabled="busy" @click="closeModal">取消</button><button class="primary-button" type="submit" :disabled="busy">{{ busy ? '正在保存…' : '保存用户' }}</button></footer>
          </form>

          <form v-else @submit.prevent="saveGroup">
            <label class="field"><span>用户组名称</span><input v-model.trim="groupForm.name" required maxlength="64" autocomplete="off" placeholder="例如：内容编辑" /></label>
            <fieldset class="permission-fieldset">
              <legend>功能权限</legend>
              <label v-for="option in permissionOptions" :key="option.key" class="permission-option">
                <input v-model="groupForm.permissions[option.key]" type="checkbox" />
                <span><strong>{{ option.title }}</strong><small>{{ option.description }}</small></span>
              </label>
            </fieldset>
            <p v-if="error" class="form-error" role="alert">{{ error }}</p>
            <footer><button class="ghost-button" type="button" :disabled="busy" @click="closeModal">取消</button><button class="primary-button" type="submit" :disabled="busy">{{ busy ? '正在保存…' : '保存用户组' }}</button></footer>
          </form>
        </section>
      </div>
    </Teleport>
  </section>
</template>
