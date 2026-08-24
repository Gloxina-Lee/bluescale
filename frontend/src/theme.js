import { ref } from 'vue'

function storedTheme() {
  try {
    return localStorage.getItem('bluescale_theme') === 'dark' ? 'dark' : 'light'
  } catch {
    return 'light'
  }
}

export const theme = ref(storedTheme())

function applyTheme(value) {
  document.documentElement.dataset.theme = value
  document.documentElement.style.colorScheme = value
}

export function initializeTheme() {
  applyTheme(theme.value)
}

export function setTheme(value) {
  theme.value = value === 'dark' ? 'dark' : 'light'
  applyTheme(theme.value)
  try {
    localStorage.setItem('bluescale_theme', theme.value)
  } catch {
    // Theme persistence is optional when storage is unavailable.
  }
}

export function toggleTheme() {
  setTheme(theme.value === 'dark' ? 'light' : 'dark')
}
