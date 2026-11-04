"""Regenerate contract fixtures: python tests/fixtures/make_fixtures.py (needs reportlab + a TTF with Turkish glyphs)."""
import io
import json
from pathlib import Path

import pypdfium2 as pdfium
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.pdfgen import canvas

HERE = Path(__file__).parent
FONTS = ["DejaVuSans.ttf", "C:/Windows/Fonts/arial.ttf"]

PAGES = [
    [
        "MAL ALIM SÖZLEŞMESİ",
        "Sözleşme No: SZL-2026-014",
        "Satıcı: Örnek Gıda Sanayi A.Ş., VKN: 1234567890",
        "Geçerlilik: 01.01.2026 – 31.12.2026",
        "Madde 5 – Ödeme: Bedel, fatura tarihinden itibaren 60 gün içinde ödenir.",
        "Madde 6 – Miktar toleransı: Teslim miktarında %5 sapma kabul edilir.",
        "Para birimi: Türk Lirası",
    ],
    [
        "EK-1 FİYAT LİSTESİ",
        "Beyaz Peynir 1 kg | kg | 185,50 TL",
        "Tereyağı 500 g | adet | 1.250,00 TL",
        "Soğuk zincir nakliye hizmeti | sefer | 2.400,00 TL",
        "Madde 9 – Yıllık hacim %3 iskonto uygulanır.",
    ],
]


def register_font() -> str:
    for f in FONTS:
        try:
            pdfmetrics.registerFont(TTFont("TR", f))
            return "TR"
        except Exception:
            continue
    raise SystemExit("no TTF with Turkish glyphs found")


def build_text_pdf() -> bytes:
    font = register_font()
    buf = io.BytesIO()
    c = canvas.Canvas(buf, pagesize=(595, 842))
    for lines in PAGES:
        c.setFont(font, 14)
        y = 780
        for line in lines:
            c.drawString(50, y, line)
            y -= 28
        c.showPage()
    c.save()
    return buf.getvalue()


def build_scanned_pdf(text_pdf: bytes) -> bytes:
    doc = pdfium.PdfDocument(text_pdf)
    imgs = [p.render(scale=200 / 72).to_pil().convert("L") for p in doc]
    buf = io.BytesIO()
    imgs[0].save(buf, "PDF", save_all=True, append_images=imgs[1:], resolution=200)
    return buf.getvalue()


if __name__ == "__main__":
    from matchwatch_ai.pdftext import extract_pages

    text_pdf = build_text_pdf()
    (HERE / "contract_text.pdf").write_bytes(text_pdf)
    (HERE / "contract_scanned.pdf").write_bytes(build_scanned_pdf(text_pdf))
    pages = [p.text for p in extract_pages(text_pdf, ocr=None)]
    (HERE / "contract_pages.json").write_text(json.dumps(pages, ensure_ascii=False, indent=2), encoding="utf-8")
