<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ApiError } from '../api'
import { useSession } from '../session'
import AuthLayout from '../components/AuthLayout.vue'

const session = useSession()
const router = useRouter()
const company = ref('')
const vkn = ref('')
const email = ref('')
const password = ref('')
const showPassword = ref(false)
const error = ref('')
const busy = ref(false)

// VKN/TCKN kontrol hanelerini sunucu doğrular; burada yalnızca biçime bakıyoruz ki alan ne istediğini söyleyebilsin
const vknShape = computed(() => vkn.value === '' || /^\d{10,11}$/.test(vkn.value))
const pwLeft = computed(() => Math.max(0, 12 - [...password.value].length))

const messages: Record<string, string> = {
  INVALID_COMPANY: 'Firma adı 2 ile 200 karakter arasında olmalı.',
  INVALID_VKN: 'VKN ya da TCKN geçerli değil. Kontrol hanesini de içeren 10 ya da 11 haneyi gir.',
  INVALID_EMAIL: 'E-posta adresi geçerli değil.',
  WEAK_PASSWORD: 'Şifre en az 12 karakter olmalı.',
  EMAIL_TAKEN: 'Bu e-postayla zaten bir hesap var. Giriş yapmayı dene.',
  TOO_MANY_SIGNUPS: 'Bu bağlantıdan çok fazla kayıt denendi. Bir saat sonra tekrar dene.',
}

async function submit() {
  error.value = ''
  busy.value = true
  try {
    await session.signup({ company: company.value, vkn: vkn.value, email: email.value, password: password.value })
    router.replace('/')
  } catch (e) {
    error.value = e instanceof ApiError ? (messages[e.code] ?? e.message) : 'Hesap oluşturulamadı.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthLayout title="Firman için hesap oluştur" lead="Birkaç bilgiyle boş bir çalışma alanı açılır; ilk faturanı hemen yükleyebilirsin.">
    <form id="kayit" @submit.prevent="submit">
      <label class="mb-4 block">
        <span class="mb-1 block text-sm font-semibold">Firma adı</span>
        <input v-model="company" type="text" autocomplete="organization" required minlength="2" maxlength="200" autofocus class="field" />
      </label>
      <label class="mb-4 block">
        <span class="mb-1 block text-sm font-semibold">VKN ya da TCKN</span>
        <input
          v-model.trim="vkn"
          type="text"
          inputmode="numeric"
          autocomplete="off"
          required
          pattern="\d{10,11}"
          maxlength="11"
          class="field num"
          :aria-invalid="!vknShape"
          aria-describedby="vkn-hint"
        />
        <span id="vkn-hint" class="mt-1 block text-xs" :class="vknShape ? 'text-ink-soft' : 'text-pencil'">
          Faturalarda alıcı olarak geçen numara: şirketler için 10 haneli VKN, şahıs firmaları için 11 haneli TCKN.
        </span>
      </label>
      <label class="mb-4 block">
        <span class="mb-1 block text-sm font-semibold">İş e-postası</span>
        <input v-model="email" type="email" autocomplete="email" required maxlength="254" class="field" />
      </label>
      <label class="mb-6 block">
        <span class="mb-1 flex items-center justify-between text-sm font-semibold">
          Şifre
          <button type="button" class="text-ballpoint text-xs font-semibold" :aria-pressed="showPassword" @click="showPassword = !showPassword">
            {{ showPassword ? 'Gizle' : 'Göster' }}
          </button>
        </span>
        <input v-model="password" :type="showPassword ? 'text' : 'password'" autocomplete="new-password" required minlength="12" class="field" aria-describedby="pw-hint" />
        <span id="pw-hint" class="mt-1 block text-xs" :class="pwLeft === 0 ? 'text-ballpoint' : 'text-ink-soft'">
          {{ pwLeft === 0 ? 'Şifre yeterince uzun.' : `En az 12 karakter (${pwLeft} kaldı).` }}
        </span>
      </label>
      <p v-if="error" role="alert" class="text-pencil mb-4 text-sm">{{ error }}</p>
      <button type="submit" :disabled="busy" class="btn-primary w-full">{{ busy ? 'Hesap oluşturuluyor…' : 'Hesap oluştur' }}</button>
      <p class="text-ink-soft mt-4 text-xs">
        Firma adın, VKN'n ve e-postan yalnızca hesabını açmak ve sana giriş yaptırmak için, Türkiye'deki sunucularımızda
        tutulur (KVKK).
      </p>
      <p class="text-ink-soft border-rule mt-6 border-t pt-4 text-sm">
        Zaten hesabın var mı? <RouterLink to="/giris" class="text-ballpoint font-semibold hover:underline">Giriş yap</RouterLink>
      </p>
    </form>
  </AuthLayout>
</template>
