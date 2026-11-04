<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api, ApiError, type InvoiceDetail } from '../api'
import { formatDate, formatDecimal, formatMinor } from '../money'
import StatusMark from '../components/StatusMark.vue'
import FindingPanel from '../components/FindingPanel.vue'

const props = defineProps<{ id: string }>()
const inv = ref<InvoiceDetail | null>(null)
const error = ref('')
const busy = ref(false)

async function load() {
  try {
    inv.value = await api.invoice(props.id)
    error.value = ''
  } catch (e) {
    error.value = e instanceof ApiError && e.status === 404 ? 'Bu fatura bulunamadı.' : 'Fatura yüklenemedi.'
  }
}
watch(() => props.id, load, { immediate: true })

async function reaudit() {
  busy.value = true
  try {
    await api.reaudit(props.id)
    await load()
  } finally {
    busy.value = false
  }
}

// Findings grouped under their invoice line; header findings (vade, mükerrer) come first.
const groups = computed(() => {
  if (!inv.value) return []
  const byLine = new Map<string | null, InvoiceDetail['findings']>()
  for (const f of inv.value.findings) {
    const list = byLine.get(f.line_id) ?? []
    list.push(f)
    byLine.set(f.line_id, list)
  }
  const out: { key: string; title: string; line?: InvoiceDetail['lines'][number]; findings: InvoiceDetail['findings'] }[] = []
  const header = byLine.get(null)
  if (header) out.push({ key: 'header', title: 'Faturanın geneli', findings: header })
  for (const line of inv.value.lines) {
    const fs = byLine.get(line.line_id)
    if (fs) out.push({ key: line.id, title: `Satır ${line.line_id}: ${line.name}`, line, findings: fs })
  }
  return out
})
const openCount = computed(() => inv.value?.findings.filter((f) => f.resolution_status === 'OPEN').length ?? 0)
</script>

<template>
  <p v-if="error" role="alert" class="text-pencil">{{ error }}</p>
  <template v-else-if="inv">
    <RouterLink to="/" class="text-ink-soft hover:text-ink text-sm underline-offset-4 hover:underline">Faturalara dön</RouterLink>
    <header class="mt-2 mb-8 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="text-3xl font-semibold tracking-tight">{{ inv.invoice_no }}</h1>
        <p class="text-ink-soft mt-1">
          {{ inv.vendor_name }}, {{ formatDate(inv.issue_date) }}{{ inv.due_date ? `, son ödeme ${formatDate(inv.due_date)}` : '' }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-4">
        <div class="text-right">
          <p class="num text-2xl font-semibold">{{ formatMinor(inv.payable_minor, inv.currency) }}</p>
          <p class="text-sm"><StatusMark :status="inv.audit_status" /></p>
        </div>
        <button type="button" class="btn-secondary" :disabled="busy" @click="reaudit">{{ busy ? 'Denetleniyor…' : 'Yeniden denetle' }}</button>
      </div>
    </header>

    <section v-if="groups.length" class="mb-10">
      <h2 class="mb-1 text-xl font-semibold">Bulgular</h2>
      <p class="text-ink-soft mb-4 text-sm">
        <template v-if="openCount">{{ openCount }} bulgu kararını bekliyor.</template>
        <template v-else>Bütün bulgular için karar verildi.</template>
      </p>
      <div v-for="g in groups" :key="g.key" class="bg-sheet border-rule rounded-sheet mb-5 border px-5">
        <h3 class="border-rule text-ink-soft -mx-5 border-b px-5 py-3 text-sm font-semibold">{{ g.title }}</h3>
        <FindingPanel
          v-for="f in g.findings"
          :key="f.id"
          :finding="f"
          :line="g.line"
          :invoice-currency="inv.currency"
          :due-date="inv.due_date"
          @changed="load"
        />
      </div>
    </section>
    <p v-else class="bg-sheet border-rule rounded-sheet mb-10 border p-5">Bu faturada fark bulunmadı.</p>

    <section>
      <h2 class="mb-3 text-xl font-semibold">Satırlar</h2>
      <div class="bg-sheet border-rule rounded-sheet overflow-x-auto border">
        <table class="w-full min-w-[44rem] border-collapse text-sm">
          <thead>
            <tr class="border-rule text-ink-soft border-b text-left">
              <th scope="col" class="px-4 py-3 font-semibold">Satır</th>
              <th scope="col" class="px-4 py-3 font-semibold">Kalem</th>
              <th scope="col" class="px-4 py-3 text-right font-semibold">Miktar</th>
              <th scope="col" class="px-4 py-3 text-right font-semibold">Tutar</th>
              <th scope="col" class="px-4 py-3 font-semibold">Sözleşme kalemi</th>
              <th scope="col" class="px-4 py-3 font-semibold">İrsaliye</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="l in inv.lines" :key="l.id" class="border-rule border-b last:border-b-0">
              <td class="num px-4 py-3">{{ l.line_id }}</td>
              <td class="px-4 py-3">{{ l.name }}<span v-if="l.item_code" class="text-ink-soft block">{{ l.item_code }}</span></td>
              <td class="num px-4 py-3 text-right">{{ formatDecimal(l.qty) }} {{ l.uom }}</td>
              <td class="num px-4 py-3 text-right">{{ formatMinor(l.line_extension_minor, inv.currency) }}</td>
              <td class="px-4 py-3">
                <template v-if="l.contract_item">
                  {{ l.contract_item.name }}
                  <span class="text-ink-soft num block">{{ formatDecimal(l.contract_item.unit_price, 2) }} {{ l.contract_item.currency === 'TRY' ? 'TL' : l.contract_item.currency }}</span>
                </template>
                <span v-else class="text-ink-soft">Eşleşmedi</span>
              </td>
              <td class="px-4 py-3">
                <template v-if="l.despatch_lines?.length">
                  <span v-for="d in l.despatch_lines" :key="d.despatch_no + d.line_id" class="num block">{{ d.despatch_no }}: {{ formatDecimal(d.allocated_qty) }}</span>
                </template>
                <span v-else class="text-ink-soft">Yok</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </template>
</template>
