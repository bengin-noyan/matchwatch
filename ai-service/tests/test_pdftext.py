import io
import shutil

import pytest
from reportlab.pdfgen import canvas

from matchwatch_ai import pdftext
from matchwatch_ai.pdftext import PdfError, extract_pages


def test_text_pdf_pages(text_pdf, fixture_pages):
    pages = extract_pages(text_pdf, ocr=None)
    assert [p.number for p in pages] == [1, 2] and not any(p.ocr for p in pages)
    assert "SZL-2026-014" in pages[0].text and "185,50" in pages[1].text
    assert [p.text for p in pages] == fixture_pages


def test_scanned_without_ocr_is_empty(scanned_pdf):
    assert [p.text.strip() for p in extract_pages(scanned_pdf, ocr=None)] == ["", ""]


def test_scanned_uses_ocr_callback(scanned_pdf):
    pages = extract_pages(scanned_pdf, ocr=lambda img: "OCR METNİ 185,50")
    assert all(p.ocr and p.text == "OCR METNİ 185,50" for p in pages)


@pytest.mark.skipif(shutil.which("tesseract") is None, reason="tesseract not installed")
def test_real_ocr(scanned_pdf):
    pages = extract_pages(scanned_pdf)
    assert "1234567890" in pages[0].text and "185,50" in pages[1].text


@pytest.mark.parametrize("data,code", [(b"hello", "NOT_PDF"), (b"%PDF-" + b"0" * (20 << 20), "TOO_LARGE")])
def test_rejects(data, code):
    with pytest.raises(PdfError) as e:
        extract_pages(data, ocr=None)
    assert e.value.code == code


def test_encrypted():
    buf = io.BytesIO()
    c = canvas.Canvas(buf, encrypt="pw")
    c.drawString(50, 750, "gizli")
    c.save()
    with pytest.raises(PdfError) as e:
        extract_pages(buf.getvalue(), ocr=None)
    assert e.value.code == "ENCRYPTED_PDF"


def test_too_many_pages(text_pdf, monkeypatch):
    monkeypatch.setattr(pdftext, "MAX_PAGES", 1)
    with pytest.raises(PdfError) as e:
        extract_pages(text_pdf, ocr=None)
    assert e.value.code == "TOO_LARGE"
