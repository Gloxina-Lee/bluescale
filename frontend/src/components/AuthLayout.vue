<script setup>
import BrandMark from './BrandMark.vue'
import { theme, toggleTheme } from '../theme'

defineProps({
  eyebrow: String,
  title: String,
  description: String,
})
</script>

<template>
  <main class="auth-page">
    <div class="auth-glow auth-glow-one"></div>
    <div class="auth-glow auth-glow-two"></div>
    <button class="auth-theme-toggle" type="button" :aria-label="theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'" @click="toggleTheme">
      <svg v-if="theme === 'light'" viewBox="0 0 24 24" aria-hidden="true"><path d="M21 15.2A9 9 0 1 1 8.8 3a7 7 0 0 0 12.2 12.2Z"/></svg>
      <svg v-else viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.93 4.93l1.42 1.42m11.3 11.3 1.42 1.42M2 12h2m16 0h2M4.93 19.07l1.42-1.42m11.3-11.3 1.42-1.42"/></svg>
      <span>{{ theme === 'dark' ? '浅色模式' : '深色模式' }}</span>
    </button>
    <section class="auth-intro">
      <div class="brand brand-large"><BrandMark /><span>BlueScale</span></div>
      <div class="intro-copy">
        <span class="eyebrow">{{ eyebrow }}</span>
        <h1>{{ title }}</h1>
        <p>{{ description }}</p>
      </div>
      <div class="feature-row">
        <span>Go 原生直出</span><i></i><span>SQLite</span><i></i><span>私有图床</span>
      </div>
    </section>
    <section class="auth-panel">
      <slot />
    </section>
  </main>
</template>
