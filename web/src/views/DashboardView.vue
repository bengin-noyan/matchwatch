<script setup lang="ts">
// Masa: where the day starts. One headline figure (the difference found in the period), the trend behind it,
// where it comes from (rule, vendor), and the decisions waiting, biggest first.
import { computed, ref, watch } from 'vue'
import { api, ApiError, type Dashboard } from '../api'
import { currencyLabel, formatDate, formatMinor } from '../money'
import { ruleLabel } from '../labels'
import TrendChart, { type TrendPoint } from '../components/charts/TrendChart.vue'
import RankedBars, { type Bar } from '../components/charts/RankedBars.vue'
import StatusBar from '../components/charts/StatusBar.vue'
import { weeksBetween } from '../components/charts/scale'

type Period = '30' | '90' | 'year' | 'all'
const periods: { value: Period; label: string; phrase: string }[] = [
  { value: '30', label: 'Son 30 gün', phrase: 'Son 30 günde' },
  { value: '90', label: 'Son 90 gün', phrase: 'Son 90 günde' },
  { value: 'year', label: 'Bu yıl', phrase: 'Bu yıl' },
  { value: 'all', label: 'Tümü', phrase: 'Bugüne kadar' },
]
const period = ref<Period>('90')
const currency = ref('TRY')
const data = ref<Dashboard | null>(null)
const loading = ref(false)
const error = ref('')

const iso = (d: Date) => d.toISOString().slice(0, 10)
const today = () => iso(new Date())
function fromDate(p: Period): string | undefined {
  const d = new Date()
  if (p === 'all') return undefined
  if (p === 'year') return `${d.getFullYear()}-01-01`
  d.setDate(d.getDate() - Number(p))
  return iso(d)
}

async function load() {
  loading.value = true
  try {
    data.value = await api.dashboard(fromDate(period.value))
    error.value = ''
    const curs = data.value.by_currency.map((c) => c.currency)
    if (curs.length && !curs.includes(currency.value)) currency.value = curs[0]!
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Masa yüklenemedi.'
  } finally {
    loading.value = false
  }
}
watch(period, load, { immediate: true })

const cur = computed(() => data.value?.by_currency.find((c) => c.currency === currency.value))
const currencies = computed(() => data.value?.by_currency.map((c) => c.currency) ?? [])
const phrase = computed(() => periods.find((p) => p.value === period.value)!.phrase)
const invoiceTotal = computed(() => Object.values(data.value?.invoices ?? {}).reduce((a, b) => a + b, 0))

const trend = computed<TrendPoint[]>(() => {
  if (!data.value) return []
  const rows = data.value.weekly.filter((w) => w.currency === currency.value)
  if (!rows.length) return []
  const byWeek = new Map(rows.map((w) => [w.week, w.caught_minor]))
  const start = fromDate(period.value) ?? rows[0]!.week
  return weeksBetween(start, today()).map((week) => {
    const minor = byWeek.get(week) ?? 0
    return { x: week, value: minor / 100, text: formatMinor(minor, currency.value) }
  })
})

const ruleBars = computed<Bar[]>(() =>
  (data.value?.by_rule ?? [])
    .filter((r) => r.currency === currency.value)
    .map((r) => ({
      key: r.rule_code,
      label: ruleLabel[r.rule_code] ?? r.rule_code,
      value: r.impact_minor,
      text: formatMinor(r.impact_minor, r.currency),
      detail: `${r.count} bulgu`,
    })),
)
const vendorBars = computed<Bar[]>(() =>
  (data.value?.by_vendor ?? [])
    .filter((v) => v.currency === currency.value)
    .slice(0, 6)
    .map((v) => ({
      key: v.vendor_id,
      label: v.vendor_name,
      value: v.impact_minor,
      text: formatMinor(v.impact_minor, v.currency),
      detail: `${v.count} bulgu`,
      to: `/tedarikciler/${v.vendor_id}`,
    })),
)
</script>

