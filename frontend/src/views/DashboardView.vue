<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Apple, ArrowRight, Boxes, CheckCircle2, CircleAlert, Inbox, LoaderCircle, MailCheck, MessageSquareText, ShieldCheck, Timer, Trash2 } from '@lucide/vue'
import { api } from '../api/client'
import { useConfirm } from '../composables/useConfirm'
import { useToast } from '../composables/useToast'

const loading = ref(true)
const loadFailed = ref(false)
const clearing = ref(false)
const dashboard = ref({ events: [] })
const runtimeTasks = ref([])
const currentTime = ref(Date.now())
let countdownTimer
let runtimeRefreshTimer
let eventRefreshTimer
let eventRefreshing = false
const { error: showError, success: showSuccess } = useToast()
const { confirm: confirmAction } = useConfirm()

const cards = computed(() => [
  { label: 'Apple 账号', value: dashboard.value.apple_account_count || 0, detail: `${dashboard.value.active_account_count || 0} 个状态正常`, icon: Apple, tone: 'text-slate-700 bg-slate-100 dark:bg-slate-700 dark:text-slate-200' },
  { label: '隐私邮箱', value: dashboard.value.mailbox_count || 0, detail: `${dashboard.value.available_count || 0} 个可用`, icon: Boxes, tone: 'text-emerald-600 bg-emerald-50 dark:bg-emerald-950/40 dark:text-emerald-300' },
  { label: '本地邮件', value: dashboard.value.message_count || 0, detail: '本地缓存的邮件记录', icon: MessageSquareText, tone: 'text-sky-600 bg-sky-50 dark:bg-sky-950/40 dark:text-sky-300' },
])

const operationalTasks = computed(() => {
  const taskIDs = ['imap-watcher', 'apple-keepalive', 'scheduler', 'public-api']
  return taskIDs.map((id) => runtimeTasks.value.find((task) => task.id === id)).filter(Boolean)
})

function taskIcon(task) {
  if (task.id === 'imap-watcher') return Inbox
  if (task.id === 'apple-keepalive') return Apple
  if (task.id === 'scheduler') return Timer
  return ShieldCheck
}

function taskStatusText(status) {
  return ({ completed: '已就绪', running: '运行中', starting: '启动中', creating: '创建中', waiting: '等待条件', failed: '运行异常', stopped: '已停止', idle: '未启动', planned: '待启用' })[status] || status
}

function taskStatusClass(status) {
  if (status === 'running' || status === 'creating') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-200'
  if (status === 'failed') return 'bg-rose-100 text-rose-700 dark:bg-rose-950/60 dark:text-rose-200'
  if (status === 'completed' || status === 'waiting' || status === 'starting') return 'bg-sky-100 text-sky-700 dark:bg-sky-950/60 dark:text-sky-200'
  return 'bg-slate-100 text-slate-500 dark:bg-slate-700 dark:text-slate-300'
}

function taskIconClass(status) {
  if (status === 'running' || status === 'creating') return 'bg-emerald-100 text-emerald-600 dark:bg-emerald-950/60 dark:text-emerald-300'
  if (status === 'failed') return 'bg-rose-100 text-rose-600 dark:bg-rose-950/60 dark:text-rose-300'
  if (status === 'completed' || status === 'waiting' || status === 'starting') return 'bg-sky-100 text-sky-600 dark:bg-sky-950/60 dark:text-sky-300'
  return 'bg-slate-100 text-slate-400 dark:bg-slate-700 dark:text-slate-300'
}

function countdownText(value) {
  if (!value) return '正在安排'
  const seconds = Math.max(0, Math.ceil((new Date(value).getTime() - currentTime.value) / 1000))
  if (!seconds) return '即将执行'
  return `${Math.floor(seconds / 60)}分${String(seconds % 60).padStart(2, '0')}秒`
}

function taskDescription(task) {
  if (task.id !== 'apple-keepalive' || task.status !== 'running') return task.description
  const jitter = task.jitter_percent ? ` · 每轮随机 ±${task.jitter_percent}%` : ''
  return `下次扫描 ${countdownText(task.next_run_at)}${jitter}`
}

async function refreshRuntimeTasks() {
  try {
    const taskData = await api('/api/tasks')
    runtimeTasks.value = taskData.items || []
  } catch {
    return
  }
}

async function refreshEvents() {
  if (eventRefreshing) return
  eventRefreshing = true
  try {
    const eventData = await api('/api/events')
    dashboard.value.events = eventData.items || []
  } catch {
    return
  } finally {
    eventRefreshing = false
  }
}

function eventTone(level) {
  if (level === 'error') return 'bg-rose-50 text-rose-600 dark:bg-rose-950/40 dark:text-rose-300'
  if (level === 'warning') return 'bg-amber-50 text-amber-600 dark:bg-amber-950/40 dark:text-amber-300'
  return 'bg-emerald-50 text-emerald-600 dark:bg-emerald-950/40 dark:text-emerald-300'
}

function formatTime(value) {
  if (!value) return '-'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value))
}

async function clearEvents() {
  if (!dashboard.value.events?.length) return
  const confirmed = await confirmAction({
    title: '清空运行记录',
    message: '确定清空控制台的所有运行记录吗？清空后这些记录将不再显示。',
    confirmText: '确认清空',
    tone: 'danger',
  })
  if (!confirmed) return
  clearing.value = true
  try {
    await api('/api/events/clear', { method: 'POST', body: '{}' })
    dashboard.value.events = []
    showSuccess('控制台运行记录已清空')
  } catch (err) {
    showError(err.message)
  } finally {
    clearing.value = false
  }
}

