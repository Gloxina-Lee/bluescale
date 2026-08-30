import { ref } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import SetupView from './views/SetupView.vue'
import LoginView from './views/LoginView.vue'
import AppShell from './components/AppShell.vue'
import UploadView from './views/UploadView.vue'
import ImagesView from './views/ImagesView.vue'
import AlbumsView from './views/AlbumsView.vue'
import SettingsView from './views/SettingsView.vue'
import APIView from './views/APIView.vue'
import APIDocsView from './views/APIDocsView.vue'
import { loadStatus, loadUser, session } from './session'

export const navigationPending = ref(false)
let navigationFinishTimer

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/setup', name: 'setup', component: SetupView },
    { path: '/login', name: 'login', component: LoginView },
    {
      path: '/',
      component: AppShell,
      children: [
        { path: '', redirect: '/images' },
        { path: 'upload', name: 'upload', component: UploadView, meta: { auth: true } },
        { path: 'images', name: 'images', component: ImagesView },
        { path: 'albums', name: 'albums', component: AlbumsView },
        { path: 'api', name: 'api', component: APIView, meta: { auth: true } },
        { path: 'docs', name: 'api-docs', component: APIDocsView },
        { path: 'settings', name: 'settings', component: SettingsView, meta: { auth: true } },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach(async (to) => {
  window.clearTimeout(navigationFinishTimer)
  navigationPending.value = true
  try {
    await loadStatus()
    if (!session.configured && to.name !== 'setup') return { name: 'setup' }
    if (!session.configured) return true

    await loadUser()
    if (to.name === 'setup') return session.user ? { name: 'upload' } : { name: 'login' }
    if (to.name === 'login' && session.user) return { name: 'upload' }
    if (to.matched.some((route) => route.meta.auth) && !session.user) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }
    return true
  } catch {
    return true
  }
})

router.afterEach(() => {
  window.clearTimeout(navigationFinishTimer)
  navigationFinishTimer = window.setTimeout(() => {
    navigationPending.value = false
  }, 180)
})

router.onError(() => {
  window.clearTimeout(navigationFinishTimer)
  navigationPending.value = false
})

export default router
