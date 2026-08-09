<script setup>
import { computed } from 'vue'
import { Activity, Apple, Boxes, ChevronRight, Cloud, Download, LayoutDashboard, MailPlus, Settings, X } from '@lucide/vue'
import ThemeToggle from './ThemeToggle.vue'
import { useUpdates } from '../composables/useUpdates'

const props = defineProps({
  open: Boolean,
  dark: Boolean,
})

const emit = defineEmits(['close', 'toggle-theme'])
const { currentVersion, currentCommit } = useUpdates()

const versionText = computed(() => {
  const value = String(currentVersion.value || '2.0.0-dev').trim()
  return /^v/i.test(value) ? value : `v${value}`
})
const commitText = computed(() => {
  const value = String(currentCommit.value || '').trim()
  return value && value !== 'unknown' ? value.slice(0, 7) : ''
})

const items = [
  { name: 'dashboard', label: '控制台', icon: LayoutDashboard },
  { name: 'apple-accounts', label: 'Apple 账号', icon: Apple },
  { name: 'mailboxes', label: '邮箱池', icon: Boxes },
  { name: 'tasks', label: '创建隐私邮箱', icon: MailPlus },
  { name: 'exports', label: '本地导出', icon: Download },
  { name: 'settings', label: '系统设置', icon: Settings },
]

const drawerClass = computed(() => props.open ? 'translate-x-0' : '-translate-x-full md:translate-x-0')
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-30 bg-slate-950/30 backdrop-blur-sm md:hidden" role="presentation" @click.stop />
  <aside :class="drawerClass" class="fixed inset-y-0 left-0 z-40 flex h-screen w-72 flex-col border-r border-slate-200 bg-white transition-transform duration-200 dark:border-slate-700 dark:bg-slate-800 md:static md:h-full md:w-64 md:shrink-0 md:translate-x-0">
    <div class="flex h-16 shrink-0 items-center justify-between border-b border-slate-100 px-6 dark:border-slate-700/70">
      <RouterLink to="/" class="flex items-center gap-3" @click="emit('close')">
        <div class="flex h-9 w-9 items-center justify-center rounded-xl bg-emerald-50 text-emerald-500 dark:bg-emerald-950/40">
          <Cloud :size="21" />
        </div>
        <div>
          <div class="text-sm font-extrabold tracking-wide text-slate-800 dark:text-slate-100">Privacy Mail</div>
          <div class="text-[10px] font-medium tracking-[0.18em] text-emerald-500">本地控制台</div>
        </div>
      </RouterLink>
      <button class="rounded-lg p-2 text-slate-400 hover:bg-slate-100 md:hidden dark:hover:bg-slate-700" title="关闭导航" @click="emit('close')"><X :size="18" /></button>
    </div>

    <nav class="flex-1 space-y-1 overflow-y-auto px-3 py-5">
      <div class="mb-3 px-3 text-[11px] font-bold uppercase tracking-[0.18em] text-slate-400">工作区</div>
      <RouterLink v-for="item in items" :key="item.name" :to="{ name: item.name }" class="group flex items-center gap-3 rounded-xl px-3.5 py-3 text-sm font-medium text-slate-500 transition hover:bg-slate-50 hover:text-emerald-600 dark:text-slate-400 dark:hover:bg-slate-700/60 dark:hover:text-emerald-300" exact-active-class="!bg-emerald-100 !font-bold !text-emerald-800 !ring-1 !ring-inset !ring-emerald-300 dark:!bg-emerald-900/60 dark:!text-emerald-100 dark:!ring-emerald-600" @click="emit('close')">
        <component :is="item.icon" :size="19" class="shrink-0 transition group-hover:scale-105" />
        <span>{{ item.label }}</span>
        <ChevronRight :size="15" class="ml-auto opacity-0 transition group-hover:opacity-60" />
      </RouterLink>
    </nav>

    <div class="border-t border-slate-100 bg-slate-50/60 p-4 dark:border-slate-700 dark:bg-slate-900/20">
      <div class="flex items-center justify-between">
        <div class="min-w-0">
          <div class="flex items-center gap-2 text-xs font-medium text-slate-500 dark:text-slate-400"><span class="h-2 w-2 animate-pulse rounded-full bg-emerald-500" />本地服务</div>
          <RouterLink :to="{ name: 'settings', hash: '#version-updates' }" class="mt-2 inline-flex max-w-full items-center gap-1.5 rounded-md text-[10px] font-semibold text-slate-400 transition hover:text-emerald-600 dark:hover:text-emerald-300" :title="`当前版本 ${versionText}${commitText ? `，提交 ${currentCommit}` : ''}`" @click="emit('close')">
            <span>版本 {{ versionText }}</span><span v-if="commitText" class="font-mono">· {{ commitText }}</span>
          </RouterLink>
        </div>
        <ThemeToggle :dark="dark" @toggle="emit('toggle-theme')" />
      </div>
    </div>
  </aside>
</template>
