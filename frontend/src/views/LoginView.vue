<script setup>
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AuthLayout from '../components/AuthLayout.vue'
import { api } from '../api'
import { session } from '../session'

const router = useRouter()
const route = useRoute()
const form = reactive({ username: '', password: '' })
const busy = ref(false)
const error = ref('')

async function submit() {
  error.value = ''
  busy.value = true
  try {
    session.user = await api('/api/login', { method: 'POST', body: JSON.stringify(form) })
    router.replace({ name: 'upload' })
  } catch (requestError) {
    error.value = requestError.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthLayout eyebrow="私人图床" title="让图片归你掌控" description="一个克制、快速的私人图片空间。上传、管理和分享，不经过第三方。">
    <form class="auth-form login-form" @submit.prevent="submit">
      <div class="form-heading">
        <span class="step-pill">WELCOME BACK</span>
        <h2>管理员登录</h2>
        <p>{{ route.query.setup === 'done' ? '配置成功，现在使用管理员账号登录。' : '输入你的账号与密码继续。' }}</p>
      </div>
      <label class="field">
        <span>登录账号</span>
        <input v-model.trim="form.username" required autocomplete="username" autofocus placeholder="输入登录账号" />
      </label>
      <label class="field">
        <span>登录密码</span>
        <input v-model="form.password" required type="password" autocomplete="current-password" placeholder="输入登录密码" />
      </label>
      <p v-if="error" class="form-error" role="alert">{{ error }}</p>
      <button class="primary-button" type="submit" :disabled="busy">
        <span>{{ busy ? '正在登录…' : '登录' }}</span>
        <svg viewBox="0 0 24 24"><path d="m9 18 6-6-6-6"/></svg>
      </button>
    </form>
  </AuthLayout>
</template>
