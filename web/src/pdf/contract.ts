// pdf.js ancak bir sözleşme sayfası ilk kez açıldığında yüklenir; böylece uygulamanın geri kalanı hafif kalır.
import { getDocument, GlobalWorkerOptions, Util, type PDFDocumentProxy } from 'pdfjs-dist'
import type { TextItem } from 'pdfjs-dist/types/src/display/api'
import workerSrc from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import { api } from '../api'
import { clauseItems } from './clause'

GlobalWorkerOptions.workerSrc = workerSrc

// Sekme açık kaldıkça her sözleşme bir kez indirilir; başarısız indirme unutulur ki tekrar denenebilsin.
// shortcut: standardFontDataUrl/wasmUrl yok; yani PDF'e gömülmemiş fontlar sistem fontlarıyla çizilir, JBIG2/JPX
// taramaları da JS çözücüler açar. Taramalar yanlış görünürse o klasörleri de pakete ekle.
const docs = new Map<string, Promise<PDFDocumentProxy>>()
function contractDoc(contractNo: string): Promise<PDFDocumentProxy> {
  let p = docs.get(contractNo)
  if (!p) {
    p = api.contractPDF(contractNo).then((data) => getDocument({ data }).promise)
    p.catch(() => docs.delete(contractNo))
    docs.set(contractNo, p)
  }
  return p
}

// Sayfanın yüzdesi cinsinden bir vurgu kutusu, böylece canvas'la birlikte ölçeklenir.
export type Box = { left: number; top: number; width: number; height: number }
export type Rendered = { pages: number; aspect: number; boxes: Box[] }

// Sözleşmenin pageNo numaralı sayfasını cssWidth genişliğinde canvas'a çizer ve maddeyi o sayfada bulur. PDF'te daha az
// sayfa varsa RangeError fırlatır.
export async function renderContractPage(contractNo: string, pageNo: number, clause: string, canvas: HTMLCanvasElement, cssWidth: number): Promise<Rendered> {
  const doc = await contractDoc(contractNo)
  if (pageNo > doc.numPages) throw new RangeError(`page ${pageNo} of ${doc.numPages}`)
  const page = await doc.getPage(pageNo)
  const base = page.getViewport({ scale: 1 })
  const viewport = page.getViewport({ scale: (cssWidth * (window.devicePixelRatio || 1)) / base.width })
  canvas.width = Math.floor(viewport.width)
  canvas.height = Math.floor(viewport.height)
  await page.render({ canvas, viewport }).promise

  const items = (await page.getTextContent()).items.filter((it): it is TextItem => 'str' in it)
  const boxes = clauseItems(items, clause).map((i) => {
    const it = items[i]!
    const tx = Util.transform(base.transform, it.transform)
    const h = Math.hypot(tx[2], tx[3]) // viewport biriminde font boyutu
    return {
      left: (tx[4] / base.width) * 100,
      top: ((tx[5] - h) / base.height) * 100,
      width: (it.width / base.width) * 100,
      height: ((h * 1.25) / base.height) * 100,
    }
  })
  return { pages: doc.numPages, aspect: base.height / base.width, boxes }
}
