<script setup lang="ts">
// Findings across all invoices, for deciding many at once: tick the rows, write one note, decide.
// Filters live in the URL like the invoice list; the selection is per page and clears when the list changes.
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter, type LocationQueryValue } from 'vue-router'
import { api, ApiError, type FindingList, type FindingListRow, type Resolution, type VendorRow } from '../api'
import { formatDate, formatMinor, formatValue } from '../money'
import { resolutionLabel, ruleLabel } from '../labels'

const PAGE = 50
const route = useRoute()
const router = useRouter()

const list = ref<FindingList | null>(null)
const vendors = ref<VendorRow[]>([])
const loading = ref(true)
const error = ref('')
const selected = ref(new Set<string>()) // finding ids ticked on this page
const done = ref('') // the last batch's confirmation

// --- URL <-> query ------------------------------------------------------------------------------------------
// res: absent = waiting for a decision (the usual job), "all" = every finding
const resolutions: Resolution[] = ['OPEN', 'DISPUTED', 'APPROVED_EXCEPTION']
const one = (v: LocationQueryValue | LocationQueryValue[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? ''
const isUUID = (v: string) => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(v)

const query = computed(() => {
  const q = route.query
  const res = one(q.res)
  return {
    resolution: res === 'all' ? undefined : resolutions.includes(res as Resolution) ? (res as Resolution) : ('OPEN' as const),
    rule: /^[A-Z_]{1,40}$/.test(one(q.rule)) ? one(q.rule) : undefined,
    vendor_id: isUUID(one(q.vendor)) ? one(q.vendor) : undefined,
    page: Math.max(1, Number.parseInt(one(q.page), 10) || 1),
  }
})

function set(changes: Record<string, string | undefined>) {
  const next: Record<string, string> = {}
  for (const [k, v] of Object.entries({ ...route.query, page: undefined, ...changes })) {
    const s = one(v)
    if (s) next[k] = s
  }
  return router.push({ query: next })
}

// --- loading ------------------------------------------------------------------------------------------------
let seq = 0
async function load() {
  const my = ++seq
  loading.value = true
  const { page, ...p } = query.value
  try {
    const res = await api.findings({ ...p, limit: PAGE, offset: (page - 1) * PAGE })
    if (my !== seq) return
    list.value = res
    error.value = ''
  } catch (e) {
    if (my !== seq) return
    error.value = e instanceof ApiError ? e.message : 'Bulgular yüklenemedi.'
  } finally {
    if (my === seq) loading.value = false
  }
}
watch(query, () => {
  selected.value = new Set()
  done.value = ''
  load()
}, { immediate: true })
onMounted(async () => {
  try {
    vendors.value = (await api.vendors()).items
  } catch {
    /* the vendor filter just stays empty */
  }
})

const rows = computed(() => list.value?.items ?? [])
const total = computed(() => list.value?.total ?? 0)
const allCount = computed(() => Object.values(list.value?.counts ?? {}).reduce((a, b) => a + b, 0))
const rules = computed(() =>
  Object.entries(list.value?.counts ?? {})
    .filter(([r, n]) => n > 0 || r === query.value.rule)
    .sort((a, b) => b[1] - a[1]),
)
const resChips: { value: string | undefined; label: string }[] = [
  { value: undefined, label: 'Karar bekleyenler' },
  { value: 'DISPUTED', label: 'İtiraz edilenler' },
  { value: 'APPROVED_EXCEPTION', label: 'İstisna onaylananlar' },
  { value: 'all', label: 'Tümü' },
]
const resValue = computed(() => (query.value.resolution === 'OPEN' ? undefined : (query.value.resolution ?? 'all')))
const first = computed(() => (total.value ? (query.value.page - 1) * PAGE + 1 : 0))
const last = computed(() => Math.min(total.value, query.value.page * PAGE))
const pages = computed(() => Math.max(1, Math.ceil(total.value / PAGE)))
const goPage = (n: number) => set({ page: n > 1 ? String(n) : undefined }).then(() => window.scrollTo({ top: 0 }))
const fmtCount = (n: number) => n.toLocaleString('tr-TR')
const impact = (f: FindingListRow) =>
  f.financial_impact_minor > 0 ? formatMinor(f.financial_impact_minor, f.impact_currency ?? f.currency) : ''

// --- selection and the decision -----------------------------------------------------------------------------
const allChecked = computed(() => rows.value.length > 0 && rows.value.every((r) => selected.value.has(r.id)))
const someChecked = computed(() => selected.value.size > 0 && !allChecked.value)
function toggle(id: string) {
  const s = new Set(selected.value)
  if (!s.delete(id)) s.add(id)
  selected.value = s
}
const toggleAll = () => (selected.value = allChecked.value ? new Set() : new Set(rows.value.map((r) => r.id)))
const anyDecided = computed(() => rows.value.some((r) => selected.value.has(r.id) && r.resolution_status !== 'OPEN'))

const note = ref('')
const busy = ref(false)
async function decide(status: Resolution) {
  busy.value = true
  error.value = ''
  try {
    const { updated } = await api.decideMany([...selected.value], status, note.value)
    selected.value = new Set()
    note.value = ''
    await load()
    done.value = `${fmtCount(updated)} bulgu için karar kaydedildi.`
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Kararlar kaydedilemedi.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <header class="mb-6">
    <h1 class="text-3xl font-semibold tracking-tight">Bulgular</h1>
    <p class="text-ink-soft mt-1 max-w-prose">
      Bütün faturalardaki bulgular, finansal etkisi en büyük olan önce. Aynı kararı birden çok bulguya vermek için
      satırları seç, bir not yaz ve kararı ver; her bulgunun karar geçmişine ayrı ayrı yazılır.
    </p>
  </header>

  <p v-if="error" role="alert" class="text-pencil mb-4">{{ error }}</p>

  <template v-if="list">
    <div class="mb-3 flex flex-wrap items-end justify-between gap-3">
      <div role="group" aria-label="Karara göre süz" class="flex flex-wrap gap-2">
        <button
          v-for="c in resChips"
          :key="c.label"
          type="button"
          class="rounded-full border px-3 py-1 text-sm"
          :class="resValue === c.value ? 'border-ink bg-ink text-paper' : 'border-rule bg-sheet text-ink-soft hover:text-ink'"
          :aria-pressed="resValue === c.value"
          @click="set({ res: c.value })"
        >{{ c.label }}</button>
      </div>
      <label class="block w-full sm:w-64">
        <span class="text-ink-soft mb-1 block text-sm">Tedarikçi</span>
        <select class="field" :value="query.vendor_id ?? ''" @change="set({ vendor: ($event.target as HTMLSelectElement).value || undefined })">
          <option value="">Bütün tedarikçiler</option>
          <option v-for="v in vendors" :key="v.id" :value="v.id">{{ v.name }}</option>
        </select>
      </label>
    </div>

    <div role="group" aria-label="Kurala göre süz" class="mb-4 flex flex-wrap gap-2">
      <button
        type="button"
        class="rounded-full border px-3 py-1 text-sm"
        :class="!query.rule ? 'border-ink bg-ink text-paper' : 'border-rule bg-sheet text-ink-soft hover:text-ink'"
        :aria-pressed="!query.rule"
        @click="set({ rule: undefined })"
      >Bütün kurallar <span class="num opacity-75">{{ fmtCount(allCount) }}</span></button>
      <button
        v-for="[r, n] in rules"
        :key="r"
        type="button"
        class="rounded-full border px-3 py-1 text-sm"
        :class="query.rule === r ? 'border-ink bg-ink text-paper' : 'border-rule bg-sheet text-ink-soft hover:text-ink'"
        :aria-pressed="query.rule === r"
        @click="set({ rule: r })"
      >{{ ruleLabel[r] ?? r }} <span class="num opacity-75">{{ fmtCount(n) }}</span></button>
    </div>

    <p v-if="done" role="status" class="text-ballpoint mb-3 font-semibold">{{ done }}</p>

    <div class="bg-sheet border-rule rounded-sheet overflow-x-auto border transition-opacity" :class="{ 'opacity-60': loading }" :aria-busy="loading">
      <table class="w-full min-w-[52rem] border-collapse text-sm">
        <thead>
          <tr class="border-rule text-ink-soft border-b text-left">
            <th scope="col" class="w-10 px-4 py-3">
              <input
                type="checkbox"
                class="accent-ballpoint size-4"
                aria-label="Bu sayfadaki bütün bulguları seç"
                :checked="allChecked"
                :indeterminate="someChecked"
                :disabled="!rows.length"
                @change="toggleAll"
              />
            </th>
            <th scope="col" class="px-4 py-3 font-semibold">Bulgu</th>
            <th scope="col" class="px-4 py-3 font-semibold">Fatura</th>
            <th scope="col" class="px-4 py-3 font-semibold">Beklenen</th>
            <th scope="col" class="px-4 py-3 font-semibold">Faturada</th>
            <th scope="col" class="px-4 py-3 text-right font-semibold">Etki</th>
            <th scope="col" class="px-4 py-3 font-semibold">Karar</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="f in rows"
            :key="f.id"
            class="border-rule border-b last:border-b-0"
            :class="selected.has(f.id) ? 'bg-ballpoint/5' : 'hover:bg-paper/60'"
          >
            <td class="px-4 py-3">
              <input
                type="checkbox"
                class="accent-ballpoint size-4"
                :aria-label="`${ruleLabel[f.rule_code] ?? f.rule_code}, ${f.invoice_no}`"
                :checked="selected.has(f.id)"
                @change="toggle(f.id)"
              />
            </td>
            <td class="px-4 py-3">
              <p class="font-semibold">{{ ruleLabel[f.rule_code] ?? f.rule_code }}</p>
              <p v-if="f.line_id" class="text-ink-soft">Satır {{ f.line_id }}<template v-if="f.item_name">: {{ f.item_name }}</template></p>
            </td>
            <td class="px-4 py-3">
              <RouterLink :to="`/faturalar/${f.invoice_id}`" class="font-semibold underline-offset-4 hover:underline">{{ f.invoice_no }}</RouterLink>
              <p class="text-ink-soft">{{ f.vendor_name }}, {{ formatDate(f.issue_date) }}</p>
            </td>
            <td class="num px-4 py-3">{{ formatValue(f.expected) }}</td>
            <td class="num px-4 py-3">{{ formatValue(f.actual) }}</td>
            <td class="num text-pencil px-4 py-3 text-right whitespace-nowrap">{{ impact(f) }}</td>
            <td class="px-4 py-3">
              <span :class="f.resolution_status === 'OPEN' ? 'text-ink-soft' : 'text-ballpoint font-semibold'">{{ resolutionLabel[f.resolution_status] }}</span>
              <p v-if="f.resolution_note" class="text-ballpoint italic">{{ f.resolution_note }}</p>
            </td>
          </tr>
          <tr v-if="rows.length === 0 && !loading">
            <td colspan="7" class="text-ink-soft px-4 py-6">
              <template v-if="query.resolution === 'OPEN' && !query.rule && !query.vendor_id">Karar bekleyen bulgu yok.</template>
              <template v-else>Bu filtrelere uyan bulgu yok.</template>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <nav v-if="total > PAGE" aria-label="Sayfalar" class="mt-4 flex flex-wrap items-center justify-between gap-3 text-sm">
      <p class="text-ink-soft num">{{ fmtCount(first) }}–{{ fmtCount(last) }} / {{ fmtCount(total) }}</p>
      <div class="flex items-center gap-2">
        <button type="button" class="btn-secondary px-3 py-1" :disabled="query.page <= 1 || loading" @click="goPage(query.page - 1)">Önceki</button>
        <span class="num text-ink-soft" aria-current="page">Sayfa {{ query.page }} / {{ pages }}</span>
        <button type="button" class="btn-secondary px-3 py-1" :disabled="query.page >= pages || loading" @click="goPage(query.page + 1)">Sonraki</button>
      </div>
    </nav>

    <!-- the decision bar stays in reach while the person scrolls the list -->
    <form
      v-if="selected.size"
      aria-label="Seçili bulgular için karar"
      class="bg-sheet border-ballpoint/40 rounded-sheet sticky bottom-4 mt-4 flex flex-wrap items-center gap-2 border p-3 shadow-lg"
      @submit.prevent
    >
      <p class="text-ballpoint font-semibold whitespace-nowrap">{{ fmtCount(selected.size) }} bulgu seçildi</p>
      <label class="min-w-[12rem] flex-1">
        <span class="sr-only">Not</span>
        <input v-model="note" type="text" maxlength="2000" placeholder="Not ekle (isteğe bağlı, hepsine yazılır)" class="field text-ballpoint" />
      </label>
      <button type="button" class="btn-primary" :disabled="busy" @click="decide('DISPUTED')">İtiraz et</button>
      <button type="button" class="btn-secondary" :disabled="busy" @click="decide('APPROVED_EXCEPTION')">İstisna olarak onayla</button>
      <button v-if="anyDecided" type="button" class="btn-secondary" :disabled="busy" @click="decide('OPEN')">Kararı geri al</button>
      <button type="button" class="text-ink-soft hover:text-ink text-sm underline underline-offset-4" :disabled="busy" @click="selected = new Set()">Seçimi temizle</button>
    </form>
  </template>
</template>
