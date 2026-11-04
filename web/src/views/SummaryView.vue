<script setup lang="ts">
// The money a person can act on, per currency, written as ledger lines rather than headline numbers.
import { computed, onMounted, ref } from 'vue'
import { api, ApiError, type AuditStatus, type Summary } from '../api'
import { formatMinor } from '../money'
import { statusLabel } from '../labels'
import StatusMark from '../components/StatusMark.vue'

const s = ref<Summary | null>(null)
const error = ref('')
onMounted(async () => {
  try {
    s.value = await api.summary()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Özet yüklenemedi.'
  }
})

const order: AuditStatus[] = ['RED', 'YELLOW', 'PENDING', 'GREEN']
const total = computed(() => order.reduce((n, k) => n + (s.value?.invoices[k] ?? 0), 0))
</script>

<template>
  <h1 class="mb-1 text-3xl font-semibold tracking-tight">Özet</h1>
  <p class="text-ink-soft mb-8 max-w-prose">
    Bulunan fark, onaylanan istisnalar dışındaki bütün bulguların toplamıdır. İtiraz edilen kısım, tedarikçiden geri
    istenen tutardır.
  </p>
  <p v-if="error" role="alert" class="text-pencil">{{ error }}</p>
  <template v-else-if="s">
    <section class="mb-10">
      <h2 class="mb-3 text-xl font-semibold">Para birimine göre</h2>
      <p v-if="!s.by_currency.length" class="bg-sheet border-rule rounded-sheet border p-5">Henüz bulgu yok.</p>
      <div v-else class="bg-sheet border-rule rounded-sheet overflow-x-auto border">
        <table class="w-full min-w-[36rem] border-collapse">
          <thead>
            <tr class="border-rule text-ink-soft border-b text-left text-sm">
              <th scope="col" class="px-5 py-3 font-semibold">Para birimi</th>
              <th scope="col" class="px-5 py-3 text-right font-semibold">Bulunan fark</th>
              <th scope="col" class="px-5 py-3 text-right font-semibold">İtiraz edilen</th>
              <th scope="col" class="px-5 py-3 text-right font-semibold">Karar bekleyen bulgu</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in s.by_currency" :key="c.currency" class="border-rule border-b last:border-b-0">
              <th scope="row" class="px-5 py-4 text-left font-semibold">{{ c.currency === 'TRY' ? 'Türk lirası' : c.currency }}</th>
              <td class="num text-pencil px-5 py-4 text-right text-xl font-semibold">{{ formatMinor(c.caught_minor, c.currency) }}</td>
              <td class="num text-ballpoint px-5 py-4 text-right text-xl font-semibold">
                {{ formatMinor(c.disputed_minor, c.currency) }}
                <span class="text-ink-soft block text-sm font-normal">{{ c.disputed_count }} bulgu</span>
              </td>
              <td class="num px-5 py-4 text-right text-xl">{{ c.open_count }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section>
      <h2 class="mb-3 text-xl font-semibold">Faturalar</h2>
      <p class="text-ink-soft mb-3">{{ total }} fatura denetlendi.</p>
      <div v-if="total" class="rounded-field mb-4 flex h-3 overflow-hidden" role="img"
        :aria-label="order.map((k) => `${statusLabel[k]}: ${s!.invoices[k] ?? 0}`).join(', ')">
        <span v-for="k in order" :key="k" :style="{ flexGrow: s.invoices[k] ?? 0 }" :class="{
          'bg-pencil': k === 'RED', 'bg-amber': k === 'YELLOW', 'bg-ink-soft/40': k === 'PENDING', 'bg-rule': k === 'GREEN' }" />
      </div>
      <ul class="flex flex-wrap gap-x-8 gap-y-2">
        <li v-for="k in order" :key="k" class="flex items-baseline gap-2">
          <StatusMark :status="k" /> <span class="num font-semibold">{{ s.invoices[k] ?? 0 }}</span>
        </li>
      </ul>
    </section>
  </template>
</template>
