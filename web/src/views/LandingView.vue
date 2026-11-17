<script setup lang="ts">
// Herkese açık ön kapı: matchwatch'ın ne yaptığı, hareketli bir masa sahnesiyle anlatılıyor. Giriş kendi sayfasında
// (/giris).
import { onMounted, onUnmounted, ref, type Directive } from 'vue'
import { ruleLabel } from '../labels'
import AuditScene from '../components/landing/AuditScene.vue'
import ProductTour from '../components/landing/ProductTour.vue'

// Sayfa içi bağlantılar URL'deki # kısmını değiştirmek yerine kaydırır (yoksa router sayfanın başına atlardı).
const reduced = () => !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
function go(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: reduced() ? 'auto' : 'smooth', block: 'start' })
}

// v-reveal: bölümler ilk kez görünür alana girdiklerinde yukarı kayarak yerine oturur.
let observer: IntersectionObserver | undefined
const vReveal: Directive<HTMLElement> = {
  mounted(el) {
    if (!observer) return el.classList.add('is-in')
    el.classList.add('reveal')
    observer.observe(el)
  },
}
if (typeof IntersectionObserver !== 'undefined') {
  observer = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (!e.isIntersecting) continue
        e.target.classList.add('is-in')
        observer?.unobserve(e.target)
        e.target.dispatchEvent(new Event('revealed'))
      }
    },
    { threshold: 0.15 },
  )
}

// Rakamlar, görünür alana girince sayarak yükselir. Buradaki her sayı ürünün bir özelliği, reklam iddiası değil.
const facts = [
  { n: 10, label: 'denetim kuralı', note: 'fiyat, miktar, vade, tevkifat, kur, mükerrerlik…' },
  { n: 3, label: 'belge, tek bakışta', note: 'sözleşme, e-İrsaliye ve e-Fatura' },
  { n: 52, label: 'GİB tevkifat kodu', note: 'UBL-TR Kod Listeleri V1.43 ile doğrulandı' },
  { n: 0, label: 'tutar yapay zekâya hesaplatılır', note: 'her kuruş kuralla, kanıtıyla' },
]
const shown = ref(facts.map((f) => (typeof IntersectionObserver === 'undefined' || reduced() ? f.n : 0)))
function countUp() {
  if (reduced()) return
  const start = performance.now()
  const tick = (t: number) => {
    const k = Math.min(1, (t - start) / 1200)
    const ease = 1 - (1 - k) ** 3
    shown.value = facts.map((f) => Math.round(f.n * ease))
    if (k < 1) requestAnimationFrame(tick)
  }
  requestAnimationFrame(tick)
}

const steps = [
  {
    title: 'Belgeleri yükle',
    text: "Sözleşmenin PDF'ini, e-İrsaliye ve e-Fatura XML'lerini (GİB UBL-TR 1.2) yükle. Sözleşme maddeleri yerel yapay zekâyla taslağa çıkar; sen kontrol edip onaylarsın.",
  },
  {
    title: 'matchwatch eşleştirir',
    text: 'Her fatura satırı sözleşmedeki kalemle ve irsaliyede teslim alınan miktarla karşılaştırılır. Farklar kuruşuna kadar, kural kural hesaplanır.',
  },
  {
    title: 'Sen karar verirsin',
    text: 'Her bulgu kanıtıyla ve sözleşmenin ilgili sayfasıyla gelir. İtiraz eder ya da istisna olarak onaylarsın; kimin, ne zaman, hangi notla karar verdiği saklanır.',
  },
]

