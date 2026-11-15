<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ApiError } from '../api'
import { useSession } from '../session'
import AuthLayout from '../components/AuthLayout.vue'

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
  <AuthLayout title="Tekrar hoş geldin" lead="Devam etmek için e-posta adresin ve şifrenle giriş yap.">
    <form id="giris" @submit.prevent="submit">
      <label class="mb-4 block">
        <span class="mb-1 block text-sm font-semibold">E-posta</span>
        <input v-model="email" type="email" autocomplete="username" required autofocus class="field" />
      </label>
      <label class="mb-6 block">
        <span class="mb-1 block text-sm font-semibold">Şifre</span>
        <input v-model="password" type="password" autocomplete="current-password" required class="field" />
      </label>
      <p v-if="error" role="alert" class="text-pencil mb-4 text-sm">{{ error }}</p>
      <button type="submit" :disabled="busy" class="btn-primary w-full">{{ busy ? 'Giriş yapılıyor…' : 'Giriş yap' }}</button>
      <details class="text-ink-soft mt-4 text-sm">
        <summary class="text-ballpoint cursor-pointer font-semibold">Şifremi unuttum</summary>
        <p class="mt-2">
          Firmandan bir ekip arkadaşın, Kullanıcılar sayfasından sana tek kullanımlık bir şifre sıfırlama bağlantısı
          oluşturabilir. Firmanda tek kullanıcıysan destekten bağlantı iste.
        </p>
      </details>
      <p class="text-ink-soft border-rule mt-6 border-t pt-4 text-sm">
        Hesabın yok mu? <RouterLink to="/kayit" class="text-ballpoint font-semibold hover:underline">Firman için hesap oluştur</RouterLink>
      </p>
    </form>
  </AuthLayout>
</template>