<template>
  <header class="mb-6 flex flex-wrap items-center justify-between gap-4">
    <h1 class="text-3xl font-semibold tracking-tight">Masa</h1>
    <div class="flex flex-wrap items-center gap-3">
      <div role="group" aria-label="Dönem" class="border-rule bg-sheet rounded-field inline-flex border p-0.5">
        <button v-for="p in periods" :key="p.value" type="button" class="rounded px-3 py-1.5 text-sm"
          :class="period === p.value ? 'bg-ink text-paper font-semibold' : 'text-ink-soft hover:text-ink'"
          :aria-pressed="period === p.value" @click="period = p.value">{{ p.label }}</button>
      </div>
      <label v-if="currencies.length > 1" class="text-sm">
        <span class="sr-only">Para birimi</span>
        <select v-model="currency" class="field py-1.5">
          <option v-for="c in currencies" :key="c" :value="c">{{ currencyLabel(c) }}</option>
        </select>
      </label>
    </div>
  </header>

  <p v-if="error" role="alert" class="text-pencil">{{ error }}</p>

  <div v-else-if="data" class="transition-opacity" :class="loading ? 'opacity-60' : ''">
    <section class="mb-8 grid gap-6 lg:grid-cols-[1fr_22rem] lg:items-end [&>*]:min-w-0">
      <div>
        <p class="text-ink-soft text-lg">{{ phrase }} sözleşmeye ve teslimata aykırı faturalarda bulunan fark</p>
        <p class="hero mt-1">{{ formatMinor(cur?.caught_minor ?? 0, currency) }}</p>
        <dl class="mt-5 flex flex-wrap gap-x-10 gap-y-3">
          <div>
            <dt class="text-ink-soft text-sm">Tedarikçiye itiraz edilen</dt>
            <dd class="text-ballpoint text-xl font-semibold">{{ formatMinor(cur?.disputed_minor ?? 0, currency) }}</dd>
          </div>
          <div>
            <dt class="text-ink-soft text-sm">Kararını bekleyen bulgu</dt>
            <dd class="text-xl font-semibold">{{ cur?.open_count ?? 0 }}</dd>
          </div>
          <div>
            <dt class="text-ink-soft text-sm">Denetlenen fatura</dt>
            <dd class="text-xl font-semibold">{{ invoiceTotal }}</dd>
          </div>
        </dl>
      </div>
      <div class="bg-sheet border-rule rounded-sheet border p-5">
        <h2 class="mb-3 text-sm font-semibold">Fatura durumları</h2>
        <StatusBar :counts="data.invoices" />
        <RouterLink to="/faturalar" class="text-ballpoint mt-4 inline-block text-sm underline-offset-4 hover:underline">Bütün faturalar</RouterLink>
      </div>
    </section>

    <section class="mb-6 grid gap-6 lg:grid-cols-[1fr_22rem] [&>*]:min-w-0">
      <div class="bg-sheet border-rule rounded-sheet border p-5">
        <h2 class="font-semibold">Haftalara göre bulunan fark</h2>
        <p class="text-ink-soft mb-4 text-sm">Fatura tarihinin haftasına göre; onaylanan istisnalar hariç.</p>
        <TrendChart v-if="trend.length" :points="trend" title="Haftalara göre bulunan fark" :unit="currencyLabel(currency)" />
        <p v-else class="text-ink-soft py-10 text-center">Bu dönemde fark bulunmadı.</p>
      </div>
      <div class="bg-sheet border-rule rounded-sheet border p-5">
        <h2 class="font-semibold">Kararını bekleyenler</h2>
        <p class="text-ink-soft mb-3 text-sm">En büyük farklar önce.</p>
        <ol v-if="data.open_top.length" class="divide-rule -mx-2 divide-y">
          <li v-for="o in data.open_top.slice(0, 5)" :key="o.finding_id">
            <RouterLink :to="`/faturalar/${o.invoice_id}`" class="hover:bg-paper/70 rounded-field flex items-start justify-between gap-3 px-2 py-2.5">
              <span class="min-w-0">
                <span class="block truncate font-semibold">{{ ruleLabel[o.rule_code] ?? o.rule_code }}</span>
                <span class="text-ink-soft block truncate text-sm">{{ o.vendor_name }}, {{ o.invoice_no }}<template v-if="o.line_id">, satır {{ o.line_id }}</template></span>
                <span class="text-ink-soft block text-sm">{{ formatDate(o.issue_date) }}</span>
              </span>
              <span v-if="o.impact_minor" class="hand text-pencil shrink-0 text-xl">+{{ formatMinor(o.impact_minor, o.currency) }}</span>
            </RouterLink>
          </li>
        </ol>
        <p v-else class="text-ink-soft text-sm">Bu dönemde kararını bekleyen bulgu yok.</p>
        <RouterLink v-if="data.open_top.length > 5" to="/faturalar" class="text-ballpoint mt-3 inline-block text-sm underline-offset-4 hover:underline">
          {{ cur?.open_count ?? 0 }} bulgunun hepsi faturalarda
        </RouterLink>
      </div>
    </section>

    <section class="grid gap-6 lg:grid-cols-2 [&>*]:min-w-0">
      <div class="bg-sheet border-rule rounded-sheet border p-5">
        <h2 class="font-semibold">Farkın kaynağı</h2>
        <p class="text-ink-soft mb-4 text-sm">Bulgu türüne göre.</p>
        <RankedBars :bars="ruleBars" empty="Bu dönemde bulgu yok." />
      </div>
      <div class="bg-sheet border-rule rounded-sheet border p-5">
        <h2 class="font-semibold">Tedarikçiler</h2>
        <p class="text-ink-soft mb-4 text-sm">En çok fark çıkan tedarikçiler.</p>
        <RankedBars :bars="vendorBars" empty="Bu dönemde bulgu yok." />
        <RouterLink to="/tedarikciler" class="text-ballpoint mt-4 inline-block text-sm underline-offset-4 hover:underline">Bütün tedarikçiler</RouterLink>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* The one hero figure: same sans as everything, proportional digits, underlined with the red pencil. */
.hero {
  display: inline-block;
  font-size: clamp(2.75rem, 6vw, 4rem);
  font-weight: 650;
  letter-spacing: -0.02em;
  line-height: 1.05;
  font-variant-numeric: proportional-nums;
  padding-bottom: 0.18em;
  background: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 300 12' preserveAspectRatio='none'%3E%3Cpath d='M2 8 C60 3 120 10 180 6 S270 4 298 7' fill='none' stroke='%23BE1E2D' stroke-width='3' stroke-linecap='round' vector-effect='non-scaling-stroke'/%3E%3C/svg%3E")
    bottom / 100% 0.3em no-repeat;
}
</style>
