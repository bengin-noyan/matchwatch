// Grafik çizim yardımcıları. Tutarlar burada yalnızca piksel yerleştirmek için Number'a çevrilir; ekranda yazı olarak
// görünen her tutar, bu sayılardan değil, API'nin kendi kuruş/ondalık değerinden biçimlendirilir (spec kural 1).

/** 0'dan en az max'a kadar temiz eksen değerleri: 1, 2 ya da 5 × 10^k adımlarla, yaklaşık `count` tane. */
export function niceTicks(max: number, count = 4): number[] {
  if (!(max > 0)) return [0]
  const raw = max / count
  const mag = 10 ** Math.floor(Math.log10(raw))
  const step = ([1, 2, 5, 10].find((m) => m * mag >= raw) ?? 10) * mag
  const ticks: number[] = []
  for (let v = 0; v < max + step / 2; v += step) ticks.push(v)
  if (ticks[ticks.length - 1]! < max) ticks.push(ticks[ticks.length - 1]! + step)
  return ticks
}

const compact = new Intl.NumberFormat('tr-TR', { notation: 'compact', maximumFractionDigits: 1 })
const full = new Intl.NumberFormat('tr-TR', { maximumFractionDigits: 2 })

/** Tam para birimi cinsinden bir eksen değerinin etiketi: 10.000'in altında tam, üstünde kısa (10 B, 1,5 Mn). */
export function tickLabel(v: number, max: number): string {
  return max >= 10_000 ? compact.format(v) : full.format(v)
}

const short = new Intl.DateTimeFormat('tr-TR', { day: 'numeric', month: 'short', timeZone: 'UTC' })
const long = new Intl.DateTimeFormat('tr-TR', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' })

export const shortDate = (iso: string) => short.format(new Date(`${iso}T00:00:00Z`))
export const longDate = (iso: string) => long.format(new Date(`${iso}T00:00:00Z`))

/** `from`'un haftasından `to`'nun haftasına kadar (uçlar dahil) pazartesiler, YYYY-MM-DD. */
export function weeksBetween(from: string, to: string): string[] {
  const monday = (iso: string) => {
    const d = new Date(`${iso}T00:00:00Z`)
    d.setUTCDate(d.getUTCDate() - ((d.getUTCDay() + 6) % 7))
    return d
  }
  const out: string[] = []
  const end = monday(to)
  for (const d = monday(from); d <= end; d.setUTCDate(d.getUTCDate() + 7)) out.push(d.toISOString().slice(0, 10))
  return out
}
