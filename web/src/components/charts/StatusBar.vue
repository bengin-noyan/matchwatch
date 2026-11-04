<script setup lang="ts">
// Thin segmented bar of invoice states, no text on it: the list beside it carries names and counts.
// Fills validated with the dataviz palette checker (light amber keeps red/amber apart under deutan vision).
import { computed } from 'vue'
import type { AuditStatus } from '../../api'
import { statusLabel } from '../../labels'

const props = defineProps<{ counts: Record<AuditStatus, number> }>()
const order: AuditStatus[] = ['RED', 'YELLOW', 'PENDING', 'GREEN']
const fill: Record<AuditStatus, string> = { RED: '#BE1E2D', YELLOW: '#E0A93B', PENDING: '#6F7D74', GREEN: '#B9C8B5' }
const total = computed(() => order.reduce((n, k) => n + (props.counts[k] ?? 0), 0))
const label = computed(() => order.map((k) => `${statusLabel[k]} ${props.counts[k] ?? 0}`).join(', '))
</script>

<template>
  <div>
    <div v-if="total" class="flex h-2 gap-0.5" role="img" :aria-label="`Fatura durumları: ${label}`">
      <span
        v-for="k in order.filter((k) => counts[k])"
        :key="k"
        class="h-2 rounded-full first:rounded-l-full"
        :style="{ flexGrow: counts[k], background: fill[k] }"
      />
    </div>
    <ul class="mt-3 grid grid-cols-2 gap-x-6 gap-y-1.5 text-sm">
      <li v-for="k in order" :key="k" class="flex items-center gap-2">
        <span class="size-2.5 shrink-0 rounded-full" :style="{ background: fill[k] }" aria-hidden="true" />
        <span class="text-ink-soft flex-1">{{ statusLabel[k] }}</span>
        <span class="num font-semibold">{{ counts[k] ?? 0 }}</span>
      </li>
    </ul>
  </div>
</template>
