import { createRouter, createWebHistory } from 'vue-router'
import SetupView from './views/SetupView.vue'
import LoginView from './views/LoginView.vue'
import AppShell from './components/AppShell.vue'
import UploadView from './views/UploadView.vue'
import ImagesView from './views/ImagesView.vue'
import UsersView from './views/UsersView.vue'
import { defaultRouteName, hasPermission, loadStatus, loadUser, session } from './session'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/setup', name: 'setup', component: SetupView },
    { path: '/login', name: 'login', component: LoginView },
    {
      path: '/',
      component: AppShell,
      meta: { auth: true },
      children: [
        { path: '', redirect: '/upload' },
        { path: 'upload', name: 'upload', component: UploadView, meta: { permission: 'upload' } },
        { path: 'images', name: 'images', component: ImagesView, meta: { permission: 'manageImages' } },
        { path: 'users', name: 'users', component: UsersView, meta: { permission: 'manageUsers' } },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach(async (to) => {
  try {
    await loadStatus()
    if (!session.configured && to.name !== 'setup') return { name: 'setup' }
    if (!session.configured) return true

    await loadUser()
    if (to.name === 'setup') return session.user ? { name: defaultRouteName() } : { name: 'login' }
    if (to.name === 'login' && session.user) return { name: defaultRouteName() }
    if (to.matched.some((route) => route.meta.auth) && !session.user) return { name: 'login' }
    const permission = to.matched.map((route) => route.meta.permission).find(Boolean)
    if (permission && !hasPermission(permission)) return { name: defaultRouteName() }
    return true
  } catch {
    return true
  }
})

export default router