const rules: { code: string; text: string }[] = [
  { code: 'PRICE_MISMATCH', text: 'Faturadaki net birim fiyat sözleşmedekinden yüksek.' },
  { code: 'QTY_EXCEEDS_DELIVERY', text: 'Faturalanan miktar, irsaliyeyle teslim alınanı toleransıyla birlikte aşıyor.' },
  { code: 'MATURITY_VIOLATION', text: 'Ödeme vadesi sözleşmede yazandan kısa.' },
  { code: 'DUPLICATE_SUSPECT', text: 'Aynı faturanın ikinci kez gelmiş olabileceğine dair işaretler.' },
  { code: 'WITHHOLDING_ERROR', text: 'KDV tevkifat kodu ya da oranı GİB listesine uymuyor.' },
  { code: 'PRICE_CREEP', text: 'Fiyat, son altı ayın ortalamasının üstüne sessizce çıkmış.' },
  { code: 'UNCONTRACTED_ITEM', text: 'Sözleşmede olmayan bir kalem faturalanmış; benzer kalemler önerilir.' },
  { code: 'UOM_MISMATCH', text: 'Faturadaki birim sözleşmedekiyle aynı değil.' },
  { code: 'FX_UNVERIFIED', text: 'Döviz kuru doğrulanamadı; faturada yoksa TCMB kuruna bakılır.' },
  { code: 'FX_RATE_DEVIATION', text: 'Faturadaki kur, TCMB döviz alış kurundan %1’den fazla sapıyor.' },
]

const trust = ['Kurulum yok, tarayıcıda çalışır', 'Veriler Türkiye’de (KVKK)', 'GİB UBL-TR 1.2']

const faqs = [
  {
    q: 'Hangi belgelerle çalışır?',
    a: "GİB UBL-TR 1.2 biçimindeki e-Fatura ve e-İrsaliye XML'leriyle, bir de satın alma sözleşmesinin PDF'iyle.",
  },
  {
    q: 'Kurulum gerekiyor mu?',
    a: 'Hayır. matchwatch tarayıcıda çalışır. Hesabını aç, belgelerini yükle; eşleştirme ve denetim yüklediğin anda yapılır.',
  },
  {
    q: 'Verilerim nerede tutuluyor?',
    a: "Belgeler ve kayıtlar Türkiye'deki altyapıda tutulur ve yurt dışına aktarılmaz (KVKK). Her firmanın verisi veritabanı düzeyinde birbirinden ayrıdır.",
  },
  {
    q: 'Yapay zekâ ne yapıyor, ne yapmıyor?',
    a: 'Kendi sunucularımızda çalışan yerel bir model sözleşme maddelerini taslağa çıkarır ve sözleşmede bulunamayan kalemler için benzerlerini önerir. Tutar hesaplamaz, onay ya da itiraz kararı vermez; bunları kurallar ve sen yaparsın.',
  },
  {
    q: 'Döviz cinsinden sözleşmeler ne olacak?',
    a: 'Faturada kur varsa o kullanılır; yoksa fatura tarihinden önceki TCMB döviz alış kuruna bakılır. Kur bulunamazsa bulgu “kur doğrulanamadı” diye işaretlenir.',
  },
]

const scrolled = ref(false)
const onScroll = () => (scrolled.value = window.scrollY > 8)
onMounted(() => window.addEventListener('scroll', onScroll, { passive: true }))
onUnmounted(() => {
  window.removeEventListener('scroll', onScroll)
  observer?.disconnect()
})
</script>

