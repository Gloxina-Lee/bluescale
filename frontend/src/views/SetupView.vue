<script setup>
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import AuthLayout from '../components/AuthLayout.vue'
import BaseSelect from '../components/BaseSelect.vue'
import ErrorToast from '../components/ErrorToast.vue'
import { api } from '../api'
import { loadStatus } from '../session'

const router = useRouter()
const databaseOptions = [{ value: 'sqlite', label: 'SQLite（本地文件）' }]
const form = reactive({
  username: '',
  password: '',
  confirmPassword: '',
  databaseType: 'sqlite',
})
const busy = ref(false)
const error = ref('')

async function submit() {
  error.value = ''
  if (form.password !== form.confirmPassword) {
    error.value = '两次输入的密码不一致'
    return
  }
  busy.value = true
  try {
    await api('/api/setup', {
      method: 'POST',
      body: JSON.stringify({
        username: form.username,
        password: form.password,
        databaseType: form.databaseType,
      }),
    })
    await loadStatus()
    router.replace({ name: 'login', query: { setup: 'done' } })
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthLayout eyebrow="首次运行" title="欢迎，先完成初始化" description="创建唯一的管理员账号，并准备好本地图片存储。全部数据都保留在你的设备上。">
    <ErrorToast :message="error" @close="error = ''" />
    <form class="auth-form" @submit.prevent="submit">
      <div class="form-heading">
        <span class="step-pill">01 / 01</span>
        <h2>首次配置</h2>
        <p>这些设置保存后即可开始使用。</p>
      </div>

      <label class="field">
        <span>用户名</span>
        <input v-model.trim="form.username" required maxlength="64" autocomplete="username" placeholder="输入管理员用户名" />
      </label>
      <div class="form-grid">
        <label class="field">
          <span>登录密码</span>
          <input v-model="form.password" required minlength="8" maxlength="72" type="password" autocomplete="new-password" placeholder="至少 8 个字符" />
        </label>
        <label class="field">
          <span>确认密码</span>
          <input v-model="form.confirmPassword" required minlength="8" maxlength="72" type="password" autocomplete="new-password" placeholder="再次输入密码" />
        </label>
      </div>
      <div class="field">
        <span>数据库类型</span>
        <BaseSelect v-model="form.databaseType" :options="databaseOptions" aria-label="数据库类型" />
        <small>当前版本仅支持 SQLite，无需额外安装数据库。</small>
      </div>

      <button class="primary-button" type="submit" :disabled="busy">
        <span>{{ busy ? '正在配置…' : '完成配置' }}</span>
        <svg viewBox="0 0 24 24"><path d="m9 18 6-6-6-6"/></svg>
      </button>
    </form>
  </AuthLayout>
</template>
