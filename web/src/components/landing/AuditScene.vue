<script setup lang="ts">
// Tanıtım sayfasındaki, matchwatch'ın ne yaptığını uygulamanın kendi görsel diliyle anlatan hareketli resim: masada
// sözleşme, e-İrsaliye ve e-Fatura var. Miktar irsaliyeyle tutuyor, fiyat tutmuyor; kırmızı kalem fiyatı daire içine
// alıyor ve parayı kenara not ediyor, sonra bir kişi mavi tükenmezle karar veriyor.
// Aşamalar döngü halinde oynar; hareket azaltma açıksa son aşamayı gösterir ve durur.
import { onMounted, onUnmounted, ref } from 'vue'

const PHASES = 6 // 0 masa, 1 tarama, 2 miktar tutuyor, 3 fiyat daire içinde, 4 karar verildi, 5 bekleme
const phase = ref(PHASES - 1)
let timer: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return
  phase.value = 0
  timer = setInterval(() => (phase.value = (phase.value + 1) % PHASES), 1700)
})
onUnmounted(() => clearInterval(timer))
const at = (p: number) => phase.value >= p
</script>

<template>
  <figure class="scene" :data-phase="phase">
    <svg viewBox="0 0 560 480" class="mx-auto w-full max-w-[34rem]" role="img" aria-labelledby="scene-title">
      <title id="scene-title">
        Sözleşmede 1,25 TL olan civata faturada 1,35 TL; miktar irsaliyeyle tutuyor, fiyat farkı 470,00 TL olarak
        işaretleniyor ve bir kişi itiraz ediyor.
      </title>
      <defs>
        <filter id="sheet-shadow" x="-10%" y="-10%" width="120%" height="130%">
          <feDropShadow dx="0" dy="6" stdDeviation="8" flood-color="#1D2A23" flood-opacity="0.12" />
        </filter>
        <linearGradient id="scan" x1="0" x2="0" y1="0" y2="1">
          <stop offset="0" stop-color="#2445A0" stop-opacity="0" />
          <stop offset="1" stop-color="#2445A0" stop-opacity="0.18" />
        </linearGradient>
        <clipPath id="invoice-clip"><rect x="190" y="200" width="330" height="262" rx="10" /></clipPath>
      </defs>

      <!-- sözleşme, en arkada -->
      <g class="sheet float-a" filter="url(#sheet-shadow)">
        <g transform="rotate(-7 130 150)">
          <rect x="30" y="40" width="230" height="250" rx="10" fill="#fff" stroke="#C7D3C4" />
          <text x="52" y="74" class="t-label">SÖZLEŞME · S-2026-001</text>
          <rect x="52" y="88" width="150" height="7" rx="3" fill="#E4EBE2" />
          <rect x="52" y="104" width="180" height="7" rx="3" fill="#E4EBE2" />
          <rect x="52" y="120" width="120" height="7" rx="3" fill="#E4EBE2" />
          <g class="clause" :class="{ lit: at(3) }">
            <rect x="46" y="140" width="196" height="48" rx="6" />
            <text x="56" y="160" class="t-small">Madde 4.1 · M8x20 civata</text>
            <text x="56" y="179" class="t-value">1,25 TL / ADET</text>
          </g>
          <rect x="52" y="204" width="170" height="7" rx="3" fill="#E4EBE2" />
          <rect x="52" y="220" width="140" height="7" rx="3" fill="#E4EBE2" />
          <rect x="52" y="236" width="160" height="7" rx="3" fill="#E4EBE2" />
        </g>
      </g>

      <!-- e-İrsaliye, ortada -->
      <g class="sheet float-b" filter="url(#sheet-shadow)">
        <g transform="rotate(5 420 110)">
          <rect x="330" y="20" width="200" height="170" rx="10" fill="#fff" stroke="#C7D3C4" />
          <text x="350" y="52" class="t-label">e-İRSALİYE</text>
          <rect x="350" y="66" width="130" height="7" rx="3" fill="#E4EBE2" />
          <g class="clause" :class="{ lit: at(2) }">
            <rect x="344" y="86" width="172" height="48" rx="6" />
            <text x="354" y="106" class="t-small">Teslim alınan</text>
            <text x="354" y="125" class="t-value">4.700 ADET</text>
          </g>
          <rect x="350" y="150" width="110" height="7" rx="3" fill="#E4EBE2" />
        </g>
      </g>

      <!-- e-Fatura, önde, karşılaştırıldığı değerlerin altında -->
      <g class="sheet float-c" filter="url(#sheet-shadow)">
        <rect x="190" y="200" width="330" height="262" rx="10" fill="#fff" stroke="#C7D3C4" />
        <text x="214" y="234" class="t-label">e-FATURA · FTR2026000000026</text>
        <rect x="214" y="248" width="160" height="7" rx="3" fill="#E4EBE2" />
        <line x1="214" y1="270" x2="496" y2="270" stroke="#C7D3C4" />
        <text x="214" y="294" class="t-small">M8x20 paslanmaz civata</text>
        <text x="214" y="324" class="t-value">4.700 ADET</text>
        <text x="392" y="324" class="t-value">1,35 TL</text>
        <line x1="214" y1="346" x2="496" y2="346" stroke="#C7D3C4" />
        <rect x="214" y="366" width="150" height="7" rx="3" fill="#E4EBE2" />
        <rect x="214" y="382" width="110" height="7" rx="3" fill="#E4EBE2" />
        <text x="214" y="440" class="t-small">Ödenecek</text>
        <text x="392" y="440" class="t-value">7.614,00 TL</text>
        <!-- sistem faturayı okuyor -->
        <g clip-path="url(#invoice-clip)">
          <rect class="scanline" :class="{ run: phase === 1 }" x="190" y="140" width="330" height="60" fill="url(#scan)" />
        </g>
      </g>

      <!-- sistemin belgeler arasında çizdiği bağlantılar -->
      <path class="link" :class="{ drawn: at(2) }" d="M418 148 C 410 220, 300 250, 262 304" pathLength="1" />
      <path class="link pencil" :class="{ drawn: at(3) }" d="M168 200 C 220 262, 330 296, 388 316" pathLength="1" />
      <!-- miktar tutuyor: sessiz bir mürekkep tiki -->
      <path class="tick ink" :class="{ drawn: at(2) }" d="M326 314 l6 7 l12 -15" pathLength="1" />

      <!-- fiyat tutmuyor: kırmızı kalem halkası ve kenar notu -->
      <path
        class="loop"
        :class="{ drawn: at(3) }"
        d="M394 330 C 380 306, 412 298, 434 300 C 464 302, 476 312, 472 324 C 468 338, 442 342, 420 340 C 400 338, 386 330, 394 316"
        pathLength="1"
      />
      <text x="396" y="376" class="hand-note" :class="{ shown: at(3) }">+470,00 TL</text>

      <!-- bir kişi karar veriyor: mavi tükenmez -->
      <path class="tick ballpoint" :class="{ drawn: at(4) }" d="M480 302 l10 12 l22 -30" pathLength="1" />
      <text x="396" y="408" class="hand-note decided" :class="{ shown: at(4) }">itiraz edildi</text>
    </svg>
  </figure>
