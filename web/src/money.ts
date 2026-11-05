// Money is never computed in the browser (spec rule 1): the API sends kuruş as integers and decimals as strings,
// and these helpers only re-format digits. No Number arithmetic on amounts.

function groupThousands(int: string): string {
  return int.replace(/\B(?=(\d{3})+(?!\d))/g, '.')
}

export function currencyLabel(code: string): string {
  return code === 'TRY' ? 'TL' : code
}

/** 21412, "TRY" -> "214,12 TL"; -500 -> "-5,00 TL". */
export function formatMinor(minor: number, currency: string): string {
  const neg = minor < 0
  const digits = String(Math.abs(Math.trunc(minor))).padStart(3, '0')
  const int = groupThousands(digits.slice(0, -2))
  return `${neg ? '-' : ''}${int},${digits.slice(-2)} ${currencyLabel(currency)}`
}

/** "1.250000" -> "1,25"; "1000.000000" -> "1.000"; "0.037500" -> "0,0375"; keeps significant decimals only. */
export function formatDecimal(s: string, minDecimals = 0): string {
  const m = /^(-?)(\d+)(?:\.(\d+))?$/.exec(s.trim())
  if (!m) return s
  const [, sign, int = '0', rawFrac = ''] = m
  let frac = rawFrac.replace(/0+$/, '')
  if (frac.length < minDecimals) frac = frac.padEnd(minDecimals, '0')
  return `${sign}${groupThousands(int)}${frac ? ',' + frac : ''}`
}

/** "0.0325 USD" style API values ("<decimal> <CUR>") -> "0,0325 USD"; anything else is returned as is. */
export function formatAmountText(s: string): string {
  const m = /^(-?\d+(?:\.\d+)?) ([A-Z]{3})$/.exec(s.trim())
  if (!m) return s
  return `${formatDecimal(m[1]!, 2)} ${currencyLabel(m[2]!)}`
}

/** "2026-03-01" -> "01.03.2026". */
export function formatDate(iso: string | null | undefined): string {
  if (!iso) return ''
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso)
  return m ? `${m[3]}.${m[2]}.${m[1]}` : iso
}

const dateTime = new Intl.DateTimeFormat('tr-TR', {
  timeZone: 'Europe/Istanbul', day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit',
})
/** "2026-11-05T11:09:27Z" -> "05.11.2026 14:09", in Turkish time whatever the browser's zone. */
export function formatDateTime(iso: string): string {
  return dateTime.format(new Date(iso))
}

/** Human form of a finding's expected/actual text: "60 days", "1.300000 TRY", "1000 ADET", "1.25" or a code. */
export function formatValue(s: string): string {
  const t = s.trim()
  const days = /^(\d+) days$/.exec(t)
  if (days) return `${days[1]} gün`
  if (/^-?\d+(?:\.\d+)? [A-Z]{3}$/.test(t)) return formatAmountText(t)
  const qty = /^(-?\d+(?:\.\d+)?) (\S+)$/.exec(t)
  if (qty) return `${formatDecimal(qty[1]!)} ${qty[2]}`
  if (/^-?\d+(?:\.\d+)?$/.test(t)) return formatDecimal(t)
  return t
}
