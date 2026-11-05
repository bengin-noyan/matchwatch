<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ApiError } from '../api'
import { useSession } from '../session'

const session = useSession()
const route = useRoute()
const router = useRouter()
const email = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

const messages: Record<string, string> = {
  INVALID_LOGIN: 'E-posta ya da şifre yanlış.',
  TOO_MANY_ATTEMPTS: 'Bu e-postayla çok fazla hatalı deneme yapıldı. 15 dakika sonra tekrar dene.',
}

async function submit() {
  error.value = ''
  busy.value = true
  try {
    await session.login(email.value, password.value)
    const next = typeof route.query.next === 'string' && route.query.next.startsWith('/') ? route.query.next : '/'
    router.replace(next)
  } catch (e) {
    error.value = e instanceof ApiError ? (messages[e.code] ?? e.message) : 'Giriş yapılamadı.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="grid min-h-dvh place-items-center px-4 py-10">
    <div class="w-full max-w-sm">
      <p class="text-ink mb-8 flex items-center gap-2 text-2xl font-semibold tracking-tight">
        <svg viewBox="0 0 32 32" class="size-7" aria-hidden="true">
          <path d="M7 17c2-6 15-9 18-3 2 5-6 10-12 9-5-1-8-3-6-6" fill="none" stroke="var(--color-pencil)" stroke-width="2.6" stroke-linecap="round" />
        </svg>
        matchwatch
      </p>
      <form class="bg-sheet border-rule rounded-sheet border p-6" @submit.prevent="submit">
        <h1 class="mb-1 text-xl font-semibold">Giriş yap</h1>
        <p class="text-ink-soft mb-6 text-sm">Faturalarını sözleşme ve irsaliyeyle karşılaştırmaya devam et.</p>
        <label class="mb-4 block">
          <span class="mb-1 block text-sm font-semibold">E-posta</span>
          <input v-model="email" type="email" autocomplete="username" required class="field" />
        </label>
        <label class="mb-6 block">
          <span class="mb-1 block text-sm font-semibold">Şifre</span>
          <input v-model="password" type="password" autocomplete="current-password" required class="field" />
        </label>
        <p v-if="error" role="alert" class="text-pencil mb-4 text-sm">{{ error }}</p>
        <button type="submit" :disabled="busy" class="btn-primary w-full">{{ busy ? 'Giriş yapılıyor…' : 'Giriş yap' }}</button>
      </form>
    </div>
  </main>
</template>
