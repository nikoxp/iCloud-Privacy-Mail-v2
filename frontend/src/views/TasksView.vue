<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { CheckCircle2, CircleAlert, Clock3, LoaderCircle, Play, Save, Settings, Square, Trash2, X } from '@lucide/vue'
import { api } from '../api/client'
import CardSelect from '../components/CardSelect.vue'
import { useConfirm } from '../composables/useConfirm'
import { useToast } from '../composables/useToast'

const loading = ref(true)
const busy = ref('')
const initialized = ref(false)
const showDefaults = ref(false)
const scheduler = ref({ running: false, status: 'idle', events: [] })
const accounts = ref([])
const lastCreatedMailbox = ref(null)
const form = reactive({ mode: 'once', account_id: '', account_ids: [], label: '', note: '', create_channel: 'auto', interval_minutes: 60, round_interval_seconds: 5 })
const defaultForm = reactive({ label: '', note: '', account_ids: [], create_channel: 'auto', scheduler_create_channel: 'auto', apple_account_two_factor_method: 'trusted_device', icloud_web_two_factor_method: 'trusted_device', scheduler_interval_minutes: 60, scheduler_round_interval_seconds: 5, mailbox_page_size: 20 })
const { success, error: showError } = useToast()
const { confirm: confirmAction } = useConfirm()

