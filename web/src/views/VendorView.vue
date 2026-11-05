<script setup lang="ts">
// One vendor: what was agreed (contract terms and prices), what was billed against it (price history per item),
// and the invoices themselves.
import { computed, ref, watch } from 'vue'
import { api, ApiError, type InvoiceRow, type VendorDetail } from '../api'
import { currencyLabel, formatDate, formatDecimal, formatMinor } from '../money'
import StatusMark from '../components/StatusMark.vue'
import PriceChart, { type PricePoint } from '../components/charts/PriceChart.vue'

const props = defineProps<{ id: string }>()
const v = ref<VendorDetail | null>(null)
const invoices = ref<InvoiceRow[]>([])
const invoiceTotal = ref(0)
const error = ref('')

watch(
  () => props.id,
  async (vid) => {
    try {
      const [d, inv] = await Promise.all([api.vendor(vid), api.invoices({ vendor_id: vid, sort: 'issue_date', dir: 'desc', limit: 20 })])
      v.value = d
      invoices.value = inv.items
      invoiceTotal.value = inv.total
      error.value = ''
    } catch (e) {
      error.value = e instanceof ApiError && e.status === 404 ? 'Bu tedarikçi bulunamadı.' : 'Tedarikçi yüklenemedi.'
    }
  },
  { immediate: true },
)

const active = computed(() => v.value?.contracts.find((c) => c.status === 'ACTIVE') ?? v.value?.contracts[0])
const others = computed(() => v.value?.contracts.filter((c) => c !== active.value) ?? [])

// Only invoices in the item's own currency are plotted against its price (FX lines would need a rate per point).
function history(itemId: string, currency: string): PricePoint[] {
  return (v.value?.price_history[itemId] ?? [])
    .filter((p) => p.currency === currency)
    .map((p) => ({
      invoiceId: p.invoice_id,
      invoiceNo: p.invoice_no,
      date: p.issue_date,
      price: Number(p.net_unit_price), // geometry only
      priceText: `${formatDecimal(p.net_unit_price, 2)} ${currencyLabel(p.currency)}`,
    }))
}
const aboveCount = (pts: PricePoint[], contract: string) => pts.filter((p) => p.price > Number(contract)).length
</script>

