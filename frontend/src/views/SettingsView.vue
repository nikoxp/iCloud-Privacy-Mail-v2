<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Database, ExternalLink, Eye, EyeOff, Globe2, KeyRound, LoaderCircle, Save, ShieldCheck, Sparkles } from '@lucide/vue'
import { api } from '../api/client'
import { useToast } from '../composables/useToast'

const loading = ref(true)
const saving = ref('')
const dataPath = ref('')
const runtime = ref({})
const showPublicAPIKey = ref(false)
const form = reactive({
  mailbox_page_size: 20,
  enable_mail_watcher: false,
  enable_apple_keep_alive: false,
  enable_public_mailbox_api: false,
  enable_public_code_page: false,
  public_api_key: '',
  apple_account_module_ready: true,
})
const { success, error: showError } = useToast()
const publicAPIKeyReady = computed(() => Boolean(String(form.public_api_key || '').trim() || runtime.value.config_api_key_configured))
const publicAPIKeySourceText = computed(() => {
  if (String(form.public_api_key || '').trim()) return '系统设置'
  if (runtime.value.config_api_key_configured) return 'config.json'
  return '尚未设置'
})

function notify(text, isError = false) {
  if (isError) showError(text)
  else success(text)
}

function generatePublicAPIKey() {
  const bytes = new Uint8Array(24)
  window.crypto.getRandomValues(bytes)
  let binary = ''
  for (const value of bytes) binary += String.fromCharCode(value)
  form.public_api_key = `ipm_${window.btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')}`
  showPublicAPIKey.value = true
  notify('公共 API Key 已生成，请保存系统设置')
}

async function load() {
  loading.value = true
  try {
    const settingsData = await api('/api/settings')
    Object.assign(form, settingsData.settings || {})
    dataPath.value = settingsData.data_path || ''
    runtime.value = settingsData.runtime || {}
  } catch (err) {
    notify(err.message, true)
  } finally {
    loading.value = false
  }
}

async function saveSystem() {
  saving.value = 'system'
  try {
    const data = await api('/api/settings', { method: 'PUT', body: JSON.stringify(form) })
    Object.assign(form, data.settings || {})
    runtime.value.api_configured = Boolean(String(form.public_api_key || '').trim() || runtime.value.config_api_key_configured)
    runtime.value.api_key_source = String(form.public_api_key || '').trim() ? 'system_settings' : (runtime.value.config_api_key_configured ? 'config' : '')
    notify('系统设置已保存')
  } catch (err) { notify(err.message, true) } finally { saving.value = '' }
}

onMounted(load)
</script>

