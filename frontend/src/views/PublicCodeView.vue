<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Check, ClipboardCopy, Cloud, KeyRound, LoaderCircle, Mail, Moon, RefreshCw, Search, ShieldCheck, Sun } from '@lucide/vue'
import { api } from '../api/client'

const email = ref('')
const enabled = ref(false)
const statusLoading = ref(true)
const loading = ref(false)
const loadingVisible = ref(false)
const result = ref(null)
const errorText = ref('')
const copied = ref(false)
const dark = ref(false)
let loadingTimer

function applyTheme(value) {
  dark.value = value
  document.documentElement.classList.toggle('dark', value)
  localStorage.setItem('ipm_v2_theme', value ? 'dark' : 'light')
}

function formatTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}

async function loadStatus() {
  statusLoading.value = true
  try {
    const data = await api('/api/v1/public-code/status')
    enabled.value = Boolean(data.enabled)
  } catch (err) {
    enabled.value = false
    errorText.value = err.message
  } finally {
    statusLoading.value = false
  }
}

async function getCode() {
  const value = email.value.trim().toLowerCase()
  if (!value) return
  loading.value = true
  loadingVisible.value = false
  clearTimeout(loadingTimer)
  loadingTimer = window.setTimeout(() => {
    if (loading.value) loadingVisible.value = true
  }, 600)
  copied.value = false
  result.value = null
  errorText.value = ''
  try {
    result.value = await api(`/api/v1/public-code?email=${encodeURIComponent(value)}&wait_ms=15000&preview=1`)
  } catch (err) {
    errorText.value = err.message
  } finally {
    clearTimeout(loadingTimer)
    loading.value = false
    loadingVisible.value = false
  }
}

async function copyCode() {
  if (!result.value?.code) return
  await navigator.clipboard.writeText(result.value.code)
  copied.value = true
  window.setTimeout(() => { copied.value = false }, 1800)
}

onMounted(() => {
  const saved = localStorage.getItem('ipm_v2_theme')
  applyTheme(saved ? saved === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches)
  const presetEmail = new URLSearchParams(window.location.search).get('email')
  if (presetEmail) email.value = presetEmail
  loadStatus()
})

onBeforeUnmount(() => clearTimeout(loadingTimer))
</script>