<template>
  <div class="landing relative min-h-dvh overflow-x-clip">
    <!-- masa: her şeyin arkasında çizgili kâğıt ve süzülen sayfalar -->
    <div class="backdrop" aria-hidden="true">
      <span class="drift d1" />
      <span class="drift d2" />
      <span class="drift d3" />
    </div>

    <header class="sticky top-0 z-20 transition-colors" :class="scrolled ? 'bg-paper/90 border-rule border-b backdrop-blur' : ''">
      <div class="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
        <p class="text-ink flex items-center gap-2 text-xl font-semibold tracking-tight">
          <svg viewBox="0 0 32 32" class="size-7" aria-hidden="true">
            <path d="M7 17c2-6 15-9 18-3 2 5-6 10-12 9-5-1-8-3-6-6" fill="none" stroke="var(--color-pencil)" stroke-width="2.6" stroke-linecap="round" />
          </svg>
          matchwatch
        </p>
        <nav aria-label="Sayfa içi" class="hidden items-center gap-6 text-sm lg:flex">
          <button type="button" class="nav-a text-ink-soft hover:text-ink" @click="go('nasil')">Nasıl çalışır</button>
          <button type="button" class="nav-a text-ink-soft hover:text-ink" @click="go('ekranlar')">Ekranlar</button>
          <button type="button" class="nav-a text-ink-soft hover:text-ink" @click="go('kurallar')">Neleri yakalar</button>
          <button type="button" class="nav-a text-ink-soft hover:text-ink" @click="go('guven')">Veri güvenliği</button>
          <button type="button" class="nav-a text-ink-soft hover:text-ink" @click="go('sss')">SSS</button>
        </nav>
        <div class="flex items-center gap-2 sm:gap-3">
          <RouterLink to="/giris" class="text-ink hover:text-ballpoint px-2 py-2 text-sm font-semibold sm:px-3">Giriş yap</RouterLink>
          <RouterLink to="/kayit" class="btn-primary">Kaydol</RouterLink>
        </div>
      </div>
    </header>

    <main class="relative z-10">
      <!-- giriş bölümü -->
      <section class="mx-auto grid max-w-6xl gap-10 px-4 pt-6 pb-16 sm:px-6 lg:grid-cols-2 lg:items-center lg:pt-12">
        <div class="min-w-0">
          <p class="hero-in text-ballpoint mb-3 text-sm font-semibold tracking-wide uppercase">Satın alma için iç denetim</p>
          <h1 class="hero-in hero-d1 text-ink max-w-2xl text-4xl leading-tight font-semibold tracking-tight sm:text-5xl">
            Sözleşmede ne yazıyorsa, faturada <span class="underline-pencil">o olsun.</span>
          </h1>
          <p class="hero-in hero-d2 text-ink-soft mt-5 max-w-xl text-lg">
            matchwatch her e-Faturayı sözleşmesiyle ve e-İrsaliyesiyle karşılaştırır. Fazla fiyatı, teslim alınmamış
            miktarı, kısa vadeyi ve hatalı tevkifatı ödeme yapılmadan önce, kanıtıyla birlikte önüne koyar.
          </p>
          <div class="hero-in hero-d3 mt-8 flex flex-wrap items-center gap-3">
            <RouterLink to="/kayit" class="btn-primary cta px-6 py-3 text-base">Firman için hesap oluştur</RouterLink>
            <RouterLink to="/giris" class="btn-secondary px-6 py-3 text-base">Giriş yap</RouterLink>
          </div>
          <ul class="hero-in hero-d3 text-ink-soft mt-6 flex flex-wrap gap-x-5 gap-y-2 text-sm">
            <li v-for="t in trust" :key="t" class="flex items-center gap-1.5">
              <svg viewBox="0 0 16 16" class="text-ballpoint size-4" aria-hidden="true"><path d="M3.5 8.5l3 3 6-7" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" /></svg>
              {{ t }}
            </li>
          </ul>
        </div>
        <div class="hero-in hero-d3 min-w-0">
          <AuditScene />
        </div>
      </section>

      <!-- rakamlar -->
      <section v-reveal aria-label="Rakamlarla" class="mx-auto max-w-6xl px-4 pb-16 sm:px-6" @revealed="countUp">
        <dl class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <div v-for="(f, i) in facts" :key="f.label" class="bg-sheet/80 border-rule rounded-sheet border p-5 backdrop-blur-sm">
            <dt class="sr-only">{{ f.label }}</dt>
            <dd>
              <span class="num text-4xl font-semibold" :class="f.n === 0 ? 'text-ballpoint' : 'text-ink'">{{ shown[i] }}</span> <span class="text-ink ml-1 font-semibold">{{ f.label }}</span>
              <span class="text-ink-soft mt-1 block text-sm">{{ f.note }}</span>
            </dd>
          </div>
        </dl>
      </section>

      <!-- nasıl çalışır -->
      <section id="nasil" class="scroll-mt-20 py-16">
        <div class="mx-auto max-w-6xl px-4 sm:px-6">
          <h2 v-reveal class="text-3xl font-semibold tracking-tight">Nasıl çalışır</h2>
          <ol class="mt-8 grid gap-6 md:grid-cols-3">
            <li v-for="(s, i) in steps" :key="s.title" v-reveal class="step bg-sheet border-rule rounded-sheet relative border p-6" :style="{ '--d': `${i * 120}ms` }">
              <span class="hand text-pencil absolute -top-5 left-5 text-5xl" aria-hidden="true">{{ i + 1 }}</span>
              <h3 class="mt-3 text-lg font-semibold">{{ s.title }}</h3>
              <p class="text-ink-soft mt-2">{{ s.text }}</p>
            </li>
          </ol>
        </div>
      </section>

      <!-- ekranlar -->
      <section id="ekranlar" class="scroll-mt-20 py-16">
        <div class="mx-auto max-w-6xl px-4 sm:px-6">
          <h2 v-reveal class="text-3xl font-semibold tracking-tight">Ekranlar</h2>
          <p v-reveal class="text-ink-soft mt-2 mb-8 max-w-2xl">Bulgudan karara üç ekran. Kendi kendine değişir; üzerine gelince durur.</p>
          <ProductTour v-reveal />
        </div>
      </section>

      <!-- neleri yakalar -->
      <section id="kurallar" class="scroll-mt-20 py-16">
        <div class="mx-auto max-w-6xl px-4 sm:px-6">
          <h2 v-reveal class="text-3xl font-semibold tracking-tight">Neleri yakalar</h2>
          <p v-reveal class="text-ink-soft mt-2 max-w-2xl">
            Her kural kuralla çalışır, tahminle değil. Bulduğu her fark, faturanın satırını, sözleşmenin maddesini ve
            sayfasını kanıt olarak taşır.
          </p>
          <ul class="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <li
              v-for="(r, i) in rules"
              :key="r.code"
              v-reveal
              class="rule bg-sheet border-rule rounded-sheet border p-5"
              :style="{ '--d': `${(i % 3) * 90}ms` }"
            >
              <h3 class="flex items-center gap-2 font-semibold">
                <svg viewBox="0 0 24 24" class="size-5 shrink-0" aria-hidden="true">
                  <path class="mini-loop" d="M5 13c1-5 12-8 14-3 2 4-5 8-9 7-4-1-6-3-4-5" fill="none" stroke="var(--color-pencil)" stroke-width="2" stroke-linecap="round" pathLength="1" />
                </svg>
                {{ ruleLabel[r.code] }}
              </h3>
              <p class="text-ink-soft mt-1 text-sm">{{ r.text }}</p>
            </li>
          </ul>
        </div>
      </section>

      <!-- güven -->
      <section id="guven" class="scroll-mt-20 py-16">
        <div class="mx-auto max-w-6xl px-4 sm:px-6">
          <div v-reveal class="bg-ink text-paper rounded-sheet grid gap-8 p-8 md:grid-cols-3 md:p-10">
            <div>
              <h2 class="text-2xl font-semibold">Veriler Türkiye'de kalır</h2>
              <p class="mt-2 opacity-80">
                Belgeler ve kayıtlar Türkiye'deki altyapıda tutulur. Sözleşmeleri okuyan yapay zekâ da kendi
                sunucularımızda çalışır; belgeler yurt dışındaki bir servise gönderilmez (KVKK).
              </p>
            </div>
            <div>
              <h2 class="text-2xl font-semibold">Yapay zekâ karar vermez</h2>
              <p class="mt-2 opacity-80">
                Yapay zekâ yalnızca sözleşme maddelerini taslağa çıkarır ve benzer kalemleri önerir. Tutarları kurallar
                hesaplar, onayı ya da itirazı bir kişi verir.
              </p>
            </div>
            <div>
              <h2 class="text-2xl font-semibold">Her karar iz bırakır</h2>
              <p class="mt-2 opacity-80">
                Kimin, ne zaman, hangi notla karar verdiği değiştirilemez bir geçmişte saklanır. Her firmanın verisi
                veritabanı düzeyinde birbirinden ayrıdır.
              </p>
            </div>
          </div>
        </div>
      </section>

      <!-- sorular -->
      <section id="sss" class="scroll-mt-20 py-16">
        <div class="mx-auto grid max-w-6xl gap-8 px-4 sm:px-6 lg:grid-cols-[1fr_2fr]">
          <div v-reveal>
            <h2 class="text-3xl font-semibold tracking-tight">Sık sorulanlar</h2>
            <p class="text-ink-soft mt-2">Başka bir sorun varsa en iyisi hesabını açıp kendi belgelerinle denemek.</p>
          </div>
          <div v-reveal class="divide-rule border-rule bg-sheet rounded-sheet divide-y border">
            <details v-for="f in faqs" :key="f.q" class="faq px-5">
              <summary class="flex cursor-pointer list-none items-center justify-between gap-4 py-4 font-semibold">
                {{ f.q }}
                <span class="plus text-ballpoint text-xl leading-none" aria-hidden="true">+</span>
              </summary>
              <p class="text-ink-soft pb-5">{{ f.a }}</p>
            </details>
          </div>
        </div>
      </section>

      <!-- son çağrı -->
      <section v-reveal class="mx-auto max-w-6xl px-4 pt-4 pb-20 sm:px-6">
        <div class="cta-band rounded-sheet bg-ballpoint text-paper relative overflow-hidden px-6 py-12 text-center sm:px-12">
          <p class="hand relative text-4xl">Ödemeden önce bir bak.</p>
          <p class="relative mx-auto mt-3 max-w-xl opacity-85">Hesabını aç, ilk sözleşmeni ve faturanı yükle; farkları kuruşuna kadar gör.</p>
          <div class="relative mt-7 flex flex-wrap justify-center gap-3">
            <RouterLink to="/kayit" class="bg-paper text-ballpoint rounded-field px-6 py-3 font-semibold hover:bg-white">Hesap oluştur</RouterLink>
            <RouterLink to="/giris" class="rounded-field border border-white/40 px-6 py-3 font-semibold hover:bg-white/10">Giriş yap</RouterLink>
          </div>
        </div>
      </section>
    </main>

    <footer class="border-rule text-ink-soft relative z-10 border-t text-sm">
      <div class="mx-auto grid max-w-6xl gap-8 px-4 py-10 sm:grid-cols-2 sm:px-6 lg:grid-cols-4">
        <div>
          <p class="text-ink flex items-center gap-2 text-lg font-semibold">
            <svg viewBox="0 0 32 32" class="size-6" aria-hidden="true">
              <path d="M7 17c2-6 15-9 18-3 2 5-6 10-12 9-5-1-8-3-6-6" fill="none" stroke="var(--color-pencil)" stroke-width="2.6" stroke-linecap="round" />
            </svg>
            matchwatch
          </p>
          <p class="mt-2">GİB UBL-TR 1.2 e-Fatura ve e-İrsaliye için üçlü eşleştirme.</p>
        </div>
        <nav aria-label="Ürün">
          <p class="text-ink mb-2 font-semibold">Ürün</p>
          <ul class="space-y-1.5">
            <li><button type="button" class="hover:text-ink" @click="go('nasil')">Nasıl çalışır</button></li>
            <li><button type="button" class="hover:text-ink" @click="go('ekranlar')">Ekranlar</button></li>
            <li><button type="button" class="hover:text-ink" @click="go('kurallar')">Denetim kuralları</button></li>
          </ul>
        </nav>
        <nav aria-label="Güven">
          <p class="text-ink mb-2 font-semibold">Güven</p>
          <ul class="space-y-1.5">
            <li><button type="button" class="hover:text-ink" @click="go('guven')">Veri güvenliği ve KVKK</button></li>
            <li><button type="button" class="hover:text-ink" @click="go('sss')">Sık sorulanlar</button></li>
          </ul>
        </nav>
        <nav aria-label="Hesap">
          <p class="text-ink mb-2 font-semibold">Hesap</p>
          <ul class="space-y-1.5">
            <li><RouterLink to="/giris" class="hover:text-ink">Giriş yap</RouterLink></li>
            <li><RouterLink to="/kayit" class="hover:text-ink">Hesap oluştur</RouterLink></li>
          </ul>
        </nav>
      </div>
    </footer>
  </div>
