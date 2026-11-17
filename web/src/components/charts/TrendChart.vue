<script setup lang="ts">
// Zaman içinde tek bir seri: 2 piksellik kırmızı kalem çizgisi ve altında %10'luk gölge, en yakın haftaya yapışan bir
// dikey çizgi; ayrıca üstüne gelemeyen ya da gelmek istemeyen herkes için aynı değerler tablo olarak.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { niceTicks, shortDate, tickLabel } from './scale'

export interface TrendPoint {
  x: string // ISO tarih
  value: number // yalnızca çizim için
  text: string // kullanıcıya gösterilen biçimlendirilmiş tutar
}

const props = defineProps<{ points: TrendPoint[]; title: string; unit: string }>()

const box = ref<HTMLElement | null>(null)
const width = ref(640)
let ro: ResizeObserver | undefined
onMounted(() => {
  ro = new ResizeObserver(([e]) => (width.value = Math.max(280, e!.contentRect.width)))
  if (box.value) ro.observe(box.value)
})
onBeforeUnmount(() => ro?.disconnect())

const H = 260
const pad = { top: 16, right: 16, bottom: 28, left: 56 }
const maxV = computed(() => Math.max(0, ...props.points.map((p) => p.value)))
const ticks = computed(() => niceTicks(maxV.value))
const top = computed(() => ticks.value[ticks.value.length - 1] || 1)
const x = (i: number) =>
  pad.left + (props.points.length < 2 ? 0 : (i * (width.value - pad.left - pad.right)) / (props.points.length - 1))
const y = (v: number) => pad.top + (1 - v / top.value) * (H - pad.top - pad.bottom)

const line = computed(() => props.points.map((p, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(p.value).toFixed(1)}`).join(''))
const area = computed(() =>
  props.points.length ? `${line.value}L${x(props.points.length - 1)},${y(0)}L${x(0)},${y(0)}Z` : '',
)
// x etiketleri çakışmasın diye her k'ıncı haftaya etiket koy (her biri yaklaşık 72 piksel)
const every = computed(() => Math.max(1, Math.ceil((props.points.length * 72) / (width.value - pad.left - pad.right))))

const hover = ref<number | null>(null)
function move(ev: PointerEvent) {
  const r = (ev.currentTarget as SVGElement).getBoundingClientRect()
  const px = ((ev.clientX - r.left) / r.width) * width.value
  let best = 0
  props.points.forEach((_, i) => {
    if (Math.abs(x(i) - px) < Math.abs(x(best) - px)) best = i
  })
  hover.value = props.points.length ? best : null
}
function key(ev: KeyboardEvent) {
  if (!props.points.length) return
  const i = hover.value ?? props.points.length - 1
  if (ev.key === 'ArrowLeft') hover.value = Math.max(0, i - 1)
  else if (ev.key === 'ArrowRight') hover.value = Math.min(props.points.length - 1, i + 1)
  else return
  ev.preventDefault()
}
const hp = computed(() => (hover.value === null ? null : props.points[hover.value]))
const last = computed(() => props.points[props.points.length - 1])
</script>

<template>
  <figure class="min-w-0">
    <div ref="box" class="relative">
      <svg
        :viewBox="`0 0 ${width} ${H}`"
        :height="H"
        class="block w-full touch-none outline-none"
        role="img"
        tabindex="0"
        :aria-label="`${title}. ${points.length} hafta. Sol ve sağ ok tuşlarıyla haftalar arasında gezin.`"
        @pointermove="move"
        @pointerleave="hover = null"
        @keydown="key"
        @blur="hover = null"
      >
        <g class="text-ink-soft" font-size="12">
          <g v-for="t in ticks" :key="t">
            <line :x1="pad.left" :x2="width - pad.right" :y1="y(t)" :y2="y(t)" stroke="var(--color-rule)" stroke-width="1" />
            <text :x="pad.left - 8" :y="y(t) + 4" text-anchor="end" fill="currentColor" class="num">{{ tickLabel(t, top) }}</text>
          </g>
          <template v-for="(p, i) in points" :key="p.x">
            <text v-if="i % every === 0" :x="x(i)" :y="H - 8" text-anchor="middle" fill="currentColor">{{ shortDate(p.x) }}</text>
          </template>
        </g>
        <path :d="area" fill="var(--color-pencil)" fill-opacity="0.1" />
        <path :d="line" fill="none" stroke="var(--color-pencil)" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />
        <template v-if="hp && hover !== null">
          <line :x1="x(hover)" :x2="x(hover)" :y1="pad.top" :y2="y(0)" stroke="var(--color-ink-soft)" stroke-width="1" />
          <circle :cx="x(hover)" :cy="y(hp.value)" r="5" fill="var(--color-pencil)" stroke="var(--color-sheet)" stroke-width="2" />
        </template>
        <circle v-else-if="last" :cx="x(points.length - 1)" :cy="y(last.value)" r="4" fill="var(--color-pencil)" stroke="var(--color-sheet)" stroke-width="2" />
      </svg>
      <div
        v-if="hp && hover !== null"
        class="bg-sheet border-rule rounded-field pointer-events-none absolute top-0 z-10 border px-3 py-2 text-sm shadow-sm"
        :style="{ left: `${Math.min(x(hover) + 12, width - 180)}px` }"
        aria-live="polite"
      >
        <p class="num text-base font-semibold">{{ hp.text }}</p>
        <p class="text-ink-soft flex items-center gap-2">
          <span class="inline-block h-0.5 w-3" style="background: var(--color-pencil)" aria-hidden="true" />
          {{ shortDate(hp.x) }} haftası
        </p>
      </div>
    </div>
    <details class="mt-2 text-sm">
      <summary class="text-ink-soft hover:text-ink cursor-pointer">Tablo olarak göster</summary>
      <table class="mt-2 w-full max-w-md">
        <thead>
          <tr class="text-ink-soft text-left"><th scope="col" class="py-1 font-semibold">Hafta</th><th scope="col" class="py-1 text-right font-semibold">{{ unit }}</th></tr>
        </thead>
        <tbody>
          <tr v-for="p in points" :key="p.x" class="border-rule border-t"><td class="py-1">{{ shortDate(p.x) }}</td><td class="num py-1 text-right">{{ p.text }}</td></tr>
        </tbody>
      </table>
    </details>
  </figure>
</template>
