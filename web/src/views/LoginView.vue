<script setup lang="ts">
// The login page: a dark showcase of what matchwatch does on the left, the form on a sheet on the right.
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

// Every line here is a property of the product, not a claim.
const highlights = [
  { title: 'Üçlü eşleştirme', text: 'Sözleşme, e-İrsaliye ve e-Fatura satır satır karşılaştırılır.' },
  { title: 'Kuruşuna kadar kanıt', text: 'Her fark, sözleşmenin maddesi ve sayfasıyla birlikte gelir.' },
  { title: 'Kararı kişi verir', text: 'İtiraz ya da istisna; kim, ne zaman, hangi notla, hepsi saklanır.' },
  { title: 'Yerel yapay zekâ', text: 'Sözleşme PDF’leri kendi sunucularımızda okunur, tutar hesaplamaz.' },
]
</script>

<template>
  <div class="grid min-h-dvh lg:grid-cols-[1.1fr_1fr]">
    <!-- showcase -->
    <section class="showcase bg-ink text-paper relative hidden overflow-hidden p-10 lg:flex lg:flex-col xl:p-14">
      <span class="glow glow-red" aria-hidden="true" />
      <span class="glow glow-blue" aria-hidden="true" />
      <RouterLink to="/tanitim" class="relative flex items-center gap-2 text-xl font-semibold tracking-tight" aria-label="matchwatch tanıtım sayfası">
        <svg viewBox="0 0 32 32" class="size-7" aria-hidden="true">
          <path d="M7 17c2-6 15-9 18-3 2 5-6 10-12 9-5-1-8-3-6-6" fill="none" stroke="#F06B76" stroke-width="2.6" stroke-linecap="round" />
        </svg>
        matchwatch
      </RouterLink>

      <div class="relative my-auto max-w-xl py-12">
        <p class="badge mb-5 inline-flex items-center gap-2 rounded-full px-3 py-1 text-sm">
          <span class="size-2 rounded-full bg-[#F06B76]" aria-hidden="true" />
          Satın alma için iç denetim
        </p>
        <h1 class="text-4xl leading-tight font-semibold tracking-tight xl:text-5xl">
          Faturayı ödemeden önce <span class="mark">sözleşmeyle</span> karşılaştır.
        </h1>
        <p class="mt-5 text-lg opacity-80">
          Fazla fiyat, teslim alınmamış miktar, kısa vade, hatalı tevkifat: matchwatch hepsini kanıtıyla önüne koyar.
        </p>
        <ul class="mt-10 grid gap-4 sm:grid-cols-2">
          <li v-for="h in highlights" :key="h.title" class="card rounded-sheet p-4">
            <h2 class="flex items-center gap-2 font-semibold">
              <svg viewBox="0 0 24 24" class="size-5 shrink-0" aria-hidden="true">
                <path d="M5 13c1-5 12-8 14-3 2 4-5 8-9 7-4-1-6-3-4-5" fill="none" stroke="#F06B76" stroke-width="2" stroke-linecap="round" />
              </svg>
              {{ h.title }}
            </h2>
            <p class="mt-1 text-sm opacity-75">{{ h.text }}</p>
          </li>
        </ul>
      </div>

      <p class="relative flex flex-wrap gap-x-5 gap-y-1 text-sm opacity-70">
        <span>KVKK: veriler Türkiye’de</span><span>GİB UBL-TR 1.2</span><span>Firma verileri birbirinden ayrı</span>
      </p>
    </section>

    <!-- the form -->
    <main class="flex flex-col items-center justify-center px-4 py-10 sm:px-6">
      <RouterLink to="/tanitim" class="text-ink mb-8 flex items-center gap-2 text-xl font-semibold tracking-tight lg:hidden">
        <svg viewBox="0 0 32 32" class="size-7" aria-hidden="true">
          <path d="M7 17c2-6 15-9 18-3 2 5-6 10-12 9-5-1-8-3-6-6" fill="none" stroke="var(--color-pencil)" stroke-width="2.6" stroke-linecap="round" />
        </svg>
        matchwatch
      </RouterLink>
      <form id="giris" class="bg-sheet border-rule rounded-sheet w-full max-w-sm border p-6 shadow-[0_12px_40px_-12px_rgb(29_42_35/0.25)] sm:p-8" @submit.prevent="submit">
        <h2 class="mb-1 text-2xl font-semibold">Giriş yap</h2>
        <p class="text-ink-soft mb-6 text-sm">Devam etmek için e-posta adresin ve şifrenle giriş yap.</p>
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
        <p class="text-ink-soft border-rule mt-6 border-t pt-4 text-xs">Hesabın yoksa firmanın matchwatch yöneticisine başvur.</p>
      </form>
      <RouterLink to="/tanitim" class="text-ink-soft hover:text-ink mt-6 text-sm">← Tanıtım sayfasına dön</RouterLink>
    </main>
  </div>
</template>

<style scoped>
/* faint ruled paper on the dark panel, and two soft glows: red pencil and blue ballpoint */
.showcase {
  background-image: repeating-linear-gradient(to bottom, transparent 0 31px, rgb(239 243 236 / 0.05) 31px 32px);
}
.glow {
  position: absolute;
  width: 30rem;
  height: 30rem;
  border-radius: 9999px;
  filter: blur(90px);
  opacity: 0.35;
  pointer-events: none;
}
.glow-red { top: -10rem; right: -8rem; background: #be1e2d; }
.glow-blue { bottom: -12rem; left: -10rem; background: #2445a0; }
.badge { background: rgb(239 243 236 / 0.08); border: 1px solid rgb(239 243 236 / 0.15); }
.card { background: rgb(239 243 236 / 0.06); border: 1px solid rgb(239 243 236 / 0.12); }
.mark {
  background: linear-gradient(#f06b76, #f06b76) 0 92% / 100% 3px no-repeat;
}
</style>