</template>

<style scoped>
/* çok silik çizgili kâğıt ve sayfanın arkasında yavaşça süzülen üç büyük sayfa */
.backdrop {
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  background-image: repeating-linear-gradient(to bottom, transparent 0 31px, rgb(199 211 196 / 0.35) 31px 32px);
  mask-image: linear-gradient(to bottom, black, transparent 90%);
}
.drift {
  position: absolute;
  width: 22rem;
  height: 28rem;
  border: 1px solid rgb(199 211 196 / 0.6);
  border-radius: 14px;
  background:
    repeating-linear-gradient(to bottom, transparent 0 22px, rgb(199 211 196 / 0.45) 22px 28px) 2.5rem 4rem / 60% 50% no-repeat,
    rgb(255 255 255 / 0.45);
}
.d1 { top: 6rem; left: -8rem; transform: rotate(-12deg); animation: drift 26s ease-in-out infinite; }
.d2 { top: 34rem; right: -9rem; transform: rotate(9deg); animation: drift 32s ease-in-out -8s infinite; }
.d3 { top: 70rem; left: 12%; transform: rotate(-4deg); animation: drift 38s ease-in-out -16s infinite; opacity: 0.7; }
@keyframes drift {
  0%, 100% { translate: 0 0; }
  50% { translate: 1.5rem -2rem; }
}

