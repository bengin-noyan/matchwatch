// pdf.js is loaded only when a contract page is first opened, so the rest of the app stays light.
import { getDocument, GlobalWorkerOptions, Util, type PDFDocumentProxy } from 'pdfjs-dist'
import type { TextItem } from 'pdfjs-dist/types/src/display/api'
import workerSrc from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import { api } from '../api'
import { clauseItems } from './clause'

GlobalWorkerOptions.workerSrc = workerSrc

// One download per contract while the tab is open; a failed one is forgotten so it can be retried.
// shortcut: no standardFontDataUrl/wasmUrl, so non-embedded fonts use system fonts and JBIG2/JPX scans the JS decoders; ship those dirs if scans look wrong.
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

// A highlight box in percent of the page, so it scales with the canvas.
export type Box = { left: number; top: number; width: number; height: number }
export type Rendered = { pages: number; aspect: number; boxes: Box[] }

// Draws page pageNo of the contract into canvas at cssWidth and finds the clause on it.
// Throws RangeError when the PDF has fewer pages.
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
    const h = Math.hypot(tx[2], tx[3]) // font size in viewport units
    return {
      left: (tx[4] / base.width) * 100,
      top: ((tx[5] - h) / base.height) * 100,
      width: (it.width / base.width) * 100,
      height: ((h * 1.25) / base.height) * 100,
    }
  })
  return { pages: doc.numPages, aspect: base.height / base.width, boxes }
}