<template>
  <div class="mx-auto max-w-5xl space-y-5">
    <div v-if="loading" class="flex min-h-64 items-center justify-center text-slate-400"><LoaderCircle :size="24" class="animate-spin" /></div>
    <template v-else>
      <form class="panel space-y-7 p-5 sm:p-7" @submit.prevent="saveSystem">
        <section><h3 class="section-title flex items-center gap-2"><Database :size="16" />本地数据</h3><div class="grid gap-4 sm:grid-cols-2"><label class="form-group"><span class="form-label">邮箱池每页数量</span><input v-model.number="form.mailbox_page_size" class="field" type="number" min="5" max="200" /><span class="form-help">控制邮箱池列表单页显示数量。</span></label><label class="form-group"><span class="form-label">状态文件</span><input :value="dataPath" class="field font-mono text-xs" readonly /><span class="form-help">账号登录态和 App 专用密码也保存在此本地文件。</span></label></div></section>
        <section><h3 class="section-title flex items-center gap-2"><ShieldCheck :size="16" />后台能力</h3><div class="grid gap-4 sm:grid-cols-2"><label class="toggle-card"><span><strong>IMAP 实时邮件监听</strong><small>使用 IDLE 接收事件，每 {{ runtime.mail_watcher_poll_ms || 3000 }} 毫秒重检分组；首次最多拉取 {{ runtime.mail_watcher_initial_fetch_limit || 20 }} 封</small></span><input v-model="form.enable_mail_watcher" class="detail-switch" type="checkbox" :disabled="!runtime.mail_watcher_available" /></label><label class="toggle-card"><span><strong>Apple 登录态保活</strong><small>基础 {{ Math.round((runtime.apple_keep_alive_ms || 240000) / 60000) }} 分钟；每轮随机 ±{{ runtime.apple_keep_alive_jitter_percent ?? 15 }}%</small></span><input v-model="form.enable_apple_keep_alive" class="detail-switch" type="checkbox" :disabled="!runtime.apple_keep_alive_available" /></label></div></section>
        <section>
          <h3 class="section-title flex items-center gap-2"><Globe2 :size="16" />公共访问</h3>
          <div class="grid gap-4 sm:grid-cols-2">
            <label class="toggle-card">
              <span><strong class="flex items-center gap-2"><Globe2 :size="15" />公共取号 API</strong><small>开放取号、批量查询和带密钥的邮箱取码接口；请求仍需 API Key。</small></span>
              <input v-model="form.enable_public_mailbox_api" class="detail-switch" type="checkbox" />
            </label>
            <label class="toggle-card">
              <span><strong class="flex items-center gap-2"><KeyRound :size="15" />公共验证码页面</strong><small>允许外部用户在独立页面输入邮箱并获取验证码，不显示后台入口。</small></span>
              <input v-model="form.enable_public_code_page" class="detail-switch" type="checkbox" />
            </label>
          </div>
          <div class="mt-4 rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-700 dark:bg-slate-800">
            <div class="flex items-start justify-between gap-3"><div><h4 class="flex items-center gap-2 text-sm font-black"><KeyRound :size="15" class="text-emerald-500" />公共取号 API Key</h4><p class="mt-1 text-xs leading-5 text-slate-400">外部调用取号、批量查询接口时使用；来源：{{ publicAPIKeySourceText }}。</p></div><span :class="publicAPIKeyReady ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300' : 'bg-amber-100 text-amber-700 dark:bg-amber-950/60 dark:text-amber-300'" class="shrink-0 rounded-full px-2.5 py-1 text-[10px] font-bold">{{ publicAPIKeyReady ? '已配置' : '待设置' }}</span></div>
            <div class="mt-3 grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
              <span class="field-wrap"><KeyRound :size="17" class="field-icon" /><input v-model.trim="form.public_api_key" class="field field-leading field-trailing font-mono text-xs" :type="showPublicAPIKey ? 'text' : 'password'" autocomplete="new-password" :placeholder="runtime.config_api_key_configured ? '留空继续使用 config.json 中的 api_key' : '输入或点击右侧按钮生成'" /><button type="button" class="absolute right-3 top-1/2 z-10 -translate-y-1/2 rounded-lg p-1 text-slate-400 transition hover:text-slate-600 dark:hover:text-slate-200" :title="showPublicAPIKey ? '隐藏公共 API Key' : '显示公共 API Key'" @click="showPublicAPIKey = !showPublicAPIKey"><EyeOff v-if="showPublicAPIKey" :size="17" /><Eye v-else :size="17" /></button></span>
              <button type="button" class="secondary-button whitespace-nowrap" @click="generatePublicAPIKey"><Sparkles :size="16" />生成新 Key</button>
            </div>
            <p class="mt-2 text-[11px] leading-5 text-slate-400">生成或修改后点击“保存系统设置”立即生效；公共验证码页面不使用这个 Key。</p>
          </div>
          <div class="mt-3 grid overflow-hidden rounded-xl border border-slate-200 bg-slate-50 text-xs text-slate-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300 sm:grid-cols-2">
            <div class="flex min-w-0 items-center gap-2 border-b border-slate-200 px-4 py-3 dark:border-slate-700 sm:border-b-0 sm:border-r"><span class="rounded-md bg-violet-100 px-1.5 py-0.5 text-[10px] font-black text-violet-600 dark:bg-violet-950/60 dark:text-violet-300">POST</span><code class="min-w-0 truncate">/api/v1/mailboxes/claim</code></div>
            <div class="flex min-w-0 items-center justify-between gap-2 px-4 py-3"><code class="min-w-0 truncate">/verification-code</code><a class="inline-flex shrink-0 items-center gap-1.5 font-bold text-emerald-600 hover:text-emerald-700 dark:text-emerald-300" href="/verification-code" target="_blank" rel="noopener"><ExternalLink :size="14" />打开页面</a></div>
          </div>
        </section>
        <div class="flex justify-end"><button class="primary-button" :disabled="saving === 'system'"><LoaderCircle v-if="saving === 'system'" :size="17" class="animate-spin" /><Save v-else :size="17" />保存系统设置</button></div>
      </form>
    </template>
  </div>
</template>
