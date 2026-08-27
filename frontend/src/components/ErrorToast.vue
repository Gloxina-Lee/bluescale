<script setup>
import { onBeforeUnmount, watch } from 'vue'

const props = defineProps({ message: { type: String, default: '' } })
const emit = defineEmits(['close'])
let timer = null

function close() {
  if (timer) window.clearTimeout(timer)
  timer = null
  emit('close')
}

watch(() => props.message, (message) => {
  if (timer) window.clearTimeout(timer)
  timer = message ? window.setTimeout(close, 3000) : null
}, { immediate: true })

onBeforeUnmount(() => {
  if (timer) window.clearTimeout(timer)
})
</script>

<template>
  <Transition name="error-toast">
    <div v-if="message" class="error-toast" role="alert" aria-live="assertive">
      <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v6m0 4h.01"/></svg>
      <span>{{ message }}</span>
      <button type="button" aria-label="关闭错误提示" @click="close">×</button>
    </div>
  </Transition>
</template>
