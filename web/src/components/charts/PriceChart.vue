<script setup lang="ts">
// Faturaların net birim fiyatı zaman içinde, referans olarak mürekkeple çizilmiş sözleşme fiyatına karşı. Çizginin
// üstündeki noktalar denetimin yakaladıklarıdır; onlar kırmızı kalemle, diğerleri soluk mürekkeple çizilir.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { niceTicks, shortDate, tickLabel } from './scale'
import { formatDate, formatDecimal } from '../../money'

export interface PricePoint {
  invoiceId: string
  invoiceNo: string
  date: string
  price: number // yalnızca çizim için
  priceText: string
}

const props = defineProps<{ points: PricePoint[]; contract: number; contractText: string; unitLabel: string }>()

const box = ref<HTMLElement | null>(null)
const width = ref(560)
let ro: ResizeObserver | undefined
onMounted(() => {
  ro = new ResizeObserver(([e]) => (width.value = Math.max(260, e!.contentRect.width)))
  if (box.value) ro.observe(box.value)
})
onBeforeUnmount(() => ro?.disconnect())

const H = 180
const pad = { top: 14, right: 20, bottom: 26, left: 56 }
// sıfırdan başlamak yerine sözleşme fiyatının etrafındaki ilginç aralığa yakınlaş
const lo = computed(() => Math.min(props.contract, ...props.points.map((p) => p.price)) * 0.94)
const hi = computed(() => Math.max(props.contract, ...props.points.map((p) => p.price)) * 1.04)
const ticks = computed(() => {
  const t = niceTicks(hi.value - lo.value, 3)
  const step = (t[1] ?? 1) - (t[0] ?? 0)
  const start = Math.floor(lo.value / step) * step
  return t.map((_, i) => start + i * step).filter((v) => v <= hi.value + step / 2)
})
const yMin = computed(() => Math.min(lo.value, ticks.value[0] ?? lo.value))
const yMax = computed(() => Math.max(hi.value, ticks.value[ticks.value.length - 1] ?? hi.value))
// zaman ölçeği: iki fatura arasındaki boşluk, aralarındaki süre kadar geniş
const day = (iso: string) => Date.parse(`${iso}T00:00:00Z`) / 86_400_000
const t0 = computed(() => Math.min(...props.points.map((p) => day(p.date))))
const t1 = computed(() => Math.max(...props.points.map((p) => day(p.date))))
const x = (i: number) => {
  const span = t1.value - t0.value
  const w = width.value - pad.left - pad.right
  return pad.left + (span ? ((day(props.points[i]!.date) - t0.value) / span) * w : w / 2)
}
// tarih etiketleri: ilk, son ve aradaki eşit aralıklı olanlar; hiçbiri 70 pikselden daha yakın değil
const xLabels = computed(() => {
  const out: number[] = []
  props.points.forEach((_, i) => {
    if (!out.length || x(i) - x(out[out.length - 1]!) >= 70) out.push(i)
  })
  const last = props.points.length - 1
  if (last > 0 && out[out.length - 1] !== last) {
    if (x(last) - x(out[out.length - 1]!) < 70) out.pop()
    out.push(last)
  }
  return out
})
const y = (v: number) => pad.top + (1 - (v - yMin.value) / (yMax.value - yMin.value || 1)) * (H - pad.top - pad.bottom)
const line = computed(() => props.points.map((p, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(p.price).toFixed(1)}`).join(''))
const above = (p: PricePoint) => p.price > props.contract
const hover = ref<number | null>(null)
const hp = computed(() => (hover.value === null ? null : props.points[hover.value]))
</script>

<template>
  <figure class="min-w-0">
    <div ref="box" class="relative">
      <svg :viewBox="`0 0 ${width} ${H}`" :height="H" class="block w-full" role="img"
        :aria-label="`Fatura birim fiyatları, sözleşme fiyatı ${contractText}. ${points.filter(above).length} fatura sözleşmenin üstünde.`">
        <g class="text-ink-soft" font-size="12">
          <g v-for="t in ticks" :key="t">
            <line :x1="pad.left" :x2="width - pad.right" :y1="y(t)" :y2="y(t)" stroke="var(--color-rule)" stroke-width="1" />
            <text :x="pad.left - 8" :y="y(t) + 4" text-anchor="end" fill="currentColor" class="num">{{ tickLabel(t, yMax) }}</text>
          </g>
          <text v-for="i in xLabels" :key="'xl' + i" :x="x(i)" :y="H - 6" text-anchor="middle" fill="currentColor">{{ shortDate(points[i]!.date) }}</text>
        </g>
        <!-- sözleşme fiyatı: mürekkeple referans çizgisi, sağ ucunda etiketiyle -->
        <line :x1="pad.left" :x2="width - pad.right" :y1="y(contract)" :y2="y(contract)" stroke="var(--color-ink)" stroke-width="1.5" />
        <text :x="pad.left + 4" :y="y(contract) - 7" font-size="12" font-weight="600" fill="var(--color-ink)"
          stroke="var(--color-sheet)" stroke-width="4" stroke-linejoin="round" paint-order="stroke">Sözleşme {{ contractText }}</text>
        <path :d="line" fill="none" stroke="var(--color-ink-soft)" stroke-opacity="0.55" stroke-width="2" stroke-linejoin="round" />
        <g v-for="(p, i) in points" :key="p.invoiceId + 'dot' + i">
          <RouterLink :to="`/faturalar/${p.invoiceId}`" :aria-label="`${p.invoiceNo}, ${formatDate(p.date)}: ${p.priceText}`"
            @pointerenter="hover = i" @pointerleave="hover = null" @focus="hover = i" @blur="hover = null">
            <circle :cx="x(i)" :cy="y(p.price)" r="12" fill="transparent" />
            <circle :cx="x(i)" :cy="y(p.price)" :r="hover === i ? 6 : 4.5" :fill="above(p) ? 'var(--color-pencil)' : 'var(--color-ink-soft)'"
              stroke="var(--color-sheet)" stroke-width="2" />
          </RouterLink>
        </g>
      </svg>
      <div v-if="hp && hover !== null" class="bg-sheet border-rule rounded-field pointer-events-none absolute top-0 z-10 border px-3 py-2 text-sm shadow-sm"
        :style="{ left: `${Math.min(x(hover) + 12, width - 190)}px` }">
        <p class="num text-base font-semibold">{{ hp.priceText }} <span class="text-ink-soft text-sm font-normal">/ {{ unitLabel }}</span></p>
        <p class="text-ink-soft">{{ hp.invoiceNo }}, {{ formatDate(hp.date) }}</p>
        <p v-if="above(hp)" class="text-pencil">Sözleşmenin üstünde</p>
      </div>
    </div>
    <details class="mt-1 text-sm">
      <summary class="text-ink-soft hover:text-ink cursor-pointer">Tablo olarak göster</summary>
      <table class="mt-2 w-full max-w-md">
        <tbody>
          <tr v-for="p in points" :key="p.invoiceId + 'row'" class="border-rule border-t">
            <td class="py-1">{{ formatDate(p.date) }}</td>
            <td class="py-1"><RouterLink :to="`/faturalar/${p.invoiceId}`" class="underline-offset-4 hover:underline">{{ p.invoiceNo }}</RouterLink></td>
            <td class="num py-1 text-right" :class="above(p) ? 'text-pencil font-semibold' : ''">{{ p.priceText }}</td>
          </tr>
        </tbody>
      </table>
    </details>
  </figure>
</template>
