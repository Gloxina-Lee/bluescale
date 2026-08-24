<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import BrandMark from './BrandMark.vue'
import { session, signOut } from '../session'

const router = useRouter()
const route = useRoute()
const initialMobile = window.matchMedia('(max-width: 900px)').matches
const mobile = ref(initialMobile)
const collapsed = ref(initialMobile)
let mobileQuery

function toggleSidebar() {
  collapsed.value = !collapsed.value
}

function handleViewportChange(event) {
  mobile.value = event.matches
  collapsed.value = event.matches
}

function handleKeydown(event) {
  if (event.key === 'Escape' && mobile.value && !collapsed.value) collapsed.value = true
}

async function logout() {
  await signOut()
  router.push({ name: 'login' })
}

watch(() => route.fullPath, () => {
  if (mobile.value) collapsed.value = true
})

onMounted(() => {
  mobileQuery = window.matchMedia('(max-width: 900px)')
  mobileQuery.addEventListener('change', handleViewportChange)
  window.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  mobileQuery?.removeEventListener('change', handleViewportChange)
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <div class="app-shell" :class="{ 'sidebar-collapsed': collapsed }">
    <aside class="app-sidebar" :class="{ collapsed }">
      <div class="sidebar-brand-row">
        <RouterLink class="brand" to="/upload" aria-label="BlueScale 首页">
          <BrandMark />
          <span>BlueScale</span>
        </RouterLink>
        <button
          class="sidebar-toggle"
          type="button"
          :aria-label="collapsed ? '展开侧边栏' : '折叠侧边栏'"
          :aria-expanded="!collapsed"
          @click="toggleSidebar"
        >
          <svg viewBox="0 0 24 24" aria-hidden="true"><path :d="collapsed ? 'm9 6 6 6-6 6' : 'm15 6-6 6 6 6'"/></svg>
        </button>
      </div>

      <nav class="sidebar-nav" aria-label="主导航">
        <RouterLink to="/upload" title="上传图片">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 16V4m0 0L7 9m5-5 5 5M5 14v4a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-4"/></svg>
          <span>上传图片</span>
        </RouterLink>
        <RouterLink to="/images" title="图片管理">
          <svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="3"/><circle cx="9" cy="10" r="2"/><path d="m4 17 5-4 3 2 3-3 5 4"/></svg>
          <span>图片管理</span>
        </RouterLink>
      </nav>

      <div class="sidebar-account">
        <div class="sidebar-user" :title="session.user?.displayName">
          <span class="avatar">{{ session.user?.displayName?.slice(0, 1) }}</span>
          <div>
            <strong>{{ session.user?.displayName }}</strong>
            <span>管理员</span>
          </div>
        </div>
        <button class="sidebar-logout" type="button" title="注销登录" @click="logout">
          <svg viewBox="0 0 24 24"><path d="M10 5H6a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h4M14 8l4 4-4 4m4-4H9"/></svg>
          <span>注销</span>
        </button>
      </div>
    </aside>

    <button v-if="mobile && collapsed" class="mobile-sidebar-trigger" type="button" aria-label="打开侧边栏" @click="toggleSidebar">
      <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h16M4 12h16M4 17h16"/></svg>
    </button>
    <button v-if="mobile && !collapsed" class="sidebar-backdrop" type="button" aria-label="关闭侧边栏" @click="collapsed = true"></button>

    <main class="page-wrap">
      <RouterView />
    </main>
  </div>
</template>