<template>
  <RouterLink to="/tedarikciler" class="text-ink-soft hover:text-ink text-sm underline-offset-4 hover:underline">Tedarikçilere dön</RouterLink>
  <p v-if="error" role="alert" class="text-pencil mt-4">{{ error }}</p>
  <template v-else-if="v">
    <header class="mt-2 mb-8 flex flex-wrap items-end justify-between gap-6">
      <div>
        <h1 class="text-3xl font-semibold tracking-tight">{{ v.name }}</h1>
        <p class="text-ink-soft num mt-1">VKN {{ v.vkn }}</p>
      </div>
      <dl class="flex flex-wrap gap-x-10 gap-y-2">
        <div v-for="c in v.caught" :key="c.currency">
          <dt class="text-ink-soft text-sm">Bulunan fark ({{ currencyLabel(c.currency) }})</dt>
          <dd class="text-pencil text-2xl font-semibold">{{ formatMinor(c.caught_minor, c.currency) }}</dd>
        </div>
        <div>
          <dt class="text-ink-soft text-sm">Fatura</dt>
          <dd class="text-2xl font-semibold">{{ v.invoice_count }}</dd>
        </div>
        <div>
          <dt class="text-ink-soft text-sm">Kararını bekleyen</dt>
          <dd class="text-2xl font-semibold">{{ v.open_findings }}</dd>
        </div>
      </dl>
    </header>

    <section v-if="active" class="mb-10">
      <div class="mb-4 flex flex-wrap items-baseline justify-between gap-2">
        <h2 class="text-xl font-semibold">Sözleşme {{ active.contract_no }}</h2>
        <p class="text-ink-soft text-sm">
          {{ formatDate(active.valid_from) }} ile {{ formatDate(active.valid_until) }} arası geçerli,
          {{ active.payment_term_days }} gün vade, %{{ formatDecimal(active.qty_tolerance_pct) }} miktar toleransı
          <template v-if="active.status !== 'ACTIVE'"> (aktif değil)</template>
        </p>
      </div>
      <div class="grid gap-5 xl:grid-cols-2 [&>*]:min-w-0">
        <article v-for="it in active.items" :key="it.id" class="bg-sheet border-rule rounded-sheet border p-5">
          <header class="mb-2 flex flex-wrap items-baseline justify-between gap-2">
            <h3 class="font-semibold">{{ it.name }}</h3>
            <p class="num text-sm"><span class="text-ink-soft">Sözleşme fiyatı</span> <strong>{{ formatDecimal(it.unit_price, 2) }} {{ currencyLabel(it.currency) }}</strong> / {{ it.uom }}</p>
          </header>
          <template v-if="history(it.id, it.currency).length">
            <p class="text-ink-soft mb-3 text-sm">
              {{ history(it.id, it.currency).length }} faturada net birim fiyat;
              <span v-if="aboveCount(history(it.id, it.currency), it.unit_price)" class="text-pencil font-semibold">{{ aboveCount(history(it.id, it.currency), it.unit_price) }} tanesi sözleşmenin üstünde</span>
              <template v-else>hepsi sözleşme fiyatında ya da altında</template>.
            </p>
            <PriceChart :points="history(it.id, it.currency)" :contract="Number(it.unit_price)"
              :contract-text="`${formatDecimal(it.unit_price, 2)} ${currencyLabel(it.currency)}`" :unit-label="it.uom" />
          </template>
          <p v-else class="text-ink-soft text-sm">Bu kalem henüz faturalanmadı.</p>
          <p v-if="it.source_clause_text" class="text-ink-soft mt-3 text-sm">Sayfa {{ it.source_page }}: <q class="italic">{{ it.source_clause_text }}</q></p>
        </article>
      </div>
      <p v-if="others.length" class="text-ink-soft mt-4 text-sm">Diğer sözleşmeler: {{ others.map((c) => c.contract_no).join(', ') }}</p>
    </section>
    <p v-else class="bg-sheet border-rule rounded-sheet mb-10 border p-5">
      Bu tedarikçinin sözleşmesi yok; faturaları yalnızca irsaliye, vade ve mükerrerlik açısından denetleniyor.
      <RouterLink to="/yukle" class="text-ballpoint underline-offset-4 hover:underline">Sözleşme yükle</RouterLink>
    </p>

    <section>
      <div class="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <h2 class="text-xl font-semibold">{{ invoiceTotal > invoices.length ? 'Son faturalar' : 'Faturalar' }}</h2>
        <RouterLink
          v-if="invoiceTotal > invoices.length"
          :to="{ path: '/faturalar', query: { vendor: props.id, sort: 'issue_date', dir: 'desc' } }"
          class="text-ballpoint text-sm underline-offset-4 hover:underline"
        >Bütün faturaları ({{ invoiceTotal.toLocaleString('tr-TR') }})</RouterLink>
      </div>
      <div class="bg-sheet border-rule rounded-sheet overflow-x-auto border">
        <table class="w-full min-w-[36rem] border-collapse text-sm">
          <thead>
            <tr class="border-rule text-ink-soft border-b text-left">
              <th scope="col" class="px-4 py-3 font-semibold">Durum</th>
              <th scope="col" class="px-4 py-3 font-semibold">Fatura no</th>
              <th scope="col" class="px-4 py-3 font-semibold">Tarih</th>
              <th scope="col" class="px-4 py-3 text-right font-semibold">Ödenecek</th>
              <th scope="col" class="px-4 py-3 text-right font-semibold">Karar bekleyen</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="i in invoices" :key="i.id" class="border-rule border-b last:border-b-0">
              <td class="px-4 py-3"><StatusMark :status="i.audit_status" /></td>
              <td class="px-4 py-3"><RouterLink :to="`/faturalar/${i.id}`" class="font-semibold underline-offset-4 hover:underline">{{ i.invoice_no }}</RouterLink></td>
              <td class="px-4 py-3">{{ formatDate(i.issue_date) }}</td>
              <td class="num px-4 py-3 text-right">{{ formatMinor(i.payable_minor, i.currency) }}</td>
              <td class="num px-4 py-3 text-right">{{ i.open_findings || '' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </template>
</template>
