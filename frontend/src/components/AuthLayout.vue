<script setup>
import BrandMark from './BrandMark.vue'

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

