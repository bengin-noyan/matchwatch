import os

import pytest

from conftest import FakeLLM
from matchwatch_ai.extract import extract
from matchwatch_ai.llm import LLMError, OpenAICompatLLM
from matchwatch_ai.pdftext import Page


def walk(v):
    yield v
    if isinstance(v, dict):
        v = list(v.values())
    if isinstance(v, list):
        for x in v:
            yield from walk(x)


def test_happy_path(text_pages, good_answer):
    out = extract(text_pages, FakeLLM([good_answer]))
    d = out["draft"]
    assert d["vendor_vkn"] == "1234567890" and d["contract_no"] == "SZL-2026-014"
    assert (d["valid_from"], d["valid_until"]) == ("2026-01-01", "2026-12-31")
    assert d["payment_term_days"] == 60 and d["payment_term_source_page"] == 1
    assert "60 gün" in d["payment_term_clause_text"] and d["currency"] == "TRY" and d["qty_tolerance_pct"] == "5"
    assert [(i["name"], i["unit_price"], i["uom"], i["requires_delivery"]) for i in d["items"]] == [
        ("Beyaz Peynir 1 kg", "185.50", "KGM", True), ("Tereyağı 500 g", "1250.00", "C62", True),
        ("Soğuk zincir nakliye hizmeti", "2400.00", "sefer", False)]
    assert all(i["source_page"] == 2 and i["currency"] == "TRY" for i in d["items"])
    assert d["items"][0]["source_clause_text"] == "Beyaz Peynir 1 kg | kg | 185,50 TL"
    assert all(i["item_code"] == "" and i["barcode"] == "" and i["withholding_code"] == "" for i in d["items"])
    codes = {(w["code"], w.get("field")) for w in out["warnings"]}
    assert ("UNKNOWN_UOM", "items[2].uom") in codes and any(c == "PRICE_CLAUSE_REVIEW" for c, _ in codes)
    assert not any(c == "UNGROUNDED" for c, _ in codes)
    assert out["meta"] == {"pages": 2, "ocr_pages": [], "model": "fake"}


def test_ungrounded_price_dropped(text_pages, good_answer):
    good_answer["items"][0]["unit_price"] = {"text": "199,90 TL", "page": 2}
    out = extract(text_pages, FakeLLM([good_answer]))
    assert out["draft"]["items"][0]["unit_price"] is None and out["draft"]["items"][0]["name"] == "Beyaz Peynir 1 kg"
    assert {"code": "UNGROUNDED", "field": "items[0].unit_price"}.items() <= next(
        w for w in out["warnings"] if w["code"] == "UNGROUNDED").items()


def test_wrong_page_is_ungrounded(text_pages, good_answer):
    good_answer["contract_no"]["page"] = 2
    assert extract(text_pages, FakeLLM([good_answer]))["draft"]["contract_no"] is None


def test_out_of_range_page_and_empty_quote_ungrounded(text_pages, good_answer):
    good_answer["contract_no"]["page"] = 9
    good_answer["vendor_name"]["text"] = "  "
    out = extract(text_pages, FakeLLM([good_answer]))
    assert out["draft"]["contract_no"] is None and out["draft"]["vendor_name"] is None
    fields = {w.get("field") for w in out["warnings"] if w["code"] == "UNGROUNDED"}
    assert {"contract_no", "vendor_name"} <= fields


def test_clause_fallback_is_page_line(text_pages, good_answer):
    good_answer["items"][1]["clause"] = None
    items = extract(text_pages, FakeLLM([good_answer]))["draft"]["items"]
    assert items[1]["source_clause_text"] == "Tereyağı 500 g | adet | 1.250,00 TL"


def test_ungrounded_item_name_drops_item(text_pages, good_answer):
    good_answer["items"][0]["name"] = {"text": "Kaşar Peyniri", "page": 2}
    out = extract(text_pages, FakeLLM([good_answer]))
    assert [i["name"] for i in out["draft"]["items"]] == ["Tereyağı 500 g", "Soğuk zincir nakliye hizmeti"]
    assert any(w["code"] == "UNGROUNDED" and w["field"] == "items[0]" for w in out["warnings"])