<template>
  <main class="relative min-h-screen overflow-hidden bg-slate-50 text-slate-800 transition-colors dark:bg-slate-950 dark:text-slate-100">
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div class="absolute -left-28 -top-36 h-[28rem] w-[28rem] rounded-full bg-emerald-200/45 blur-3xl dark:bg-emerald-900/20" />
      <div class="absolute -bottom-48 -right-32 h-[34rem] w-[34rem] rounded-full bg-sky-200/45 blur-3xl dark:bg-sky-900/20" />
      <div class="absolute left-1/2 top-1/3 h-72 w-72 -translate-x-1/2 rounded-full bg-violet-100/40 blur-3xl dark:bg-violet-950/15" />
    </div>

    <div class="relative mx-auto flex min-h-screen w-full max-w-6xl flex-col px-4 py-5 sm:px-7 sm:py-7">
      <header class="flex items-center">
        <div class="flex items-center gap-3">
          <span class="flex h-11 w-11 items-center justify-center rounded-2xl bg-emerald-500 text-white shadow-lg shadow-emerald-500/20"><Cloud :size="23" /></span>
          <div><strong class="block text-sm font-black text-slate-900 dark:text-white">iCloud Privacy Mail</strong><span class="text-[11px] text-slate-400">公共验证码服务</span></div>
        </div>
      </header>

      <div class="grid flex-1 items-center gap-10 py-10 lg:grid-cols-[0.92fr_1.08fr] lg:gap-16">
        <section class="hidden lg:block">
          <span class="inline-flex items-center gap-2 rounded-full border border-emerald-200 bg-emerald-50/80 px-3 py-1.5 text-xs font-bold text-emerald-700 backdrop-blur dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-300"><ShieldCheck :size="14" />独立取码页面</span>
          <h1 class="mt-6 text-5xl font-black leading-[1.12] tracking-tight text-slate-950 dark:text-white">输入邮箱，<br /><span class="text-emerald-500">快速查看验证码。</span></h1>
          <div class="mt-8 grid max-w-md grid-cols-3 gap-3">
            <div class="rounded-2xl border border-white/80 bg-white/60 p-4 shadow-sm backdrop-blur dark:border-slate-800 dark:bg-slate-900/55"><Mail :size="18" class="text-sky-500" /><strong class="mt-3 block text-xs">输入邮箱</strong></div>
            <div class="rounded-2xl border border-white/80 bg-white/60 p-4 shadow-sm backdrop-blur dark:border-slate-800 dark:bg-slate-900/55"><Search :size="18" class="text-violet-500" /><strong class="mt-3 block text-xs">同步查找</strong></div>
            <div class="rounded-2xl border border-white/80 bg-white/60 p-4 shadow-sm backdrop-blur dark:border-slate-800 dark:bg-slate-900/55"><KeyRound :size="18" class="text-emerald-500" /><strong class="mt-3 block text-xs">复制验证码</strong></div>
          </div>
        </section>

        <section class="mx-auto w-full max-w-xl rounded-[1.75rem] border border-white/90 bg-white/90 p-5 shadow-2xl shadow-slate-200/60 backdrop-blur-xl dark:border-slate-800 dark:bg-slate-900/90 dark:shadow-black/25 sm:p-8">
          <div class="flex items-start justify-between gap-4">
            <div><span class="text-[11px] font-black uppercase tracking-[0.2em] text-emerald-500">Verification code</span><h2 class="mt-2 text-2xl font-black text-slate-950 dark:text-white">获取邮箱验证码</h2><p class="mt-2 text-xs leading-5 text-slate-400">仅查询当前邮箱，不会显示其他邮箱信息。</p></div>
            <div class="flex shrink-0 items-center gap-2">
              <span v-if="statusLoading" class="flex h-8 w-8 items-center justify-center text-slate-400"><LoaderCircle :size="17" class="animate-spin" /></span>
              <span v-else :class="enabled ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300' : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-300'" class="shrink-0 rounded-full px-3 py-1.5 text-[11px] font-bold">{{ enabled ? '服务已开启' : '服务未开启' }}</span>
              <button type="button" class="icon-button h-8 w-8 rounded-lg border border-slate-200 bg-white shadow-sm dark:border-slate-700 dark:bg-slate-800" :title="dark ? '切换亮色主题' : '切换暗色主题'" @click="applyTheme(!dark)"><Sun v-if="dark" :size="16" /><Moon v-else :size="16" /></button>
            </div>
          </div>

          <form class="mt-7" @submit.prevent="getCode">
            <label class="form-group">
              <span class="form-label">隐私邮箱地址</span>
              <span class="field-wrap"><Mail :size="17" class="field-icon" /><input v-model.trim="email" class="field field-leading" type="email" inputmode="email" autocomplete="email" spellcheck="false" placeholder="name@icloud.com" :disabled="statusLoading || !enabled || loading" required /></span>
              <span class="form-help">请输入完整邮箱地址。</span>
            </label>
            <button class="primary-button mt-5 w-full py-3" :disabled="statusLoading || !enabled || loading || !email.trim()"><LoaderCircle v-if="loadingVisible" :size="18" class="animate-spin" /><Search v-else :size="18" />{{ loadingVisible ? '正在同步并查找...' : '获取验证码' }}</button>
          </form>

          <div class="mt-6 min-h-[236px]" aria-live="polite">
            <div v-if="result" class="min-h-[236px] rounded-2xl border border-emerald-200 bg-emerald-50/75 p-5 dark:border-emerald-900 dark:bg-emerald-950/30">
              <div class="flex items-center justify-between gap-3"><span class="flex items-center gap-2 text-xs font-black text-emerald-700 dark:text-emerald-300"><span class="flex h-7 w-7 items-center justify-center rounded-lg bg-emerald-500 text-white"><Check :size="15" /></span>已找到验证码</span><button type="button" class="secondary-button h-8 rounded-lg px-2.5 py-1 text-xs" @click="copyCode"><Check v-if="copied" :size="14" /><ClipboardCopy v-else :size="14" />{{ copied ? '已复制' : '复制' }}</button></div>
              <button type="button" class="mt-5 w-full rounded-xl border border-emerald-200 bg-white px-4 py-4 font-mono text-3xl font-black tracking-[0.3em] text-slate-950 shadow-sm transition hover:border-emerald-400 dark:border-emerald-900 dark:bg-slate-900 dark:text-white" title="点击复制验证码" @click="copyCode">{{ result.code }}</button>
              <dl class="mt-4 grid gap-2 text-[11px] text-slate-500 dark:text-slate-400 sm:grid-cols-2"><div class="min-w-0"><dt class="text-slate-400">发件人</dt><dd class="mt-0.5 truncate font-semibold">{{ result.from || '-' }}</dd></div><div><dt class="text-slate-400">接收时间</dt><dd class="mt-0.5 font-semibold">{{ formatTime(result.received_at) }}</dd></div><div class="min-w-0 sm:col-span-2"><dt class="text-slate-400">邮件主题</dt><dd class="mt-0.5 truncate font-semibold">{{ result.subject || '-' }}</dd></div></dl>
            </div>
            <div v-else-if="errorText" class="flex min-h-[236px] flex-col items-center justify-center rounded-2xl border border-rose-200 bg-rose-50/70 px-5 text-center dark:border-rose-900 dark:bg-rose-950/25"><span class="flex h-11 w-11 items-center justify-center rounded-2xl bg-rose-100 text-rose-500 dark:bg-rose-950 dark:text-rose-300"><KeyRound :size="21" /></span><strong class="mt-4 text-sm text-rose-700 dark:text-rose-300">这次还没有取到验证码</strong><p class="mt-2 max-w-sm text-xs leading-5 text-rose-500/80 dark:text-rose-300/70">{{ errorText }}</p><button v-if="enabled" type="button" class="secondary-button mt-4 h-9 px-3 py-2 text-xs" :disabled="loading" @click="getCode"><RefreshCw :size="14" />重新获取</button></div>
            <div v-else class="flex min-h-[236px] flex-col items-center justify-center rounded-2xl border border-dashed border-slate-200 bg-slate-50/70 px-5 text-center dark:border-slate-700 dark:bg-slate-800/45"><span class="flex h-11 w-11 items-center justify-center rounded-2xl bg-white text-slate-400 shadow-sm dark:bg-slate-900"><KeyRound :size="21" /></span><strong class="mt-4 text-sm text-slate-600 dark:text-slate-300">{{ enabled ? '等待输入邮箱' : '公共验证码服务当前未开放' }}</strong><p class="mt-2 max-w-sm text-xs leading-5 text-slate-400">{{ enabled ? '提交邮箱后，最新验证码会固定显示在这里。' : '请稍后再试。' }}</p></div>
          </div>
        </section>
      </div>

      <footer class="pb-2 text-center text-[11px] text-slate-400">验证码仅从最近收到的匹配邮件中提取</footer>
    </div>
  </main>
</template>
