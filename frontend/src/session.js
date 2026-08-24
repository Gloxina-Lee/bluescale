import { reactive } from 'vue'
import { api } from './api'

export const session = reactive({
  configured: null,
  user: null,
})

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

