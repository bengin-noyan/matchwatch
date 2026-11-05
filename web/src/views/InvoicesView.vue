<script setup lang="ts">
// The invoice list: the server searches, filters, sorts and pages; the filters live in the URL so the back
// button and a shared link bring the same list back.
import { computed, h, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter, type LocationQueryValue } from 'vue-router'
import { FlexRender, createColumnHelper, rowSortingFeature, tableFeatures, useTable, type SortingState } from '@tanstack/vue-table'
import { api, ApiError, type AuditStatus, type InvoiceList, type InvoiceQuery, type InvoiceRow, type VendorRow } from '../api'
import { formatDate, formatMinor } from '../money'
import { statusLabel } from '../labels'
import StatusMark from '../components/StatusMark.vue'

const PAGE = 50
const route = useRoute()
const router = useRouter()

const list = ref<InvoiceList | null>(null)
const vendors = ref<VendorRow[]>([])
const loading = ref(true)
const error = ref('')

// --- URL <-> query ------------------------------------------------------------------------------------------
type Sort = NonNullable<InvoiceQuery['sort']>
const sorts: Sort[] = ['issue_date', 'invoice_no', 'vendor', 'payable', 'status', 'open_findings']
const statuses: AuditStatus[] = ['RED', 'YELLOW', 'PENDING', 'GREEN']
const one = (v: LocationQueryValue | LocationQueryValue[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? ''
const isDate = (v: string) => /^\d{4}-\d{2}-\d{2}$/.test(v)
const isUUID = (v: string) => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(v)

const query = computed(() => {
  const q = route.query
  const status = one(q.status) as AuditStatus
  const sort = one(q.sort) as Sort
  const page = Math.max(1, Number.parseInt(one(q.page), 10) || 1)
  return {
    q: one(q.q).slice(0, 100),
    status: statuses.includes(status) ? status : undefined,
    vendor_id: isUUID(one(q.vendor)) ? one(q.vendor) : undefined,
    from: isDate(one(q.from)) ? one(q.from) : undefined,
    to: isDate(one(q.to)) ? one(q.to) : undefined,
    open: one(q.open) === '1',
    sort: sorts.includes(sort) ? sort : undefined,
    dir: one(q.dir) === 'desc' ? ('desc' as const) : undefined,
    page,
  }
})

// set merges changes into the URL; any change but paging goes back to page 1.
function set(changes: Record<string, string | undefined>, replace = false) {
  const next: Record<string, string> = {}
  for (const [k, v] of Object.entries({ ...route.query, page: undefined, ...changes })) {
    const s = one(v)
    if (s) next[k] = s
  }
  return replace ? router.replace({ query: next }) : router.push({ query: next })
}
const filtered = computed(() => !!(query.value.q || query.value.status || query.value.vendor_id || query.value.from || query.value.to || query.value.open))
const clear = () => router.push({ query: query.value.sort ? { sort: query.value.sort, dir: query.value.dir } : {} })

// --- loading ------------------------------------------------------------------------------------------------
let seq = 0
async function load() {
  const my = ++seq
  loading.value = true
  const { page, ...p } = query.value
  try {
    const res = await api.invoices({ ...p, limit: PAGE, offset: (page - 1) * PAGE })
    if (my !== seq) return // a newer request is on its way
    list.value = res
    error.value = ''
  } catch (e) {
    if (my !== seq) return
    error.value = e instanceof ApiError ? e.message : 'Faturalar yüklenemedi.'
  } finally {
    if (my === seq) loading.value = false
  }
}
watch(query, load, { immediate: true })
onMounted(async () => {
  try {
    vendors.value = (await api.vendors()).items
  } catch {
    /* the vendor filter just stays empty */
  }
})

// The search box writes to the URL after a short pause, not on every key.
const search = ref(query.value.q)
watch(() => query.value.q, (v) => { if (v !== search.value.trim()) search.value = v })
let timer: ReturnType<typeof setTimeout> | undefined
watch(search, (v) => {
  clearTimeout(timer)
  timer = setTimeout(() => { if (v.trim() !== query.value.q) set({ q: v.trim() || undefined }, true) }, 300)
})

// --- summary, chips, paging ---------------------------------------------------------------------------------
const rows = computed(() => list.value?.items ?? [])
const total = computed(() => list.value?.total ?? 0)
const allCount = computed(() => Object.values(list.value?.counts ?? {}).reduce((a, b) => a + (b ?? 0), 0))
const chips: { value: AuditStatus | undefined; label: string }[] = [
  { value: undefined, label: 'Tümü' },
  ...statuses.map((s) => ({ value: s, label: statusLabel[s] })),
]
const first = computed(() => (total.value ? (query.value.page - 1) * PAGE + 1 : 0))
const last = computed(() => Math.min(total.value, query.value.page * PAGE))
const pages = computed(() => Math.max(1, Math.ceil(total.value / PAGE)))
const goPage = (n: number) => set({ page: n > 1 ? String(n) : undefined }).then(() => window.scrollTo({ top: 0 }))
const fmtCount = (n: number) => n.toLocaleString('tr-TR')

// --- table --------------------------------------------------------------------------------------------------
const features = tableFeatures({ rowSortingFeature })
const col = createColumnHelper<typeof features, InvoiceRow>()
const numeric = { numeric: true }
// column id -> server sort key
const sortKey: Record<string, Sort> = {
  status: 'status', invoice_no: 'invoice_no', vendor_name: 'vendor', issue_date: 'issue_date', payable_minor: 'payable', open_findings: 'open_findings',
}
const columns = col.columns([
  col.accessor('audit_status', { id: 'status', header: 'Durum', cell: ({ row }) => h(StatusMark, { status: row.original.audit_status }) }),
  col.accessor('invoice_no', {
    header: 'Fatura no',
    cell: ({ row }) =>
      h(RouterLink, { to: `/faturalar/${row.original.id}`, class: 'text-ink font-semibold underline-offset-4 hover:underline' },
        () => row.original.invoice_no),
  }),
  col.accessor('vendor_name', { header: 'Tedarikçi' }),
  col.accessor('issue_date', { header: 'Tarih', cell: ({ row }) => formatDate(row.original.issue_date) }),
  col.accessor('payable_minor', { header: 'Ödenecek', meta: numeric, cell: ({ row }) => formatMinor(row.original.payable_minor, row.original.currency) }),
  col.accessor('open_findings', {
    header: 'Karar bekleyen',
    meta: numeric,
    cell: ({ row }) => (row.original.open_findings ? String(row.original.open_findings) : ''),
  }),
])
const sorting = computed<SortingState>(() => {
  const id = Object.keys(sortKey).find((k) => sortKey[k] === query.value.sort)
  return id ? [{ id, desc: query.value.dir === 'desc' }] : []
})
const table = useTable({
  features,
  columns,
  data: rows,
  manualSorting: true,
  state: { get sorting() { return sorting.value } },
  onSortingChange: (updater: SortingState | ((old: SortingState) => SortingState)) => {
    const s = (typeof updater === 'function' ? updater(sorting.value) : updater)[0]
    set({ sort: s ? sortKey[s.id] : undefined, dir: s?.desc ? 'desc' : undefined })
  },
})
function ariaSort(dir: false | 'asc' | 'desc') {
  return dir === 'asc' ? 'ascending' : dir === 'desc' ? 'descending' : 'none'
}
</script>

<template>
  <header class="mb-6 flex flex-wrap items-end justify-between gap-4">
    <div>
      <h1 class="text-3xl font-semibold tracking-tight">Faturalar</h1>
      <p v-if="list && !error" class="text-ink-soft mt-1">
        <template v-if="filtered">{{ fmtCount(total) }} fatura bu filtrelere uyuyor.</template>
        <template v-else>{{ fmtCount(total) }} fatura.</template>
      </p>
    </div>
    <RouterLink to="/yukle" class="btn-primary">Belge yükle</RouterLink>
  </header>

  <p v-if="error" role="alert" class="text-pencil mb-4">{{ error }}</p>

  <div v-if="list && !filtered && allCount === 0" class="bg-sheet border-rule rounded-sheet border p-8">
    <h2 class="text-xl font-semibold">Henüz fatura yok</h2>
    <p class="text-ink-soft mt-2 max-w-prose">
      Önce tedarikçinin sözleşmesini, sonra irsaliyeyi ve faturayı yükle. matchwatch her faturayı yüklendiği anda
      denetler ve farkları burada işaretler.
    </p>
    <RouterLink to="/yukle" class="btn-primary mt-5">Belge yükle</RouterLink>
  </div>

  <template v-else-if="list">
    <form role="search" class="bg-sheet border-rule rounded-sheet mb-4 grid gap-3 border p-4 md:grid-cols-[minmax(12rem,1fr)_minmax(12rem,1fr)_auto] md:items-end" @submit.prevent>
      <label class="block">
        <span class="text-ink-soft mb-1 block text-sm">Ara</span>
        <input v-model="search" type="search" maxlength="100" placeholder="Fatura no, tedarikçi veya ETTN" class="field" />
      </label>
      <label class="block">
        <span class="text-ink-soft mb-1 block text-sm">Tedarikçi</span>
        <select class="field" :value="query.vendor_id ?? ''" @change="set({ vendor: ($event.target as HTMLSelectElement).value || undefined })">
          <option value="">Bütün tedarikçiler</option>
          <option v-for="v in vendors" :key="v.id" :value="v.id">{{ v.name }}</option>
        </select>
      </label>
      <fieldset class="flex min-w-0 flex-wrap items-end gap-2 md:flex-nowrap">
        <legend class="text-ink-soft mb-1 text-sm">Fatura tarihi</legend>
        <label>
          <span class="sr-only">Başlangıç</span>
          <input type="date" class="field num w-[10.5rem]" :value="query.from ?? ''" :max="query.to" @change="set({ from: ($event.target as HTMLInputElement).value || undefined })" />
        </label>
        <span class="text-ink-soft pb-2" aria-hidden="true">–</span>
        <label>
          <span class="sr-only">Bitiş</span>
          <input type="date" class="field num w-[10.5rem]" :value="query.to ?? ''" :min="query.from" @change="set({ to: ($event.target as HTMLInputElement).value || undefined })" />
        </label>
      </fieldset>
    </form>

    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div role="group" aria-label="Duruma göre süz" class="flex flex-wrap gap-2">
        <button
          v-for="c in chips"
          :key="c.label"
          type="button"
          class="rounded-full border px-3 py-1 text-sm"
          :class="query.status === c.value ? 'border-ink bg-ink text-paper' : 'border-rule bg-sheet text-ink-soft hover:text-ink'"
          :aria-pressed="query.status === c.value"
          @click="set({ status: c.value })"
        >
          {{ c.label }} <span class="num opacity-75">{{ fmtCount(c.value ? (list.counts[c.value] ?? 0) : allCount) }}</span>
        </button>
      </div>
      <div class="flex flex-wrap items-center gap-4">
        <label class="flex items-center gap-2 text-sm whitespace-nowrap">
          <input type="checkbox" class="accent-ballpoint size-4" :checked="query.open" @change="set({ open: ($event.target as HTMLInputElement).checked ? '1' : undefined })" />
          Yalnızca karar bekleyenler
        </label>
        <button v-if="filtered" type="button" class="text-ink-soft hover:text-ink text-sm underline underline-offset-4" @click="clear">Filtreleri temizle</button>
      </div>
    </div>

    <div class="bg-sheet border-rule rounded-sheet overflow-x-auto border transition-opacity" :class="{ 'opacity-60': loading }" :aria-busy="loading">
      <table class="w-full min-w-[46rem] border-collapse text-sm">
        <thead>
          <tr v-for="group in table.getHeaderGroups()" :key="group.id" class="border-rule border-b">
            <th
              v-for="header in group.headers"
              :key="header.id"
              scope="col"
              :aria-sort="ariaSort(header.column.getIsSorted())"
              class="text-ink-soft px-4 py-3 font-semibold"
              :class="(header.column.columnDef.meta as any)?.numeric ? 'text-right' : 'text-left'"
            >
              <button type="button" class="inline-flex items-center gap-1 hover:text-ink" @click="header.column.getToggleSortingHandler()?.($event)">
                <FlexRender :header="header" />
                <span aria-hidden="true" class="w-3">{{ { asc: '↑', desc: '↓' }[header.column.getIsSorted() as string] ?? '' }}</span>
              </button>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in table.getRowModel().rows" :key="row.id" class="border-rule hover:bg-paper/60 border-b last:border-b-0">
            <td
              v-for="cell in row.getAllCells()"
              :key="cell.id"
              class="px-4 py-3"
              :class="(cell.column.columnDef.meta as any)?.numeric ? 'num text-right' : ''"
            >
              <FlexRender :cell="cell" />
            </td>
          </tr>
          <tr v-if="rows.length === 0 && !loading">
            <td :colspan="columns.length" class="text-ink-soft px-4 py-6">
              Bu filtrelere uyan fatura yok.
              <button v-if="filtered" type="button" class="text-ink ml-1 underline underline-offset-4" @click="clear">Filtreleri temizle</button>
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
  </template>
</template>
