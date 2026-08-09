<script setup>
import { Moon, Sun } from '@lucide/vue'

defineProps({
  dark: Boolean,
})

defineEmits(['toggle'])
</script>

<template>
  <button class="flex h-9 w-9 items-center justify-center rounded-xl bg-slate-100 text-slate-500 transition hover:bg-slate-200 dark:bg-slate-700 dark:text-slate-300 dark:hover:bg-slate-600" title="切换主题" @click="$emit('toggle')">
    <Moon v-if="dark" :size="17" />
    <Sun v-else :size="17" class="text-amber-500" />
  </button>
</template>