const accountOptions = computed(() => accounts.value.map((account) => ({ value: account.id, label: `${account.label || account.apple_id}（${account.apple_id}）` })))
const schedulerEvents = computed(() => [...(scheduler.value.events || [])].reverse())
const displayedStatus = computed(() => {
  if (scheduler.value.running) return scheduler.value.status
  if (busy.value === 'create-one') return 'creating'
  return 'idle'
})
const selectedAccountCount = computed(() => {
  if (!scheduler.value.running && form.mode === 'once') return form.account_id ? 1 : 0
  const ids = scheduler.value.running ? scheduler.value.account_ids : form.account_ids
  return Array.isArray(ids) ? ids.length : 0
})
const intervalSummary = computed(() => {
  const seconds = scheduler.value.running ? scheduler.value.interval_seconds : Number(form.interval_minutes || 0) * 60
  if (!seconds) return '-'
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`
  if (seconds % 60 === 0) return `${seconds / 60} 分钟`
  return `${seconds} 秒`
})
const nextRunSummary = computed(() => {
  if (!scheduler.value.running && form.mode === 'once') return '点击后立即创建'
  if (!scheduler.value.running) return '未安排'
  if (scheduler.value.next_run_at) return formatTime(scheduler.value.next_run_at)
  if (scheduler.value.status === 'creating') return '本轮执行中'
  return '准备执行'
})

const createChannelOptions = [
  { value: 'auto', label: '自动接口：新接口优先，失败用旧接口', dot: 'bg-slate-400' },
  { value: 'apple_account', label: 'Apple Account 新接口', dot: 'bg-violet-500' },
  { value: 'icloud_web', label: 'iCloud Web 旧接口', dot: 'bg-sky-500' },
]
const modeOptions = [
  { value: 'once', label: '创建一个', description: '立即为一个账号创建邮箱', dot: 'bg-sky-500' },
  { value: 'scheduled', label: '自动创建', description: '按设定间隔持续创建', dot: 'bg-emerald-500' },
]

function statusText(status) {
  return ({ ready: '准备创建', running: '运行中', creating: '创建中', waiting: '等待下一轮', stopped: '已停止', idle: '未启动' })[status] || status
}

function statusClass(status) {
  if (status === 'running' || status === 'creating') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-200'
  if (status === 'waiting' || status === 'ready') return 'bg-sky-100 text-sky-700 dark:bg-sky-950/60 dark:text-sky-200'
  return 'bg-slate-100 text-slate-500 dark:bg-slate-700 dark:text-slate-300'
}

function channelText(channel) {
  return ({ auto: '自动接口：新接口优先，失败用旧接口', apple_account: 'Apple Account 新接口', icloud_web: 'iCloud Web 旧接口' })[channel] || channel || '-'
}

function modeText(mode) {
  return mode === 'scheduled' ? '自动创建' : '创建一个'
}

function syncModeDefaults(mode) {
  if (mode === 'once' && !form.account_id) form.account_id = form.account_ids[0] || ''
  if (mode === 'scheduled' && !form.account_ids.length && form.account_id) form.account_ids = [form.account_id]
  form.create_channel = mode === 'scheduled'
    ? (defaultForm.scheduler_create_channel || 'auto')
    : (defaultForm.create_channel || 'auto')
}

function applyDefaults() {
  const validAccountIDs = (defaultForm.account_ids || []).filter((id) => accounts.value.some((account) => account.id === id))
  const firstAccountID = validAccountIDs[0] || accounts.value[0]?.id || ''
  form.account_id = firstAccountID
  form.account_ids = firstAccountID ? [firstAccountID] : []
  form.label = defaultForm.label || ''
  form.note = defaultForm.note || ''
  form.create_channel = form.mode === 'scheduled'
    ? (defaultForm.scheduler_create_channel || 'auto')
    : (defaultForm.create_channel || 'auto')
  form.interval_minutes = defaultForm.scheduler_interval_minutes || 60
  form.round_interval_seconds = defaultForm.scheduler_round_interval_seconds || 5
}

function assignDefaultSettings(settings = {}) {
  const displaySettings = { ...settings }
  if (String(displaySettings.label || '').trim().toLowerCase() === 'x') displaySettings.label = ''
  Object.assign(defaultForm, {
    label: '',
    note: '',
    account_ids: [],
    create_channel: 'auto',
    scheduler_create_channel: 'auto',
    apple_account_two_factor_method: 'trusted_device',
    icloud_web_two_factor_method: 'trusted_device',
    scheduler_interval_minutes: 60,
    scheduler_round_interval_seconds: 5,
    mailbox_page_size: 20,
  }, displaySettings)
}

function eventTypeText(type) {
  return ({ started: '启动', stopped: '停止', round_started: '新一轮', created: '已创建', failed: '失败', waiting: '等待' })[type] || type
}

function eventTone(type) {
  if (type === 'failed') return 'bg-rose-100 text-rose-600 dark:bg-rose-950/60 dark:text-rose-300'
  if (type === 'created') return 'bg-emerald-100 text-emerald-600 dark:bg-emerald-950/60 dark:text-emerald-300'
  return 'bg-sky-100 text-sky-600 dark:bg-sky-950/60 dark:text-sky-300'
}

function formatTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getFullYear() <= 1) return '-'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(date)
}

function notify(text, isError = false) {
  if (isError) showError(text)
  else success(text)
}

async function load() {
  loading.value = true
  try {
    const [schedulerData, accountData, createData] = await Promise.all([api('/api/scheduler/status'), api('/api/apple-accounts'), api('/api/create-settings')])
    scheduler.value = schedulerData.scheduler || scheduler.value
    accounts.value = accountData.items || []
    assignDefaultSettings(createData.settings)
    if (!defaultForm.account_ids.length && accounts.value.length) defaultForm.account_ids = [accounts.value[0].id]
    if (!initialized.value) {
      applyDefaults()
      initialized.value = true
    }
  } catch (err) {
    notify(err.message, true)
  } finally {
    loading.value = false
  }
}

async function saveDefaults() {
  busy.value = 'save-defaults'
  try {
    const data = await api('/api/create-settings', { method: 'PUT', body: JSON.stringify(defaultForm) })
    assignDefaultSettings(data.settings)
    applyDefaults()
    showDefaults.value = false
    notify('创建与调度默认设置已保存')
  } catch (err) {
    notify(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function start() {
  busy.value = 'start'
  try {
    const data = await api('/api/scheduler/start', { method: 'POST', body: JSON.stringify({ account_ids: form.account_ids, label: form.label, note: form.note, create_channel: form.create_channel, interval_minutes: form.interval_minutes, round_interval_seconds: form.round_interval_seconds }) })
    scheduler.value = data.scheduler || data.data?.scheduler || scheduler.value
    notify('定时创建已启动')
    await load()
  } catch (err) {
    notify(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function createOne() {
  if (!form.account_id) return
  busy.value = 'create-one'
  try {
    const result = await api(`/api/apple-accounts/${form.account_id}/mailboxes`, { method: 'POST', body: JSON.stringify({ label: form.label, note: form.note, channel: form.create_channel }) })
    lastCreatedMailbox.value = result.mailbox
    scheduler.value = result.scheduler || scheduler.value
    form.label = defaultForm.label || ''
    form.note = defaultForm.note || ''
    notify(`已创建隐私邮箱：${result.mailbox.email}`)
  } catch (err) {
    notify(err.message, true)
    try {
      const status = await api('/api/scheduler/status')
      scheduler.value = status.scheduler || scheduler.value
    } catch {
      // 保留原始创建错误提示，状态刷新失败时不重复提示。
    }
  } finally {
    busy.value = ''
  }
}

async function stop() {
  busy.value = 'stop'
  try {
    const data = await api('/api/scheduler/stop', { method: 'POST', body: '{}' })
    scheduler.value = data.scheduler || data.data?.scheduler || scheduler.value
    notify('定时创建已停止')
    await load()
  } catch (err) {
    notify(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function clearLogs() {
  const confirmed = await confirmAction({
    title: '清除调度日志',
    message: '确定清除当前调度任务的所有运行日志吗？',
    confirmText: '确认清除',
    tone: 'danger',
  })
  if (!confirmed) return
  busy.value = 'clear'
  try {
    await api('/api/scheduler/logs/clear', { method: 'POST', body: '{}' })
    scheduler.value.events = []
    notify('调度日志已清除')
  } catch (err) {
    notify(err.message, true)
  } finally {
    busy.value = ''
  }
}

onMounted(load)
</script>

<template>
  <div class="mx-auto max-w-7xl space-y-5">
    <div v-if="loading" class="flex min-h-64 items-center justify-center text-slate-400"><LoaderCircle :size="24" class="animate-spin" /></div>
    <template v-else>
      <div class="grid items-stretch gap-5 xl:grid-cols-[minmax(0,1fr)_330px]">
        <section class="panel p-5 sm:p-6">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <h2 class="text-lg font-black">创建隐私邮箱</h2>
            <div class="flex shrink-0 items-center gap-2 self-start"><span :class="statusClass(displayedStatus)" class="rounded-full px-3 py-1.5 text-xs font-bold">{{ statusText(displayedStatus) }}</span><button type="button" class="icon-button h-8 w-8 rounded-lg" title="创建与调度设置" aria-label="打开创建与调度设置" @click="showDefaults = true"><Settings :size="16" /></button></div>
          </div>

          <div class="mt-6 grid gap-4 md:grid-cols-2">
            <div class="form-group"><span class="form-label">执行方式</span><CardSelect v-model="form.mode" :options="modeOptions" aria-label="执行方式" @change="syncModeDefaults" /><span class="form-help">创建一个会立即执行；自动创建会持续运行。</span></div>
            <div class="form-group"><span class="form-label">参与 Apple 账号</span><CardSelect v-if="form.mode === 'once'" v-model="form.account_id" :options="accountOptions" placeholder="请选择一个账号" aria-label="参与 Apple 账号" /><CardSelect v-else v-model="form.account_ids" :options="accountOptions" placeholder="请选择参与账号" aria-label="参与 Apple 账号" multiple /><span class="form-help">{{ form.mode === 'once' ? '单次创建只选择一个账号。' : '默认选择一个账号，也可以继续多选。' }}</span></div>
            <div class="form-group"><span class="form-label">创建通道</span><CardSelect v-model="form.create_channel" :options="createChannelOptions" aria-label="创建通道" /><span class="form-help">自动接口会先尝试 Apple Account 新接口，失败后使用 iCloud Web 旧接口。</span></div>
            <label class="form-group"><span class="form-label">邮箱标签前缀</span><input v-model.trim="form.label" class="field" placeholder="可选" /><span class="form-help">留空默认使用 x，并根据已有最大编号生成 x_1、x_2、x_3。</span></label>
            <label class="form-group md:col-span-2"><span class="form-label">备注</span><textarea v-model.trim="form.note" class="field min-h-20 resize-y" placeholder="可选备注" /></label>
          </div>

          <div class="mt-5 flex justify-end gap-2">
            <button v-if="scheduler.running" class="secondary-button" :disabled="busy === 'stop'" @click="stop"><LoaderCircle v-if="busy === 'stop'" :size="16" class="animate-spin" /><Square v-else :size="16" />停止定时创建</button>
            <button v-else-if="form.mode === 'once'" class="primary-button" :disabled="busy === 'create-one' || !form.account_id" @click="createOne"><LoaderCircle v-if="busy === 'create-one'" :size="16" class="animate-spin" /><Play v-else :size="16" />创建一个</button>
            <button v-else class="primary-button" :disabled="busy === 'start' || !form.account_ids.length" @click="start"><LoaderCircle v-if="busy === 'start'" :size="16" class="animate-spin" /><Play v-else :size="16" />启动自动创建</button>
          </div>
        </section>

        <aside class="panel flex h-full flex-col p-5">
          <div class="flex items-center justify-between gap-3"><div class="flex items-center gap-2"><span class="flex h-9 w-9 items-center justify-center rounded-xl bg-sky-100 text-sky-600 dark:bg-sky-950/60 dark:text-sky-300"><Clock3 :size="18" /></span><div><h3 class="font-black">任务概览</h3><p class="mt-0.5 text-[10px] text-slate-400">当前调度运行数据</p></div></div></div>
          <div class="mt-5 grid grid-cols-2 gap-2">
            <div class="rounded-xl bg-slate-50 p-3 dark:bg-slate-800/70"><span class="text-[10px] text-slate-400">参与账号</span><strong class="mt-1 block text-xl">{{ selectedAccountCount }}</strong></div>
            <div class="rounded-xl bg-slate-50 p-3 dark:bg-slate-800/70"><span class="text-[10px] text-slate-400">执行方式</span><strong class="mt-1 block text-base">{{ scheduler.running ? '自动创建' : modeText(form.mode) }}</strong></div>
            <div class="rounded-xl bg-emerald-50 p-3 dark:bg-emerald-950/40"><span class="text-[10px] text-emerald-600 dark:text-emerald-300">创建成功</span><strong class="mt-1 block text-xl text-emerald-700 dark:text-emerald-200">{{ scheduler.success || 0 }}</strong></div>
            <div class="rounded-xl bg-rose-50 p-3 dark:bg-rose-950/40"><span class="text-[10px] text-rose-500 dark:text-rose-300">创建失败</span><strong class="mt-1 block text-xl text-rose-600 dark:text-rose-200">{{ scheduler.failed || 0 }}</strong></div>
          </div>
          <dl class="mt-5 divide-y divide-slate-100 text-xs dark:divide-slate-700/70">
            <div class="flex items-center justify-between gap-3 py-3"><dt class="text-slate-400">创建通道</dt><dd class="font-bold text-slate-700 dark:text-slate-200">{{ channelText(scheduler.running ? scheduler.create_channel : form.create_channel) }}</dd></div>
            <div v-if="scheduler.running || form.mode === 'scheduled'" class="flex items-center justify-between gap-3 py-3"><dt class="text-slate-400">轮次间隔</dt><dd class="font-bold text-slate-700 dark:text-slate-200">{{ intervalSummary }}</dd></div>
            <div class="flex items-center justify-between gap-3 py-3"><dt class="text-slate-400">下次执行</dt><dd class="font-bold text-slate-700 dark:text-slate-200">{{ nextRunSummary }}</dd></div>
            <div class="flex items-center justify-between gap-3 py-3"><dt class="text-slate-400">最近执行</dt><dd class="font-bold text-slate-700 dark:text-slate-200">{{ formatTime(scheduler.last_run_at) }}</dd></div>
          </dl>
          <div v-if="lastCreatedMailbox" class="mt-auto rounded-xl bg-emerald-50 p-3 text-xs leading-5 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-200"><span class="block text-[10px] text-emerald-500">刚刚创建</span><strong class="mt-0.5 block break-all">{{ lastCreatedMailbox.email }}</strong></div>
          <div v-if="scheduler.last_error" class="mt-auto rounded-xl bg-rose-50 p-3 text-xs leading-5 text-rose-600 dark:bg-rose-950/40 dark:text-rose-300">{{ scheduler.last_error }}</div>
        </aside>
      </div>

      <section class="panel overflow-hidden">
        <div class="flex items-center justify-between gap-4 border-b border-slate-100 px-5 py-4 dark:border-slate-700"><div><h3 class="font-black">调度日志</h3><p class="mt-0.5 text-xs text-slate-400">记录启动、轮次、创建结果和等待状态</p></div><div class="flex items-center gap-2"><span class="text-xs text-slate-400">{{ schedulerEvents.length }} 条</span><button class="icon-button" title="清除调度日志" :disabled="busy === 'clear' || !schedulerEvents.length" @click="clearLogs"><LoaderCircle v-if="busy === 'clear'" :size="16" class="animate-spin" /><Trash2 v-else :size="16" /></button></div></div>
        <div v-if="!schedulerEvents.length" class="flex h-32 items-center justify-center text-sm text-slate-400">暂无调度记录</div>
        <div v-else class="h-32 overflow-y-auto [scrollbar-gutter:stable]">
          <div v-for="event in schedulerEvents" :key="event.id" class="flex h-16 items-center gap-3 overflow-hidden border-b border-slate-100 px-5 py-3.5 dark:border-slate-700/70">
            <span :class="eventTone(event.type)" class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"><CircleAlert v-if="event.type === 'failed'" :size="15" /><CheckCircle2 v-else :size="15" /></span>
            <div class="min-w-0 flex-1"><div class="flex min-w-0 items-center gap-2"><span class="shrink-0 rounded-md bg-slate-100 px-2 py-0.5 text-[10px] font-bold text-slate-500 dark:bg-slate-700 dark:text-slate-300">{{ eventTypeText(event.type) }}</span><strong class="truncate text-sm font-semibold text-slate-700 dark:text-slate-200" :title="event.message">{{ event.message }}</strong><span v-if="event.email" class="min-w-0 truncate font-mono text-xs text-emerald-600" :title="event.email">{{ event.email }}</span></div><p v-if="event.error" class="mt-1 truncate text-xs leading-5 text-rose-500" :title="event.error">{{ event.error }}</p></div>
            <time class="shrink-0 text-[10px] text-slate-400">{{ formatTime(event.at) }}</time>
          </div>
        </div>
      </section>

      <Teleport to="body">
        <div v-if="showDefaults" class="fixed inset-0 z-[70] !m-0 flex items-center justify-center bg-slate-950/55 p-4 backdrop-blur-[3px]" role="presentation" @click.stop>
          <form class="panel max-h-[calc(100vh-2rem)] w-full max-w-2xl overflow-y-auto p-5 shadow-2xl sm:p-6" role="dialog" aria-modal="true" aria-labelledby="create-defaults-title" @submit.prevent="saveDefaults">
            <div class="flex items-start justify-between gap-4 border-b border-slate-100 pb-4 dark:border-slate-700">
              <div><h2 id="create-defaults-title" class="flex items-center gap-2 text-lg font-black text-slate-900 dark:text-slate-100"><Settings :size="19" />创建与调度设置</h2><p class="mt-1 text-xs leading-5 text-slate-400">设置创建一个和自动创建的默认参数。</p></div>
              <button type="button" class="icon-button h-8 w-8 rounded-lg" title="关闭设置" aria-label="关闭创建与调度设置" :disabled="busy === 'save-defaults'" @click="showDefaults = false"><X :size="17" /></button>
            </div>

            <div class="mt-5 grid gap-4 sm:grid-cols-2">
              <label class="form-group"><span class="form-label">默认标签前缀</span><input v-model.trim="defaultForm.label" class="field" placeholder="可选" /><span class="form-help">留空默认使用 x，并从现有最大编号继续创建。</span></label>
              <label class="form-group"><span class="form-label">默认备注</span><input v-model.trim="defaultForm.note" class="field" placeholder="可选" /></label>
              <div class="form-group"><span class="form-label">创建一个通道</span><CardSelect v-model="defaultForm.create_channel" :options="createChannelOptions" aria-label="创建一个通道" /></div>
              <div class="form-group"><span class="form-label">自动创建通道</span><CardSelect v-model="defaultForm.scheduler_create_channel" :options="createChannelOptions" aria-label="自动创建通道" /></div>
              <label class="form-group"><span class="form-label">下一轮间隔（分钟）</span><input v-model.number="defaultForm.scheduler_interval_minutes" class="field" type="number" min="1" max="10080" /></label>
              <label class="form-group"><span class="form-label">账号间隔（秒）</span><input v-model.number="defaultForm.scheduler_round_interval_seconds" class="field" type="number" min="1" max="3600" /></label>
            </div>
            <div class="mt-4 form-group"><span class="form-label">默认参与账号</span><CardSelect v-model="defaultForm.account_ids" :options="accountOptions" placeholder="请选择默认参与账号" aria-label="默认参与账号" multiple /><span class="form-help">可以保存多个默认账号；进入页面时先选择第一个，也可以在自动创建中继续多选。</span></div>

            <div class="mt-6 flex justify-end gap-2">
              <button type="button" class="secondary-button" :disabled="busy === 'save-defaults'" @click="showDefaults = false">取消</button>
              <button type="submit" class="primary-button" :disabled="busy === 'save-defaults'"><LoaderCircle v-if="busy === 'save-defaults'" :size="16" class="animate-spin" /><Save v-else :size="16" />保存设置</button>
            </div>
          </form>
        </div>
      </Teleport>
    </template>
  </div>
</template>
