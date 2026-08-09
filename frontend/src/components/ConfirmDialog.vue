<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { CircleAlert, CircleHelp } from '@lucide/vue'
import { useConfirm } from '../composables/useConfirm'

const { confirmState, accept, cancel } = useConfirm()
const confirmButton = ref(null)

const danger = computed(() => confirmState.tone === 'danger')
const iconClass = computed(() => danger.value
  ? 'bg-rose-100 text-rose-600 dark:bg-rose-950/50 dark:text-rose-300'
  : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300')
const confirmClass = computed(() => danger.value
  ? 'bg-rose-600 text-white shadow-rose-500/20 hover:bg-rose-700 focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-rose-200 dark:bg-rose-500 dark:hover:bg-rose-400 dark:focus-visible:ring-rose-900/70'
  : 'bg-emerald-600 text-white shadow-emerald-500/20 hover:bg-emerald-700 focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-emerald-200 dark:bg-emerald-500 dark:hover:bg-emerald-400 dark:focus-visible:ring-emerald-900/70')

function handleKeydown(event) {
  if (confirmState.visible && event.key === 'Escape') cancel()
}

watch(() => confirmState.id, async () => {
  await nextTick()
  confirmButton.value?.focus()
})

onMounted(() => window.addEventListener('keydown', handleKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', handleKeydown))
</script>

<template>
  <Teleport to="body">
    <Transition name="dialog">
      <div v-if="confirmState.visible" class="fixed inset-0 z-[110] flex items-center justify-center bg-slate-950/45 p-4 backdrop-blur-[2px]" role="presentation" @click.stop>
        <section :key="confirmState.id" role="alertdialog" aria-modal="true" aria-labelledby="confirm-dialog-title" aria-describedby="confirm-dialog-message" class="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-5 shadow-2xl dark:border-slate-700 dark:bg-slate-800 sm:p-6">
          <div class="flex items-start gap-4">
            <div :class="iconClass" class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl">
              <CircleAlert v-if="danger" :size="20" />
              <CircleHelp v-else :size="20" />
            </div>
            <div class="min-w-0 flex-1">
              <h2 id="confirm-dialog-title" class="text-base font-black text-slate-900 dark:text-slate-100">{{ confirmState.title }}</h2>
              <p id="confirm-dialog-message" class="mt-2 whitespace-pre-line break-words text-sm leading-6 text-slate-500 dark:text-slate-300">{{ confirmState.message }}</p>
            </div>
          </div>
          <div class="mt-6 flex justify-end gap-2.5">
            <button type="button" class="rounded-xl bg-slate-100 px-4 py-2.5 text-sm font-bold text-slate-600 transition hover:bg-slate-200 focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-slate-200 active:scale-[0.98] dark:bg-slate-700 dark:text-slate-200 dark:hover:bg-slate-600 dark:focus-visible:ring-slate-600" @click="cancel">{{ confirmState.cancelText }}</button>
            <button ref="confirmButton" type="button" :class="confirmClass" class="rounded-xl px-4 py-2.5 text-sm font-bold shadow-sm transition active:scale-[0.98]" @click="accept">{{ confirmState.confirmText }}</button>
          </div>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>
