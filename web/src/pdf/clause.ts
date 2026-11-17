// Bir sözleşme maddesini sayfanın metin parçaları arasında bulur. ai-service'teki doğrulama gibi büyük-küçük harfe ve
// boşluğa bakmaz; boşlukları tamamen atar, çünkü pdf.js metni pdfium'dan farklı böler ve boşluklar. İ küçülünce i +
// U+0307 olur ve noktasız ı ayrı kalır; bu yüzden dört Türkçe i'nin hepsini tek harfe indiriyoruz.
const fold = (s: string) => s.normalize('NFKC').toLowerCase().replace(/̇/g, '').replace(/ı/g, 'i')

// Maddenin kapsadığı parçaların sıra numaralarını döner; sayfada madde yoksa [] döner.
export function clauseItems(items: readonly { str: string }[], clause: string): number[] {
  const needle = [...fold(clause)].filter((c) => !/\s/.test(c)).join('')
  if (!needle) return []
  let hay = ''
  const owner: number[] = [] // owner[i] = hay[i]'nin geldiği parça
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
