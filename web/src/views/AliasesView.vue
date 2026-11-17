<script setup lang="ts">
// Bir kişinin onayladığı kalem eşleşmeleri ("faturadaki bu ad, sözleşmedeki şu kalem demek"). Yanlış olan buradan
// kaldırılır; ona dayanan faturalar yeniden denetlenir ve satır yeniden eşleşme ister.
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { api, ApiError, type AliasRow, type RemovedAlias, type VendorRow } from '../api'
import { currencyLabel, formatDateTime, formatDecimal } from '../money'

const route = useRoute()
const router = useRouter()
const isUUID = (v: unknown): v is string => typeof v === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(v)
const vendorId = computed(() => (isUUID(route.query.vendor) ? route.query.vendor : undefined))

const items = ref<AliasRow[] | null>(null)
const removed = ref<RemovedAlias[]>([])
const vendors = ref<VendorRow[]>([])
const error = ref('')
const done = ref('')
const confirming = ref('') // "Evet, kaldır" onayını bekleyen eşleşmenin id'si
const busy = ref(false)

async function load() {
  try {
    const res = await api.aliases(vendorId.value)
    items.value = res.items
    removed.value = res.removed
    error.value = ''
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Eşleşmeler yüklenemedi.'
  }
}
watch(vendorId, () => {
  confirming.value = ''
  done.value = ''
  load()
}, { immediate: true })
onMounted(async () => {
  try {
    vendors.value = (await api.vendors()).items
  } catch {
    /* tedarikçi filtresi boş kalır, o kadar */
  }
})

