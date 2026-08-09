<script setup>
import { computed, onMounted, ref } from 'vue'
import { Database, Download, FileJson, Globe2, KeyRound, Mail, ShieldAlert } from '@lucide/vue'
import { api } from '../api/client'
import { useToast } from '../composables/useToast'

const runtime = ref({})
const settings = ref({})
const dataPath = ref('')
const { error: showError } = useToast()

const publicAPIKeyReady = computed(() => Boolean(String(settings.value.public_api_key || '').trim() || runtime.value.config_api_key_configured))
const publicAPIKeySource = computed(() => {
  if (String(settings.value.public_api_key || '').trim()) return '系统设置'
  if (runtime.value.config_api_key_configured) return 'config.json'
  return '尚未设置'
})

const exportItems = [
  { title: '运行数据', description: '导出账号、邮箱、登录态和系统设置，不包含本地邮件正文。', href: '/api/runtime/export', icon: FileJson, tone: 'text-violet-600 bg-violet-100 dark:bg-violet-950/60 dark:text-violet-300', format: 'JSON' },
  { title: '运行数据与邮件', description: '在完整运行数据中加入所有已同步的本地邮件内容。', href: '/api/runtime/export?include_messages=1', icon: Database, tone: 'text-sky-600 bg-sky-100 dark:bg-sky-950/60 dark:text-sky-300', format: 'JSON' },
  { title: '邮箱地址', description: '只导出邮箱池中的隐私邮箱地址，方便导入其他本地工具。', href: '/api/runtime/export-mailbox-emails?format=txt', icon: Mail, tone: 'text-emerald-600 bg-emerald-100 dark:bg-emerald-950/60 dark:text-emerald-300', format: 'TXT' },
  { title: '取码 API', description: '导出每个邮箱的地址与独立取码 API 链接。', href: '/api/runtime/export-mailbox-apis?format=txt', icon: KeyRound, tone: 'text-amber-600 bg-amber-100 dark:bg-amber-950/60 dark:text-amber-300', format: 'TXT' },
]

async function load() {
  try {
    const data = await api('/api/settings')
    runtime.value = data.runtime || {}
    settings.value = data.settings || {}
    dataPath.value = data.data_path || ''
  } catch (err) {
    showError(err.message)
  }
}

onMounted(load)
</script>

<template>
  <div class="mx-auto max-w-6xl space-y-5">
    <section class="panel overflow-hidden">
      <div class="flex flex-col gap-3 border-b border-slate-100 px-5 py-5 dark:border-slate-700 sm:flex-row sm:items-center sm:justify-between sm:px-6">
        <div><h2 class="text-lg font-black">选择导出内容</h2><p class="mt-1 text-sm leading-6 text-slate-400">根据用途下载完整备份、邮件数据、邮箱地址或取码 API。</p></div>
        <span class="inline-flex self-start items-center gap-1.5 rounded-full bg-amber-100 px-3 py-1.5 text-xs font-bold text-amber-700 dark:bg-amber-950/60 dark:text-amber-300"><ShieldAlert :size="14" />包含敏感本地数据</span>
      </div>

      <div class="grid gap-4 p-5 sm:grid-cols-2 sm:p-6">
        <a v-for="item in exportItems" :key="item.title" :href="item.href" download class="group flex min-h-36 items-start gap-4 rounded-2xl border border-slate-200 bg-slate-50 p-5 transition hover:-translate-y-0.5 hover:border-emerald-300 hover:bg-emerald-50/60 hover:shadow-lg hover:shadow-emerald-500/5 dark:border-slate-700 dark:bg-slate-800/70 dark:hover:border-emerald-700 dark:hover:bg-emerald-950/20">
          <span :class="item.tone" class="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl"><component :is="item.icon" :size="20" /></span>
          <span class="min-w-0 flex-1"><span class="flex items-center justify-between gap-3"><strong class="text-sm text-slate-800 dark:text-slate-100">{{ item.title }}</strong><span class="rounded-md bg-white px-2 py-0.5 font-mono text-[10px] font-black text-slate-400 shadow-sm dark:bg-slate-900">{{ item.format }}</span></span><small class="mt-2 block text-xs leading-5 text-slate-400">{{ item.description }}</small><span class="mt-4 inline-flex items-center gap-1.5 text-xs font-bold text-emerald-600 dark:text-emerald-300"><Download :size="14" />下载文件</span></span>
        </a>
      </div>
    </section>

    <section class="panel p-5 sm:p-6">
      <h3 class="section-title flex items-center gap-2"><Globe2 :size="16" />导出环境</h3>
      <div class="grid gap-3 text-xs sm:grid-cols-2 lg:grid-cols-3">
        <div class="rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-700 dark:bg-slate-800"><span class="text-slate-400">状态文件</span><strong class="mt-1.5 block truncate font-mono text-slate-700 dark:text-slate-200" :title="dataPath">{{ dataPath || '使用当前运行状态文件' }}</strong></div>
        <div class="rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-700 dark:bg-slate-800"><span class="text-slate-400">公共基础地址</span><strong class="mt-1.5 block truncate text-slate-700 dark:text-slate-200">{{ runtime.public_base_url || '按当前访问地址生成' }}</strong></div>
        <div class="rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-700 dark:bg-slate-800 sm:col-span-2 lg:col-span-1"><span class="text-slate-400">全局 API Key</span><strong :class="publicAPIKeyReady ? 'text-emerald-600 dark:text-emerald-300' : 'text-amber-600 dark:text-amber-300'" class="mt-1.5 block">{{ publicAPIKeyReady ? `已配置（${publicAPIKeySource}）` : '尚未设置' }}</strong></div>
      </div>
      <p class="mt-4 text-xs leading-5 text-slate-400">运行数据导出包含 Apple 登录态、Cookie 和 App 专用密码等内容，请将下载文件保存在可信位置。</p>
    </section>
  </div>
</template>
