<script setup>
import { ref } from 'vue'

const props = defineProps({
  albums: { type: Array, required: true },
  initialSelected: { type: Array, default: () => [] },
  title: { type: String, default: '选择相册' },
  description: { type: String, default: '一张图片可以同时加入多个相册。' },
  confirmText: { type: String, default: '确认选择' },
  busy: Boolean,
})
const emit = defineEmits(['close', 'confirm'])
const selected = ref(new Set(props.initialSelected))

function toggle(id) {
  const next = new Set(selected.value)
  next.has(id) ? next.delete(id) : next.add(id)
  selected.value = next
}

function close() {
  if (!props.busy) emit('close')
}
</script>

<template>
  <div class="modal-layer" @mousedown.self="close">
    <section class="management-modal album-picker-modal" role="dialog" aria-modal="true" aria-labelledby="album-picker-title">
      <header>
        <div><span class="eyebrow">ALBUMS</span><h2 id="album-picker-title">{{ title }}</h2><p>{{ description }}</p></div>
        <button type="button" aria-label="关闭" :disabled="busy" @click="close">×</button>
      </header>
      <div class="album-picker-body">
        <label v-for="album in albums" :key="album.id" class="album-picker-option" :class="{ selected: selected.has(album.id) }">
          <input type="checkbox" :checked="selected.has(album.id)" @change="toggle(album.id)" />
          <span class="album-option-icon"><svg viewBox="0 0 24 24"><path d="M3 7h7l2 2h9v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/><path d="M3 7V5a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v2"/></svg></span>
          <span><strong>{{ album.name }}</strong><small>{{ album.imageCount }} 张图片</small></span>
          <i>{{ selected.has(album.id) ? '✓' : '' }}</i>
        </label>
      </div>
      <footer class="modal-actions">
        <span>已选择 {{ selected.size }} 个相册</span>
        <button class="secondary-button" type="button" :disabled="busy" @click="close">取消</button>
        <button class="primary-button" type="button" :disabled="busy || !selected.size" @click="emit('confirm', Array.from(selected))">{{ busy ? '正在处理…' : confirmText }}</button>
      </footer>
    </section>
  </div>
</template>