def test_bad_vkn_kept_with_warning(fixture_pages, good_answer):
    pages = [Page(1, fixture_pages[0].replace("1234567890", "1234567891"), False), Page(2, fixture_pages[1], False)]
    good_answer["vendor_vkn"] = {"text": "1234567891", "page": 1}
    out = extract(pages, FakeLLM([good_answer]))
    assert out["draft"]["vendor_vkn"] == "1234567891"
    assert any(w["code"] == "VKN_CHECKSUM" and w["field"] == "vendor_vkn" for w in out["warnings"])


def test_withholding_code_only_literal(fixture_pages, good_answer):
    pages = [Page(1, fixture_pages[0], False),
             Page(2, fixture_pages[1] + "\nTevkifat kodu 604, yemek hizmeti", False)]
    good_answer["items"][0]["withholding_code"] = {"text": "604", "page": 2}
    good_answer["items"][1]["withholding_code"] = {"text": "yemek hizmeti", "page": 2}
    items = extract(pages, FakeLLM([good_answer]))["draft"]["items"]
    assert [i["withholding_code"] for i in items] == ["604", "", ""]


def test_chunking_merges_and_dedupes(text_pages, good_answer):
    llm = FakeLLM([good_answer, good_answer])
    out = extract(text_pages, llm, max_chars=1)
    assert len(llm.calls) == 2 and "=== SAYFA 1 ===" in llm.calls[0] and "=== SAYFA 2 ===" in llm.calls[1]
    assert "=== SAYFA 2 ===" not in llm.calls[0]
    assert len(out["draft"]["items"]) == 3 and out["draft"]["contract_no"] == "SZL-2026-014"
    # header fields are filled by group 1, so group 2's repeats do not warn
    assert not any(w["code"] == "UNGROUNDED" for w in out["warnings"])


def test_chunking_later_group_fills_header(text_pages, good_answer):
    first = {"contract_no": {"text": "SZL-9999", "page": 1}, "items": []}
    out = extract(text_pages, FakeLLM([first, good_answer]), max_chars=1)
    assert out["draft"]["contract_no"] == "SZL-2026-014"
    assert not any(w.get("field") == "contract_no" for w in out["warnings"])


def test_bad_output_retried_once(text_pages, good_answer):
    llm = FakeLLM([LLMError("LLM_BAD_OUTPUT"), good_answer])
    assert extract(text_pages, llm)["draft"]["contract_no"] == "SZL-2026-014" and len(llm.calls) == 2
    with pytest.raises(LLMError) as e:
        extract(text_pages, FakeLLM([LLMError("LLM_BAD_OUTPUT"), {"items": "nope"}]))
    assert e.value.code == "LLM_BAD_OUTPUT"
    with pytest.raises(LLMError) as e:
        extract(text_pages, FakeLLM([LLMError("LLM_UNAVAILABLE"), good_answer]))
    assert e.value.code == "LLM_UNAVAILABLE"


@pytest.mark.parametrize("bad", [
    [], {"contract_no": "SZL"}, {"contract_no": {"text": "x", "page": True}},
    {"items": [{"name": {"text": "x", "page": 1}, "is_service": "yes"}]},
])
def test_out_of_schema_is_bad_output(text_pages, bad):
    with pytest.raises(LLMError) as e:
        extract(text_pages, FakeLLM([bad, bad]))
    assert e.value.code == "LLM_BAD_OUTPUT"


def test_ocr_pages_reported(fixture_pages, good_answer):
    pages = [Page(i, t, True) for i, t in enumerate(fixture_pages, start=1)]
    out = extract(pages, FakeLLM([good_answer]))
    assert out["meta"]["ocr_pages"] == [1, 2]
    assert [w["page"] for w in out["warnings"] if w["code"] == "OCR_USED"] == [1, 2]


def test_no_float_in_output(text_pages, good_answer):
    assert not any(isinstance(v, float) for v in walk(extract(text_pages, FakeLLM([good_answer]))))


@pytest.mark.skipif(not os.environ.get("MATCHWATCH_TEST_LLM_URL"), reason="MATCHWATCH_TEST_LLM_URL unset")
def test_real_llm_fixture(text_pages):
    llm = OpenAICompatLLM(os.environ["MATCHWATCH_TEST_LLM_URL"],
                          os.environ.get("MATCHWATCH_TEST_LLM_MODEL", "qwen2.5:7b-instruct"))
    d = extract(text_pages, llm)["draft"]
    assert d["contract_no"] == "SZL-2026-014"
    assert any(i["unit_price"] is not None for i in d["items"])
