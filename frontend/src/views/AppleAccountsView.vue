<script setup>
import { onMounted, reactive, ref } from 'vue'
import { Apple, CheckCircle2, CloudDownload, Eye, EyeOff, KeyRound, LoaderCircle, Mail, MailPlus, Plus, RefreshCw, Server, ShieldCheck, Trash2, X } from '@lucide/vue'
import { api } from '../api/client'
import CardSelect from '../components/CardSelect.vue'
import { useConfirm } from '../composables/useConfirm'
import { useToast } from '../composables/useToast'

const loading = ref(true)
const busy = ref('')
const showLogin = ref(false)
const showIMAP = ref(false)
const showCreate = ref(false)
const showPassword = ref(false)
const showIMAPPassword = ref(false)
const data = ref({ items: [], module_ready: true })
const selected = ref(null)
const pending = reactive({ id: '', code: '', phoneNumber: '' })
const login = reactive({ flow: 'apple_account', apple_id: '', password: '', two_factor_method: 'trusted_device' })
const imap = reactive({ email: '', app_password: '' })
const create = reactive({ label: '', note: '', channel: 'auto' })
const { success, error: showError } = useToast()
const { confirm: confirmAction } = useConfirm()
const loginFlowOptions = [
  { value: 'apple_account', label: 'Apple Account 新接口', dot: 'bg-violet-500' },
  { value: 'icloud_web', label: 'iCloud Web 旧接口', dot: 'bg-sky-500' },
]
const twoFactorOptions = [
  { value: 'trusted_device', label: '受信任设备', dot: 'bg-emerald-500' },
  { value: 'sms', label: '短信', dot: 'bg-amber-500' },
]
const createChannelOptions = [
  { value: 'auto', label: '自动接口：新接口优先，失败用旧接口', dot: 'bg-slate-400' },
  ...loginFlowOptions,
]

function flash(text, isError = false) {
  if (isError) showError(text)
  else success(text)
}

function statusLabel(value) {
  return ({ active: '正常', need_login: '需要登录', need_2fa: '等待 2FA', no_icloud_plus: '无 iCloud+', rate_limited: '访问受限', failed: '失败' })[value] || value || '未知'
}

function stateLabel(kind) {
  return ({ apple_account: 'Apple Account 新接口', icloud_web: 'iCloud Web 旧接口', icloud_imap: 'iCloud IMAP' })[kind] || kind
}

function stateMeta(kind) {
  return ({
    apple_account: { description: '创建隐私邮箱', icon: Apple, tone: 'text-violet-600 bg-violet-50 dark:bg-violet-950/30 dark:text-violet-300' },
    icloud_web: { description: '同步与远端管理', icon: CloudDownload, tone: 'text-sky-600 bg-sky-50 dark:bg-sky-950/30 dark:text-sky-300' },
    icloud_imap: { description: '邮件与验证码', icon: Mail, tone: 'text-amber-600 bg-amber-50 dark:bg-amber-950/30 dark:text-amber-300' },
  })[kind] || { description: '登录态', icon: Server, tone: 'text-slate-500 bg-slate-100 dark:bg-slate-700 dark:text-slate-300' }
}

function stateStatusLabel(state) {
  if (!state.saved) return '未配置'
  if (!state.last_checked_at || String(state.last_checked_at).startsWith('0001-')) return '已保存'
  return state.last_check_ok ? '正常' : '需检查'
}

function stateStatusClass(state) {
  if (!state.saved) return 'bg-slate-100 text-slate-400 dark:bg-slate-700 dark:text-slate-400'
  if (!state.last_checked_at || String(state.last_checked_at).startsWith('0001-')) return 'bg-amber-50 text-amber-600 dark:bg-amber-950/30 dark:text-amber-300'
  return state.last_check_ok ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-950/30 dark:text-emerald-300' : 'bg-rose-50 text-rose-600 dark:bg-rose-950/30 dark:text-rose-300'
}

function accountStatusClass(account) {
  const status = account.icloud_status || account.status
  if (status === 'active') return 'bg-emerald-50 text-emerald-600 dark:bg-emerald-950/30 dark:text-emerald-300'
  if (status === 'need_2fa' || status === 'need_login') return 'bg-amber-50 text-amber-600 dark:bg-amber-950/30 dark:text-amber-300'
  return 'bg-rose-50 text-rose-600 dark:bg-rose-950/30 dark:text-rose-300'
}

