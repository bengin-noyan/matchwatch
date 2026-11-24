<script setup lang="ts">
// RED kırmızı kalem halkası, YELLOW kehribar renkli bir halka alır, GREEN işaretsiz kalır: temiz bir sayfaya mürekkep
// gerekmez.
import type { AuditStatus } from '../api'
import { statusLabel } from '../labels'
import PencilCircle from './PencilCircle.vue'

defineProps<{ status: AuditStatus; reaudit?: boolean }>()
</script>

<template>
  <PencilCircle v-if="status === 'RED'" class="text-pencil font-semibold">{{ statusLabel[status] }}</PencilCircle>
  <PencilCircle v-else-if="status === 'YELLOW'" tone="amber" class="text-amber font-semibold">{{ statusLabel[status] }}</PencilCircle>
  <span v-else class="text-ink-soft">{{ statusLabel[status] }}</span>
  <span v-if="reaudit" class="text-ink-soft block text-xs">Yeniden denetlenecek</span>
</template>
