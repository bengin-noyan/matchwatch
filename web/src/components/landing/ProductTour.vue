<script setup lang="ts">
// Bir tarayıcı çerçevesinde, kendi kendine değişen üç ekranlık tur. Veriler örnek ve öyle olduğu yazıyor.
import { onMounted, onUnmounted, ref } from 'vue'

const tabs = [
  { id: 'masa', label: 'Masa', text: 'Açık bulgular, yakalanan tutar ve ay ay eğilim tek ekranda.' },
  { id: 'fatura', label: 'Fatura', text: 'Satır satır karşılaştırma; fark kırmızı kalemle, kanıtı yanında.' },
  { id: 'bulgular', label: 'Bulgular', text: 'Bütün faturaların bulguları; seç, tek notla karar ver.' },
] as const
const active = ref(0)
const paused = ref(false)
const PERIOD = 7000

let timer: number | undefined
const reduced = () => !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
function pick(i: number) {
  active.value = i
  restart()
}
function restart() {
  window.clearInterval(timer)
  if (reduced()) return
  timer = window.setInterval(() => {
    if (!paused.value) active.value = (active.value + 1) % tabs.length
  }, PERIOD)
}
function onKey(e: KeyboardEvent) {
  const d = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0
  if (!d) return
  pick((active.value + d + tabs.length) % tabs.length)
  document.getElementById(`tour-tab-${tabs[active.value]!.id}`)?.focus()
}
onMounted(restart)
onUnmounted(() => window.clearInterval(timer))

const bars = [38, 52, 44, 61, 47, 70, 58, 83, 66, 74, 91, 79]
const findings = [
  { rule: 'Sözleşmeden yüksek fiyat', vendor: 'Demir Hırdavat', amount: '470,00', on: true },
  { rule: 'Teslim alınandan fazla miktar', vendor: 'Kuzey Ambalaj', amount: '1.236,80', on: true },
  { rule: 'Vade sözleşmeden kısa', vendor: 'Ege Kimya', amount: '312,45', on: false },
  { rule: 'Hatalı tevkifat', vendor: 'Anadolu Lojistik', amount: '88,20', on: false },
]
</script>