/* başlığın altındaki elle çizilmiş çizgi */
.underline-pencil {
  background: linear-gradient(var(--color-pencil), var(--color-pencil)) 0 92% / 0 3px no-repeat;
  animation: underline 0.9s 0.7s ease-out forwards;
}
@keyframes underline {
  to { background-size: 100% 3px; }
}

/* ilk açılış: giriş bölümü yerine oturur */
.hero-in { animation: rise 0.7s ease-out both; }
.hero-d1 { animation-delay: 0.08s; }
.hero-d2 { animation-delay: 0.16s; }
.hero-d3 { animation-delay: 0.26s; }
@keyframes rise {
  from { opacity: 0; transform: translateY(14px); }
}

/* kaydırınca görünme (v-reveal önce .reveal, sonra .is-in ekler) */
.reveal {
  opacity: 0;
  transform: translateY(18px);
  transition: opacity 0.7s ease-out var(--d, 0ms), transform 0.7s ease-out var(--d, 0ms);
}
.reveal.is-in {
  opacity: 1;
  transform: none;
}

.step.is-in,
.rule.is-in {
  transition: opacity 0.7s ease-out var(--d, 0ms), transform 0.7s ease-out var(--d, 0ms), box-shadow 0.2s, border-color 0.2s;
}
.rule:hover,
.step:hover {
  border-color: var(--color-ink-soft);
  box-shadow: 0 10px 30px -14px rgb(29 42 35 / 0.35);
}
.mini-loop {
  stroke-dasharray: 1;
  stroke-dashoffset: 0;
}
.rule:hover .mini-loop {
  animation: loop 0.8s ease-out;
}
@keyframes loop {
  from { stroke-dashoffset: 1; }
}

