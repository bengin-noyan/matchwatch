<script setup lang="ts">
// Bulgunun okunduğu sözleşme sayfası; madde, aslının üstünde kırmızı kalemle işaretli.
import { nextTick, onMounted, ref } from 'vue'
import { ApiError } from '../api'
import type { Rendered } from '../pdf/contract'

const props = defineProps<{ contractNo: string; page: number; clause: string }>()

const box = ref<HTMLDivElement>()
const canvas = ref<HTMLCanvasElement>()
const state = ref<'loading' | 'ready' | 'error'>('loading')
const message = ref('')
const result = ref<Rendered | null>(null)

onMounted(async () => {
  try {
    const { renderContractPage } = await import('../pdf/contract')
    result.value = await renderContractPage(props.contractNo, props.page, props.clause, canvas.value!, box.value!.clientWidth)
    state.value = 'ready'
    await nextTick()
    box.value?.querySelector('mark')?.scrollIntoView({ block: 'center', behavior: 'smooth' })
  } catch (e) {
    state.value = 'error'
    if (e instanceof ApiError && e.status === 404) message.value = 'Bu sözleşme PDF olmadan girilmiş; yalnızca yukarıdaki madde metni var.'
    else if (e instanceof RangeError) message.value = `PDF'te ${props.page}. sayfa yok.`
    else message.value = 'Sözleşme PDF’i açılamadı.'
  }
})
</script>

<template>
  <figure class="mt-4">
    <figcaption class="text-ink-soft mb-2 flex flex-wrap justify-between gap-x-4 text-sm">
      <span>Sözleşme {{ contractNo }}, sayfa {{ page }}<template v-if="result">/{{ result.pages }}</template></span>
      <span v-if="state === 'ready' && result && !result.boxes.length">Madde metni bu sayfanın metin katmanında bulunamadı (taranmış sayfa olabilir).</span>
    </figcaption>
    <p v-if="state === 'error'" role="status" class="bg-paper/70 rounded-field border-rule border p-4 text-sm">{{ message }}</p>
    <div v-show="state !== 'error'" ref="box" class="border-rule rounded-field max-h-[36rem] overflow-y-auto border bg-white">
      <p v-if="state === 'loading'" role="status" class="text-ink-soft p-4 text-sm">Sözleşme sayfası yükleniyor…</p>
      <div class="relative" :class="{ hidden: state === 'loading' }">
        <canvas ref="canvas" class="block w-full" role="img" :aria-label="`Sözleşme ${contractNo}, sayfa ${page}. İşaretli madde: ${clause}`" />
        <mark
          v-for="(b, i) in result?.boxes ?? []"
          :key="i"
          aria-hidden="true"
          class="border-pencil pointer-events-none absolute border-b-2 bg-[color-mix(in_srgb,var(--color-pencil)_18%,transparent)] mix-blend-multiply"
          :style="{ left: `${b.left}%`, top: `${b.top}%`, width: `${b.width}%`, height: `${b.height}%` }"
        />
      </div>
    </div>
  </figure>
</template>