<template>
  <div @mouseenter="paused = true" @mouseleave="paused = false" @focusin="paused = true" @focusout="paused = false">
    <div role="tablist" aria-label="Ekranlar" class="mb-6 grid gap-3 sm:grid-cols-3" @keydown="onKey">
      <button
        v-for="(t, i) in tabs"
        :id="`tour-tab-${t.id}`"
        :key="t.id"
        type="button"
        role="tab"
        :aria-selected="active === i"
        :aria-controls="`tour-panel-${t.id}`"
        :tabindex="active === i ? 0 : -1"
        class="tour-tab bg-sheet border-rule rounded-sheet relative overflow-hidden border p-4 text-left"
        :class="{ 'is-on': active === i }"
        @click="pick(i)"
      >
        <span class="block font-semibold">{{ t.label }}</span>
        <span class="text-ink-soft mt-1 block text-sm">{{ t.text }}</span>
        <span v-if="active === i" :key="`bar-${active}`" class="progress" :class="{ paused }" :style="{ animationDuration: `${PERIOD}ms` }" aria-hidden="true" />
      </button>
    </div>

    <div class="frame bg-sheet border-rule rounded-sheet overflow-hidden border">
      <div class="border-rule bg-paper flex items-center gap-2 border-b px-4 py-2.5" aria-hidden="true">
        <span class="bg-rule size-2.5 rounded-full" /><span class="bg-rule size-2.5 rounded-full" /><span class="bg-rule size-2.5 rounded-full" />
        <span class="bg-sheet border-rule text-ink-soft ml-3 rounded-full border px-3 py-0.5 text-xs">matchwatch / {{ tabs[active]!.label.toLowerCase() }}</span>
        <span class="text-ink-soft ml-auto text-xs">Örnek veriler</span>
      </div>

      <Transition name="swap" mode="out-in">
        <!-- Masa -->
        <div v-if="active === 0" :id="`tour-panel-masa`" key="masa" role="tabpanel" aria-labelledby="tour-tab-masa" class="grid gap-4 p-5 sm:p-6 md:grid-cols-3">
          <div v-for="k in [
            { l: 'Açık bulgu', v: '12', c: 'text-pencil' },
            { l: 'Yakalanan tutar', v: '2.107,45 TL', c: 'text-ink' },
            { l: 'Karar verilen', v: '31', c: 'text-ballpoint' },
          ]" :key="k.l" class="border-rule rounded-sheet border p-4">
            <p class="text-ink-soft text-sm">{{ k.l }}</p>
            <p class="num mt-1 text-2xl font-semibold" :class="k.c">{{ k.v }}</p>
          </div>
          <div class="border-rule rounded-sheet border p-4 md:col-span-3">
            <p class="text-ink-soft mb-3 text-sm">Aylara göre yakalanan tutar</p>
            <svg viewBox="0 0 240 64" class="h-32 w-full" preserveAspectRatio="none" aria-hidden="true">
              <line x1="0" y1="63.5" x2="240" y2="63.5" stroke="var(--color-rule)" />
              <rect v-for="(b, i) in bars" :key="i" class="bar" :x="i * 20 + 4" :y="64 - b * 0.66" width="12" :height="b * 0.66" rx="2" fill="var(--color-pencil)" :style="{ animationDelay: `${i * 45}ms` }" />
            </svg>
          </div>
        </div>

        <!-- Fatura -->
        <div v-else-if="active === 1" id="tour-panel-fatura" key="fatura" role="tabpanel" aria-labelledby="tour-tab-fatura" class="grid gap-4 p-5 sm:p-6 md:grid-cols-[1fr_17rem]">
          <div class="min-w-0 overflow-x-auto">
            <p class="text-ink-soft mb-3 text-sm">FTR2026000000026 · Demir Hırdavat</p>
            <table class="w-full min-w-[26rem] text-sm">
              <thead class="text-ink-soft text-left">
                <tr><th class="pb-2 font-normal">Kalem</th><th class="pb-2 text-right font-normal">Miktar</th><th class="pb-2 text-right font-normal">Sözleşme</th><th class="pb-2 text-right font-normal">Fatura</th></tr>
              </thead>
              <tbody class="num">
                <tr class="border-rule border-t"><td class="py-2.5">M8x20 civata</td><td class="text-right">4.700</td><td class="text-right">1,25</td><td class="text-right"><span class="circled text-pencil font-semibold">1,35</span></td></tr>
                <tr class="border-rule border-t"><td class="py-2.5">M8 somun</td><td class="text-right">4.700</td><td class="text-right">0,40</td><td class="text-right">0,40</td></tr>
                <tr class="border-rule border-t"><td class="py-2.5">Pul, 8 mm</td><td class="text-right">9.400</td><td class="text-right">0,08</td><td class="text-right">0,08</td></tr>
              </tbody>
            </table>
          </div>
          <div class="finding border-pencil bg-pencil-wash rounded-sheet border-l-4 p-4">
            <p class="text-pencil font-semibold">Sözleşmeden yüksek fiyat</p>
            <p class="num mt-1 text-2xl font-semibold">+470,00 TL</p>
            <p class="text-ink-soft mt-2 text-sm">Madde 4.1, sayfa 2: “M8x20 civata birim fiyatı 1,25 TL”</p>
            <p class="text-ballpoint mt-3 text-sm font-semibold">Sözleşmede göster →</p>
          </div>
        </div>

        <!-- Bulgular -->
        <div v-else id="tour-panel-bulgular" key="bulgular" role="tabpanel" aria-labelledby="tour-tab-bulgular" class="p-5 sm:p-6">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <p class="text-ink-soft text-sm">2 bulgu seçili</p>
            <span class="btn-primary pointer-events-none text-sm">İtiraz et</span>
          </div>
          <ul class="divide-rule divide-y text-sm">
            <li v-for="(f, i) in findings" :key="f.rule" class="row flex items-center gap-3 py-3" :style="{ animationDelay: `${i * 70}ms` }">
              <span class="grid size-4 shrink-0 place-items-center rounded border" :class="f.on ? 'bg-ballpoint border-ballpoint text-white' : 'border-rule'" aria-hidden="true">
                <svg v-if="f.on" viewBox="0 0 12 12" class="size-3"><path d="M2.5 6.5l2.2 2L9.5 3.5" fill="none" stroke="currentColor" stroke-width="1.8" /></svg>
              </span>
              <span class="min-w-0 flex-1"><span class="block truncate font-semibold">{{ f.rule }}</span><span class="text-ink-soft">{{ f.vendor }}</span></span>
              <span class="num text-pencil font-semibold">{{ f.amount }} TL</span>
            </li>
          </ul>
        </div>
      </Transition>
    </div>
  </div>
</template>

<style scoped>
.frame { box-shadow: 0 30px 60px -30px rgb(29 42 35 / 0.35); }
.tour-tab { transition: border-color 0.2s, box-shadow 0.2s; }
.tour-tab:hover { border-color: var(--color-ink-soft); }
.tour-tab.is-on { border-color: var(--color-ballpoint); box-shadow: 0 10px 30px -16px rgb(36 69 160 / 0.5); }
.progress {
  position: absolute;
  left: 0;
  bottom: 0;
  height: 3px;
  width: 100%;
  background: var(--color-ballpoint);
  transform-origin: left;
  animation: fill linear both;
}
.progress.paused { animation-play-state: paused; }
@keyframes fill {
  from { transform: scaleX(0); }
}
.swap-enter-active, .swap-leave-active { transition: opacity 0.25s, transform 0.25s; }
.swap-enter-from { opacity: 0; transform: translateY(8px); }
.swap-leave-to { opacity: 0; transform: translateY(-8px); }
.bar { transform-box: fill-box; transform-origin: bottom; animation: grow 0.6s ease-out both; }
@keyframes grow {
  from { transform: scaleY(0); }
}
.circled { padding: 0.1rem 0.4rem; border: 2px solid var(--color-pencil); border-radius: 999px; }
.finding { animation: slide 0.5s 0.15s ease-out both; }
@keyframes slide {
  from { opacity: 0; transform: translateX(12px); }
}
.row { animation: fadein 0.4s ease-out both; }
@keyframes fadein {
  from { opacity: 0; transform: translateY(6px); }
}
@media (prefers-reduced-motion: reduce) {
  .progress { display: none; }
  .bar, .finding, .row { animation: none; }
  .swap-enter-active, .swap-leave-active { transition: none; }
}
</style>
