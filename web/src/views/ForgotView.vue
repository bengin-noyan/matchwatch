<script setup lang="ts">
// "Şifremi unuttum": the reset link goes only to the address by e-mail. The answer is the same whether or not an
// account exists, so this page cannot be used to find out who has one.
import { ref } from 'vue'
import { api, ApiError } from '../api'
import AuthLayout from '../components/AuthLayout.vue'

const email = ref('')
const sent = ref(false)
const error = ref('')
const busy = ref(false)

const messages: Record<string, string> = {
  INVALID_EMAIL: 'E-posta adresi geçerli değil.',
  TOO_MANY_RESETS: 'Çok fazla sıfırlama istendi. Bir saat sonra tekrar dene.',
  MAIL_DISABLED: 'Bu sunucuda e-posta gönderimi kapalı. Destekten sıfırlama bağlantısı iste.',
}

async function submit() {
  error.value = ''
  busy.value = true
  try {
    await api.forgot(email.value)
    sent.value = true
  } catch (e) {
    error.value = e instanceof ApiError ? (messages[e.code] ?? e.message) : 'İstek gönderilemedi.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthLayout title="Şifreni mi unuttun?" lead="E-posta adresini yaz; hesabın varsa sana bir sıfırlama bağlantısı gönderelim.">
    <div v-if="sent" role="status">
      <p class="text-ballpoint font-semibold">E-postanı kontrol et.</p>
      <p class="text-ink-soft mt-2 text-sm">
        {{ email }} ile bir hesap varsa sıfırlama bağlantısı birkaç dakika içinde gelir. Bağlantı 1 saat geçerlidir; yeni
        şifreni kaydettiğinde açık bütün oturumların kapanır. Gelmezse gereksiz klasörüne de bak.
      </p>
      <RouterLink to="/giris" class="btn-secondary mt-5 w-full">Giriş sayfasına dön</RouterLink>
    </div>
    <form v-else @submit.prevent="submit">
      <label class="mb-6 block">
        <span class="mb-1 block text-sm font-semibold">E-posta</span>
        <input v-model="email" type="email" autocomplete="username" required maxlength="254" autofocus class="field" />
      </label>
      <p v-if="error" role="alert" class="text-pencil mb-4 text-sm">{{ error }}</p>
      <button type="submit" :disabled="busy" class="btn-primary w-full">{{ busy ? 'Gönderiliyor…' : 'Sıfırlama bağlantısı gönder' }}</button>
      <p class="mt-4 text-center text-sm">
        <RouterLink to="/giris" class="text-ballpoint font-semibold hover:underline">Giriş sayfasına dön</RouterLink>
      </p>
    </form>
  </AuthLayout>
</template>
