from collections.abc import Callable
from dataclasses import dataclass

import pypdfium2 as pdfium
import pytesseract
from PIL import Image

MAX_BYTES = 20 << 20
MAX_PAGES = 200
MIN_TEXT_CHARS = 20


@dataclass(frozen=True)
class Page:
    number: int
    text: str
    ocr: bool


class PdfError(Exception):
    def __init__(self, code: str, message: str = ""):
        super().__init__(message or code)
        self.code = code


def tesseract_ocr(img: Image.Image) -> str:
    return pytesseract.image_to_string(img, lang="tur")


def extract_pages(
    data: bytes, ocr: Callable[[Image.Image], str] | None = tesseract_ocr
) -> list[Page]:
    if not data.startswith(b"%PDF-"):
        raise PdfError("NOT_PDF")
    if len(data) > MAX_BYTES:
        raise PdfError("TOO_LARGE")
    try:
        doc = pdfium.PdfDocument(data)
    except pdfium.PdfiumError as e:
        if e.err_code == pdfium.raw.FPDF_ERR_PASSWORD:
            raise PdfError("ENCRYPTED_PDF") from e
        raise PdfError("NOT_PDF") from e
    try:
        if len(doc) > MAX_PAGES:
            raise PdfError("TOO_LARGE")
        pages = []
        for i, page in enumerate(doc, start=1):
            try:
                tp = page.get_textpage()
                try:
                    text = tp.get_text_range().replace("\r\n", "\n")
                finally:
                    tp.close()
                used_ocr = False
                if len(text.strip()) < MIN_TEXT_CHARS and ocr is not None:
                    bitmap = page.render(scale=300 / 72)
                    img = bitmap.to_pil()
                    try:
                        text = ocr(img)
                    finally:
                        img.close()
                        bitmap.close()
                    used_ocr = True
                pages.append(Page(i, text, used_ocr))
            finally:
                page.close()
        return pages
    finally:
        doc.close()
