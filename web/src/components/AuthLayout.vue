<script setup lang="ts">
// /giris ve /kayit sayfalarının ortak çerçevesi: solda matchwatch'ın ne yaptığını gösteren koyu bir vitrin, sağda bir
// kâğıdın üstünde form ve üstünde "Giriş yap | Kaydol" geçişi.
defineProps<{ title: string; lead: string }>()

// Buradaki her satır ürünün bir özelliği, reklam cümlesi değil.
const highlights = [
  { title: 'Üçlü eşleştirme', text: 'Sözleşme, e-İrsaliye ve e-Fatura satır satır karşılaştırılır.' },
  { title: 'Kuruşuna kadar kanıt', text: 'Her fark, sözleşmenin maddesi ve sayfasıyla birlikte gelir.' },
  { title: 'Kararı kişi verir', text: 'İtiraz ya da istisna; kim, ne zaman, hangi notla, hepsi saklanır.' },
  { title: 'Yerel yapay zekâ', text: 'Sözleşme PDF’leri kendi sunucularımızda okunur, tutar hesaplamaz.' },
]
</script>

<template>
  <div class="grid min-h-dvh lg:grid-cols-[1.1fr_1fr]">
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
          <span class="pulse size-2 rounded-full bg-[#F06B76]" aria-hidden="true" />
          Satın alma için iç denetim
        </p>
        <p class="text-4xl leading-tight font-semibold tracking-tight xl:text-5xl">
          Faturayı ödemeden önce <span class="mark">sözleşmeyle</span> karşılaştır.
        </p>
        <p class="mt-5 text-lg opacity-80">
          Fazla fiyat, teslim alınmamış miktar, kısa vade, hatalı tevkifat: matchwatch hepsini kanıtıyla önüne koyar.
        </p>
        <ul class="mt-10 grid gap-4 sm:grid-cols-2">
          <li v-for="(h, i) in highlights" :key="h.title" class="card rounded-sheet p-4" :style="{ '--d': `${200 + i * 90}ms` }">
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

    <main class="flex flex-col items-center justify-center px-4 py-10 sm:px-6">
      <RouterLink to="/tanitim" class="text-ink mb-8 flex items-center gap-2 text-xl font-semibold tracking-tight lg:hidden">
        <svg viewBox="0 0 32 32" class="size-7" aria-hidden="true">
          <path d="M7 17c2-6 15-9 18-3 2 5-6 10-12 9-5-1-8-3-6-6" fill="none" stroke="var(--color-pencil)" stroke-width="2.6" stroke-linecap="round" />
        </svg>
        matchwatch
      </RouterLink>
      <div class="sheet bg-sheet border-rule rounded-sheet w-full max-w-md border p-6 shadow-[0_12px_40px_-12px_rgb(29_42_35/0.25)] sm:p-8">
        <nav aria-label="Hesap" class="bg-paper border-rule mb-6 grid grid-cols-2 rounded-full border p-1 text-sm font-semibold">
          <RouterLink to="/giris" class="tab rounded-full py-2 text-center" exact-active-class="is-on">Giriş yap</RouterLink>
          <RouterLink to="/kayit" class="tab rounded-full py-2 text-center" exact-active-class="is-on">Kaydol</RouterLink>
        </nav>
        <h1 class="mb-1 text-2xl font-semibold">{{ title }}</h1>
        <p class="text-ink-soft mb-6 text-sm">{{ lead }}</p>
        <slot />
      </div>
      <RouterLink to="/tanitim" class="text-ink-soft hover:text-ink mt-6 text-sm">← Tanıtım sayfasına dön</RouterLink>
    </main>
  </div>
</template>

<style scoped>
/* koyu panelde çok silik çizgili kâğıt ve iki yumuşak parıltı: kırmızı kalem ve mavi tükenmez */
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
  animation: float 18s ease-in-out infinite;
}
.glow-red { top: -10rem; right: -8rem; background: #be1e2d; }
.glow-blue { bottom: -12rem; left: -10rem; background: #2445a0; animation-delay: -9s; }
@keyframes float {
  50% { translate: -3rem 2rem; }
}
.badge { background: rgb(239 243 236 / 0.08); border: 1px solid rgb(239 243 236 / 0.15); }
.pulse { animation: pulse 2s ease-in-out infinite; }
@keyframes pulse {
  50% { box-shadow: 0 0 0 6px rgb(240 107 118 / 0.15); }
}
.card {
  background: rgb(239 243 236 / 0.06);
  border: 1px solid rgb(239 243 236 / 0.12);
  animation: rise 0.6s ease-out var(--d) both;
  transition: background 0.2s, border-color 0.2s;
}
.card:hover { background: rgb(239 243 236 / 0.1); border-color: rgb(239 243 236 / 0.25); }
.mark {
  background: linear-gradient(#f06b76, #f06b76) 0 92% / 100% 3px no-repeat;
}
.sheet { animation: rise 0.5s ease-out both; }
@keyframes rise {
  from { opacity: 0; transform: translateY(12px); }
}
.tab { color: var(--color-ink-soft); transition: background 0.2s, color 0.2s; }
.tab:hover { color: var(--color-ink); }
.tab.is-on { background: var(--color-sheet); color: var(--color-ink); box-shadow: 0 1px 3px rgb(29 42 35 / 0.15); }
@media (prefers-reduced-motion: reduce) {
  .glow, .pulse, .card, .sheet { animation: none; }
}
</style>
