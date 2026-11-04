import copy
import json
from pathlib import Path

import pytest

from matchwatch_ai.pdftext import Page

FIX = Path(__file__).parent / "fixtures"


@pytest.fixture
def text_pdf() -> bytes:
    return (FIX / "contract_text.pdf").read_bytes()


@pytest.fixture
def scanned_pdf() -> bytes:
    return (FIX / "contract_scanned.pdf").read_bytes()


@pytest.fixture
def fixture_pages() -> list[str]:
    return json.loads((FIX / "contract_pages.json").read_text(encoding="utf-8"))


class FakeLLM:
    """Returns (dict) or raises (Exception) the given answers in order; records user messages."""

    model = "fake"

    def __init__(self, answers):
        self.answers = list(answers)
        self.calls = []

    def complete_json(self, system, user, schema):
        self.calls.append(user)
        a = self.answers.pop(0)
        if isinstance(a, Exception):
            raise a
        return copy.deepcopy(a)


@pytest.fixture
def text_pages(fixture_pages) -> list[Page]:
    return [Page(i, t, False) for i, t in enumerate(fixture_pages, start=1)]


def _q(text, page):
    return {"text": text, "page": page}


_ROWS = [("Beyaz Peynir 1 kg", "kg", "185,50 TL", False), ("Tereyağı 500 g", "adet", "1.250,00 TL", False),
         ("Soğuk zincir nakliye hizmeti", "sefer", "2.400,00 TL", True)]

_GOOD = {
    "vendor_vkn": _q("1234567890", 1),
    "vendor_name": _q("Örnek Gıda Sanayi A.Ş.", 1),
    "contract_no": _q("SZL-2026-014", 1),
    "valid_from": _q("01.01.2026", 1),
    "valid_until": _q("31.12.2026", 1),
    "payment_term": _q("Madde 5 – Ödeme: Bedel, fatura tarihinden itibaren 60 gün içinde ödenir.", 1),
    "currency": _q("Türk Lirası", 1),
    "qty_tolerance": _q("%5", 1),
    "items": [{"name": _q(n, 2), "item_code": None, "barcode": None, "unit_price": _q(p, 2), "uom": _q(u, 2),
               "withholding_code": None, "is_service": s, "clause": _q(f"{n} | {u} | {p}", 2)}
              for n, u, p, s in _ROWS],
}


@pytest.fixture
def good_answer() -> dict:
    return copy.deepcopy(_GOOD)
