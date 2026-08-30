<script setup>
import { navigationPending } from './router'
</script>

<template>
  <Transition name="route-progress">
    <div v-if="navigationPending" class="route-progress" role="progressbar" aria-label="正在切换页面"><i></i></div>
  </Transition>
  <RouterView v-slot="{ Component, route }">
    <Transition name="app-route" mode="out-in">
      <component :is="Component" :key="route.matched[0]?.path || route.path" />
    </Transition>
  </RouterView>
</template>

