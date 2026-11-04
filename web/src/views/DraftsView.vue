<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, ApiError, type DraftSummary } from '../api'
import { draftStatusLabel } from '../labels'

const drafts = ref<DraftSummary[]>([])
const loading = ref(true)
const error = ref('')

onMounted(async () => {
  try {
    drafts.value = (await api.drafts()).items
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Taslaklar yüklenemedi.'
  } finally {
    loading.value = false
  }
})

const when = (iso: string) =>
  new Intl.DateTimeFormat('tr-TR', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(iso))
</script>

<template>
  <header class="mb-6 flex flex-wrap items-end justify-between gap-4">
    <div>
      <h1 class="text-3xl font-semibold tracking-tight">Sözleşme taslakları</h1>
      <p class="text-ink-soft mt-1 max-w-prose">PDF’ten okunan sözleşmeler. Onayladığın taslak aktif sözleşme olur.</p>
    </div>
    <RouterLink to="/yukle" class="btn-primary">Sözleşme yükle</RouterLink>
  </header>
  <p v-if="error" role="alert" class="text-pencil">{{ error }}</p>
  <div v-else-if="!loading && !drafts.length" class="bg-sheet border-rule rounded-sheet border p-8">
    <h2 class="text-xl font-semibold">Henüz taslak yok</h2>
    <p class="text-ink-soft mt-2">Bir sözleşme PDF’i yüklediğinde okunan değerler burada incelemeni bekler.</p>
  </div>
  <ul v-else-if="!loading" class="bg-sheet border-rule rounded-sheet divide-rule divide-y border">
    <li v-for="d in drafts" :key="d.id">
      <RouterLink :to="`/taslaklar/${d.id}`" class="hover:bg-paper/60 flex flex-wrap items-baseline justify-between gap-2 px-5 py-4">
        <span>
          <span class="font-semibold">{{ d.contract_no || 'Numarası okunamadı' }}</span>
          <span v-if="d.vendor_name" class="text-ink-soft">, {{ d.vendor_name }}</span>
        </span>
        <span class="text-sm" :class="{
          'text-ballpoint font-semibold': d.status === 'APPROVED',
          'text-amber font-semibold': d.status === 'READY',
          'text-pencil': d.status === 'FAILED',
          'text-ink-soft': d.status === 'EXTRACTING',
        }">
          {{ draftStatusLabel[d.status] }} <span class="text-ink-soft font-normal">{{ when(d.created_at) }}</span>
        </span>
      </RouterLink>
    </li>
  </ul>
</template>
