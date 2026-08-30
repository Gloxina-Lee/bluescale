<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import BrandMark from './BrandMark.vue'
import ProfileSettingsModal from './ProfileSettingsModal.vue'
import { session, signOut } from '../session'
import { theme, toggleTheme } from '../theme'

const router = useRouter()
const route = useRoute()
const initialMobile = window.matchMedia('(max-width: 900px)').matches
const mobile = ref(initialMobile)
const collapsed = ref(initialMobile)
const accountMenuOpen = ref(false)
const profileSettingsOpen = ref(false)
const accountArea = ref(null)
let mobileQuery

function toggleSidebar() {
  collapsed.value = !collapsed.value
}

function handleViewportChange(event) {
  mobile.value = event.matches
  collapsed.value = event.matches
  accountMenuOpen.value = false
}

function handleKeydown(event) {
  if (event.key !== 'Escape') return
  if (profileSettingsOpen.value) return
  if (accountMenuOpen.value) accountMenuOpen.value = false
  else if (mobile.value && !collapsed.value) collapsed.value = true
}

function handleOutsideClick(event) {
  if (accountMenuOpen.value && !accountArea.value?.contains(event.target)) accountMenuOpen.value = false
}

async function logout() {
  await signOut()
  router.push({ name: 'login' })
}

function openProfileSettings() {
  accountMenuOpen.value = false
  profileSettingsOpen.value = true
}

watch(() => route.fullPath, () => {
  accountMenuOpen.value = false
  if (mobile.value) collapsed.value = true
})

onMounted(() => {
  mobileQuery = window.matchMedia('(max-width: 900px)')
  mobileQuery.addEventListener('change', handleViewportChange)
  window.addEventListener('keydown', handleKeydown)
  document.addEventListener('pointerdown', handleOutsideClick)
})

onBeforeUnmount(() => {
  mobileQuery?.removeEventListener('change', handleViewportChange)
  window.removeEventListener('keydown', handleKeydown)
  document.removeEventListener('pointerdown', handleOutsideClick)
})
</script>

