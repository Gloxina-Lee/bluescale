import { reactive } from 'vue'
import { api } from './api'

export const session = reactive({
  configured: null,
  user: null,
})

export function hasPermission(permission, user = session.user) {
  return Boolean(user?.permissions?.[permission])
}

export function defaultRouteName(user = session.user) {
  if (hasPermission('upload', user)) return 'upload'
  if (hasPermission('manageImages', user)) return 'images'
  if (hasPermission('manageUsers', user)) return 'users'
  return 'login'
}

export async function loadStatus() {
  const status = await api('/api/status')
  session.configured = status.configured
  return status
}

export async function loadUser() {
  try {
    session.user = await api('/api/me')
  } catch (error) {
    if (error.status !== 401) throw error
    session.user = null
  }
  return session.user
}

export async function signOut() {
  try {
    await api('/api/logout', { method: 'POST' })
  } finally {
    session.user = null
  }
}
