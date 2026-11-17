<script setup lang="ts">
// /davet#<token> and /sifre#<token>: set a password from an invite or a reset link, then land signed in. The token
// lives in the URL fragment, so it never reaches a server or proxy log; the API gets it in a POST body.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, ApiError, type LinkInfo } from '../api'
import { useSession } from '../session'
import AuthLayout from '../components/AuthLayout.vue'

const route = useRoute()
const router = useRouter()
const session = useSession()
const token = route.hash.slice(1)
const info = ref<LinkInfo | null>(null)
const invalid = ref(false)
const password = ref('')
const showPassword = ref(false)
const error = ref('')
const busy = ref(false)
const pwLeft = computed(() => Math.max(0, 12 - [...password.value].length))

const messages: Record<string, string> = {
  INVALID_LINK: 'Bu bağlantı kullanılmış ya da süresi dolmuş.',
  WEAK_PASSWORD: 'Şifre en az 12 karakter olmalı.',
  EMAIL_TAKEN: 'Bu e-postayla zaten bir hesap var. Giriş yapmayı dene.',
}

onMounted(async () => {
  if (!token) {
    invalid.value = true
    return
  }
  try {
    info.value = await api.linkInfo(token)
  } catch (e) {
    invalid.value = e instanceof ApiError && e.code === 'INVALID_LINK'
    if (!invalid.value) error.value = e instanceof ApiError ? e.message : 'Bağlantı kontrol edilemedi.'
  }
})

async function submit() {
  error.value = ''
  busy.value = true
  try {
    await session.redeem(token, password.value)
    router.replace('/')
  } catch (e) {
    error.value = e instanceof ApiError ? (messages[e.code] ?? e.message) : 'Şifre kaydedilemedi.'
  } finally {
    busy.value = false
  }
}

const title = computed(() => (info.value?.kind === 'RESET' ? 'Yeni şifreni belirle' : invalid.value ? 'Bağlantı geçersiz' : 'Davete hoş geldin'))
const lead = computed(() => {
  if (invalid.value) return 'Bağlantı kullanılmış, süresi dolmuş ya da eksik kopyalanmış.'
  if (!info.value) return ''
  return info.value.kind === 'RESET'
    ? `${info.value.email} hesabı için yeni bir şifre seç.`
    : `${info.value.tenant_name} seni matchwatch'a davet etti. Şifreni seçince hesabın açılır.`
})
</script>

<template>
  <AuthLayout :title="title" :lead="lead">
    <div v-if="invalid">
      <p class="text-ink-soft text-sm">
        Davetler 7 gün, şifre sıfırlama bağlantıları 1 saat geçerlidir ve her biri bir kez kullanılabilir. Yeni bir davet
        için seni davet eden kişiye yaz; şifren içinse yeni bir sıfırlama bağlantısı iste.
      </p>
      <RouterLink to="/sifremi-unuttum" class="btn-primary mt-5 w-full">Yeni sıfırlama bağlantısı iste</RouterLink>
      <RouterLink to="/giris" class="btn-secondary mt-3 w-full">Giriş sayfasına git</RouterLink>
    </div>
    <form v-else-if="info" @submit.prevent="submit">
      <label class="mb-4 block">
        <span class="mb-1 block text-sm font-semibold">E-posta</span>
        <input :value="info.email" type="email" autocomplete="username" readonly class="field text-ink-soft" />
      </label>
      <label class="mb-6 block">
        <span class="mb-1 flex items-center justify-between text-sm font-semibold">
          {{ info.kind === 'RESET' ? 'Yeni şifre' : 'Şifre' }}
          <button type="button" class="text-ballpoint text-xs font-semibold" :aria-pressed="showPassword" @click="showPassword = !showPassword">
            {{ showPassword ? 'Gizle' : 'Göster' }}
          </button>
        </span>
        <input v-model="password" :type="showPassword ? 'text' : 'password'" autocomplete="new-password" required minlength="12" autofocus class="field" aria-describedby="pw-hint" />
        <span id="pw-hint" class="mt-1 block text-xs" :class="pwLeft === 0 ? 'text-ballpoint' : 'text-ink-soft'">
          {{ pwLeft === 0 ? 'Şifre yeterince uzun.' : `En az 12 karakter (${pwLeft} kaldı).` }}
        </span>
      </label>
      <p v-if="error" role="alert" class="text-pencil mb-4 text-sm">{{ error }}</p>
      <button type="submit" :disabled="busy" class="btn-primary w-full">
        {{ busy ? 'Kaydediliyor…' : info.kind === 'RESET' ? 'Şifreyi kaydet ve giriş yap' : 'Hesabımı aç' }}
      </button>
    </form>
    <p v-else-if="error" role="alert" class="text-pencil text-sm">{{ error }}</p>
  </AuthLayout>
</template>
