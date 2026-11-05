<script setup lang="ts">
// A contract read from a PDF, shown as an editable form. Nothing here is used by any audit until a person
// approves it (spec rule 2): every value sits next to the page and sentence it was read from.
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api, ApiError, type ContractDraft, type Draft, type DraftItem } from '../api'
import { draftStatusLabel } from '../labels'

const props = defineProps<{ id: string }>()
const router = useRouter()
const d = ref<Draft | null>(null)
const form = ref<ContractDraft | null>(null)
const error = ref('')
const busy = ref<'' | 'approve' | 'extract'>('')

function blankItem(): DraftItem {
  return { name: '', unit_price: '', currency: form.value?.currency ?? 'TRY', uom: '', requires_delivery: true }
}

async function load() {
  try {
    d.value = await api.draft(props.id)
    form.value = d.value.draft ? structuredClone(d.value.draft) : null
    error.value = ''
  } catch (e) {
    error.value = e instanceof ApiError && e.status === 404 ? 'Bu taslak bulunamadı.' : 'Taslak yüklenemedi.'
  }
}
watch(() => props.id, load, { immediate: true })

const editable = computed(() => d.value?.status === 'READY')
const warningFor = (field: string) => d.value?.warnings.filter((w) => w.field === field) ?? []
const general = computed(() => d.value?.warnings.filter((w) => !w.field) ?? [])

const fields: { key: keyof ContractDraft; label: string; type?: string; inputmode?: 'numeric' | 'decimal' }[] = [
  { key: 'contract_no', label: 'Sözleşme no' },
  { key: 'vendor_name', label: 'Tedarikçi' },
  { key: 'vendor_vkn', label: 'Tedarikçi VKN', inputmode: 'numeric' },
  { key: 'valid_from', label: 'Başlangıç', type: 'date' },
  { key: 'valid_until', label: 'Bitiş', type: 'date' },
  { key: 'currency', label: 'Para birimi' },
  { key: 'payment_term_days', label: 'Vade (gün)', type: 'number' },
  { key: 'qty_tolerance_pct', label: 'Miktar toleransı (%)', inputmode: 'decimal' },
]

async function approve() {
  if (!form.value || !d.value) return
  busy.value = 'approve'
  error.value = ''
  try {
    const doc = { ...form.value, payment_term_days: Number(form.value.payment_term_days) }
    await api.approveDraft(d.value.id, doc)
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? `Onaylanamadı: ${e.message}` : 'Onaylanamadı.'
  } finally {
    busy.value = ''
  }
}

