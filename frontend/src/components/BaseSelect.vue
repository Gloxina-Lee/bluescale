<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

let nextSelectID = 0

const props = defineProps({
  modelValue: {
    type: [String, Number, Boolean],
    default: '',
  },
  options: {
    type: Array,
    required: true,
  },
  disabled: Boolean,
  placeholder: {
    type: String,
    default: '请选择',
  },
  ariaLabel: {
    type: String,
    default: '选择选项',
  },
})

const emit = defineEmits(['update:modelValue', 'change'])
const instanceID = `base-select-${++nextSelectID}`
const trigger = ref(null)
const menu = ref(null)
const open = ref(false)
const activeIndex = ref(-1)
const menuStyle = ref({})

const selectedIndex = computed(() => props.options.findIndex((option) => Object.is(option.value, props.modelValue)))
const selectedOption = computed(() => props.options[selectedIndex.value] || null)

function enabledIndex(start, direction) {
  if (!props.options.length) return -1
  let index = start
  for (let count = 0; count < props.options.length; count += 1) {
    index = (index + direction + props.options.length) % props.options.length
    if (!props.options[index]?.disabled) return index
  }
  return -1
}

function positionMenu() {
  if (!open.value || !trigger.value) return
  const rect = trigger.value.getBoundingClientRect()
  const viewportMargin = 12
  const gap = 8
  const estimatedHeight = Math.min(288, props.options.length * 42 + 14)
  const spaceBelow = window.innerHeight - rect.bottom - viewportMargin
  const openAbove = spaceBelow < Math.min(estimatedHeight, 190) && rect.top > spaceBelow
  const width = Math.min(Math.max(rect.width, 128), window.innerWidth - viewportMargin * 2)
  const left = Math.min(Math.max(viewportMargin, rect.left), window.innerWidth - width - viewportMargin)

  menuStyle.value = openAbove
    ? { left: `${left}px`, bottom: `${Math.max(viewportMargin, window.innerHeight - rect.top + gap)}px`, width: `${width}px` }
    : { left: `${left}px`, top: `${Math.min(rect.bottom + gap, window.innerHeight - viewportMargin)}px`, width: `${width}px` }
}

async function showMenu(direction = 1) {
  if (props.disabled || !props.options.length) return
  if (selectedIndex.value >= 0 && !props.options[selectedIndex.value]?.disabled) activeIndex.value = selectedIndex.value
  else activeIndex.value = enabledIndex(direction > 0 ? -1 : 0, direction)
  open.value = true
  await nextTick()
  positionMenu()
  menu.value?.querySelector(`[data-option-index="${activeIndex.value}"]`)?.scrollIntoView({ block: 'nearest' })
}

function closeMenu({ restoreFocus = false } = {}) {
  if (!open.value) return
  open.value = false
  if (restoreFocus) nextTick(() => trigger.value?.focus())
}

function toggleMenu() {
  if (open.value) closeMenu()
  else showMenu()
}

function selectOption(option) {
  if (option.disabled) return
  emit('update:modelValue', option.value)
  emit('change', option.value)
  closeMenu({ restoreFocus: true })
}

function moveActive(direction) {
  const next = enabledIndex(activeIndex.value, direction)
  if (next < 0) return
  activeIndex.value = next
  nextTick(() => menu.value?.querySelector(`[data-option-index="${next}"]`)?.scrollIntoView({ block: 'nearest' }))
}

function handleKeydown(event) {
  if (props.disabled) return
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    if (!open.value) showMenu(event.key === 'ArrowDown' ? 1 : -1)
    else moveActive(event.key === 'ArrowDown' ? 1 : -1)
    return
  }
  if (!open.value) {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      showMenu()
    }
    return
  }
  if (event.key === 'Home' || event.key === 'End') {
    event.preventDefault()
    activeIndex.value = enabledIndex(event.key === 'Home' ? -1 : 0, event.key === 'Home' ? 1 : -1)
  } else if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    const option = props.options[activeIndex.value]
    if (option) selectOption(option)
  } else if (event.key === 'Escape') {
    event.preventDefault()
    closeMenu({ restoreFocus: true })
  } else if (event.key === 'Tab') {
    closeMenu()
  }
}

function handlePointerDown(event) {
  if (trigger.value?.contains(event.target) || menu.value?.contains(event.target)) return
  closeMenu()
}

watch(() => props.options, () => {
  if (open.value) nextTick(positionMenu)
})

watch(() => props.disabled, (disabled) => {
  if (disabled) closeMenu()
})

onMounted(() => {
  document.addEventListener('pointerdown', handlePointerDown)
  window.addEventListener('resize', positionMenu)
  window.addEventListener('scroll', positionMenu, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', handlePointerDown)
  window.removeEventListener('resize', positionMenu)
  window.removeEventListener('scroll', positionMenu, true)
})
</script>

<template>
  <div class="base-select" :class="{ open, disabled }" @keydown="handleKeydown">
    <button
      ref="trigger"
      class="base-select-trigger"
      type="button"
      role="combobox"
      aria-haspopup="listbox"
      :aria-label="ariaLabel"
      :aria-controls="instanceID"
      :aria-expanded="open"
      :aria-activedescendant="open && activeIndex >= 0 ? `${instanceID}-option-${activeIndex}` : undefined"
      :disabled="disabled"
      @click="toggleMenu"
    >
      <span :class="{ placeholder: !selectedOption }">{{ selectedOption?.label || placeholder }}</span>
      <svg class="base-select-chevron" viewBox="0 0 20 20" aria-hidden="true"><path d="m5.5 7.75 4.5 4.5 4.5-4.5" /></svg>
    </button>

    <Teleport to="body">
      <Transition name="select-menu">
        <div
          v-if="open"
          :id="instanceID"
          ref="menu"
          class="base-select-menu"
          :style="menuStyle"
          role="listbox"
          :aria-label="ariaLabel"
        >
          <button
            v-for="(option, index) in options"
            :id="`${instanceID}-option-${index}`"
            :key="`${String(option.value)}-${index}`"
            class="base-select-option"
            :class="{ active: index === activeIndex, selected: Object.is(option.value, modelValue) }"
            :data-option-index="index"
            type="button"
            role="option"
            :aria-selected="Object.is(option.value, modelValue)"
            :disabled="option.disabled"
            @mouseenter="activeIndex = index"
            @click="selectOption(option)"
          >
            <span>{{ option.label }}</span>
            <svg v-if="Object.is(option.value, modelValue)" viewBox="0 0 20 20" aria-hidden="true"><path d="m4.5 10.25 3.35 3.35L15.7 5.8" /></svg>
          </button>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
