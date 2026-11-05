// Finds a contract clause among a page's text items. Like the ai-service's grounding it ignores case and
// whitespace; it drops whitespace entirely because pdf.js splits and spaces text items differently from pdfium.
// İ lowercases to i + U+0307 and the dotless ı stays apart, so all four Turkish i's fold to one.
const fold = (s: string) => s.normalize('NFKC').toLowerCase().replace(/̇/g, '').replace(/ı/g, 'i')

// Returns the indexes of the items the clause spans, or [] when the page does not contain it.
export function clauseItems(items: readonly { str: string }[], clause: string): number[] {
  const needle = [...fold(clause)].filter((c) => !/\s/.test(c)).join('')
  if (!needle) return []
  let hay = ''
  const owner: number[] = [] // owner[i] = item that hay[i] came from
  items.forEach((it, i) => {
    for (const c of fold(it.str)) {
      if (/\s/.test(c)) continue
      hay += c
      owner.push(i)
    }
  })
  const at = hay.indexOf(needle)
  if (at < 0) return []
  return [...new Set(owner.slice(at, at + needle.length))]
}
