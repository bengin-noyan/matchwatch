<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, ApiError, type VendorRow } from '../api'
import { formatMinor } from '../money'

const vendors = ref<VendorRow[]>([])
const loading = ref(true)
const error = ref('')
const q = ref('')

onMounted(async () => {
  try {
    vendors.value = (await api.vendors()).items
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Tedarikçiler yüklenemedi.'
  } finally {
    loading.value = false
  }
})

const fold = (s: string) => s.toLocaleLowerCase('tr-TR')
const shown = computed(() =>
  vendors.value
    .filter((v) => !q.value || fold(v.name).includes(fold(q.value)) || v.vkn.includes(q.value))
    .sort((a, b) => (b.caught[0]?.caught_minor ?? 0) - (a.caught[0]?.caught_minor ?? 0)),
)
</script>

<template>
  <header class="mb-6 flex flex-wrap items-end justify-between gap-4">
    <div>
      <h1 class="text-3xl font-semibold tracking-tight">Tedarikçiler</h1>
      <p class="text-ink-soft mt-1">En çok fark çıkan tedarikçi en üstte.</p>
    </div>
    <label class="w-full max-w-xs">
      <span class="sr-only">Tedarikçi ara</span>
      <input v-model="q" type="search" placeholder="Ad ya da VKN ile ara" class="field" />
    </label>
  </header>
  <p v-if="error" role="alert" class="text-pencil">{{ error }}</p>
  <div v-else-if="!loading && !vendors.length" class="bg-sheet border-rule rounded-sheet border p-8">
    <h2 class="text-xl font-semibold">Henüz tedarikçi yok</h2>
    <p class="text-ink-soft mt-2">İlk sözleşmeyi ya da faturayı yüklediğinde tedarikçi burada görünür.</p>
  </div>
  <div v-else-if="!loading" class="bg-sheet border-rule rounded-sheet overflow-x-auto border">
    <table class="w-full min-w-[44rem] border-collapse text-sm">
      <thead>
        <tr class="border-rule text-ink-soft border-b text-left">
          <th scope="col" class="px-4 py-3 font-semibold">Tedarikçi</th>
          <th scope="col" class="px-4 py-3 font-semibold">Aktif sözleşme</th>
          <th scope="col" class="px-4 py-3 text-right font-semibold">Fatura</th>
          <th scope="col" class="px-4 py-3 text-right font-semibold">Karar bekleyen</th>
          <th scope="col" class="px-4 py-3 text-right font-semibold">Bulunan fark</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="v in shown" :key="v.id" class="border-rule hover:bg-paper/60 border-b last:border-b-0">
          <td class="px-4 py-3">
            <RouterLink :to="`/tedarikciler/${v.id}`" class="font-semibold underline-offset-4 hover:underline">{{ v.name }}</RouterLink>
            <span class="text-ink-soft num block">VKN {{ v.vkn }}</span>
          </td>
          <td class="px-4 py-3">{{ v.contract_no || '' }}<span v-if="!v.contract_no" class="text-ink-soft">Yok</span></td>
          <td class="num px-4 py-3 text-right">{{ v.invoice_count }}</td>
          <td class="num px-4 py-3 text-right">{{ v.open_findings || '' }}</td>
          <td class="num px-4 py-3 text-right font-semibold">
            <span v-for="c in v.caught" :key="c.currency" class="block" :class="c.caught_minor ? 'text-pencil' : 'text-ink-soft font-normal'">{{ formatMinor(c.caught_minor, c.currency) }}</span>
          </td>
        </tr>
        <tr v-if="!shown.length"><td colspan="5" class="text-ink-soft px-4 py-6">Aramaya uyan tedarikçi yok.</td></tr>
      </tbody>
    </table>
  </div>
</template>
