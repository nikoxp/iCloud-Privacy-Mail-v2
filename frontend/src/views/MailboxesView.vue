<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { Boxes, ChevronLeft, ChevronRight, Clipboard, CloudDownload, CloudOff, KeyRound, LoaderCircle, MailOpen, MailPlus, RefreshCw, Save, Search, ShieldX, Trash2, X } from '@lucide/vue'
import { api } from '../api/client'
import CardSelect from '../components/CardSelect.vue'
import FormDialog from '../components/FormDialog.vue'
import { useConfirm } from '../composables/useConfirm'
import { useToast } from '../composables/useToast'

const loading = ref(true)
const loadingVisible = ref(false)
const busy = ref('')
const codeBusyVisible = ref('')
const query = ref('')
const accountID = ref('')
const status = ref('')
const page = ref(1)
const result = ref({ items: [], page: 1, page_size: 7, total: 0, total_pages: 1 })
const selected = ref(null)
const selectedMessage = ref(null)
const messages = ref([])
const code = ref(null)
const codeMailbox = ref(null)
const codeError = ref('')
const codeDialogOpen = ref(false)
const accounts = ref([])
const showBatchClean = ref(false)
const showImport = ref(false)
const showSync = ref(false)
const quickEditOpen = ref(false)
const quickEditField = ref('note')
const quickEditMailbox = ref(null)
const deleteConfirmID = ref('')
const deleteQueue = ref([])
const deletingMailboxID = ref('')
const edit = reactive({ status: 'available', api_active: true, icloud_active: true, note: '' })
const quickEdit = reactive({ status: 'available', note: '' })
const remoteClean = reactive({ move_synced: true, empty_trash: true })
const batchClean = reactive({ account_id: '', move_synced: true, empty_trash: true })
const mailboxImport = reactive({ account_id: '', email: '', label: '', note: '' })
const syncAccountID = ref('')
let searchTimer
let loadingTimer
let codeBusyTimer
let autoRefreshTimer
let loadRequestID = 0
let deleteQueueRunning = false
let autoRefreshing = false
let deleteNoticeID = null
let deleteSucceeded = 0
let deleteFailed = 0
let deleteLastError = ''
const { success, error: showError, update: updateToast } = useToast()
const { confirm: confirmAction } = useConfirm()
const mailboxStatusOptions = [
  { value: '', label: '全部状态', dot: 'bg-slate-400' },
  { value: 'available', label: '可用', dot: 'bg-emerald-500' },
  { value: 'reserved', label: '已预留', dot: 'bg-violet-500' },
  { value: 'used', label: '已使用', dot: 'bg-amber-500' },
  { value: 'failed', label: '失败', dot: 'bg-rose-500' },
  { value: 'disabled', label: '已停用', dot: 'bg-slate-500' },
]
const mailboxDetailStatusOptions = [
  { value: 'available', label: '可用', dot: 'bg-emerald-500' },
  { value: 'reserved', label: '已预留（由租约管理）', dot: 'bg-violet-500', disabled: true },
  { value: 'used', label: '已使用', dot: 'bg-amber-500' },
  { value: 'active', label: '活跃', dot: 'bg-sky-500' },
  { value: 'failed', label: '失败', dot: 'bg-rose-500' },
  { value: 'disabled', label: '已停用', dot: 'bg-slate-500' },
]
const accountOptions = computed(() => accounts.value.map((account) => ({ value: account.id, label: account.label || account.apple_id || account.id })))
const accountRangeOptions = computed(() => [{ value: '', label: '全部 Apple 账号', dot: 'bg-slate-400' }, ...accountOptions.value])
const accountFilterOptions = computed(() => [
  { value: '', label: '全部 Apple 账号', description: '显示所有账号创建的邮箱', dot: 'bg-slate-400' },
  ...accounts.value.map((account) => ({
    value: account.id,
    label: account.label || account.apple_id || account.id,
    description: account.label && account.apple_id && account.label !== account.apple_id ? account.apple_id : '只显示此账号创建的邮箱',
  })),
])
const syncAccountOptions = computed(() => [{ value: '', label: '全部 Apple 账号', dot: 'bg-slate-400' }, ...accounts.value.map((account) => ({ value: account.id, label: `${account.label || account.apple_id || account.id}（${account.apple_id}）` }))])
const codeDialogBusy = computed(() => busy.value === 'code' || busy.value.startsWith('code-row:'))
const quickEditTitle = computed(() => quickEditField.value === 'note' ? '修改备注' : '修改邮箱状态')

const rangeText = computed(() => {
  if (!result.value.total) return '0 条记录'
  const start = (result.value.page - 1) * result.value.page_size + 1
  const end = Math.min(result.value.page * result.value.page_size, result.value.total)
  return `${start}-${end} / ${result.value.total}`
})

function statusLabel(value) {
  return ({ available: '可用', reserved: '已预留', used: '已使用', failed: '失败', disabled: '已停用', active: '活跃' })[value] || value || '未知'
}

function statusClass(value) {
  if (value === 'available' || value === 'active') return 'bg-emerald-50 text-emerald-600 dark:bg-emerald-950/40 dark:text-emerald-300'
  if (value === 'reserved') return 'bg-violet-50 text-violet-600 dark:bg-violet-950/40 dark:text-violet-300'
  if (value === 'failed') return 'bg-rose-50 text-rose-600 dark:bg-rose-950/40 dark:text-rose-300'
  if (value === 'disabled') return 'bg-slate-100 text-slate-500 dark:bg-slate-700 dark:text-slate-300'
  return 'bg-amber-50 text-amber-600 dark:bg-amber-950/40 dark:text-amber-300'
}

function flash(text, isError = false) {
  if (isError) showError(text)
  else success(text)
}

async function copyText(value) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  const copied = document.execCommand('copy')
  textarea.remove()
  if (!copied) throw new Error('复制失败')
}

async function copyMailboxEmail(mailbox) {
  try {
    await copyText(mailbox.email)
    flash(`邮箱已复制：${mailbox.email}`)
  } catch (err) {
    flash(err.message || '复制邮箱失败，请重试', true)
  }
}

function startCodeBusy(key) {
  busy.value = key
  codeBusyVisible.value = ''
  clearTimeout(codeBusyTimer)
  codeBusyTimer = window.setTimeout(() => {
    if (busy.value === key) codeBusyVisible.value = key
  }, 600)
}

function finishCodeBusy(key) {
  clearTimeout(codeBusyTimer)
  if (busy.value === key) busy.value = ''
  if (codeBusyVisible.value === key) codeBusyVisible.value = ''
}

function formatTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) || date.getFullYear() < 2000 ? '-' : date.toLocaleString('zh-CN')
}

function formatMessageTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 2000) return '-'
  const today = new Date()
  const sameDay = date.getFullYear() === today.getFullYear() && date.getMonth() === today.getMonth() && date.getDate() === today.getDate()
  return sameDay
    ? date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
    : date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function cleanupText(cleanup) {
  const moved = cleanup?.moved_to_trash || 0
  const destroyed = cleanup?.destroyed || 0
  const skipped = cleanup?.skipped || 0
  const localRemoved = cleanup?.local_removed || 0
  return `移动 ${moved} 封，彻底清除 ${destroyed} 封${localRemoved ? `，本地同步清理 ${localRemoved} 封` : ''}${skipped ? `，未匹配 ${skipped} 封` : ''}`
}

function openSyncDialog() {
  if (!accounts.value.length) {
    flash('请先添加 Apple 账号和登录态', true)
    return
  }
  syncAccountID.value = ''
  showImport.value = false
  showBatchClean.value = false
  showSync.value = true
}

async function load(options = {}) {
  const silent = Boolean(options.silent)
  const requestID = ++loadRequestID
  if (!silent) {
    loading.value = true
    loadingVisible.value = false
    clearTimeout(loadingTimer)
    loadingTimer = setTimeout(() => {
      if (loading.value && requestID === loadRequestID) loadingVisible.value = true
    }, 600)
  }
  try {
    const params = new URLSearchParams({ page: String(page.value) })
    if (query.value.trim()) params.set('q', query.value.trim())
    if (accountID.value) params.set('account_id', accountID.value)
    if (status.value) params.set('status', status.value)
    const data = await api(`/api/mailboxes?${params}`)
    if (requestID !== loadRequestID) return
    result.value = data
    page.value = result.value.page
  } catch (err) {
    if (!silent && requestID === loadRequestID) flash(err.message, true)
  } finally {
    if (!silent && requestID === loadRequestID) {
      clearTimeout(loadingTimer)
      loading.value = false
      loadingVisible.value = false
    }
  }
}

async function refreshMailboxPool() {
  if (document.hidden || loading.value || autoRefreshing || deleteQueueRunning || busy.value) return
  autoRefreshing = true
  try {
    await load({ silent: true })
    const selectedID = selected.value?.id
    if (!selectedID || quickEditOpen.value) return
    const [detail, messageData] = await Promise.all([
      api(`/api/mailboxes/${selectedID}`),
      api(`/api/mailboxes/${selectedID}/messages`),
    ])
    if (selected.value?.id !== selectedID) return
    const previous = selected.value
    if (edit.status === previous.status) edit.status = detail.mailbox.status
    if (edit.api_active === previous.api_active) edit.api_active = detail.mailbox.api_active
    if (edit.icloud_active === previous.icloud_active) edit.icloud_active = detail.mailbox.icloud_active
    if (edit.note === (previous.note || '')) edit.note = detail.mailbox.note || ''
    selected.value = detail.mailbox
    messages.value = messageData.items || []
  } catch {
    return
  } finally {
    autoRefreshing = false
  }
}

async function loadAccounts() {
  try {
    const data = await api('/api/apple-accounts')
    accounts.value = data.items || []
  } catch (err) {
    flash(err.message, true)
  }
}

