<script setup lang="ts">
// One finding as evidence side by side: the contract (what was agreed) on the left, the invoice (what was billed)
// on the right, the differing value circled in red pencil and its money impact noted in the margin.
// The decision row below writes in ballpoint blue.
import { computed, ref } from 'vue'
import { api, ApiError, type DetailLine, type Finding, type Resolution } from '../api'
import { currencyLabel, formatDate, formatDateTime, formatDecimal, formatMinor, formatValue } from '../money'
import { resolutionLabel, ruleLabel } from '../labels'
import PencilCircle from './PencilCircle.vue'
import ContractPage from './ContractPage.vue'

const props = defineProps<{ finding: Finding; line?: DetailLine; invoiceCurrency: string; dueDate: string | null }>()
const emit = defineEmits<{ changed: [] }>()

const ev = computed(() => props.finding.evidence)
const str = (k: string) => (typeof ev.value[k] === 'string' ? (ev.value[k] as string) : '')
const page = computed(() => (typeof ev.value.source_page === 'number' && ev.value.source_page > 0 ? ev.value.source_page : null))
const clause = computed(() => str('source_clause_text') || str('payment_term_clause_text'))
const contractNo = computed(() => str('contract_no'))
const decided = computed(() => props.finding.resolution_status !== 'OPEN')
const rule = computed(() => props.finding.rule_code)

// What each side of the comparison is called, per rule.
const sides = computed<[string, string]>(() => {
  switch (rule.value) {
    case 'PRICE_MISMATCH':
      return ['Sözleşmedeki birim fiyat', 'Faturadaki net birim fiyat']
    case 'QTY_EXCEEDS_DELIVERY':
      return ['Teslim alınan miktar', 'Faturalanan miktar']
    case 'MATURITY_VIOLATION':
      return ['Sözleşmedeki vade', 'Faturadaki vade']
    case 'WITHHOLDING_ERROR':
      return ['Beklenen tevkifat kodu', 'Faturadaki tevkifat kodu']
    case 'PRICE_CREEP':
      return ['Son 6 ayın ortalama fiyatı', 'Bu faturadaki fiyat']
    case 'UOM_MISMATCH':
      return ['Sözleşmedeki birim', 'Faturadaki birim']
    case 'FX_UNVERIFIED':
      return ['Sözleşme para birimi', 'Fatura para birimi']
    case 'FX_RATE_DEVIATION':
      return [`TCMB döviz alış kuru (${formatDate(str('tcmb_rate_date'))} bülteni)`, 'Faturadaki kur']
    default:
      return ['Beklenen', 'Faturada']
  }
})
const expected = computed(() =>
  rule.value === 'FX_UNVERIFIED' ? str('contract_currency') : formatValue(props.finding.expected),
)
const actual = computed(() => (rule.value === 'FX_UNVERIFIED' ? str('invoice_currency') : formatValue(props.finding.actual)))
const impact = computed(() =>
  props.finding.financial_impact_minor > 0
    ? formatMinor(props.finding.financial_impact_minor, props.finding.impact_currency ?? props.invoiceCurrency)
    : '',
)
const fxNote = computed(() => {
  if (str('fx_source') === 'TCMB') return `TCMB döviz alış kuru ${formatDecimal(str('fx_rate'))} (${formatDate(str('fx_rate_date'))} bülteni) ile çevrildi.`
  if (str('fx_rate')) return `Faturadaki ${formatDecimal(str('fx_rate'))} kuruyla çevrildi.`
  return ''
})
const suggestions = computed(() => ev.value.suggestions ?? [])

const showPage = ref(false)

// What the person did, for the trail (resolutionLabel names the state instead).
const decisionVerb: Record<Resolution, string> = {
  OPEN: 'kararı geri aldı',
  DISPUTED: 'itiraz etti',
  APPROVED_EXCEPTION: 'istisna olarak onayladı',
}

const note = ref(props.finding.resolution_note ?? '')
const busy = ref(false)
const error = ref('')

async function decide(status: Resolution) {
  busy.value = true
  error.value = ''
  try {
    await api.resolve(props.finding.id, status, note.value)
    emit('changed')
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Karar kaydedilemedi.'
  } finally {
    busy.value = false
  }
}