</template>

<style scoped>
.scene {
  margin: 0;
}
.t-label {
  font: 700 11px var(--font-sans);
  letter-spacing: 0.08em;
  fill: var(--color-ink-soft);
}
.t-small {
  font: 400 13px var(--font-sans);
  fill: var(--color-ink-soft);
}
.t-value {
  font: 700 17px var(--font-sans);
  fill: var(--color-ink);
  font-variant-numeric: tabular-nums;
}
.clause rect {
  fill: transparent;
  stroke: transparent;
  transition: fill 0.5s, stroke 0.5s;
}
.clause.lit rect {
  fill: var(--color-paper);
  stroke: var(--color-rule);
}
.hand-note {
  font: 500 26px var(--font-hand);
  fill: var(--color-pencil);
  opacity: 0;
  transform: translateY(6px);
  transition: opacity 0.6s, transform 0.6s;
}
.hand-note.decided {
  fill: var(--color-ballpoint);
  font-size: 24px;
}
.hand-note.shown {
  opacity: 1;
  transform: none;
}
/* çizilen çizgiler: pathLength=1 sayesinde kesik çizgi hesabı birimden bağımsız */
.link,
.loop,
.tick {
  fill: none;
  stroke-linecap: round;
  stroke-dasharray: 1;
  stroke-dashoffset: 1;
  opacity: 0; /* yuvarlak uç, çizilmemişken bir nokta bırakırdı */
  transition: stroke-dashoffset 0.9s ease-in-out, opacity 0.2s;
}
.link {
  stroke: var(--color-ink-soft);
  stroke-width: 1.5;
}
.link.pencil {
  stroke: var(--color-pencil);
}
.loop {
  stroke: var(--color-pencil);
  stroke-width: 2.6;
  transition-duration: 1.1s;
}
.tick {
  stroke-width: 3;
  stroke-linejoin: round;
}
.tick.ink {
  stroke: var(--color-ink-soft);
}
.tick.ballpoint {
  stroke: var(--color-ballpoint);
  stroke-width: 3.4;
}
.drawn {
  stroke-dashoffset: 0;
  opacity: 1;
}
.link.drawn {
  opacity: 0.55;
}
.scanline {
  opacity: 0;
}
.scanline.run {
  animation: scan 1.6s ease-in-out;
}
@keyframes scan {
  0% { opacity: 1; transform: translateY(0); }
  90% { opacity: 1; }
  100% { opacity: 0; transform: translateY(320px); }
}
/* masa hafifçe nefes alıyor */
.float-a { animation: float 7s ease-in-out infinite; }
.float-b { animation: float 6s ease-in-out -2s infinite; }
.float-c { animation: float 8s ease-in-out -4s infinite; }
@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-6px); }
}
@media (prefers-reduced-motion: reduce) {
  .float-a, .float-b, .float-c, .scanline.run { animation: none; }
}
</style>
