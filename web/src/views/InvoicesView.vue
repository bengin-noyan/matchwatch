<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import {
  FlexRender,
  createColumnHelper,
  createSortedRowModel,
  rowSortingFeature,
  sortFn_alphanumeric,
  tableFeatures,
  useTable,
  type SortingState,
} from '@tanstack/vue-table'
import { api, ApiError, type AuditStatus, type InvoiceRow } from '../api'
import { formatDate, formatMinor } from '../money'
import { statusLabel } from '../labels'
import StatusMark from '../components/StatusMark.vue'

const rows = ref<InvoiceRow[]>([])
const loading = ref(true)
const error = ref('')
const filter = ref<AuditStatus | 'ALL'>('ALL')
const sorting = ref<SortingState>([])

onMounted(async () => {
  try {
    rows.value = (await api.invoices()).items
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Faturalar yüklenemedi.'
  } finally {
    loading.value = false
  }
})

const statusRank: Record<AuditStatus, number> = { RED: 0, YELLOW: 1, PENDING: 2, GREEN: 3 }
const filters: { value: AuditStatus | 'ALL'; label: string }[] = [
  { value: 'ALL', label: 'Tümü' },
  { value: 'RED', label: statusLabel.RED },
  { value: 'YELLOW', label: statusLabel.YELLOW },
  { value: 'PENDING', label: statusLabel.PENDING },
  { value: 'GREEN', label: statusLabel.GREEN },
]
const counts = computed(() => {
  const c: Record<string, number> = { ALL: rows.value.length }
  for (const r of rows.value) c[r.audit_status] = (c[r.audit_status] ?? 0) + 1
  return c
})
const visible = computed(() => (filter.value === 'ALL' ? rows.value : rows.value.filter((r) => r.audit_status === filter.value)))
const waiting = computed(() => rows.value.filter((r) => r.open_findings > 0).length)

const features = tableFeatures({
  rowSortingFeature,
  sortedRowModel: createSortedRowModel(),
  sortFns: { alphanumeric: sortFn_alphanumeric },
})

const col = createColumnHelper<typeof features, InvoiceRow>()
const numeric = { numeric: true }
const columns = col.columns([
  col.accessor((r) => statusRank[r.audit_status], {
    id: 'status',
    header: 'Durum',
    cell: ({ row }) => h(StatusMark, { status: row.original.audit_status }),
  }),
  col.accessor('invoice_no', {
    header: 'Fatura no',
    sortFn: 'alphanumeric',
    cell: ({ row }) =>
      h(RouterLink, { to: `/faturalar/${row.original.id}`, class: 'text-ink font-semibold underline-offset-4 hover:underline' },
        () => row.original.invoice_no),
  }),
  col.accessor('vendor_name', { header: 'Tedarikçi', sortFn: 'alphanumeric' }),
  col.accessor('issue_date', { header: 'Tarih', cell: ({ row }) => formatDate(row.original.issue_date) }),
  col.accessor('payable_minor', {
    header: 'Ödenecek',
    meta: numeric,
    cell: ({ row }) => formatMinor(row.original.payable_minor, row.original.currency),
  }),
  col.accessor('open_findings', {
    header: 'Karar bekleyen',
    meta: numeric,
    cell: ({ row }) => (row.original.open_findings ? String(row.original.open_findings) : ''),
  }),
])

const table = useTable({
  features,
  columns,
  data: visible,
  state: { get sorting() { return sorting.value } },
  onSortingChange: (updater: SortingState | ((old: SortingState) => SortingState)) => {
    sorting.value = typeof updater === 'function' ? updater(sorting.value) : updater
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
      <p v-if="!loading && !error" class="text-ink-soft mt-1">
        <template v-if="waiting">{{ waiting }} faturada karar bekleyen bulgu var.</template>
        <template v-else-if="rows.length">Karar bekleyen bulgu yok.</template>
      </p>
    </div>
    <RouterLink to="/yukle" class="btn-primary">Belge yükle</RouterLink>
  </header>

  <p v-if="error" role="alert" class="text-pencil">{{ error }}</p>

  <div v-else-if="!loading && rows.length === 0" class="bg-sheet border-rule rounded-sheet border p-8">
    <h2 class="text-xl font-semibold">Henüz fatura yok</h2>
    <p class="text-ink-soft mt-2 max-w-prose">
      Önce tedarikçinin sözleşmesini, sonra irsaliyeyi ve faturayı yükle. matchwatch her faturayı yüklendiği anda
      denetler ve farkları burada işaretler.
    </p>
    <RouterLink to="/yukle" class="btn-primary mt-5">Belge yükle</RouterLink>
  </div>

  <template v-else-if="!loading">
    <div role="group" aria-label="Duruma göre süz" class="mb-4 flex flex-wrap gap-2">
      <button
        v-for="f in filters"
        :key="f.value"
        type="button"
        class="rounded-full border px-3 py-1 text-sm"
        :class="filter === f.value ? 'border-ink bg-ink text-paper' : 'border-rule bg-sheet text-ink-soft hover:text-ink'"
        :aria-pressed="filter === f.value"
        @click="filter = f.value"
      >
        {{ f.label }} <span class="num opacity-75">{{ counts[f.value] ?? 0 }}</span>
      </button>
    </div>

    <div class="bg-sheet border-rule rounded-sheet overflow-x-auto border">
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
          <tr v-if="visible.length === 0">
            <td :colspan="columns.length" class="text-ink-soft px-4 py-6">Bu durumda fatura yok.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </template>
</template>