async function remove(a: AliasRow) {
  busy.value = true
  error.value = ''
  try {
    const { reaudited } = await api.deleteAlias(a.id)
    confirming.value = ''
    await load()
    done.value = reaudited.length
      ? `“${a.name_key}” eşleşmesi kaldırıldı; ${reaudited.length} fatura yeniden denetlendi.`
      : `“${a.name_key}” eşleşmesi kaldırıldı.`
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Eşleşme kaldırılamadı.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <header class="mb-6 flex flex-wrap items-end justify-between gap-4">
    <div>
      <h1 class="text-3xl font-semibold tracking-tight">Kalem eşleşmeleri</h1>
      <p class="text-ink-soft mt-1 max-w-prose">
        Faturadaki bir kalem adının hangi sözleşme kalemi olduğunu bir kişi onayladığında burada saklanır ve o
        tedarikçinin sonraki faturalarında kullanılır. Yanlış bir eşleşmeyi kaldırınca ona dayanan faturalar yeniden
        denetlenir ve satır yeniden eşleştirme ister.
      </p>
    </div>
    <label class="block w-full sm:w-64">
      <span class="text-ink-soft mb-1 block text-sm">Tedarikçi</span>
      <select class="field" :value="vendorId ?? ''" @change="router.push({ query: ($event.target as HTMLSelectElement).value ? { vendor: ($event.target as HTMLSelectElement).value } : {} })">
        <option value="">Bütün tedarikçiler</option>
        <option v-for="v in vendors" :key="v.id" :value="v.id">{{ v.name }}</option>
      </select>
    </label>
  </header>

  <p v-if="error" role="alert" class="text-pencil mb-4">{{ error }}</p>
  <p v-if="done" role="status" class="text-ballpoint mb-4 font-semibold">{{ done }}</p>

  <template v-if="items">
    <div v-if="items.length === 0" class="bg-sheet border-rule rounded-sheet border p-8">
      <h2 class="text-xl font-semibold">{{ vendorId ? 'Bu tedarikçi için eşleşme yok' : 'Henüz eşleşme yok' }}</h2>
      <p class="text-ink-soft mt-2 max-w-prose">
        Sözleşmede olmayan bir kalem bulgusunda önerilen sözleşme kalemini seçtiğinde eşleşme burada görünür.
      </p>
      <RouterLink :to="{ path: '/bulgular', query: { rule: 'UNCONTRACTED_ITEM' } }" class="btn-secondary mt-5">Eşleşme bekleyen kalemler</RouterLink>
    </div>

    <div v-else class="bg-sheet border-rule rounded-sheet overflow-x-auto border">
      <table class="w-full min-w-[46rem] border-collapse text-sm">
        <thead>
          <tr class="border-rule text-ink-soft border-b text-left">
            <th scope="col" class="px-4 py-3 font-semibold">Tedarikçi</th>
            <th scope="col" class="px-4 py-3 font-semibold">Faturadaki ad</th>
            <th scope="col" class="px-4 py-3 font-semibold">Sözleşme kalemi</th>
            <th scope="col" class="px-4 py-3 font-semibold">Onaylandı</th>
            <th scope="col" class="px-4 py-3"><span class="sr-only">İşlem</span></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="a in items" :key="a.id" class="border-rule border-b last:border-b-0">
            <td class="px-4 py-3">
              <RouterLink :to="`/tedarikciler/${a.vendor_id}`" class="underline-offset-4 hover:underline">{{ a.vendor_name }}</RouterLink>
            </td>
            <td class="px-4 py-3">
              <span class="font-semibold">{{ a.name_key }}</span> <span class="text-ink-soft">({{ a.uom }})</span>
            </td>
            <td class="px-4 py-3">
              <p class="text-ballpoint font-semibold">{{ a.contract_item.name }}</p>
              <p class="text-ink-soft num">
                Sözleşme {{ a.contract_item.contract_no }}<template v-if="a.contract_item.item_code">, kod {{ a.contract_item.item_code }}</template>,
                {{ formatDecimal(a.contract_item.unit_price, 2) }} {{ currencyLabel(a.contract_item.currency) }} / {{ a.contract_item.uom }}
              </p>
            </td>
            <td class="px-4 py-3">
              <p class="text-ink-soft num whitespace-nowrap">{{ formatDateTime(a.created_at) }}</p>
              <p v-if="a.confirmed_by_email" class="text-ballpoint">{{ a.confirmed_by_email }}</p>
            </td>
            <td class="px-4 py-3 text-right">
              <div v-if="confirming === a.id" class="flex flex-wrap justify-end gap-2">
                <button type="button" class="btn-primary px-3 py-1 text-sm" :disabled="busy" @click="remove(a)">Evet, kaldır</button>
                <button type="button" class="btn-secondary px-3 py-1 text-sm" :disabled="busy" @click="confirming = ''">Vazgeç</button>
              </div>
              <button v-else type="button" class="btn-secondary px-3 py-1 text-sm" :disabled="busy" @click="confirming = a.id">Kaldır</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <section v-if="removed.length" class="mt-10" aria-labelledby="kaldirilanlar">
      <h2 id="kaldirilanlar" class="text-xl font-semibold">Kaldırılan eşleşmeler</h2>
      <p class="text-ink-soft mt-1 mb-4 max-w-prose text-sm">Kimin hangi eşleşmeyi ne zaman kaldırdığı. Son 50 kayıt.</p>
      <div class="bg-sheet border-rule rounded-sheet overflow-x-auto border">
        <table class="w-full min-w-[46rem] border-collapse text-sm">
          <thead>
            <tr class="border-rule text-ink-soft border-b text-left">
              <th scope="col" class="px-4 py-3 font-semibold">Tedarikçi</th>
              <th scope="col" class="px-4 py-3 font-semibold">Faturadaki ad</th>
              <th scope="col" class="px-4 py-3 font-semibold">Gösterdiği sözleşme kalemi</th>
              <th scope="col" class="px-4 py-3 font-semibold">Kaldırıldı</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(r, i) in removed" :key="i" class="border-rule border-b last:border-b-0">
              <td class="px-4 py-3">{{ r.vendor_name }}</td>
              <td class="px-4 py-3">
                <span class="font-semibold">{{ r.name_key }}</span> <span class="text-ink-soft">({{ r.uom }})</span>
              </td>
              <td class="px-4 py-3">
                <p>{{ r.item_name }}</p>
                <p class="text-ink-soft num">Sözleşme {{ r.contract_no }}</p>
              </td>
              <td class="px-4 py-3">
                <p class="text-ink-soft num whitespace-nowrap">{{ formatDateTime(r.removed_at) }}</p>
                <p class="text-ballpoint">{{ r.removed_by_email ?? 'Geliştirici anahtarı' }}</p>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </template>
</template>
