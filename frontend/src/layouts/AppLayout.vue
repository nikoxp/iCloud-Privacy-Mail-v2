<script setup>
import { computed, onMounted, ref } from 'vue'
import { Activity, Bell, LogOut, Menu, UserCircle } from '@lucide/vue'
import { useRoute, useRouter } from 'vue-router'
import Sidebar from '../components/Sidebar.vue'
import ThemeToggle from '../components/ThemeToggle.vue'
import { useAuth } from '../composables/useAuth'

const route = useRoute()
const router = useRouter()
const { authState, logout } = useAuth()
const sidebarOpen = ref(false)
const dark = ref(false)
const profileOpen = ref(false)

const title = computed(() => route.meta.title || '控制台')
const subtitle = computed(() => route.meta.subtitle || '')

function applyTheme(value) {
  dark.value = value
  document.documentElement.classList.toggle('dark', value)
  localStorage.setItem('ipm_v2_theme', value ? 'dark' : 'light')
}

async function signOut() {
  try {
    await logout()
  } finally {
    profileOpen.value = false
    router.replace({ name: 'login' })
  }
}

onMounted(() => {
  const saved = localStorage.getItem('ipm_v2_theme')
  applyTheme(saved ? saved === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches)
})
</script>

<template>
  <div class="flex h-screen overflow-hidden bg-slate-50 text-slate-800 transition-colors dark:bg-slate-950 dark:text-slate-100">
    <Sidebar :open="sidebarOpen" :dark="dark" @close="sidebarOpen = false" @toggle-theme="applyTheme(!dark)" />
    <div class="flex min-h-0 min-w-0 flex-1 flex-col">
      <header class="sticky top-0 z-20 flex h-16 shrink-0 items-center justify-between border-b border-slate-200/80 bg-white/85 px-4 backdrop-blur-md dark:border-slate-800 dark:bg-slate-900/85 md:px-7">
        <div class="flex min-w-0 items-center gap-3">
          <button class="rounded-xl p-2 text-slate-500 hover:bg-slate-100 md:hidden dark:hover:bg-slate-800" title="打开导航" @click="sidebarOpen = true"><Menu :size="20" /></button>
          <div class="min-w-0">
            <div class="flex items-center gap-2 text-sm font-bold text-slate-800 dark:text-slate-100"><span class="hidden text-slate-400 sm:inline">控制台</span><span class="hidden text-slate-300 sm:inline">/</span><span class="truncate">{{ title }}</span></div>
            <div class="hidden truncate text-xs text-slate-400 sm:block">{{ subtitle }}</div>
          </div>
        </div>
        <div class="flex items-center gap-2 sm:gap-4">
          <span class="hidden items-center gap-2 text-xs font-medium text-slate-400 lg:flex"><Activity :size="15" class="text-emerald-500" />本地模式</span>
          <ThemeToggle :dark="dark" class="hidden sm:flex" @toggle="applyTheme(!dark)" />
          <button class="relative rounded-xl p-2 text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800" title="通知"><Bell :size="18" /><span class="absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full bg-emerald-500" /></button>
          <div class="relative">
            <button class="flex items-center gap-2 rounded-xl p-1.5 hover:bg-slate-100 dark:hover:bg-slate-800" @click="profileOpen = !profileOpen"><UserCircle :size="27" class="text-emerald-500" /><span class="hidden max-w-28 truncate text-xs font-bold sm:block">{{ authState.admin?.username || '管理员' }}</span></button>
            <div v-if="profileOpen" class="absolute right-0 top-12 w-48 rounded-xl border border-slate-200 bg-white p-2 shadow-xl dark:border-slate-700 dark:bg-slate-800">
              <div class="border-b border-slate-100 px-3 py-2 text-xs text-slate-400 dark:border-slate-700">单用户本地控制台</div>
              <button class="mt-1 flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-950/30" @click="signOut"><LogOut :size="15" />退出登录</button>
            </div>
          </div>
        </div>
      </header>
      <main class="min-h-0 flex-1 overflow-y-auto p-4 md:p-7">
        <Transition name="page" mode="out-in"><RouterView /></Transition>
      </main>
    </div>
  </div>
</template>
