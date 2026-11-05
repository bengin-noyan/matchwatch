<script setup lang="ts">
// Ranked horizontal bars, largest first, one hue. Bars share one baseline so close values compare exactly;
// the value rides at the tip, and hover or focus adds the count.
import { computed } from 'vue'

export interface Bar {
  key: string
  label: string
  value: number // geometry only
  text: string // formatted amount
  detail: string // e.g. "3 bulgu"
  to?: string
}

const props = defineProps<{ bars: Bar[]; empty: string }>()
const max = computed(() => Math.max(1, ...props.bars.map((b) => b.value)))
const pct = (v: number) => `${Math.max(1.5, (v / max.value) * 100)}%`
</script>

<template>
  <p v-if="!bars.length" class="text-ink-soft text-sm">{{ empty }}</p>
  <ul v-else class="space-y-3">
    <li v-for="b in bars" :key="b.key" class="group">
      <component :is="b.to ? 'RouterLink' : 'div'" :to="b.to" class="block rounded-sm" :title="`${b.label}: ${b.text}, ${b.detail}`">
        <div class="mb-1 flex items-baseline justify-between gap-3 text-sm">
          <span class="truncate" :class="b.to ? 'group-hover:underline underline-offset-4' : ''">{{ b.label }}</span>
          <span class="text-ink-soft shrink-0">{{ b.detail }}</span>
        </div>
        <div class="flex items-center gap-2">
          <div class="h-3 flex-1">
            <div class="bar h-3" :style="{ width: pct(b.value) }" />
          </div>
          <span class="num w-28 shrink-0 text-right text-sm font-semibold">{{ b.text }}</span>
        </div>
      </component>
    </li>
  </ul>
</template>

<style scoped>
.bar {
  background: var(--color-pencil);
  border-radius: 0 4px 4px 0; /* square at the baseline, rounded data end */
  transition: filter 120ms;
}
.group:hover .bar,
.group:focus-within .bar {
  filter: brightness(1.18);
}
</style>