function openLoginDialog() {
  showIMAP.value = false
  showCreate.value = false
  showPassword.value = false
  showLogin.value = true
}

function closeLoginDialog(force = false) {
  if (!force && (busy.value === 'login' || busy.value === '2fa')) return
  showLogin.value = false
  showPassword.value = false
  login.password = ''
  pending.id = ''
  pending.code = ''
  pending.phoneNumber = ''
}

function openIMAPDialog() {
  if (!selected.value) {
    flash('请先选择一个 Apple 账号', true)
    return
  }
  showLogin.value = false
  showCreate.value = false
  showIMAPPassword.value = false
  imap.email = selected.value.apple_id || imap.email
  showIMAP.value = true
}

function openCreateDialog() {
  if (!selected.value) {
    flash('请先选择一个 Apple 账号', true)
    return
  }
  showLogin.value = false
  showIMAP.value = false
  showCreate.value = true
}

async function load() {
  loading.value = true
  try {
    data.value = await api('/api/apple-accounts')
    if (selected.value) {
      const current = data.value.items.find((item) => item.id === selected.value.id)
      if (current) selected.value = current
      else selected.value = null
    }
  } catch (err) {
    flash(err.message, true)
  } finally {
    loading.value = false
  }
}

async function deleteAccount(account) {
  const name = account.label || account.apple_id || account.id
  const confirmed = await confirmAction({
    title: '删除 Apple 账号',
    message: `确定删除“${name}”吗？\n\n本地登录态、关联隐私邮箱和本地邮件会一并删除；Apple 服务器上的隐私邮箱不会删除。`,
    confirmText: '删除账号',
    tone: 'danger',
  })
  if (!confirmed) return
  busy.value = `delete:${account.id}`
  try {
    const result = await api(`/api/apple-accounts/${account.id}`, { method: 'DELETE' })
    if (selected.value?.id === account.id) selected.value = null
    await load()
    const deleted = result.deleted || {}
    flash(`已删除 Apple 账号：${name}；清理邮箱 ${deleted.mailboxes || 0} 个，邮件 ${deleted.messages || 0} 封`)
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function selectAccount(account) {
  busy.value = `detail:${account.id}`
  try {
    const result = await api(`/api/apple-accounts/${account.id}`)
    selected.value = result.account
    imap.email = result.account.apple_id || ''
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function startLogin() {
  busy.value = 'login'
  flash('正在与 Apple 建立登录态，请稍候…')
  try {
    const result = await api('/api/apple-accounts/login/start', { method: 'POST', body: JSON.stringify(login) })
    if (result.needs_2fa) {
      pending.id = result.pending_id
      flash(result.message || '请输入 Apple 两步验证码')
    } else {
      closeLoginDialog(true)
      flash(result.message || 'Apple 登录已完成')
      await load()
    }
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function submit2FA() {
  busy.value = '2fa'
  try {
    const body = { pending_id: pending.id, code: pending.code }
    if (pending.phoneNumber.trim()) {
      try { body.phone_number = JSON.parse(pending.phoneNumber) } catch { body.phone_number = pending.phoneNumber.trim() }
    }
    const result = await api('/api/apple-accounts/login/2fa', { method: 'POST', body: JSON.stringify(body) })
    closeLoginDialog(true)
    flash(result.message || 'Apple 登录和 2FA 已完成')
    await load()
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function checkAccount() {
  if (!selected.value) return
  busy.value = 'check'
  try {
    const result = await api(`/api/apple-accounts/${selected.value.id}/check`, { method: 'POST' })
    selected.value = result.account
    flash('登录态检测完成')
    await load()
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function saveIMAP() {
  if (!selected.value) return
  busy.value = 'imap'
  try {
    const result = await api(`/api/apple-accounts/${selected.value.id}/imap`, { method: 'POST', body: JSON.stringify(imap) })
    selected.value = result.account
    imap.app_password = ''
    showIMAPPassword.value = false
    showIMAP.value = false
    flash('IMAP App 专用密码已验证并保存')
    await load()
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function createMailbox() {
  if (!selected.value) return
  busy.value = 'create'
  try {
    const result = await api(`/api/apple-accounts/${selected.value.id}/mailboxes`, { method: 'POST', body: JSON.stringify(create) })
    create.label = ''
    create.note = ''
    showCreate.value = false
    flash(`已创建隐私邮箱：${result.mailbox.email}`)
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function syncMailboxes() {
  if (!selected.value) return
  busy.value = 'sync'
  try {
    const result = await api(`/api/apple-accounts/${selected.value.id}/mailboxes/sync`, { method: 'POST' })
    showCreate.value = false
    flash(`已从 Apple 同步 ${result.count} 个隐私邮箱`)
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

onMounted(load)
</script>

<template>
  <div class="mx-auto flex max-w-7xl flex-col gap-5">
    <section class="panel flex flex-col gap-4 p-5 lg:flex-row lg:items-center lg:justify-between">
      <div class="flex min-w-0 flex-1 items-start gap-4">
        <div class="rounded-xl bg-slate-100 p-3 text-slate-700 dark:bg-slate-700 dark:text-slate-200"><Apple :size="22" /></div>
        <div class="min-w-0"><h2 class="font-black">Apple 账号工作区</h2><p class="mt-1 max-w-2xl text-sm leading-6 text-slate-500 dark:text-slate-400">集中管理 Apple Account 新接口、iCloud Web 旧接口和 IMAP 取码登录态。</p></div>
      </div>
      <div class="flex flex-wrap gap-2 lg:shrink-0 lg:flex-nowrap lg:justify-end">
        <button class="primary-button whitespace-nowrap" @click="openLoginDialog"><Plus :size="17" />添加 Apple 账号</button>
        <button class="secondary-button whitespace-nowrap" :disabled="!selected" :title="selected ? `为 ${selected.apple_id} 配置 IMAP` : '请先选择一个 Apple 账号'" @click="openIMAPDialog"><KeyRound :size="17" />IMAP 取码</button>
        <button class="secondary-button whitespace-nowrap" :disabled="!selected" :title="selected ? `使用 ${selected.apple_id} 创建隐私邮箱` : '请先选择一个 Apple 账号'" @click="openCreateDialog"><MailPlus :size="17" />创建隐私邮箱</button>
      </div>
    </section>

    <div v-if="loading" class="flex min-h-64 items-center justify-center text-slate-400"><LoaderCircle :size="24" class="animate-spin" /></div>
    <div v-else class="grid items-stretch gap-5 xl:grid-cols-[minmax(0,1fr)_400px]">
      <section class="panel flex min-h-[520px] flex-col overflow-hidden">
        <div class="border-b border-slate-100 px-5 py-4 dark:border-slate-700"><h3 class="font-bold">账号与登录态</h3><p class="mt-1 text-xs text-slate-400">共 {{ data.items?.length || 0 }} 个账号</p></div>
        <div v-if="!data.items?.length" class="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center text-slate-400"><Server :size="34" /><div class="font-bold text-slate-600 dark:text-slate-300">还没有 Apple 账号</div><div class="text-sm">点击“添加 Apple 账号”完成首次协议登录。</div></div>
        <div v-else class="flex-1 space-y-2 p-3">
          <div v-for="account in data.items" :key="account.id" class="group relative rounded-xl border border-slate-200 transition hover:border-emerald-300 hover:bg-slate-50 dark:border-slate-700 dark:hover:border-emerald-700 dark:hover:bg-slate-700/30" :class="selected?.id === account.id ? 'border-emerald-500 bg-emerald-100 ring-1 ring-inset ring-emerald-300 shadow-sm dark:border-emerald-400 dark:bg-emerald-900/60 dark:ring-emerald-600' : ''">
            <button type="button" class="flex w-full items-start gap-3 rounded-xl px-4 py-3 pr-14 text-left" :disabled="busy === `delete:${account.id}`" @click="selectAccount(account)">
              <div class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-200"><Apple :size="18" /></div>
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2"><div class="min-w-0 truncate font-bold text-slate-800 dark:text-slate-100">{{ account.label || account.apple_id || 'Apple 账号' }}</div><span :class="accountStatusClass(account)" class="shrink-0 rounded-full px-2 py-1 text-[10px] font-bold">{{ statusLabel(account.icloud_status || account.status) }}</span></div>
                <div class="mt-1 truncate text-xs text-slate-400">{{ account.apple_id || account.id }}</div>
                <div class="mt-2 flex flex-wrap gap-1.5">
                  <span v-for="state in account.login_states" :key="state.kind" class="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2 py-1 text-[10px] font-semibold text-slate-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300">
                    <span :class="stateMeta(state.kind).tone" class="flex h-5 w-5 items-center justify-center rounded-md"><component :is="stateMeta(state.kind).icon" :size="11" /></span>
                    <span>{{ stateLabel(state.kind) }}</span>
                    <span :class="stateStatusClass(state)" class="rounded-full px-1.5 py-0.5 text-[9px] font-bold">{{ stateStatusLabel(state) }}</span>
                  </span>
                  <span v-if="!account.login_states?.length" class="text-[10px] text-slate-400">暂无登录态</span>
                </div>
              </div>
            </button>
            <button type="button" class="absolute right-3 top-3 flex h-8 w-8 items-center justify-center rounded-lg border border-rose-200 bg-white/90 text-rose-500 shadow-sm transition hover:border-rose-300 hover:bg-rose-50 hover:text-rose-600 disabled:opacity-50 dark:border-rose-900/70 dark:bg-slate-800/90 dark:text-rose-300 dark:hover:bg-rose-950/60" :title="`删除 ${account.label || account.apple_id || 'Apple 账号'}`" :aria-label="`删除 ${account.label || account.apple_id || 'Apple 账号'}`" :disabled="busy === `delete:${account.id}`" @click.stop="deleteAccount(account)"><LoaderCircle v-if="busy === `delete:${account.id}`" :size="15" class="animate-spin" /><Trash2 v-else :size="15" /></button>
          </div>
        </div>
      </section>

      <aside class="h-full min-h-[520px]">
        <section v-if="!selected" class="panel flex h-full min-h-[520px] flex-col items-center justify-center gap-3 p-6 text-center text-slate-400"><Apple :size="34" /><div class="text-sm">选择一个账号查看和操作登录态</div></section>
        <section v-else class="panel flex h-full min-h-[520px] flex-col p-5">
          <div class="flex items-start justify-between gap-3"><div class="min-w-0"><div class="section-title">所选 Apple 账号</div><h3 class="truncate font-black">{{ selected.label || selected.apple_id }}</h3><p class="mt-1 truncate text-xs text-slate-400">{{ selected.apple_id }}</p></div><button class="secondary-button min-w-[92px] px-3 py-2" :disabled="busy === 'check'" @click="checkAccount"><LoaderCircle v-if="busy === 'check'" :size="15" class="animate-spin" /><RefreshCw v-else :size="15" />{{ busy === 'check' ? '检查中' : '检测' }}</button></div>
          <div class="mt-5 flex-1 space-y-2">
            <div v-for="state in selected.login_states" :key="state.kind" class="rounded-2xl border border-slate-100 bg-slate-50 p-3.5 dark:border-slate-700 dark:bg-slate-900/40"><div class="flex items-center gap-3"><div :class="stateMeta(state.kind).tone" class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl"><component :is="stateMeta(state.kind).icon" :size="17" /></div><div class="min-w-0 flex-1"><div class="truncate text-xs font-bold">{{ stateLabel(state.kind) }}</div><p class="mt-1 truncate text-[11px] text-slate-400">{{ stateMeta(state.kind).description }}</p></div><span v-if="busy === 'check'" class="inline-flex shrink-0 items-center gap-1 rounded-full bg-sky-50 px-2.5 py-1 text-[10px] font-bold text-sky-600 dark:bg-sky-950/30 dark:text-sky-300"><LoaderCircle :size="11" class="animate-spin" />检查中</span><span v-else :class="stateStatusClass(state)" class="shrink-0 rounded-full px-2.5 py-1 text-[10px] font-bold">{{ stateStatusLabel(state) }}</span></div><p class="mt-3 border-t border-slate-200/70 pt-2 text-xs leading-5 text-slate-400 dark:border-slate-700">{{ busy === 'check' ? `正在检查 ${stateLabel(state.kind)} 登录态…` : (state.last_status_message || '尚未检测') }}</p></div>
            <div v-if="!selected.login_states?.length" class="rounded-xl bg-slate-50 p-4 text-center text-xs text-slate-400 dark:bg-slate-900/40">该账号还没有已保存的登录态</div>
          </div>
        </section>
      </aside>
    </div>

    <div v-if="showLogin" class="fixed inset-0 z-50 !m-0 flex items-center justify-center bg-slate-950/45 p-4 backdrop-blur-[2px]" role="presentation" @click.stop>
      <section role="dialog" aria-modal="true" aria-labelledby="apple-login-title" class="panel max-h-[calc(100vh-2rem)] w-full max-w-3xl overflow-y-auto p-5 shadow-2xl sm:p-6">
        <div class="mb-5 flex items-start justify-between gap-4"><div><h3 id="apple-login-title" class="font-black">登录 Apple 账号</h3><p class="mt-1 text-xs leading-5 text-slate-400">登录凭据仅用于本次 Apple 协议请求；保存的是本地登录态。</p></div><button class="icon-button" title="关闭" :disabled="busy === 'login' || busy === '2fa'" @click="closeLoginDialog()"><X :size="18" /></button></div>
        <form v-if="!pending.id" class="grid gap-4 md:grid-cols-2" @submit.prevent="startLogin">
          <div class="form-group"><span class="form-label">登录通道</span><CardSelect v-model="login.flow" :options="loginFlowOptions" aria-label="登录通道" /><span class="form-help">新接口用于创建；旧接口支持同步、删除和 Web 收信。</span></div>
          <div class="form-group"><span class="form-label">两步验证方式</span><CardSelect v-model="login.two_factor_method" :options="twoFactorOptions" aria-label="两步验证方式" /><span class="form-help">优先使用受信任设备弹出的验证码。</span></div>
          <label class="form-group"><span class="form-label">Apple ID</span><span class="field-wrap"><Mail :size="17" class="field-icon" /><input v-model.trim="login.apple_id" class="field field-leading" autocomplete="username" placeholder="name@example.com" required /></span></label>
          <label class="form-group"><span class="form-label">Apple ID 密码</span><span class="field-wrap"><KeyRound :size="17" class="field-icon" /><input v-model="login.password" class="field field-leading field-trailing" :type="showPassword ? 'text' : 'password'" autocomplete="current-password" placeholder="输入 Apple ID 密码" required /><button type="button" class="absolute right-3 top-1/2 z-10 -translate-y-1/2 rounded-lg p-1 text-slate-400 transition hover:text-slate-600 dark:hover:text-slate-200" :title="showPassword ? '隐藏密码' : '显示密码'" @click="showPassword = !showPassword"><EyeOff v-if="showPassword" :size="17" /><Eye v-else :size="17" /></button></span></label>
          <div class="md:col-span-2 flex justify-end"><button class="primary-button" :disabled="busy === 'login'"><LoaderCircle v-if="busy === 'login'" :size="17" class="animate-spin" /><ShieldCheck v-else :size="17" />开始登录</button></div>
        </form>
        <form v-else class="grid gap-4 md:grid-cols-2" @submit.prevent="submit2FA">
          <label class="form-group"><span class="form-label">Apple 验证码</span><input v-model.trim="pending.code" class="field font-mono tracking-[0.3em]" inputmode="numeric" maxlength="8" placeholder="000000" required /><span class="form-help">输入受信任设备或短信收到的验证码。</span></label>
          <label class="form-group"><span class="form-label">短信号码参数（可选）</span><input v-model.trim="pending.phoneNumber" class="field" placeholder='例如 {"id":1}' /><span class="form-help">只有短信流程要求选择号码时才填写。</span></label>
          <div class="md:col-span-2 flex justify-end"><button class="primary-button" :disabled="busy === '2fa'"><LoaderCircle v-if="busy === '2fa'" :size="17" class="animate-spin" /><CheckCircle2 v-else :size="17" />提交验证码</button></div>
        </form>
      </section>
    </div>

    <div v-if="showIMAP" class="fixed inset-0 z-50 !m-0 flex items-center justify-center bg-slate-950/45 p-4 backdrop-blur-[2px]" role="presentation" @click.stop>
      <section role="dialog" aria-modal="true" aria-labelledby="imap-title" class="panel max-h-[calc(100vh-2rem)] w-full max-w-lg overflow-y-auto p-5 shadow-2xl sm:p-6">
        <div class="mb-5 flex items-start justify-between gap-4"><div><h3 id="imap-title" class="flex items-center gap-2 font-black"><KeyRound :size="18" />IMAP 取码</h3><p class="mt-1 text-xs leading-5 text-slate-400">当前账号：{{ selected?.label || selected?.apple_id }}</p></div><button class="icon-button" title="关闭" :disabled="busy === 'imap'" @click="showIMAP = false"><X :size="18" /></button></div>
        <form class="space-y-4" @submit.prevent="saveIMAP"><label class="form-group"><span class="form-label">iCloud 邮箱</span><input v-model.trim="imap.email" class="field" type="email" placeholder="name@icloud.com" required /></label><label class="form-group"><span class="form-label">App 专用密码</span><span class="field-wrap"><KeyRound :size="17" class="field-icon" /><input v-model="imap.app_password" class="field field-leading field-trailing" :type="showIMAPPassword ? 'text' : 'password'" placeholder="xxxx-xxxx-xxxx-xxxx" required /><button type="button" class="absolute right-3 top-1/2 z-10 -translate-y-1/2 rounded-lg p-1 text-slate-400 transition hover:text-slate-600 dark:hover:text-slate-200" :title="showIMAPPassword ? '隐藏 App 专用密码' : '显示 App 专用密码'" @click="showIMAPPassword = !showIMAPPassword"><EyeOff v-if="showIMAPPassword" :size="17" /><Eye v-else :size="17" /></button></span><span class="form-help">保存前会连接 imap.mail.me.com 验证。</span></label><button class="secondary-button w-full" :disabled="busy === 'imap'"><LoaderCircle v-if="busy === 'imap'" :size="16" class="animate-spin" /><ShieldCheck v-else :size="16" />验证并保存</button></form>
      </section>
    </div>

    <div v-if="showCreate" class="fixed inset-0 z-50 !m-0 flex items-center justify-center bg-slate-950/45 p-4 backdrop-blur-[2px]" role="presentation" @click.stop>
      <section role="dialog" aria-modal="true" aria-labelledby="create-mailbox-title" class="panel max-h-[calc(100vh-2rem)] w-full max-w-lg overflow-y-auto p-5 shadow-2xl sm:p-6">
        <div class="mb-5 flex items-start justify-between gap-4"><div><h3 id="create-mailbox-title" class="flex items-center gap-2 font-black"><MailPlus :size="18" />创建隐私邮箱</h3><p class="mt-1 text-xs leading-5 text-slate-400">当前账号：{{ selected?.label || selected?.apple_id }}</p></div><button class="icon-button" title="关闭" :disabled="busy === 'create' || busy === 'sync'" @click="showCreate = false"><X :size="18" /></button></div>
        <form class="space-y-4" @submit.prevent="createMailbox"><div class="form-group"><span class="form-label">创建通道</span><CardSelect v-model="create.channel" :options="createChannelOptions" aria-label="创建通道" /></div><label class="form-group"><span class="form-label">标签前缀</span><input v-model.trim="create.label" class="field" placeholder="可选" /><span class="form-help">留空默认使用 x，并根据已有最大编号生成 x_1、x_2、x_3。</span></label><label class="form-group"><span class="form-label">备注</span><textarea v-model.trim="create.note" class="field min-h-20 resize-y" placeholder="可选备注" /></label><div class="grid grid-cols-2 gap-2"><button class="primary-button" :disabled="busy === 'create'"><LoaderCircle v-if="busy === 'create'" :size="16" class="animate-spin" /><Plus v-else :size="16" />创建</button><button class="secondary-button" type="button" :disabled="busy === 'sync'" @click="syncMailboxes"><CloudDownload :size="16" :class="busy === 'sync' ? 'animate-pulse' : ''" />同步已有</button></div></form>
      </section>
    </div>
  </div>
</template>
