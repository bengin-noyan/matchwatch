<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, ApiError } from '../api'

const router = useRouter()
type Kind = 'invoices' | 'despatches' | 'receipts'
const kinds: { value: Kind; label: string; hint: string }[] = [
  { value: 'invoices', label: 'e-Fatura', hint: 'Yüklenir yüklenmez sözleşme ve irsaliyeyle karşılaştırılır.' },
  { value: 'despatches', label: 'e-İrsaliye', hint: 'Teslim edilen miktarlar sonraki faturaların denetiminde kullanılır.' },
  { value: 'receipts', label: 'İrsaliye yanıtı', hint: 'Reddedilen miktarlar teslim alınandan düşülür; ilgili faturalar yeniden denetlenir.' },
]
const kind = ref<Kind>('invoices')
const xmlFile = ref<File | null>(null)
const pdfFile = ref<File | null>(null)
const busy = ref<'' | 'xml' | 'pdf'>('')
const message = ref('')
const error = ref('')

const errorText: Record<string, string> = {
  DUPLICATE_ETTN: 'Bu belge (aynı ETTN) daha önce yüklenmiş.',
  CUSTOMER_MISMATCH: 'Belgedeki alıcı VKN’si senin şirketinle eşleşmiyor.',
  DESPATCH_NOT_FOUND: 'Yanıtın ait olduğu irsaliye bulunamadı; önce irsaliyeyi yükle.',
  NOT_PDF: 'Seçilen dosya PDF değil.',
  PAYLOAD_TOO_LARGE: 'Dosya çok büyük. PDF en fazla 20 MB olabilir.',
  AI_DISABLED: 'Sözleşme okuma servisi bu kurulumda kapalı.',
}
const explain = (e: unknown) => (e instanceof ApiError ? (errorText[e.code] ?? e.message) : 'Yükleme başarısız oldu.')

function pick(target: 'xml' | 'pdf', ev: Event) {
  const f = (ev.target as HTMLInputElement).files?.[0] ?? null
  if (target === 'xml') xmlFile.value = f
  else pdfFile.value = f
  error.value = message.value = ''
}

async function uploadXML() {
  if (!xmlFile.value) return
  busy.value = 'xml'
  error.value = message.value = ''
  try {
    const res = await api.uploadXML(kind.value, xmlFile.value)
    if (kind.value === 'invoices' && res.id) return router.push(`/faturalar/${res.id}`)
    message.value = res.reaudited?.length
      ? `Yüklendi. ${res.reaudited.length} fatura yeniden denetlendi.`
      : 'Yüklendi.'
    xmlFile.value = null
  } catch (e) {
    error.value = explain(e)
  } finally {
    busy.value = ''
  }
}

async function uploadPDF() {
  if (!pdfFile.value) return
  busy.value = 'pdf'
  error.value = message.value = ''
  try {
    const d = await api.uploadContractPDF(pdfFile.value)
    router.push(`/taslaklar/${d.id}`)
  } catch (e) {
    error.value = explain(e)
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <h1 class="mb-1 text-3xl font-semibold tracking-tight">Belge yükle</h1>
  <p class="text-ink-soft mb-8 max-w-prose">
    Sözleşmeyi bir kez, irsaliye ve faturaları geldikçe yükle. Doğru sıra: sözleşme, irsaliye, fatura.
  </p>

  <p v-if="error" role="alert" class="text-pencil mb-4">{{ error }}</p>
  <p v-if="message" role="status" class="text-ballpoint mb-4">{{ message }}</p>

  <div class="grid gap-5 lg:grid-cols-2">
    <form class="bg-sheet border-rule rounded-sheet border p-6" @submit.prevent="uploadPDF">
      <h2 class="text-xl font-semibold">Sözleşme (PDF)</h2>
      <p class="text-ink-soft mt-1 mb-5 text-sm">
        Fiyatlar, vade ve tolerans PDF’ten okunur ve taslak olarak önüne gelir. Sen onaylamadan hiçbir denetimde kullanılmaz.
        Okuma bir iki dakika sürebilir.
      </p>
      <label class="mb-5 block">
        <span class="mb-1 block text-sm font-semibold">PDF dosyası</span>
        <input type="file" accept="application/pdf,.pdf" class="field" @change="pick('pdf', $event)" />
      </label>
      <button type="submit" class="btn-primary" :disabled="!pdfFile || busy !== ''">
        {{ busy === 'pdf' ? 'Sözleşme okunuyor…' : 'Sözleşmeyi oku' }}
      </button>
    </form>

    <form class="bg-sheet border-rule rounded-sheet border p-6" @submit.prevent="uploadXML">
      <h2 class="text-xl font-semibold">e-Belge (UBL-TR XML)</h2>
      <fieldset class="mt-4 mb-4">
        <legend class="mb-2 text-sm font-semibold">Belge türü</legend>
        <div class="flex flex-wrap gap-2">
          <label v-for="k in kinds" :key="k.value" class="rounded-field border-rule has-[:checked]:border-ink has-[:checked]:bg-paper cursor-pointer border px-3 py-2 text-sm">
            <input v-model="kind" type="radio" name="kind" :value="k.value" class="mr-1 accent-[var(--color-ink)]" />
            {{ k.label }}
          </label>
        </div>
        <p class="text-ink-soft mt-2 text-sm">{{ kinds.find((k) => k.value === kind)?.hint }}</p>
      </fieldset>
      <label class="mb-5 block">
        <span class="mb-1 block text-sm font-semibold">XML dosyası</span>
        <input type="file" accept=".xml,application/xml,text/xml" class="field" @change="pick('xml', $event)" />
      </label>
      <button type="submit" class="btn-primary" :disabled="!xmlFile || busy !== ''">
        {{ busy === 'xml' ? 'Yükleniyor…' : 'Yükle' }}
      </button>
    </form>
  </div>
</template>
