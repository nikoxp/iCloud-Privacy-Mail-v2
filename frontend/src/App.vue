<template>
  <RouterView />
  <ToastStack />
  <ConfirmDialog />
</template>

<script setup>
import ConfirmDialog from './components/ConfirmDialog.vue'
import ToastStack from './components/ToastStack.vue'
</script>
