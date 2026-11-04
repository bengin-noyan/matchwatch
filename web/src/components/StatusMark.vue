<script setup lang="ts">
// RED gets the red-pencil loop, YELLOW an amber one, GREEN stays unmarked: a clean sheet needs no ink.
import type { AuditStatus } from '../api'
import { statusLabel } from '../labels'
import PencilCircle from './PencilCircle.vue'

defineProps<{ status: AuditStatus }>()
</script>

<template>
  <PencilCircle v-if="status === 'RED'" class="text-pencil font-semibold">{{ statusLabel[status] }}</PencilCircle>
  <PencilCircle v-else-if="status === 'YELLOW'" tone="amber" class="text-amber font-semibold">{{ statusLabel[status] }}</PencilCircle>
  <span v-else class="text-ink-soft">{{ statusLabel[status] }}</span>
</template>