<template>
  <div class="app-shell" :class="{ 'sidebar-collapsed': collapsed }">
    <aside class="app-sidebar" :class="{ collapsed }">
      <div class="sidebar-brand-row">
        <RouterLink class="brand" :to="session.user ? '/upload' : '/images'" aria-label="BlueScale 首页">
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
        <RouterLink v-if="session.user" to="/upload" title="上传图片">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 16V4m0 0L7 9m5-5 5 5M5 14v4a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-4"/></svg>
          <span>上传图片</span>
        </RouterLink>
        <RouterLink to="/images" title="图片管理">
          <svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="3"/><circle cx="9" cy="10" r="2"/><path d="m4 17 5-4 3 2 3-3 5 4"/></svg>
          <span>{{ session.user ? '图片管理' : '公开图片' }}</span>
        </RouterLink>
        <RouterLink to="/albums" title="相册管理">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 7h7l2 2h9v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/><path d="M3 7V5a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v2"/></svg>
          <span>{{ session.user ? '相册管理' : '公开相册' }}</span>
        </RouterLink>
        <RouterLink v-if="session.user" to="/api" title="API">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 9 5 12l3 3m8-6 3 3-3 3M14 5l-4 14"/></svg>
          <span>API</span>
        </RouterLink>
        <RouterLink v-if="session.user" to="/settings" title="系统设置">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.86 2.86-.06-.06A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 .6l-.04.08V21h-4v-.92l-.04-.08a1.7 1.7 0 0 0-1-.6 1.7 1.7 0 0 0-1.88.34l-.06.06-2.86-2.86.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-.6-1L3.92 14H3v-4h.92L4 9.96a1.7 1.7 0 0 0 .6-1 1.7 1.7 0 0 0-.34-1.88l-.06-.06 2.86-2.86.06.06A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-.6l.04-.08V3h4v.92l.04.08a1.7 1.7 0 0 0 1 .6 1.7 1.7 0 0 0 1.88-.34l.06-.06 2.86 2.86-.06.06A1.7 1.7 0 0 0 19.4 9c.1.4.3.75.6 1l.08.04H21v4h-.92L20 14a1.7 1.7 0 0 0-.6 1Z"/></svg>
          <span>系统设置</span>
        </RouterLink>
      </nav>

      <div v-if="session.user" ref="accountArea" class="sidebar-account" :class="{ open: accountMenuOpen }">
        <div v-if="accountMenuOpen" class="account-popover" role="menu">
          <button class="account-menu-item" type="button" @click="openProfileSettings">
            <svg viewBox="0 0 24 24"><path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.86 2.86-.06-.06A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 .6l-.04.08V21h-4v-.92l-.04-.08a1.7 1.7 0 0 0-1-.6 1.7 1.7 0 0 0-1.88.34l-.06.06-2.86-2.86.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-.6-1L3.92 14H3v-4h.92L4 9.96a1.7 1.7 0 0 0 .6-1 1.7 1.7 0 0 0-.34-1.88l-.06-.06 2.86-2.86.06.06A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-.6l.04-.08V3h4v.92l.04.08a1.7 1.7 0 0 0 1 .6 1.7 1.7 0 0 0 1.88-.34l.06-.06 2.86 2.86-.06.06A1.7 1.7 0 0 0 19.4 9c.1.4.3.75.6 1l.08.04H21v4h-.92L20 14a1.7 1.7 0 0 0-.6 1Z"/></svg>
            <span>个人设置</span>
          </button>
          <button class="account-menu-item theme-menu-item" type="button" :aria-pressed="theme === 'dark'" @click="toggleTheme">
            <svg v-if="theme === 'light'" viewBox="0 0 24 24"><path d="M21 15.2A9 9 0 1 1 8.8 3a7 7 0 0 0 12.2 12.2Z"/></svg>
            <svg v-else viewBox="0 0 24 24"><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.93 4.93l1.42 1.42m11.3 11.3 1.42 1.42M2 12h2m16 0h2M4.93 19.07l1.42-1.42m11.3-11.3 1.42-1.42"/></svg>
            <span>{{ theme === 'dark' ? '浅色模式' : '深色模式' }}</span>
          </button>
          <div class="account-menu-separator"></div>
          <button class="account-menu-item logout-menu-item" type="button" @click="logout">
            <svg viewBox="0 0 24 24"><path d="M10 5H6a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h4M14 8l4 4-4 4m4-4H9"/></svg>
            <span>注销</span>
          </button>
        </div>

        <button class="sidebar-user" type="button" :title="session.user?.username" :aria-expanded="accountMenuOpen" aria-haspopup="menu" @click="accountMenuOpen = !accountMenuOpen">
          <span class="avatar">{{ session.user?.username?.slice(0, 1) }}</span>
          <div>
            <strong>{{ session.user?.username }}</strong>
            <span>管理员</span>
          </div>
          <svg class="account-chevron" viewBox="0 0 24 24"><path d="m8 10 4 4 4-4"/></svg>
        </button>
      </div>
      <div v-else class="sidebar-account guest-account">
        <button class="guest-theme-button" type="button" :title="theme === 'dark' ? '浅色模式' : '深色模式'" @click="toggleTheme">
          <svg v-if="theme === 'light'" viewBox="0 0 24 24"><path d="M21 15.2A9 9 0 1 1 8.8 3a7 7 0 0 0 12.2 12.2Z"/></svg>
          <svg v-else viewBox="0 0 24 24"><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.93 4.93l1.42 1.42m11.3 11.3 1.42 1.42M2 12h2m16 0h2M4.93 19.07l1.42-1.42m11.3-11.3 1.42-1.42"/></svg>
          <span>{{ theme === 'dark' ? '浅色模式' : '深色模式' }}</span>
        </button>
        <RouterLink class="guest-login-button" :to="{ name: 'login', query: { redirect: route.fullPath } }">
          <svg viewBox="0 0 24 24"><path d="M14 8l4 4-4 4m4-4H9M10 5H6a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h4"/></svg>
          <span>管理员登录</span>
        </RouterLink>
      </div>
    </aside>

    <button v-if="mobile && collapsed" class="mobile-sidebar-trigger" type="button" aria-label="打开侧边栏" @click="toggleSidebar">
      <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h16M4 12h16M4 17h16"/></svg>
    </button>
    <button v-if="mobile && !collapsed" class="sidebar-backdrop" type="button" aria-label="关闭侧边栏" @click="collapsed = true"></button>

    <ProfileSettingsModal v-if="session.user && profileSettingsOpen" @close="profileSettingsOpen = false" />

    <main class="page-wrap">
      <RouterView v-slot="{ Component, route: childRoute }">
        <Transition name="page-fade" mode="out-in">
          <component :is="Component" :key="childRoute.name" />
        </Transition>
      </RouterView>
    </main>
  </div>
</template>