async function importMailbox() {
  if (!mailboxImport.account_id || !mailboxImport.email.trim()) return
  busy.value = 'import'
  try {
    const data = await api('/api/mailboxes', { method: 'POST', body: JSON.stringify(mailboxImport) })
    flash(data.created ? `已导入 ${data.mailbox.email}` : `${data.mailbox.email} 已存在，已更新绑定信息`)
    Object.assign(mailboxImport, { account_id: mailboxImport.account_id, email: '', label: '', note: '' })
    showImport.value = false
    await load()
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function syncExistingMailboxes() {
  const targets = syncAccountID.value
    ? accounts.value.filter((item) => item.id === syncAccountID.value)
    : accounts.value
  if (!targets.length) {
    flash('没有可同步的 Apple 账号', true)
    return
  }
  busy.value = 'sync-existing'
  const failures = []
  let total = 0
  let succeeded = 0
  try {
    for (const account of targets) {
      try {
        const data = await api(`/api/apple-accounts/${account.id}/mailboxes/sync`, { method: 'POST', body: '{}' })
        total += data.count || data.items?.length || 0
        succeeded += 1
      } catch (err) {
        failures.push(`${account.label || account.apple_id || account.id}：${err.message}`)
      }
    }
    showSync.value = false
    page.value = 1
    await load()
    if (failures.length && succeeded) {
      flash(`同步完成 ${succeeded} 个账号，共发现 ${total} 个已有邮箱；${failures.join('；')}`, true)
    } else if (failures.length) {
      flash(`同步已有邮箱失败：${failures.join('；')}`, true)
    } else {
      flash(`同步完成：${succeeded} 个账号，共发现 ${total} 个已有邮箱`)
    }
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function openMailbox(mailbox) {
  busy.value = `detail:${mailbox.id}`
  code.value = null
  codeMailbox.value = null
  codeError.value = ''
  codeDialogOpen.value = false
  selectedMessage.value = null
  try {
    const [detail, messageData] = await Promise.all([
      api(`/api/mailboxes/${mailbox.id}`),
      api(`/api/mailboxes/${mailbox.id}/messages`),
    ])
    selected.value = detail.mailbox
    messages.value = messageData.items || []
    Object.assign(edit, {
      status: selected.value.status,
      api_active: selected.value.api_active,
      icloud_active: selected.value.icloud_active,
      note: selected.value.note || '',
    })
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function saveStatus() {
  if (!selected.value) return
  busy.value = 'save'
  try {
    const data = await api(`/api/mailboxes/${selected.value.id}/status`, { method: 'POST', body: JSON.stringify(edit) })
    selected.value = data.mailbox
    flash('邮箱状态已保存')
    await load()
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function syncMailbox() {
  if (!selected.value) return
  busy.value = 'sync'
  try {
    const data = await api(`/api/mailboxes/${selected.value.id}/sync`, { method: 'POST' })
    flash(`同步完成，新增 ${data.synced} 封邮件`)
    await openMailbox(selected.value)
    await load()
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function quickSyncMailbox(mailbox) {
  const rowKey = `sync-row:${mailbox.id}`
  busy.value = rowKey
  try {
    const data = await api(`/api/mailboxes/${mailbox.id}/sync`, { method: 'POST' })
    flash(`同步完成，新增 ${data.synced} 封邮件`)
    await load()
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function getCode() {
  if (!selected.value) return
  openCodeDialog(selected.value)
  startCodeBusy('code')
  try {
    code.value = await api(`/api/mailboxes/${selected.value.id}/code?allow_stale=1`)
    const [detail, messageData] = await Promise.all([
      api(`/api/mailboxes/${selected.value.id}`),
      api(`/api/mailboxes/${selected.value.id}/messages`),
    ])
    selected.value = detail.mailbox
    codeMailbox.value = detail.mailbox
    messages.value = messageData.items || []
    await load()
    flash(`已提取验证码 ${code.value.code}`)
  } catch (err) {
    codeError.value = err.message
    flash(err.message, true)
  } finally {
    finishCodeBusy('code')
  }
}

async function quickGetCode(mailbox) {
  const rowKey = `code-row:${mailbox.id}`
  openCodeDialog(mailbox)
  startCodeBusy(rowKey)
  try {
    code.value = await api(`/api/mailboxes/${mailbox.id}/code?allow_stale=1`)
    const detail = await api(`/api/mailboxes/${mailbox.id}`)
    codeMailbox.value = detail.mailbox
    await load()
    flash(`已提取验证码 ${code.value.code}`)
  } catch (err) {
    codeError.value = err.message
    flash(err.message, true)
  } finally {
    finishCodeBusy(rowKey)
  }
}

function openCodeDialog(mailbox) {
  code.value = null
  codeMailbox.value = { ...mailbox }
  codeError.value = ''
  codeDialogOpen.value = true
}

function closeCodeDialog() {
  if (codeDialogBusy.value) return
  codeDialogOpen.value = false
  code.value = null
  codeMailbox.value = null
  codeError.value = ''
}

async function copyCode() {
  if (!code.value?.code) return
  try {
    await copyText(code.value.code)
    flash('验证码已复制')
  } catch (err) {
    flash(err.message || '复制验证码失败，请重试', true)
  }
}

function openQuickEdit(mailbox, field) {
  quickEditMailbox.value = mailbox
  quickEditField.value = field
  quickEdit.status = mailbox.status || 'available'
  quickEdit.note = mailbox.note || ''
  quickEditOpen.value = true
}

function closeQuickEdit() {
  if (busy.value === 'quick-edit') return
  quickEditOpen.value = false
  quickEditMailbox.value = null
}

async function saveQuickEdit() {
  if (!quickEditMailbox.value) return
  const field = quickEditField.value
  const payload = field === 'note'
    ? { note: quickEdit.note }
    : { status: quickEdit.status }
  busy.value = 'quick-edit'
  try {
    await api(`/api/mailboxes/${quickEditMailbox.value.id}/status`, { method: 'POST', body: JSON.stringify(payload) })
    flash(field === 'note' ? '邮箱备注已保存' : '邮箱状态已保存')
    quickEditOpen.value = false
    quickEditMailbox.value = null
    await load()
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function cleanRemote() {
  if (!selected.value || (!remoteClean.move_synced && !remoteClean.empty_trash)) return
  const actions = [remoteClean.move_synced ? '把已同步邮件移入废纸篓' : '', remoteClean.empty_trash ? '清空该账号废纸篓' : ''].filter(Boolean).join('，并')
  const confirmed = await confirmAction({
    title: '清理 Apple 远端邮件',
    message: `将${actions}，这项操作会修改 Apple 服务器上的邮件。`,
    confirmText: '确认清理',
    tone: 'danger',
  })
  if (!confirmed) return
  busy.value = 'clean'
  try {
    const data = await api(`/api/mailboxes/${selected.value.id}/remote-clean`, { method: 'POST', body: JSON.stringify(remoteClean) })
    await openMailbox(selected.value)
    await load()
    flash(`远端清理完成：${cleanupText(data.cleanup)}`)
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

async function cleanRemoteBatch() {
  if (!batchClean.move_synced && !batchClean.empty_trash) return
  const scope = batchClean.account_id
    ? accounts.value.find((item) => item.id === batchClean.account_id)?.label || '所选 Apple 账号'
    : '全部 Apple 账号'
  const confirmed = await confirmAction({
    title: '批量清理远端邮件',
    message: `将批量清理${scope}的远端邮件；清空废纸篓时每个账号只执行一次。`,
    confirmText: '确认批量清理',
    tone: 'danger',
  })
  if (!confirmed) return
  busy.value = 'clean-batch'
  try {
    const data = await api('/api/mailboxes/remote-clean', { method: 'POST', body: JSON.stringify(batchClean) })
    await load()
    flash(`批量清理完成：处理 ${data.mailboxes} 个邮箱，${cleanupText(data.cleanup)}，失败 ${data.failed_mailboxes} 个`)
    showBatchClean.value = false
  } catch (err) {
    flash(err.message, true)
  } finally {
    busy.value = ''
  }
}

function isMailboxDeleteQueued(mailboxID) {
  return deleteQueue.value.some((item) => item.id === mailboxID)
}

function isMailboxDeleteBusy(mailboxID) {
  return deleteConfirmID.value === mailboxID || deletingMailboxID.value === mailboxID || isMailboxDeleteQueued(mailboxID)
}

function updateDeleteProgress() {
  const current = deleteQueue.value[0]
  if (!current) return
  const waiting = Math.max(0, deleteQueue.value.length - (deletingMailboxID.value ? 1 : 0))
  const action = deletingMailboxID.value ? `正在清理邮件并彻底删除：${current.email}` : `等待彻底删除：${current.email}`
  const queueText = waiting ? `；另有 ${waiting} 个排队中` : ''
  const finished = deleteSucceeded + deleteFailed
  const finishedText = finished ? `；已处理 ${finished} 个（成功 ${deleteSucceeded}，失败 ${deleteFailed}）` : ''
  deleteNoticeID = updateToast(deleteNoticeID, `${action}${queueText}${finishedText}`, 'info', 0)
}

function finishDeleteProgress() {
  const total = deleteSucceeded + deleteFailed
  const type = deleteFailed ? (deleteSucceeded ? 'warning' : 'error') : 'success'
  const text = deleteFailed
    ? `删除队列已结束：成功 ${deleteSucceeded} 个，失败 ${deleteFailed} 个${deleteLastError ? `；最近错误：${deleteLastError}` : ''}`
    : `删除队列已完成：成功彻底删除 ${total} 个邮箱`
  deleteNoticeID = updateToast(deleteNoticeID, text, type, 7000)
}

async function processDeleteQueue() {
  if (deleteQueueRunning) return
  deleteQueueRunning = true
  try {
    while (deleteQueue.value.length) {
      const mailbox = deleteQueue.value[0]
      deletingMailboxID.value = mailbox.id
      updateDeleteProgress()
      try {
        await api(`/api/mailboxes/${mailbox.id}`, { method: 'DELETE' })
        deleteSucceeded += 1
        if (selected.value?.id === mailbox.id) {
          selected.value = null
          messages.value = []
        }
      } catch (err) {
        deleteFailed += 1
        deleteLastError = err.message
      } finally {
        deleteQueue.value.shift()
        deletingMailboxID.value = ''
        await load()
      }
    }
  } finally {
    deleteQueueRunning = false
    deletingMailboxID.value = ''
    finishDeleteProgress()
  }
}

async function deleteMailbox(mailbox, localOnly) {
  if (!mailbox) return
  if (deleteConfirmID.value || deletingMailboxID.value || deleteQueue.value.length || busy.value) {
    flash('请等待删除队列和当前操作完成', true)
    return
  }
  deleteConfirmID.value = mailbox.id
  const text = localOnly
    ? '将先清空本地邮件，再删除本地邮箱记录；Apple 服务器上的隐私邮箱会保留。继续吗？'
    : `将先清空 ${mailbox.email} 的 Apple 远端邮件和本地邮件，再从 Apple 服务器彻底删除邮箱并确认远端不存在。此操作不可恢复，继续吗？`
  try {
    const confirmed = await confirmAction({
      title: localOnly ? '只删除本地记录' : '彻底删除隐私邮箱',
      message: text,
      confirmText: localOnly ? '确认删除本地记录' : '确认彻底删除',
      tone: 'danger',
    })
    if (!confirmed) return
    const busyKey = localOnly ? 'delete-local' : 'delete-remote'
    busy.value = busyKey
    deleteNoticeID = updateToast(deleteNoticeID, localOnly ? `正在清理本地邮件并删除记录：${mailbox.email}` : `正在清理邮件并彻底删除：${mailbox.email}`, 'info', 0)
    try {
      const suffix = localOnly ? '?local_only=1' : ''
      await api(`/api/mailboxes/${mailbox.id}${suffix}`, { method: 'DELETE' })
      deleteNoticeID = updateToast(deleteNoticeID, localOnly ? '本地邮件和邮箱记录均已删除' : 'Apple 远端邮件、本地邮件和邮箱记录均已删除', 'success', 7000)
      if (selected.value?.id === mailbox.id) {
        selected.value = null
        messages.value = []
      }
      await load()
    } catch (err) {
      deleteNoticeID = updateToast(deleteNoticeID, `删除失败：${err.message}`, 'error', 7000)
    } finally {
      busy.value = ''
    }
  } finally {
    deleteConfirmID.value = ''
  }
}

function removeMailbox(localOnly) {
  return deleteMailbox(selected.value, localOnly)
}

async function removeMailboxFromRow(mailbox) {
  if (!mailbox) return
  if (deleteConfirmID.value) {
    flash('请先完成当前邮箱的删除确认', true)
    return
  }
  if (isMailboxDeleteBusy(mailbox.id)) {
    flash(`${mailbox.email} 已在删除队列中`, true)
    return
  }
  deleteConfirmID.value = mailbox.id
  try {
    const confirmed = await confirmAction({
      title: '彻底删除隐私邮箱',
      message: `将先清空 ${mailbox.email} 的 Apple 远端邮件和本地邮件，再从 Apple 服务器彻底删除邮箱并确认远端不存在。此操作不可恢复，继续吗？`,
      confirmText: '确认彻底删除',
      tone: 'danger',
    })
    if (!confirmed) return
    if (!deleteQueueRunning) {
      deleteSucceeded = 0
      deleteFailed = 0
      deleteLastError = ''
    }
    deleteQueue.value.push({ id: mailbox.id, email: mailbox.email })
    updateDeleteProgress()
    void processDeleteQueue()
  } finally {
    deleteConfirmID.value = ''
  }
}

function move(delta) {
  if (loading.value) return
  page.value = Math.max(1, Math.min(result.value.total_pages, page.value + delta))
  load()
}

function handlePageKeydown(event) {
  if (event.key !== 'Escape') return
  if (selectedMessage.value) selectedMessage.value = null
  else if (codeDialogOpen.value) closeCodeDialog()
  else if (quickEditOpen.value) closeQuickEdit()
  else if (selected.value) selected.value = null
}

watch(query, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { page.value = 1; load() }, 300)
})
watch(accountID, () => { page.value = 1; load() })
watch(status, () => { page.value = 1; load() })
watch([selected, codeDialogOpen, quickEditOpen], ([mailbox, codeOpen, quickEditOpenValue]) => {
  document.body.style.overflow = mailbox || codeOpen || quickEditOpenValue ? 'hidden' : ''
})
watch(selected, (value) => {
  if (!value) selectedMessage.value = null
})
onMounted(() => {
  document.addEventListener('keydown', handlePageKeydown)
  Promise.all([load(), loadAccounts()])
  autoRefreshTimer = window.setInterval(refreshMailboxPool, 4000)
})
onBeforeUnmount(() => {
  clearTimeout(searchTimer)
  clearTimeout(loadingTimer)
  clearTimeout(codeBusyTimer)
  window.clearInterval(autoRefreshTimer)
  document.body.style.overflow = ''
  document.removeEventListener('keydown', handlePageKeydown)
})
</script>

<template>
  <div class="mx-auto flex max-w-7xl flex-col gap-5">
    <section class="panel grid gap-3 p-4 2xl:grid-cols-[minmax(0,1fr)_auto] 2xl:items-center">
      <div class="grid min-w-0 gap-3 sm:grid-cols-[minmax(180px,300px)_minmax(180px,240px)_110px]">
        <label class="field-wrap w-full"><Search :size="17" class="field-icon" /><input v-model="query" class="field field-leading" placeholder="搜索邮箱、标签或备注" /></label>
        <CardSelect v-model="accountID" class="min-w-0" :options="accountFilterOptions" aria-label="Apple 账号" />
        <CardSelect v-model="status" class="w-full" :options="mailboxStatusOptions" aria-label="邮箱状态" />
      </div>
      <div class="flex flex-wrap items-center justify-end gap-2"><span class="mailbox-range mr-1 whitespace-nowrap text-right text-xs font-medium tabular-nums text-slate-400">{{ rangeText }}</span><button class="secondary-button whitespace-nowrap px-3 py-2" @click="openSyncDialog"><CloudDownload :size="15" />同步已有邮箱</button><button class="secondary-button whitespace-nowrap px-3 py-2" @click="showImport = !showImport"><MailPlus :size="15" />导入本地邮箱</button><button class="secondary-button whitespace-nowrap px-3 py-2" @click="showBatchClean = !showBatchClean"><CloudOff :size="15" />批量清理远端邮件</button></div>
    </section>

    <section v-if="showImport" class="panel space-y-4 p-4 sm:p-5">
      <div><h2 class="section-title">导入已有隐私邮箱</h2><p class="mt-1 text-xs leading-5 text-slate-400">只创建或更新本地记录，不会在 Apple 服务器新建邮箱；导入后使用所选账号的 IMAP 登录态收信。</p></div>
      <form class="grid gap-3 lg:grid-cols-2" @submit.prevent="importMailbox">
        <div class="form-group"><span class="form-label">绑定 Apple 账号</span><CardSelect v-model="mailboxImport.account_id" :options="accountOptions" placeholder="请选择 Apple 账号" aria-label="绑定 Apple 账号" /></div>
        <label class="form-group"><span class="form-label">隐私邮箱地址</span><input v-model.trim="mailboxImport.email" type="email" class="field" placeholder="example@icloud.com" required /></label>
        <label class="form-group"><span class="form-label">标签</span><input v-model.trim="mailboxImport.label" class="field" placeholder="例如：手动导入" /></label>
        <label class="form-group"><span class="form-label">备注</span><input v-model.trim="mailboxImport.note" class="field" placeholder="可选" /></label>
        <div class="lg:col-span-2"><button class="primary-button" :disabled="busy === 'import'"><LoaderCircle v-if="busy === 'import'" :size="16" class="animate-spin" /><MailPlus v-else :size="16" />保存本地邮箱</button></div>
      </form>
    </section>

    <section v-if="showBatchClean" class="panel space-y-4 p-4 sm:p-5">
      <div><h2 class="section-title">批量清理 Apple 远端邮件</h2><p class="mt-1 text-xs leading-5 text-slate-400">只处理本地已保存远端 ID 的邮件；清空废纸篓会影响所选 Apple 账号整个废纸篓。</p></div>
      <div class="grid gap-3 lg:grid-cols-[minmax(220px,1fr)_minmax(180px,1fr)_minmax(180px,1fr)_auto] lg:items-end">
        <div class="form-group"><span class="form-label">Apple 账号范围</span><CardSelect v-model="batchClean.account_id" :options="accountRangeOptions" aria-label="Apple 账号范围" /></div>
        <label class="toggle-card"><span><strong>移动已同步邮件</strong><small>将本地有远端 ID 的邮件移入废纸篓</small></span><input v-model="batchClean.move_synced" type="checkbox" /></label>
        <label class="toggle-card"><span><strong>清空废纸篓</strong><small>每个 Apple 登录态只清空一次</small></span><input v-model="batchClean.empty_trash" type="checkbox" /></label>
        <button class="primary-button min-h-11" :disabled="busy === 'clean-batch' || (!batchClean.move_synced && !batchClean.empty_trash)" @click="cleanRemoteBatch"><LoaderCircle v-if="busy === 'clean-batch'" :size="16" class="animate-spin" /><CloudOff v-else :size="16" />开始批量清理</button>
      </div>
    </section>

    <div v-if="showSync" class="fixed inset-0 z-50 !m-0 flex items-center justify-center bg-slate-950/45 p-4 backdrop-blur-[2px]" role="presentation" @click.stop>
      <section role="dialog" aria-modal="true" aria-labelledby="sync-mailboxes-title" class="panel w-full max-w-lg p-5 shadow-2xl sm:p-6">
        <div class="mb-5 flex items-start justify-between gap-4"><div><h2 id="sync-mailboxes-title" class="flex items-center gap-2 font-black"><CloudDownload :size="18" />同步已有邮箱</h2><p class="mt-1 text-xs leading-5 text-slate-400">从所选 Apple 账号的 iCloud 隐私邮箱列表读取已有地址，并更新到本地邮箱池。</p></div><button class="icon-button" title="关闭" :disabled="busy === 'sync-existing'" @click="showSync = false"><X :size="18" /></button></div>
        <form class="space-y-4" @submit.prevent="syncExistingMailboxes"><div class="form-group"><span class="form-label">同步范围</span><CardSelect v-model="syncAccountID" :options="syncAccountOptions" aria-label="同步范围" /><span class="form-help">同步使用账号已保存的 iCloud Web 旧接口登录态。</span></div><button class="primary-button w-full" :disabled="busy === 'sync-existing'"><LoaderCircle v-if="busy === 'sync-existing'" :size="16" class="animate-spin" /><CloudDownload v-else :size="16" />{{ busy === 'sync-existing' ? '同步中' : '开始同步' }}</button></form>
      </section>
    </div>

    <section class="panel relative overflow-hidden">
      <div v-if="loadingVisible" class="absolute inset-0 z-30 flex items-center justify-center bg-white/55 backdrop-blur-[1px] dark:bg-slate-950/50"><div class="flex items-center gap-2 rounded-xl border border-slate-200 bg-white px-4 py-2.5 text-xs font-bold text-slate-600 shadow-lg dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200"><LoaderCircle :size="17" class="animate-spin text-emerald-500" />正在加载邮箱</div></div>
      <div v-if="!loading && !result.items?.length" class="mailbox-list-viewport flex flex-col items-center justify-center gap-3 text-center"><div class="rounded-2xl bg-slate-100 p-4 text-slate-400 dark:bg-slate-700"><Boxes :size="31" /></div><div class="font-bold">没有符合条件的邮箱</div><div class="text-sm text-slate-400">从 Apple 账号页创建或同步隐私邮箱后会显示在这里。</div></div>
      <template v-else>
        <div class="mailbox-list-viewport mailbox-list-scroll">
          <table class="w-full min-w-[1230px] table-fixed text-left text-sm">
            <colgroup>
              <col class="w-[240px]" />
              <col class="w-[150px]" />
              <col class="w-[90px]" />
              <col class="w-[150px]" />
              <col class="w-[70px]" />
              <col class="w-[170px]" />
              <col class="w-[360px]" />
            </colgroup>
            <thead class="sticky top-0 z-10 bg-slate-50 text-xs text-slate-400 shadow-[0_1px_0_rgba(148,163,184,0.18)] dark:bg-slate-900">
              <tr><th class="px-4 py-3 font-semibold">邮箱</th><th class="px-3 py-3 text-center font-semibold">标签/备注</th><th class="px-3 py-3 text-center font-semibold">状态</th><th class="px-3 py-3 text-center font-semibold">API / iCloud</th><th class="px-3 py-3 text-center font-semibold">收件</th><th class="px-3 py-3 text-center font-semibold">最近同步</th><th class="px-3 py-3 text-right font-semibold">操作</th></tr>
            </thead>
            <tbody class="divide-y divide-slate-100 dark:divide-slate-700">
              <tr v-for="mailbox in result.items" :key="mailbox.id" class="hover:bg-emerald-50/30 dark:hover:bg-slate-700/30">
                <td class="px-4 py-4"><button type="button" class="block max-w-full truncate text-left font-semibold transition hover:text-emerald-600 dark:hover:text-emerald-300" :title="`点击复制邮箱：${mailbox.email}`" @click.stop="copyMailboxEmail(mailbox)">{{ mailbox.email }}</button><div class="mt-1 truncate font-mono text-[11px] text-slate-400" :title="mailbox.id">{{ mailbox.id }}</div></td>
                <td class="px-3 py-3 text-center"><div class="truncate font-semibold text-slate-600 dark:text-slate-200" :title="mailbox.label || '-'">{{ mailbox.label || '-' }}</div><button type="button" class="mt-1 max-w-full truncate text-xs text-slate-400 transition hover:text-emerald-600 hover:underline dark:hover:text-emerald-300" :title="mailbox.note ? `点击修改备注：${mailbox.note}` : '点击添加备注'" @click.stop="openQuickEdit(mailbox, 'note')">{{ mailbox.note || '添加备注' }}</button></td>
                <td class="px-3 py-4 text-center"><button type="button" :class="statusClass(mailbox.status)" class="rounded-full px-2.5 py-1 text-xs font-bold transition hover:ring-2 hover:ring-emerald-400/40" title="点击修改邮箱状态" @click.stop="openQuickEdit(mailbox, 'status')">{{ statusLabel(mailbox.status) }}</button></td>
                <td class="px-3 py-4"><div class="flex justify-center gap-1.5"><span :class="mailbox.api_active ? 'text-emerald-600 bg-emerald-50 dark:bg-emerald-950/40' : 'text-slate-400 bg-slate-100 dark:bg-slate-700'" class="rounded-lg px-2 py-1 text-[11px] font-bold">API</span><span :class="mailbox.icloud_active ? 'text-sky-600 bg-sky-50 dark:bg-sky-950/40' : 'text-slate-400 bg-slate-100 dark:bg-slate-700'" class="rounded-lg px-2 py-1 text-[11px] font-bold">iCloud</span></div></td>
                <td class="px-3 py-4 text-center font-mono text-slate-500">{{ mailbox.receive_count || 0 }}</td>
                <td class="whitespace-nowrap px-3 py-4 text-center text-xs text-slate-400">{{ formatTime(mailbox.last_sync_at) }}</td>
                <td class="px-3 py-3">
                  <div class="flex justify-end gap-1.5">
                    <button class="mailbox-action-button mailbox-action-sync" :class="{ 'mailbox-action-sync-selected': busy === `sync-row:${mailbox.id}` }" :disabled="Boolean(busy) || isMailboxDeleteBusy(mailbox.id)" title="同步该邮箱的最新邮件" @click.stop="quickSyncMailbox(mailbox)"><LoaderCircle v-if="busy === `sync-row:${mailbox.id}`" :size="13" class="animate-spin" /><RefreshCw v-else :size="13" />同步</button>
                    <button class="mailbox-action-button mailbox-action-code" :class="{ 'mailbox-action-code-selected': busy === `code-row:${mailbox.id}` || (codeDialogOpen && codeMailbox?.id === mailbox.id) }" :disabled="Boolean(busy) || isMailboxDeleteBusy(mailbox.id)" title="获取该邮箱的最新验证码" @click.stop="quickGetCode(mailbox)"><LoaderCircle v-if="codeBusyVisible === `code-row:${mailbox.id}`" :size="13" class="animate-spin" /><KeyRound v-else :size="13" />取码</button>
                    <button class="mailbox-action-button mailbox-action-detail" :class="{ 'mailbox-action-detail-selected': selected?.id === mailbox.id }" :disabled="Boolean(busy) || isMailboxDeleteBusy(mailbox.id)" title="查看邮箱详情" @click.stop="openMailbox(mailbox)"><LoaderCircle v-if="busy === `detail:${mailbox.id}`" :size="13" class="animate-spin" /><MailOpen v-else :size="13" />详情</button>
                    <button class="mailbox-action-button mailbox-action-delete" :class="{ 'mailbox-action-delete-selected': isMailboxDeleteBusy(mailbox.id) }" :disabled="Boolean(busy) || isMailboxDeleteBusy(mailbox.id)" :title="deletingMailboxID === mailbox.id ? '正在清理邮件并彻底删除邮箱' : isMailboxDeleteQueued(mailbox.id) ? '已加入彻底删除队列' : '先清空 Apple 远端及本地邮件，再彻底删除邮箱'" @click.stop="removeMailboxFromRow(mailbox)"><LoaderCircle v-if="deletingMailboxID === mailbox.id" :size="13" class="animate-spin" /><LoaderCircle v-else-if="isMailboxDeleteQueued(mailbox.id)" :size="13" class="animate-spin" /><Trash2 v-else :size="13" />{{ deletingMailboxID === mailbox.id ? '删除中' : isMailboxDeleteQueued(mailbox.id) ? '排队中' : '删除' }}</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
      <div v-if="result.items?.length || !loading" class="flex items-center justify-between border-t border-slate-100 px-5 py-3 dark:border-slate-700"><span class="text-xs text-slate-400">第 {{ result.page }} / {{ result.total_pages }} 页</span><div class="flex gap-2"><button class="secondary-button px-3 py-2" :disabled="loading || result.page <= 1" title="上一页" @click="move(-1)"><ChevronLeft :size="16" /></button><button class="secondary-button px-3 py-2" :disabled="loading || result.page >= result.total_pages" title="下一页" @click="move(1)"><ChevronRight :size="16" /></button></div></div>
    </section>

    <FormDialog :open="quickEditOpen" :title="quickEditTitle" :description="quickEditMailbox?.email || ''" :busy="busy === 'quick-edit'" @close="closeQuickEdit" @submit="saveQuickEdit">
      <label v-if="quickEditField === 'note'" class="form-group"><span class="form-label">备注</span><textarea v-model="quickEdit.note" class="field min-h-24 resize-none" maxlength="1000" placeholder="请输入邮箱备注，留空可清除备注" autofocus /></label>
      <div v-else class="form-group"><span class="form-label">邮箱状态</span><CardSelect v-model="quickEdit.status" :options="mailboxDetailStatusOptions" aria-label="邮箱状态" /></div>
    </FormDialog>

    <div v-if="selected" class="fixed inset-0 z-40 !m-0 flex items-center justify-center bg-slate-950/55 p-3 backdrop-blur-[3px] sm:p-5" role="presentation" @click.stop>
      <aside class="max-h-[calc(100vh-1.5rem)] w-full max-w-2xl overflow-y-auto rounded-2xl border border-slate-200 bg-slate-50 shadow-2xl dark:border-slate-700 dark:bg-slate-950 sm:max-h-[calc(100vh-2.5rem)]" role="dialog" aria-modal="true" aria-labelledby="mailbox-detail-title">
        <header class="sticky top-0 z-10 rounded-t-2xl border-b border-slate-200 bg-white/95 px-4 py-3 backdrop-blur dark:border-slate-800 dark:bg-slate-900/95">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="mb-1.5 flex flex-wrap items-center gap-1.5">
                <span :class="statusClass(selected.status)" class="rounded-md px-2 py-0.5 text-[10px] font-bold">{{ statusLabel(selected.status) }}</span>
                <span :class="selected.api_active ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/60 dark:text-emerald-200' : 'bg-slate-200 text-slate-500 dark:bg-slate-700 dark:text-slate-300'" class="rounded-md px-2 py-0.5 text-[10px] font-bold">API</span>
                <span :class="selected.icloud_active ? 'bg-sky-100 text-sky-700 dark:bg-sky-900/60 dark:text-sky-200' : 'bg-slate-200 text-slate-500 dark:bg-slate-700 dark:text-slate-300'" class="rounded-md px-2 py-0.5 text-[10px] font-bold">iCloud</span>
              </div>
              <h2 id="mailbox-detail-title" class="truncate text-base font-black">{{ selected.email }}</h2>
              <p class="mt-0.5 truncate font-mono text-[10px] text-slate-400">{{ selected.id }}</p>
              <p v-if="selected.active_lease_id" class="mt-0.5 truncate font-mono text-[10px] text-violet-500">当前租约：{{ selected.active_lease_id }}</p>
            </div>
            <button class="icon-button h-8 w-8 rounded-lg" title="关闭" @click="selected = null"><X :size="17" /></button>
          </div>
        </header>

        <div class="space-y-3 p-4">
          <div class="grid grid-cols-2 gap-2">
            <button class="detail-button detail-button-primary" :disabled="busy === 'sync'" @click="syncMailbox"><RefreshCw :size="14" :class="busy === 'sync' ? 'animate-spin' : ''" />同步邮件</button>
            <button class="detail-button detail-button-secondary" :disabled="busy === 'code'" @click="getCode"><LoaderCircle v-if="codeBusyVisible === 'code'" :size="14" class="animate-spin" /><KeyRound v-else :size="14" />获取验证码</button>
          </div>

          <form class="rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900" @submit.prevent="saveStatus">
            <div class="mb-1 flex flex-wrap items-center justify-between gap-2">
              <h3 class="text-xs font-black text-slate-700 dark:text-slate-200">状态与接收</h3>
              <div class="ml-auto flex items-center gap-2"><div class="flex items-center gap-1.5"><span class="whitespace-nowrap text-[10px] font-bold text-slate-400">使用状态</span><CardSelect v-model="edit.status" class="detail-status-select" :options="mailboxDetailStatusOptions" aria-label="使用状态" compact /></div><button class="detail-button detail-button-secondary h-8" :disabled="busy === 'save'"><LoaderCircle v-if="busy === 'save'" :size="13" class="animate-spin" /><Save v-else :size="13" />保存</button></div>
            </div>
            <div>
              <label class="block space-y-1.5"><span class="text-[11px] font-bold text-slate-500 dark:text-slate-300">备注</span><textarea v-model="edit.note" class="field detail-field min-h-[62px] resize-y" placeholder="可选备注" /></label>
            </div>
            <div class="detail-setting-grid mt-3">
              <label class="detail-setting-row"><span><strong>公共取码 API</strong><small>控制外部接口取码</small></span><input v-model="edit.api_active" class="detail-switch" type="checkbox" /></label>
              <label class="detail-setting-row"><span><strong>iCloud 远端状态</strong><small>标记邮箱是否可收信</small></span><input v-model="edit.icloud_active" class="detail-switch" type="checkbox" /></label>
            </div>
          </form>

          <section class="rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900">
            <div class="mb-2.5 flex items-start justify-between gap-3"><h3 class="text-xs font-black text-slate-700 dark:text-slate-200">本地邮件 <span class="ml-1 text-slate-400">{{ messages.length }}</span></h3><span class="whitespace-nowrap text-[10px] text-slate-400">同步于 {{ formatTime(selected.last_sync_at) }}</span></div>
            <div v-if="!messages.length" class="mail-message-list flex items-center justify-center text-xs text-slate-400">暂无本地邮件</div>
            <div v-else class="mail-message-list">
              <button v-for="item in messages" :key="item.id" type="button" class="mail-message-row" :title="`查看完整邮件：${item.subject || '无主题'}`" @click="selectedMessage = item">
                <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-sky-100 text-sky-600 dark:bg-sky-950/50 dark:text-sky-300"><MailOpen :size="15" /></span>
                <span class="min-w-0 flex-1 text-left"><strong class="block truncate text-xs">{{ item.subject || '无主题' }}</strong><small class="mt-0.5 block truncate text-[10px] text-slate-400">{{ item.from || '未知发件人' }}</small></span>
                <span class="flex shrink-0 items-center gap-1.5"><time class="text-[9px] text-slate-400">{{ formatMessageTime(item.received_at) }}</time><ChevronRight :size="14" class="text-slate-300 dark:text-slate-600" /></span>
              </button>
            </div>
          </section>

          <section class="rounded-xl border border-rose-200/70 bg-white p-3 dark:border-rose-950 dark:bg-slate-900">
            <div class="mb-2.5"><h3 class="text-xs font-black text-slate-700 dark:text-slate-200">清理与删除</h3><p class="mt-0.5 text-[10px] leading-4 text-slate-400">彻底删除会先清空 Apple 远端邮件和本地邮件，再删除 Apple 邮箱及本地记录。</p></div>
            <div class="detail-setting-grid"><label class="detail-setting-row"><span><strong>移动已同步邮件</strong><small>移入 Apple 废纸篓</small></span><input v-model="remoteClean.move_synced" class="detail-switch" type="checkbox" /></label><label class="detail-setting-row"><span><strong>清空废纸篓</strong><small>彻底清除废纸篓邮件</small></span><input v-model="remoteClean.empty_trash" class="detail-switch" type="checkbox" /></label></div>
            <div class="mt-2 grid gap-2 sm:grid-cols-2">
              <button class="detail-button detail-button-secondary sm:col-span-2" :disabled="busy === 'clean' || (!remoteClean.move_synced && !remoteClean.empty_trash)" @click="cleanRemote"><LoaderCircle v-if="busy === 'clean'" :size="14" class="animate-spin" /><CloudOff v-else :size="14" />清理 Apple 远端邮件</button>
              <button class="detail-button detail-button-danger" :disabled="busy === 'delete-remote'" @click="removeMailbox(false)"><LoaderCircle v-if="busy === 'delete-remote'" :size="14" class="animate-spin" /><Trash2 v-else :size="14" />彻底删除</button>
              <button class="detail-button detail-button-ghost" :disabled="busy === 'delete-local'" @click="removeMailbox(true)"><LoaderCircle v-if="busy === 'delete-local'" :size="14" class="animate-spin" /><ShieldX v-else :size="14" />只删本地</button>
            </div>
          </section>
        </div>
      </aside>
    </div>

    <div v-if="codeDialogOpen" class="fixed inset-0 z-[70] !m-0 flex items-center justify-center bg-slate-950/65 p-4 backdrop-blur-[4px]" role="presentation" @click.stop>
      <section class="panel w-full max-w-md overflow-hidden p-0 shadow-2xl" role="dialog" aria-modal="true" aria-labelledby="mailbox-code-title">
        <header class="flex items-start justify-between gap-4 border-b border-slate-200 px-5 py-4 dark:border-slate-700">
          <div class="min-w-0">
            <div class="mb-1.5 flex items-center gap-1.5"><span class="rounded-md bg-emerald-100 px-2 py-0.5 text-[10px] font-bold text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-200">邮箱取码</span><span v-if="codeMailbox?.label" class="max-w-40 truncate rounded-md bg-slate-100 px-2 py-0.5 text-[10px] font-bold text-slate-500 dark:bg-slate-800 dark:text-slate-300">{{ codeMailbox.label }}</span></div>
            <h2 id="mailbox-code-title" class="truncate text-base font-black">{{ codeMailbox?.email || code?.email || '获取验证码' }}</h2>
            <p class="mt-1 text-[11px] text-slate-400">同步最新邮件并提取验证码</p>
          </div>
          <button class="icon-button h-8 w-8 rounded-lg" title="关闭验证码弹窗" :disabled="codeDialogBusy" @click="closeCodeDialog"><X :size="17" /></button>
        </header>

        <div class="p-5">
          <div v-if="codeDialogBusy" class="flex min-h-44 flex-col items-center justify-center gap-3 text-center">
            <span class="flex h-12 w-12 items-center justify-center rounded-2xl bg-emerald-50 text-emerald-600 dark:bg-emerald-950/50 dark:text-emerald-300"><LoaderCircle v-if="codeBusyVisible" :size="24" class="animate-spin" /><KeyRound v-else :size="23" /></span>
            <div><strong class="block text-sm">正在获取验证码</strong><span class="mt-1 block text-xs text-slate-400">正在同步并检查最新邮件，请稍候……</span></div>
          </div>

          <div v-else-if="code" class="space-y-4">
            <div class="rounded-2xl border border-emerald-200 bg-emerald-50 p-4 text-center dark:border-emerald-900 dark:bg-emerald-950/35">
              <div class="text-[11px] font-bold text-emerald-600 dark:text-emerald-300">最新验证码</div>
              <div class="mt-1 font-mono text-4xl font-black tracking-[0.18em] text-emerald-700 dark:text-emerald-200">{{ code.code }}</div>
              <div class="mt-2 truncate text-xs text-emerald-700/70 dark:text-emerald-300/70" :title="code.subject">{{ code.subject || '未提供邮件主题' }}</div>
            </div>
            <div class="grid grid-cols-2 gap-2 text-xs">
              <div class="rounded-xl bg-slate-50 px-3 py-2.5 dark:bg-slate-800/70"><span class="block text-[10px] text-slate-400">收件数量</span><strong class="mt-0.5 block">{{ codeMailbox?.receive_count || 0 }} 封</strong></div>
              <div class="rounded-xl bg-slate-50 px-3 py-2.5 dark:bg-slate-800/70"><span class="block text-[10px] text-slate-400">收件时间</span><strong class="mt-0.5 block truncate" :title="formatTime(code.received_at)">{{ formatTime(code.received_at) }}</strong></div>
            </div>
            <button class="primary-button w-full" @click="copyCode"><Clipboard :size="16" />复制验证码</button>
          </div>

          <div v-else class="flex min-h-44 flex-col items-center justify-center gap-3 text-center">
            <span class="flex h-12 w-12 items-center justify-center rounded-2xl bg-rose-50 text-rose-600 dark:bg-rose-950/50 dark:text-rose-300"><KeyRound :size="23" /></span>
            <div><strong class="block text-sm">暂未获取到验证码</strong><span class="mt-1 block max-w-sm text-xs leading-5 text-slate-400">{{ codeError || '请稍后重新取码。' }}</span></div>
            <button class="secondary-button" @click="closeCodeDialog">关闭</button>
          </div>
        </div>
      </section>
    </div>

    <div v-if="selectedMessage" class="fixed inset-0 z-[60] !m-0 flex items-center justify-center bg-slate-950/65 p-3 backdrop-blur-[4px] sm:p-5" role="presentation" @click.stop>
      <article class="mail-message-dialog flex flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl dark:border-slate-700 dark:bg-slate-900" role="dialog" aria-modal="true" aria-labelledby="mail-message-title">
        <header class="flex items-start justify-between gap-4 border-b border-slate-200 px-4 py-3.5 dark:border-slate-700 sm:px-5">
          <div class="min-w-0"><div class="mb-1.5 flex flex-wrap items-center gap-1.5"><span class="rounded-md bg-sky-100 px-2 py-0.5 text-[10px] font-bold text-sky-700 dark:bg-sky-950/60 dark:text-sky-200">完整邮件</span><span v-if="selectedMessage.source" class="rounded-md bg-slate-100 px-2 py-0.5 text-[10px] font-bold text-slate-500 dark:bg-slate-800 dark:text-slate-300">{{ selectedMessage.source }}</span></div><h2 id="mail-message-title" class="break-words text-base font-black leading-6">{{ selectedMessage.subject || '无主题' }}</h2><p class="mt-1 break-all text-[11px] text-slate-400">{{ selectedMessage.from || '未知发件人' }}</p></div>
          <button class="icon-button h-8 w-8 rounded-lg" title="关闭完整邮件" @click="selectedMessage = null"><X :size="17" /></button>
        </header>
        <div class="flex items-center justify-between gap-3 border-b border-slate-100 bg-slate-50 px-4 py-2 text-[10px] text-slate-400 dark:border-slate-800 dark:bg-slate-950/50 sm:px-5"><span class="truncate">收件邮箱：{{ selected?.email }}</span><time class="shrink-0">{{ formatTime(selectedMessage.received_at) }}</time></div>
        <div class="min-h-0 flex-1 overflow-y-auto p-4 sm:p-5"><div class="min-h-40 whitespace-pre-wrap break-words rounded-xl border border-slate-200 bg-slate-50 p-4 text-sm leading-6 text-slate-700 dark:border-slate-700 dark:bg-slate-950/50 dark:text-slate-300">{{ selectedMessage.body || '这封邮件没有正文内容。' }}</div></div>
      </article>
    </div>
  </div>
</template>