async function match(contractItemId: string) {
  if (!props.line) return
  busy.value = true
  error.value = ''
  try {
    await api.matchLine(props.line.id, contractItemId)
    emit('changed')
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Eşleştirme kaydedilemedi.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <article class="border-rule border-t py-5 first:border-t-0">
    <header class="mb-3 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
      <h3 class="text-lg font-semibold">{{ ruleLabel[rule] ?? rule }}</h3>
      <p class="text-sm" :class="decided ? 'text-ballpoint font-semibold' : 'text-ink-soft'">
        {{ resolutionLabel[finding.resolution_status] }}
      </p>
    </header>

    <div class="grid gap-3 md:grid-cols-[1fr_1fr_9rem] md:items-start">
      <!-- the contract side -->
      <section class="bg-paper/70 rounded-field border-rule border p-4" aria-label="Sözleşme">
        <template v-if="rule === 'UNCONTRACTED_ITEM'">
          <p class="text-ink-soft mb-2 text-sm">
            Sözleşme {{ contractNo }} bu kalemi içermiyor.
            <template v-if="suggestions.length">Kalemin adı şu sözleşme kalemlerine benziyor; doğru olanı seç.</template>
            <template v-else>Benzer bir sözleşme kalemi önerilemedi.</template>
          </p>
          <ul v-if="suggestions.length" class="space-y-2">
            <li v-for="s in suggestions" :key="s.contract_item_id" class="flex flex-wrap items-center justify-between gap-2">
              <span>
                {{ s.item_name }}
                <span class="text-ink-soft num text-sm">benzerlik {{ formatDecimal(s.similarity, 2) }}</span>
              </span>
              <button type="button" class="btn-secondary text-ballpoint px-3 py-1 text-sm" :disabled="busy || !line" @click="match(s.contract_item_id)">
                Bu kalemle eşleştir
              </button>
            </li>
          </ul>
        </template>
        <template v-else>
          <p class="text-ink-soft mb-2 text-sm">
            <template v-if="contractNo">Sözleşme {{ contractNo }}<template v-if="page">, sayfa {{ page }}</template></template>
            <button
              v-if="contractNo && page"
              type="button"
              class="text-ballpoint ml-2 font-semibold underline-offset-4 hover:underline"
              :aria-expanded="showPage"
              @click="showPage = !showPage"
            >{{ showPage ? 'Sayfayı gizle' : 'Sözleşmede göster' }}</button>
            <template v-else-if="rule === 'QTY_EXCEEDS_DELIVERY'">
              İrsaliyeler<template v-if="str('tolerance_pct') && str('tolerance_pct') !== '0'">, sözleşmedeki %{{ formatDecimal(str('tolerance_pct')) }} tolerans hesaba katıldı</template>
            </template>
            <template v-else>Karşılaştırma</template>
          </p>
          <blockquote v-if="clause" class="border-rule mb-3 border-l-2 pl-3 text-sm italic">{{ clause }}</blockquote>
          <p class="text-ink-soft text-sm">{{ sides[0] }}</p>
          <p class="num text-lg font-semibold">{{ expected }}</p>
        </template>
      </section>

      <!-- the invoice side -->
      <section class="bg-sheet rounded-field border-rule border p-4" aria-label="Fatura">
        <p class="text-ink-soft mb-2 text-sm">
          <template v-if="line">Fatura satırı {{ line.line_id }}: {{ line.name }}</template>
          <template v-else-if="rule === 'MATURITY_VIOLATION' && dueDate">Son ödeme tarihi {{ formatDate(dueDate) }}</template>
          <template v-else>Fatura</template>
        </p>
        <template v-if="rule === 'UNCONTRACTED_ITEM' && line">
          <p class="num">{{ formatDecimal(line.qty) }} {{ line.uom }} × {{ formatDecimal(line.unit_price, 2) }} {{ currencyLabel(invoiceCurrency) }}</p>
          <p v-if="line.item_code" class="text-ink-soft text-sm">Ürün kodu {{ line.item_code }}</p>
        </template>
        <template v-else>
          <p class="text-ink-soft text-sm">{{ sides[1] }}</p>
          <p class="num text-lg font-semibold">
            <PencilCircle :resolved="decided">{{ actual }}</PencilCircle>
          </p>
          <p v-if="fxNote" class="text-ink-soft mt-2 text-sm">{{ fxNote }}</p>
          <p v-if="ev.no_despatch_found" class="text-ink-soft mt-2 text-sm">Bu kalem için irsaliye bulunamadı.</p>
        </template>
        <p v-if="ev.match_method === 'ALIAS'" class="text-ballpoint mt-2 text-sm">Bu satır, onaylanmış bir kalem eşleşmesiyle sözleşmeye bağlandı.</p>
      </section>

      <!-- margin note -->
      <p v-if="impact" class="hand text-pencil text-2xl md:pt-8" :aria-label="`Finansal etki ${impact}`">+{{ impact }}</p>
    </div>

    <ContractPage v-if="showPage && contractNo && page" :contract-no="contractNo" :page="page" :clause="clause" />

    <form v-if="rule !== 'UNCONTRACTED_ITEM' || !suggestions.length" class="mt-4 flex flex-wrap items-center gap-2" @submit.prevent>
      <label class="min-w-[14rem] flex-1">
        <span class="sr-only">Not</span>
        <input v-model="note" type="text" maxlength="2000" placeholder="Not ekle (isteğe bağlı)" class="field text-ballpoint" />
      </label>
      <button type="button" class="btn-primary" :disabled="busy" @click="decide('DISPUTED')">İtiraz et</button>
      <button type="button" class="btn-secondary" :disabled="busy" @click="decide('APPROVED_EXCEPTION')">İstisna olarak onayla</button>
      <button v-if="decided" type="button" class="btn-secondary" :disabled="busy" @click="decide('OPEN')">Kararı geri al</button>
    </form>
    <p v-if="error" role="alert" class="text-pencil mt-2 text-sm">{{ error }}</p>

    <section v-if="finding.decisions.length" class="mt-4" aria-label="Karar geçmişi">
      <h4 class="text-ink-soft mb-1 text-sm font-semibold">Karar geçmişi</h4>
      <ol class="border-rule space-y-1 border-l-2 pl-3 text-sm">
        <li v-for="(d, i) in finding.decisions" :key="i">
          <time :datetime="d.decided_at" class="text-ink-soft num">{{ formatDateTime(d.decided_at) }}</time> · <span class="text-ballpoint">{{ d.decided_by_email ?? 'Geliştirici anahtarı' }} {{ decisionVerb[d.resolution_status] }}<template v-if="d.note">: <q class="italic">{{ d.note }}</q></template></span>
        </li>
      </ol>
    </section>
  </article>
</template>