/* üstteki bağlantıların alt çizgisi soldan gelir */
.nav-a { background: linear-gradient(currentColor, currentColor) 0 100% / 0 1px no-repeat; transition: background-size 0.25s, color 0.2s; }
.nav-a:hover { background-size: 100% 1px; }

/* ana çağrı butonu biraz yukarı kalkar */
.cta { box-shadow: 0 10px 30px -12px rgb(36 69 160 / 0.6); transition: transform 0.2s, box-shadow 0.2s; }
.cta:hover { transform: translateY(-2px); box-shadow: 0 16px 36px -12px rgb(36 69 160 / 0.7); }

.faq .plus { transition: transform 0.2s; }
.faq[open] .plus { transform: rotate(45deg); }
.faq summary::-webkit-details-marker { display: none; }

/* kapanış bandında süzülen çizgiler ve kırmızı kalem parıltısı */
.cta-band {
  background-image: repeating-linear-gradient(to bottom, transparent 0 31px, rgb(255 255 255 / 0.07) 31px 32px);
}
.cta-band::after {
  content: '';
  position: absolute;
  width: 18rem;
  height: 18rem;
  right: -5rem;
  top: -6rem;
  border-radius: 9999px;
  background: rgb(190 30 45 / 0.35);
  filter: blur(70px);
  animation: drift 20s ease-in-out infinite;
}

@media (prefers-reduced-motion: reduce) {
  .drift, .hero-in, .underline-pencil, .cta-band::after { animation: none; }
  .underline-pencil { background-size: 100% 3px; }
  .reveal { opacity: 1; transform: none; }
}
</style>