async function reextract() {
  busy.value = 'extract'
  try {
    d.value = await api.reextract(props.id)
    form.value = d.value.draft ? structuredClone(d.value.draft) : null
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Sözleşme yeniden okunamadı.'
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <RouterLink to="/taslaklar" class="text-ink-soft hover:text-ink text-sm underline-offset-4 hover:underline">Taslaklara dön</RouterLink>
  <p v-if="error && !d" role="alert" class="text-pencil mt-4">{{ error }}</p>
  <template v-if="d">
    <header class="mt-2 mb-6 flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 class="text-3xl font-semibold tracking-tight">{{ form?.contract_no || 'Sözleşme taslağı' }}</h1>
        <p class="mt-1" :class="d.status === 'APPROVED' ? 'text-ballpoint font-semibold' : 'text-ink-soft'">{{ draftStatusLabel[d.status] }}</p>
      </div>
      <button v-if="d.status !== 'APPROVED'" type="button" class="btn-secondary" :disabled="busy !== ''" @click="reextract">
        {{ busy === 'extract' ? 'Okunuyor…' : 'PDF’i yeniden oku' }}
      </button>
    </header>

    <p v-if="d.status === 'FAILED'" class="bg-sheet border-rule rounded-sheet text-pencil border p-5">
      PDF okunamadı ({{ d.error_code }}). Dosyanın şifresiz ve okunaklı olduğundan emin olup yeniden okut.
    </p>

    <form v-if="form" class="space-y-6" @submit.prevent="approve">
      <ul v-if="general.length" class="bg-amber-wash text-amber rounded-sheet space-y-1 p-4 text-sm">
        <li v-for="(w, i) in general" :key="i">{{ w.message || w.code }}<template v-if="w.page"> (sayfa {{ w.page }})</template></li>
      </ul>

      <fieldset class="bg-sheet border-rule rounded-sheet border p-6" :disabled="!editable">
        <legend class="px-1 text-xl font-semibold">Genel şartlar</legend>
        <div class="mt-2 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <label v-for="f in fields" :key="f.key" class="block">
            <span class="mb-1 block text-sm font-semibold">{{ f.label }}</span>
            <input v-model="(form as any)[f.key]" :type="f.type ?? 'text'" :inputmode="f.inputmode" class="field num" />
            <span v-for="(w, i) in warningFor(f.key)" :key="i" class="text-amber mt-1 block text-sm">{{ w.message || w.code }}</span>
          </label>
        </div>
        <p v-if="form.payment_term_clause_text" class="text-ink-soft mt-4 text-sm">
          Vade, sayfa {{ form.payment_term_source_page }}: <q class="italic">{{ form.payment_term_clause_text }}</q>
        </p>
      </fieldset>

      <fieldset class="bg-sheet border-rule rounded-sheet border p-6" :disabled="!editable">
        <legend class="px-1 text-xl font-semibold">Kalemler ve fiyatlar</legend>
        <div class="mt-2 space-y-4">
          <div v-for="(it, i) in form.items" :key="i" class="border-rule rounded-field border p-4">
            <div class="grid gap-3 sm:grid-cols-[2fr_1fr_1fr_1fr_1fr]">
              <label class="block"><span class="mb-1 block text-sm font-semibold">Kalem adı</span><input v-model="it.name" class="field" /></label>
              <label class="block"><span class="mb-1 block text-sm font-semibold">Birim fiyat</span><input v-model="it.unit_price" inputmode="decimal" class="field num" /></label>
              <label class="block"><span class="mb-1 block text-sm font-semibold">Para birimi</span><input v-model="it.currency" class="field" /></label>
              <label class="block"><span class="mb-1 block text-sm font-semibold">Birim</span><input v-model="it.uom" class="field" /></label>
              <label class="block"><span class="mb-1 block text-sm font-semibold">Ürün kodu</span><input v-model="it.item_code" class="field" /></label>
            </div>
            <div class="mt-3 flex flex-wrap items-center justify-between gap-3 text-sm">
              <label class="inline-flex items-center gap-2">
                <input v-model="it.requires_delivery" type="checkbox" class="accent-[var(--color-ballpoint)]" />
                Teslimat gerektirir (hizmet değil)
              </label>
              <button v-if="editable" type="button" class="text-pencil underline-offset-4 hover:underline" @click="form.items.splice(i, 1)">Kalemi çıkar</button>
            </div>
            <p v-if="it.source_clause_text" class="text-ink-soft mt-2 text-sm">
              Sayfa {{ it.source_page }}: <q class="italic">{{ it.source_clause_text }}</q>
            </p>
          </div>
          <button v-if="editable" type="button" class="btn-secondary" @click="form.items.push(blankItem())">Kalem ekle</button>
        </div>
      </fieldset>

      <p v-if="error" role="alert" class="text-pencil">{{ error }}</p>
      <div v-if="editable" class="flex flex-wrap items-center gap-4">
        <button type="submit" class="btn-primary" :disabled="busy !== ''">{{ busy === 'approve' ? 'Onaylanıyor…' : 'Taslağı onayla' }}</button>
        <p class="text-ink-soft text-sm">Onayladığında sözleşme aktif olur ve bu tedarikçinin yeni faturaları buna göre denetlenir.</p>
      </div>
    </form>
  </template>
</template>
