<script setup>
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import AuthLayout from '../components/AuthLayout.vue'
import { api } from '../api'
import { loadStatus } from '../session'

const router = useRouter()
const form = reactive({
  displayName: '',
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
        displayName: form.displayName,
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
  <AuthLayout eyebrow="首次运行" title="欢迎，先完成初始化" description="创建初始管理员账号，并准备好本地图片存储。之后可继续添加用户与用户组。">
    <form class="auth-form" @submit.prevent="submit">
      <div class="form-heading">
        <span class="step-pill">01 / 01</span>
        <h2>首次配置</h2>
        <p>这些设置保存后即可开始使用。</p>
      </div>

      <label class="field">
        <span>管理员名称</span>
        <input v-model.trim="form.displayName" required maxlength="64" autocomplete="name" placeholder="例如：Blue 管理员" />
      </label>
      <label class="field">
        <span>登录账号</span>
        <input v-model.trim="form.username" required maxlength="64" autocomplete="username" placeholder="输入登录账号" />
      </label>
      <div class="form-grid">
        <label class="field">
          <span>登录密码</span>
          <input v-model="form.password" required minlength="8" maxlength="128" type="password" autocomplete="new-password" placeholder="至少 8 个字符" />
        </label>
        <label class="field">
          <span>确认密码</span>
          <input v-model="form.confirmPassword" required minlength="8" maxlength="128" type="password" autocomplete="new-password" placeholder="再次输入密码" />
        </label>
      </div>
      <label class="field">
        <span>数据库类型</span>
        <span class="select-wrap">
          <select v-model="form.databaseType">
            <option value="sqlite">SQLite（本地文件）</option>
          </select>
        </span>
        <small>当前版本仅支持 SQLite，无需额外安装数据库。</small>
      </label>

      <p v-if="error" class="form-error" role="alert">{{ error }}</p>
      <button class="primary-button" type="submit" :disabled="busy">
        <span>{{ busy ? '正在配置…' : '完成配置' }}</span>
        <svg viewBox="0 0 24 24"><path d="m9 18 6-6-6-6"/></svg>
      </button>
    </form>
  </AuthLayout>
</template>
