<script setup>
import { nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { api } from '../api'
import { session } from '../session'
import ErrorToast from './ErrorToast.vue'

const emit = defineEmits(['close'])
const form = reactive({ displayName: '', username: '', currentPassword: '', newPassword: '', confirmPassword: '' })
const saving = ref(false)
const error = ref('')
const success = ref('')
const displayNameInput = ref(null)

function resetForm() {
  Object.assign(form, {
    displayName: session.user?.displayName || '',
    username: session.user?.username || '',
    currentPassword: '',
    newPassword: '',
    confirmPassword: '',
  })
  error.value = ''
  success.value = ''
}

function close() {
  if (!saving.value) emit('close')
}

function handleKeydown(event) {
  if (event.key === 'Escape') close()
}

async function save() {
  error.value = ''
  success.value = ''
  if (form.newPassword !== form.confirmPassword) {
    error.value = '两次输入的新密码不一致'
    return
  }
  saving.value = true
  try {
    session.user = await api('/api/me', {
      method: 'PUT',
      body: JSON.stringify({
        displayName: form.displayName,
        username: form.username,
        currentPassword: form.currentPassword,
        newPassword: form.newPassword,
      }),
    })
    resetForm()
    success.value = '个人信息已更新'
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  resetForm()
  window.addEventListener('keydown', handleKeydown)
  await nextTick()
  displayNameInput.value?.focus()
})

onBeforeUnmount(() => window.removeEventListener('keydown', handleKeydown))
</script>

<template>
  <Teleport to="body">
    <ErrorToast :message="error" @close="error = ''" />
    <div class="modal-layer profile-settings-layer" @mousedown.self="close">
      <section class="management-modal profile-settings-modal" role="dialog" aria-modal="true" aria-labelledby="profile-settings-title">
        <header>
          <div>
            <span class="eyebrow">ACCOUNT SETTINGS</span>
            <h2 id="profile-settings-title">个人设置</h2>
          </div>
          <button type="button" aria-label="关闭个人设置" :disabled="saving" @click="close">×</button>
        </header>

        <form @submit.prevent="save">
          <div class="profile-dialog-identity">
            <span class="avatar">{{ session.user?.displayName?.slice(0, 1) }}</span>
            <div>
              <strong>{{ session.user?.displayName }}</strong>
              <span>管理员账号</span>
            </div>
          </div>

          <div class="settings-section-heading">
            <span class="settings-icon"><svg viewBox="0 0 24 24"><path d="M20 21a8 8 0 0 0-16 0M12 13a5 5 0 1 0 0-10 5 5 0 0 0 0 10"/></svg></span>
            <div><h2>账号信息</h2><p>更新侧边栏中显示的昵称与登录用户名。</p></div>
          </div>
          <div class="settings-form-grid">
            <label class="field"><span>显示名称</span><input ref="displayNameInput" v-model.trim="form.displayName" required maxlength="64" autocomplete="name" placeholder="输入显示名称" /></label>
            <label class="field"><span>登录用户名</span><input v-model.trim="form.username" required maxlength="64" autocomplete="username" placeholder="输入登录账号" /></label>
          </div>

          <div class="settings-divider"></div>
          <div class="settings-section-heading compact-heading">
            <span class="settings-icon"><svg viewBox="0 0 24 24"><rect x="4" y="10" width="16" height="11" rx="3"/><path d="M8 10V7a4 4 0 0 1 8 0v3M12 15v2"/></svg></span>
            <div><h2>修改密码</h2><p>不修改密码时，将下面三项保持为空即可。</p></div>
          </div>
          <label class="field"><span>当前密码</span><input v-model="form.currentPassword" :required="Boolean(form.newPassword)" type="password" autocomplete="current-password" placeholder="修改密码时需要验证" /></label>
          <div class="settings-form-grid">
            <label class="field"><span>新密码</span><input v-model="form.newPassword" minlength="8" maxlength="128" type="password" autocomplete="new-password" placeholder="至少 8 个字符" /></label>
            <label class="field"><span>确认新密码</span><input v-model="form.confirmPassword" :required="Boolean(form.newPassword)" minlength="8" maxlength="128" type="password" autocomplete="new-password" placeholder="再次输入新密码" /></label>
          </div>

          <p v-if="success" class="inline-message success" role="status"><span>✓</span>{{ success }}</p>
          <footer>
            <button class="secondary-button" type="button" :disabled="saving" @click="close">取消</button>
            <button class="primary-button" type="submit" :disabled="saving">{{ saving ? '正在保存…' : '保存修改' }}</button>
          </footer>
        </form>
      </section>
    </div>
  </Teleport>
</template>