onMounted(async () => {
  try {
    const [dashboardData, taskData] = await Promise.all([api('/api/dashboard'), api('/api/tasks')])
    dashboard.value = dashboardData
    runtimeTasks.value = taskData.items || []
  } catch (err) {
    loadFailed.value = true
    showError(err.message)
  } finally {
    loading.value = false
  }
  countdownTimer = window.setInterval(() => { currentTime.value = Date.now() }, 1000)
  runtimeRefreshTimer = window.setInterval(refreshRuntimeTasks, 10000)
  eventRefreshTimer = window.setInterval(refreshEvents, 2000)
})

onBeforeUnmount(() => {
  window.clearInterval(countdownTimer)
  window.clearInterval(runtimeRefreshTimer)
  window.clearInterval(eventRefreshTimer)
})
</script>

<template>
  <div class="mx-auto max-w-7xl space-y-6">
    <div v-if="loading" class="flex min-h-72 items-center justify-center text-slate-400"><LoaderCircle class="animate-spin" :size="24" /></div>
    <div v-else-if="loadFailed" class="panel border-rose-200 p-5 text-rose-600 dark:border-rose-900">控制台数据加载失败，请稍后刷新。</div>
    <template v-else>
      <div class="grid gap-4 sm:grid-cols-3">
        <article v-for="card in cards" :key="card.label" class="panel p-5">
          <div class="flex items-start justify-between"><div><div class="text-sm font-medium text-slate-400">{{ card.label }}</div><div class="mt-2 text-3xl font-black text-slate-900 dark:text-white">{{ card.value }}</div></div><div :class="card.tone" class="rounded-xl p-2.5"><component :is="card.icon" :size="20" /></div></div>
          <div class="mt-3 text-xs text-slate-400">{{ card.detail }}</div>
        </article>
      </div>

      <div class="grid items-stretch gap-6 xl:grid-cols-[1fr_330px]">
        <section class="panel h-full overflow-hidden">
          <div class="flex items-center justify-between border-b border-slate-100 px-5 py-4 dark:border-slate-700"><div><h2 class="font-bold text-slate-800 dark:text-slate-100">运行记录</h2><p class="mt-0.5 text-xs text-slate-400">最近产生的系统事件</p></div><div class="flex items-center gap-1"><button class="icon-button" title="清空运行记录" :disabled="clearing || !dashboard.events?.length" @click="clearEvents"><LoaderCircle v-if="clearing" :size="17" class="animate-spin" /><Trash2 v-else :size="17" /></button><Inbox :size="19" class="text-slate-400" /></div></div>
          <div v-if="!dashboard.events?.length" class="flex h-[486.5px] flex-col items-center justify-center gap-3 text-slate-400"><MailCheck :size="35" class="text-emerald-400" /><div class="text-sm">暂无运行事件</div></div>
          <div v-else class="h-[486.5px] overflow-y-auto">
            <div v-for="event in dashboard.events" :key="event.id" class="flex min-h-[69.5px] items-center gap-3 border-b border-slate-100 px-5 py-3.5 dark:border-slate-700/70"><div :class="eventTone(event.level)" class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg"><CircleAlert v-if="event.level === 'error'" :size="14" /><CheckCircle2 v-else :size="14" /></div><div class="min-w-0 flex-1"><div class="break-words text-sm text-slate-700 dark:text-slate-200">{{ event.message }}</div><div class="mt-1 flex gap-2 text-[11px] text-slate-400"><span>{{ event.category }}</span><span>·</span><span>{{ formatTime(event.created_at) }}</span></div></div></div>
          </div>
        </section>

        <aside class="h-full">
          <section class="panel flex h-full flex-col overflow-hidden">
            <div class="bg-gradient-to-br from-emerald-500 to-teal-700 p-5 text-white">
              <div class="text-xs font-bold tracking-[0.18em] text-emerald-100">运行状态</div>
              <h2 class="mt-2 text-xl font-black">核心服务</h2>
              <p class="mt-2 text-xs leading-5 text-emerald-50/90">集中查看后台监听、登录态保活、定时创建和公共 API。</p>
            </div>
            <div class="flex min-h-0 flex-1 flex-col p-4">
              <div v-if="operationalTasks.length" class="divide-y divide-slate-100 dark:divide-slate-700/70">
                <div v-for="task in operationalTasks" :key="task.id" class="flex items-center gap-3 py-3.5">
                  <span :class="taskIconClass(task.status)" class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl"><component :is="taskIcon(task)" :size="17" /></span>
                  <span class="min-w-0 flex-1"><strong class="block truncate text-sm text-slate-700 dark:text-slate-200">{{ task.name }}</strong><small class="mt-0.5 block truncate text-[10px] text-slate-400" :title="taskDescription(task)">{{ taskDescription(task) }}</small></span>
                  <span :class="taskStatusClass(task.status)" class="shrink-0 rounded-full px-2 py-1 text-[10px] font-bold">{{ taskStatusText(task.status) }}</span>
                </div>
              </div>
              <div v-else class="flex flex-1 items-center justify-center text-xs text-slate-400">暂无运行状态</div>
              <RouterLink to="/tasks" class="secondary-button mt-auto w-full justify-center">创建隐私邮箱 <ArrowRight :size="15" /></RouterLink>
            </div>
          </section>
        </aside>
      </div>
    </template>
  </div>
</template>
