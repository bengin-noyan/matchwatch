# matchwatch Faz 2a — PDF Sözleşme Çıkarımı Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** PDF sözleşmeden Faz 1 `ContractDoc` JSON şemasında, kanıtlı ve kaynak metne dayandırılmış bir taslak üreten durumsuz Python servisi ile, bu taslağı saklayan, insana gösteren ve onaylanınca mevcut `POST /api/v1/contracts` yolundan aktif sözleşmeye çeviren Go uçları.

**Architecture:** `ai-service/` (Python 3.12, FastAPI) `PDF bytes → {draft, warnings, meta}` fonksiyonudur: `pypdfium2` metin katmanı, boş sayfada Tesseract `tur` OCR, yerel LLM'den (Ollama, OpenAI-uyumlu) yalnızca **alıntı** + sayfa, alıntının sayfa metninde geçtiği doğrulaması, sayıların deterministik Türkçe ayrıştırması. Go API PDF'i saklar, `contract_drafts` satırını önce commit eder, servisi çağırır, sonucu yazar; onay mevcut sözleşme yoluna `draft_id` ekler.

**Tech Stack:** Python 3.12 (yerelde 3.14 da çalışmalı), FastAPI, uvicorn, httpx, pydantic v2, pypdfium2, pytesseract, Pillow, pytest; geliştirme: reportlab. Go tarafı Faz 1 ile aynı (`net/http`, pgx/v5, shopspring/decimal). Ollama + `qwen2.5:7b-instruct`.

**Spec:** `docs/superpowers/specs/2026-11-04-faz2a-pdf-contract-extraction-design.md` (bu plan ondan türetilir; çelişkide tasarım belgesi geçerli). Üst spec: `docs/spec.md` (D1–D15).

## Global Constraints

- Para, miktar, oran için hiçbir yerde `float` yok — Python'da `decimal.Decimal`, JSON'da string; Go'da `decimal.Decimal`. Python testlerinden biri `ai-service/matchwatch_ai/` altında `float(` ve `: float` arar ve bulursa başarısız olur.
- LLM sayı hesaplamaz ve karar vermez; yalnızca `{"text": <sözleşmede aynen geçen ifade>, "page": <int>}` alıntısı döndürür. Sayıyı Python ayrıştırır.
- Dayandırılamayan (alıntısı belirtilen sayfa metninde geçmeyen) hiçbir değer taslağa girmez: alan `null` + `UNGROUNDED` uyarısı.
- Tevkifat kodu yalnızca alıntı `^(6\d\d|8\d\d)$` ise doldurulur; tahmin yok.
- Python servisi DB'ye bağlanmaz, PDF/metni diske yazmaz ve loglamaz (KVKK). Loglarda yalnızca boyut, sayfa sayısı, süre, hata kodu.
- `LLM_BASE_URL` host'u loopback, RFC1918/özel adres veya nokta içermeyen bir ad (compose servis adı) değilse servis başlamaz.
- Python ↔ Go: `Authorization: Bearer <AI_SERVICE_TOKEN>`, `hmac.compare_digest`. Token boşsa servis başlamaz.
- Limitler: PDF ≤ 20 MiB (20 << 20 bayt), ≤ 200 sayfa; Go `AI_EXTRACT_TIMEOUT` varsayılan 5 dk.
- Go multi-tenancy kuralları (CLAUDE.md): yeni tablo `ENABLE`+`FORCE` RLS, `NULLIF(current_setting('app.current_tenant_id', true), '')::uuid` politikası, composite FK, her sorgu `Store.InTenant` içinde. Yeni migration `005`; `001`–`004`'e dokunulmaz.
- Entegrasyon testleri tekrar çalıştırılabilir (benzersiz tenant/sözleşme no). Python'da gerçek LLM testi yalnız `MATCHWATCH_TEST_LLM_URL` varsa; OCR testi yalnız `tesseract` PATH'teyse.
- Commit mesajlarında Claude imzası/`Co-Authored-By` yok.

### Ortak şema (Task 3 üretir, Task 4 sunar, Task 6 tüketir)

Servis yanıtı `200`:

```json
{
  "draft": {
    "vendor_vkn": "1234567890", "vendor_name": "Örnek Gıda A.Ş.", "contract_no": "SZL-2026-014",
    "valid_from": "2026-01-01", "valid_until": "2026-12-31",
    "payment_term_days": 60, "payment_term_source_page": 1, "payment_term_clause_text": "…",
    "currency": "TRY", "qty_tolerance_pct": "5",
    "items": [{ "item_code": "", "barcode": "", "name": "Beyaz Peynir 1 kg", "unit_price": "185.50",
                "currency": "TRY", "uom": "KGM", "withholding_code": "", "requires_delivery": true,
                "source_page": 2, "source_clause_text": "…" }]
  },
  "warnings": [{ "code": "UNGROUNDED", "field": "items[1].unit_price", "page": 2, "message": "…" }],
  "meta": { "pages": 2, "ocr_pages": [], "model": "qwen2.5:7b-instruct", "duration_ms": 41230 }
}
```

Bilinmeyen skaler alan `null`; metin alanları (`item_code`, `barcode`, `withholding_code`) bilinmiyorsa `""` (Go JSON'u boş string bekler). Uyarı kodları: `UNGROUNDED`, `UNPARSEABLE`, `AMBIGUOUS_NUMBER`, `VKN_CHECKSUM`, `UNKNOWN_UOM`, `UNKNOWN_CURRENCY`, `PRICE_CLAUSE_REVIEW`, `OCR_USED`. Hata gövdesi: `{"code": "...", "error": "..."}` (Go `errorBody` ile aynı alanlar).

## Review Focus

1. **Halüsinasyon fiyatı** — LLM sayfada olmayan `"199,90 TL"` döndürür → `unit_price: null`, `UNGROUNDED`; kalem yine listelenir ki insan görsün. Test: Task 3 `test_ungrounded_price_dropped`.
2. **Binlik/ondalık karışıklığı** — `1.250` (bin iki yüz elli mi, bir virgül iki beş mi) → `1250` + `AMBIGUOUS_NUMBER`; `1.250,00` → `1250.00` uyarısız. Test: Task 1 `test_parse_tr_number`.
3. **İstemci zaman aşımı / süreç çökmesi çıkarım sırasında** — taslak `EXTRACTING` olarak kalır, `GET` görür, `POST …/extract` kurtarır; PDF kaybolmaz. Test: Task 7 `TestDraftRecoverAfterFailure`.
4. **Başka tenant'ın taslağını onaylama** — `draft_id` başka tenant'a aitse `404`, sözleşme oluşmaz (RLS). Test: Task 7 `TestDraftCrossTenant`.
5. **Go `WriteTimeout` (60 s) çıkarımdan kısa** — uzun çıkarımda yanıt kesilmemeli; handler `http.NewResponseController(w).SetWriteDeadline` ile süreyi `AI_EXTRACT_TIMEOUT + 30s`'e uzatır. Test: Task 7 `TestDraftSlowExtraction` (sahte servis 2 s bekler, sunucu `WriteTimeout` 1 s ile kurulur, yanıt `201` gelir).

---

### Task 1: Python iskeleti + Türkçe deterministik ayrıştırıcılar

**Files:**
- Create: `ai-service/requirements.txt`, `ai-service/requirements-dev.txt`, `ai-service/pyproject.toml` (yalnızca `[tool.pytest.ini_options] testpaths=["tests"]`), `ai-service/matchwatch_ai/__init__.py`, `ai-service/matchwatch_ai/parsing.py`, `ai-service/.gitignore` (`.venv/`, `__pycache__/`)
- Test: `ai-service/tests/test_parsing.py`, `ai-service/tests/test_no_float.py`

**Interfaces:**
- Produces (hepsi `matchwatch_ai.parsing`; ayrıştırılamazsa `None` döner, istisna atmaz):
  - `parse_tr_number(text: str) -> tuple[Decimal | None, bool]` — ikinci değer "belirsizdi".
  - `parse_currency(text: str) -> str | None` — ISO 4217.
  - `parse_date(text: str) -> str | None` — `YYYY-MM-DD`.
  - `parse_days(text: str) -> int | None`
  - `parse_percent(text: str) -> Decimal | None`
  - `vkn_valid(v: str) -> bool` — 10 hane VKN kontrol hanesi; 11 hane TCKN algoritması.
  - `map_uom(text: str) -> tuple[str, bool]` — (kod veya ham metin, tanındı mı).

`requirements.txt`: `fastapi`, `uvicorn[standard]`, `httpx`, `pydantic>=2`, `pypdfium2`, `pytesseract`, `Pillow`, `python-multipart` — hepsi `==` ile o günün son sürümüne sabitlenir. `requirements-dev.txt`: `-r requirements.txt`, `pytest`, `reportlab`.

- [ ] **Step 1: Write the failing tests**

```python
D = Decimal
@pytest.mark.parametrize("s,val,amb", [
    ("1.250,00", D("1250.00"), False), ("1250,5", D("1250.5"), False), ("12,50 TL", D("12.50"), False),
    ("185,50", D("185.50"), False), ("1.250", D("1250"), True), ("1.234.567,8", D("1234567.8"), False),
    ("0,0375", D("0.0375"), False), ("12.5", D("12.5"), False), ("1,250.00", D("1250.00"), False),
    ("abc", None, False), ("", None, False)])
def test_parse_tr_number(s, val, amb): assert parse_tr_number(s) == (val, amb)

@pytest.mark.parametrize("s,c", [("12,50 TL", "TRY"), ("₺10", "TRY"), ("Türk Lirası", "TRY"), ("TRY", "TRY"),
    ("$5", "USD"), ("ABD Doları", "USD"), ("€3", "EUR"), ("Avro", "EUR"), ("Euro", "EUR"), ("GBP 4", "GBP"), ("10", None)])
def test_parse_currency(s, c): assert parse_currency(s) == c

@pytest.mark.parametrize("s,d", [("01.01.2026", "2026-01-01"), ("31/12/2026", "2026-12-31"),
    ("1 Ocak 2026", "2026-01-01"), ("15 Şubat 2026", "2026-02-15"), ("31 Aralık 2026", "2026-12-31"),
    ("31.02.2026", None), ("yarın", None)])
def test_parse_date(s, d): assert parse_date(s) == d

@pytest.mark.parametrize("s,n", [("60 gün", 60), ("altmış (60) gün", 60), ("fatura tarihinden itibaren 30 gün", 30),
    ("altmış gün", None), ("peşin", None)])
def test_parse_days(s, n): assert parse_days(s) == n

@pytest.mark.parametrize("s,p", [("%5", D("5")), ("% 5", D("5")), ("yüzde 2,5", D("2.5")), ("5", None)])
def test_parse_percent(s, p): assert parse_percent(s) == p

def test_vkn_valid():
    assert vkn_valid("1234567890") and vkn_valid("9876543217") and vkn_valid("10000000146")
    assert not vkn_valid("1234567891") and not vkn_valid("123") and not vkn_valid("01000000146")

@pytest.mark.parametrize("s,r", [("adet", ("C62", True)), ("Adet", ("C62", True)), ("kg", ("KGM", True)),
    ("kilogram", ("KGM", True)), ("litre", ("LTR", True)), ("lt", ("LTR", True)), ("m", ("MTR", True)),
    ("ton", ("TNE", True)), ("saat", ("HUR", True)), ("ay", ("MON", True)), ("koli", ("koli", False))])
def test_map_uom(s, r): assert map_uom(s) == r
```

`test_no_float.py`: `matchwatch_ai/**/*.py` dosyalarında `re.search(r"\bfloat\s*\(|:\s*float\b|->\s*float\b", src)` eşleşmesi varsa dosya adını basarak başarısız olur.

- [ ] **Step 2: Run tests to verify they fail**

Run (`ai-service/` içinde): `python -m venv .venv && .venv/Scripts/python -m pip install -r requirements-dev.txt && .venv/Scripts/python -m pytest -q`
Expected: FAIL, `ImportError` (`parsing` yok). `test_no_float` geçer.

- [ ] **Step 3: Implement `matchwatch_ai/parsing.py`**

Sayı kuralı: metinden ilk `[\d.,]+` grubu alınır. Hem `.` hem `,` varsa en sağdaki ayırıcı ondalıktır. Yalnız `,` varsa ondalıktır. Yalnız `.` varsa: birden çok `.` → binlik; tek `.` ve ardından tam 3 hane → binlik + belirsiz=True; aksi halde ondalık. Türkçe ay adları: ocak, şubat, mart, nisan, mayıs, haziran, temmuz, ağustos, eylül, ekim, kasım, aralık (karşılaştırmadan önce `casefold()`). Tarih geçerliliği `datetime.date(y,m,d)` ile sınanır. VKN: `t=(int(v[i])+9-i)%10`, `i=0..8`; `t==9` ise toplam `+=9`, değilse `+= (t*2**(9-i))%9`; kontrol hanesi `(10-toplam%10)%10`. TCKN: ilk hane ≠ 0, `((tek konumlar 1,3,5,7,9 toplamı)*7 − (çift konumlar 2,4,6,8 toplamı)) % 10 == d10`, `sum(d1..d10) % 10 == d11`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `.venv/Scripts/python -m pytest -q`
Expected: tümü PASS.

- [ ] **Step 5: Commit**

```bash
git add ai-service
git commit -m "feat(ai): scaffold ai-service with Turkish number, date and VKN parsers"
```

---

### Task 2: PDF metin çıkarma + OCR + fikstürler

**Files:**
- Create: `ai-service/matchwatch_ai/pdftext.py`, `ai-service/tests/fixtures/make_fixtures.py`, `ai-service/tests/fixtures/contract_text.pdf`, `ai-service/tests/fixtures/contract_scanned.pdf`, `ai-service/tests/fixtures/contract_pages.json` (fikstürün beklenen sayfa metinleri; Task 3 sahte LLM'i bundan alıntılar), `ai-service/tests/conftest.py`
- Test: `ai-service/tests/test_pdftext.py`

**Interfaces:**
- Consumes: —
- Produces (`matchwatch_ai.pdftext`):
  - `@dataclass(frozen=True) class Page: number: int; text: str; ocr: bool`
  - `class PdfError(Exception): code: str` — kodlar `NOT_PDF`, `ENCRYPTED_PDF`, `TOO_LARGE`.
  - `MAX_BYTES = 20 << 20`, `MAX_PAGES = 200`, `MIN_TEXT_CHARS = 20`
  - `extract_pages(data: bytes, ocr: Callable[[Image.Image], str] | None = tesseract_ocr) -> list[Page]` — `ocr=None` ise OCR atlanır, boş sayfa `text=""`, `ocr=False` döner.
  - `tesseract_ocr(img: Image.Image) -> str` — `pytesseract.image_to_string(img, lang="tur")`.
- `conftest.py` fikstürleri: `text_pdf: bytes`, `scanned_pdf: bytes`, `fixture_pages: list[str]`.

Fikstür sözleşme içeriği (uydurma; `make_fixtures.py` reportlab ile DejaVuSans ya da Türkçe karakter destekli yerleşik bir TTF kullanır, bulunamazsa Windows `C:/Windows/Fonts/arial.ttf`):
- Sayfa 1: "MAL ALIM SÖZLEŞMESİ", "Sözleşme No: SZL-2026-014", "Satıcı: Örnek Gıda Sanayi A.Ş., VKN: 1234567890", "Geçerlilik: 01.01.2026 – 31.12.2026", "Madde 5 – Ödeme: Bedel, fatura tarihinden itibaren 60 gün içinde ödenir.", "Madde 6 – Miktar toleransı: Teslim miktarında %5 sapma kabul edilir.", "Para birimi: Türk Lirası".
- Sayfa 2: "EK-1 FİYAT LİSTESİ" ve tablo satırları: "Beyaz Peynir 1 kg | kg | 185,50 TL", "Tereyağı 500 g | adet | 1.250,00 TL", "Soğuk zincir nakliye hizmeti | sefer | 2.400,00 TL"; ayrıca "Madde 9 – Yıllık hacim %3 iskonto uygulanır."

Taranmış sürüm: `pypdfium2` ile her sayfa 200 DPI render → Pillow `save(..., "PDF", save_all=True)` (metin katmanı yok).

- [ ] **Step 1: Generate fixtures**

Run: `.venv/Scripts/python tests/fixtures/make_fixtures.py`
Expected: iki PDF ve `contract_pages.json` oluşur; her PDF < 300 KB.

- [ ] **Step 2: Write the failing tests**

```python
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
    assert "SZL-2026-014" in extract_pages(scanned_pdf)[0].text

@pytest.mark.parametrize("data,code", [(b"hello", "NOT_PDF"), (b"%PDF-" + b"0" * (20 << 20), "TOO_LARGE")])
def test_rejects(data, code):
    with pytest.raises(PdfError) as e: extract_pages(data, ocr=None)
    assert e.value.code == code
```

Ayrıca `test_encrypted`: `make_fixtures.py`'ın şifreli bir sürüm üretmesi gerekmez; testte reportlab `canvas.Canvas(..., encrypt="pw")` ile bellekte üretilir → `ENCRYPTED_PDF`. `test_too_many_pages`: `MAX_PAGES` `monkeypatch` ile 1'e indirilir, 2 sayfalık fikstür → `TOO_LARGE`.

- [ ] **Step 3: Run tests to verify they fail**

Run: `.venv/Scripts/python -m pytest tests/test_pdftext.py -q`
Expected: FAIL, `ImportError`.

- [ ] **Step 4: Implement `pdftext.py`**

`%PDF-` imzası ve boyut önce kontrol edilir; `pypdfium2.PdfDocument(data)` parola hatasında `ENCRYPTED_PDF`, diğer açma hatalarında `NOT_PDF`. Metin `page.get_textpage().get_text_range()`; `\r\n` → `\n`. `len(text.strip()) < MIN_TEXT_CHARS` ve `ocr` verilmişse sayfa `page.render(scale=300/72).to_pil()` ile OCR'lanır.

- [ ] **Step 5: Run tests to verify they pass**

Run: `.venv/Scripts/python -m pytest -q`
Expected: tümü PASS (`test_real_ocr` Windows'ta SKIP).

- [ ] **Step 6: Commit**

```bash
git add ai-service
git commit -m "feat(ai): extract PDF page text with OCR fallback and add contract fixtures"
```

---

### Task 3: LLM istemcisi + çıkarım boru hattı (alıntı → dayandırma → ayrıştırma)

**Files:**
- Create: `ai-service/matchwatch_ai/llm.py`, `ai-service/matchwatch_ai/extract.py`
- Test: `ai-service/tests/test_extract.py`, `ai-service/tests/test_llm.py`

**Interfaces:**
- Consumes: `Page`, `parsing.*` (Task 1–2).
- Produces:
  - `llm.LLMError(Exception)` — `code` `LLM_UNAVAILABLE` | `LLM_BAD_OUTPUT`.
  - `llm.LLM` (Protocol): `model: str`; `complete_json(system: str, user: str, schema: dict) -> dict`.
  - `llm.OpenAICompatLLM(base_url: str, model: str, timeout_s: int = 240)` — `POST {base_url}/v1/chat/completions`, gövde `{"model", "messages", "temperature": 0, "response_format": {"type": "json_schema", "json_schema": {"name": "contract", "schema": schema}}}`; bağlantı/5xx/zaman aşımı → `LLM_UNAVAILABLE`; içerik JSON değilse → `LLM_BAD_OUTPUT`.
  - `extract.QUOTE_SCHEMA: dict` — LLM yanıt şeması (aşağıda).
  - `extract.extract(pages: list[Page], llm: LLM, max_chars: int = 24000) -> dict` — Global Constraints'teki `{draft, warnings, meta}` (meta'daki `duration_ms` Task 4'te eklenir; burada `pages`, `ocr_pages`, `model`).

LLM yanıt şeması (`Q = {"text": str, "page": int} | null`):

```
{ vendor_vkn: Q, vendor_name: Q, contract_no: Q, valid_from: Q, valid_until: Q,
  payment_term: Q, currency: Q, qty_tolerance: Q,
  items: [ { name: Q, item_code: Q, barcode: Q, unit_price: Q, uom: Q,
             withholding_code: Q, is_service: bool | null, clause: Q } ] }
```

Sistem istemi (Türkçe) şunları açıkça söyler: yalnızca sözleşmede **aynen** geçen ifadeyi alıntıla; hesaplama, çevirme, tahmin yapma; bilmiyorsan `null`; `page` alıntının geçtiği `=== SAYFA n ===` numarasıdır. Kullanıcı mesajı sayfa metinleridir.

Eşleme kuralları (`extract`):
- Her `Q` dayandırılır: `norm(q.text) in norm(pages[q.page-1].text)`, `norm = lambda s: " ".join(s.casefold().split())`; sayfa aralık dışıysa da dayandırılamaz.
- `vendor_vkn` → yalnız rakamlar; `vkn_valid` değilse değer korunur + `VKN_CHECKSUM`. `valid_from/until` → `parse_date`. `payment_term` → `payment_term_days = parse_days`, `payment_term_source_page = q.page`, `payment_term_clause_text = q.text`. `currency` → `parse_currency`. `qty_tolerance` → `qty_tolerance_pct = str(parse_percent)`; alıntı yoksa `null` (insan 0 girebilir, otomatik 0 yazılmaz).
- Kalem: `unit_price` → `parse_tr_number` (string olarak); kalem `currency` = `parse_currency(unit_price.text)` yoksa sözleşme `currency`; `uom` → `map_uom` (tanınmazsa ham + `UNKNOWN_UOM`); `withholding_code` regex dışıysa `""`; `requires_delivery = not is_service` (`null` → `true`); `source_page = unit_price.page`; `source_clause_text = clause.text` dayandırılmışsa, değilse `unit_price.text`'i içeren sayfa satırı.
- Dayandırılamayan alan `null` + `UNGROUNDED`; ayrıştırılamayan `null` + `UNPARSEABLE`; belirsiz sayı `AMBIGUOUS_NUMBER`. `name` dayandırılamayan kalem tamamen atılır + `UNGROUNDED` (`field: "items[i]"`).
- Sayfa grupları: sayfalar sırayla `max_chars`'ı aşmayacak şekilde gruplanır (tek sayfa aşarsa tek başına grup); her grup için bir LLM çağrısı. Başlık alanları: ilk dayandırılmış değer kazanır. Kalemler birleştirilir; `(norm(name), norm(unit_price.text))` aynıysa tekrar atılır.
- `LLM_BAD_OUTPUT` (şema dışı/JSON değil): aynı grup için 1 tekrar, sonra istisna yayılır.
- `PRICE_CLAUSE_REVIEW`: herhangi bir sayfada `iskonto|indirim|fiyat farkı|endeks` (casefold) geçerse sayfa başına bir uyarı.
- `OCR_USED`: `ocr=True` her sayfa için bir uyarı.

- [ ] **Step 1: Write the failing tests**

`FakeLLM` testte tanımlanır: kurucuya verilen `dict` listesini sırayla döndürür, aldığı `user` mesajlarını kaydeder. `good_answer` fikstürü `contract_pages.json`'dan aynen kopyalanmış alıntılarla kurulur (3 kalem, VKN, tarih, vade, %5, "Türk Lirası").

```python
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
    codes = {(w["code"], w.get("field")) for w in out["warnings"]}
    assert ("UNKNOWN_UOM", "items[2].uom") in codes and any(c == "PRICE_CLAUSE_REVIEW" for c, _ in codes)
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

def test_ungrounded_item_name_drops_item(text_pages, good_answer): ...   # name "Kaşar Peyniri" → 2 kalem kalır
def test_bad_vkn_kept_with_warning(...): ...                           # alıntı sayfada "1234567891" geçecek şekilde sayfa metni değiştirilir → değer korunur, VKN_CHECKSUM
def test_withholding_code_only_literal(...): ...                       # "604" → "604"; "yemek hizmeti" → ""
def test_chunking_merges_and_dedupes(text_pages, good_answer):         # max_chars=1 → 2 LLM çağrısı; 2. yanıt aynı kalemleri tekrarlar → 3 kalem
def test_bad_output_retried_once(...): ...                             # FakeLLM önce LLMError("LLM_BAD_OUTPUT"), sonra iyi yanıt → başarılı; iki kez hata → LLMError
def test_ocr_pages_reported(...): ...                                  # Page(ocr=True) → meta.ocr_pages == [1, 2], OCR_USED uyarıları
def test_no_float_in_output(text_pages, good_answer):
    assert not any(isinstance(v, float) for v in walk(extract(text_pages, FakeLLM([good_answer]))))
```

`test_llm.py`: `httpx.MockTransport` ile `OpenAICompatLLM` — istek gövdesinde `temperature == 0` ve `response_format.type == "json_schema"`; `choices[0].message.content` JSON'u döner; 503 → `LLM_UNAVAILABLE`; içerik `"not json"` → `LLM_BAD_OUTPUT`. `OpenAICompatLLM` test için `transport: httpx.BaseTransport | None = None` parametresi alır.

`MATCHWATCH_TEST_LLM_URL` varsa `test_real_llm_fixture` gerçek modelle `text_pages`'i çıkarır ve yalnız `contract_no == "SZL-2026-014"` ile en az bir dayandırılmış `unit_price` bekler.

- [ ] **Step 2: Run tests to verify they fail**

Run: `.venv/Scripts/python -m pytest tests/test_extract.py tests/test_llm.py -q`
Expected: FAIL, `ImportError`.

- [ ] **Step 3: Implement `llm.py` and `extract.py`** (kurallar yukarıda; `extract` saf fonksiyondur, ağ yalnız `LLM` üzerinden)

- [ ] **Step 4: Run tests to verify they pass**

Run: `.venv/Scripts/python -m pytest -q`
Expected: tümü PASS.

- [ ] **Step 5: Commit**

```bash
git add ai-service
git commit -m "feat(ai): grounded quote-based contract extraction pipeline"
```

---

### Task 4: FastAPI uygulaması, güvenlik kapıları, Dockerfile

**Files:**
- Create: `ai-service/matchwatch_ai/config.py`, `ai-service/matchwatch_ai/app.py`, `ai-service/Dockerfile`, `ai-service/.dockerignore`, `ai-service/README.md` (çalıştırma + KVKK notu, kısa)
- Test: `ai-service/tests/test_app.py`, `ai-service/tests/test_config.py`

**Interfaces:**
- Consumes: `extract_pages`, `PdfError`, `extract`, `OpenAICompatLLM`, `LLMError`.
- Produces:
  - `config.Settings` (dataclass): `token: str`, `llm_base_url: str`, `llm_model: str` (varsayılan `qwen2.5:7b-instruct`), `llm_timeout_s: int` (240), `max_chars: int` (24000). `config.load(env: Mapping[str, str] = os.environ) -> Settings` — env adları `AI_SERVICE_TOKEN`, `LLM_BASE_URL`, `LLM_MODEL`, `LLM_TIMEOUT_S`, `LLM_MAX_CHARS`; `ValueError` atar: token boş/< 32 karakter, URL boş, host özel değil.
  - `config.is_private_host(host: str) -> bool` — `localhost`, nokta içermeyen ad, `ipaddress.ip_address(host).is_private or .is_loopback`. DNS çözümlemesi yapılmaz (çözümleme değişebilir; ad kuralı yeterli). `shortcut: host adına bakar, DNS'e değil; TR bulutunda iç DNS adları (ör. ollama.internal) gerekirse izinli sonek listesi eklenir`.
  - `app.create_app(settings: Settings, llm: LLM | None = None, ocr=tesseract_ocr) -> FastAPI` — `llm` verilmezse `OpenAICompatLLM` kurulur. Modül düzeyinde `app = create_app(load())` yalnız `if __name__`/uvicorn factory ile: uvicorn `matchwatch_ai.app:factory --factory`.
  - `POST /v1/extract`: gövde ham PDF baytları (`Content-Type: application/pdf`), multipart değil. Başarı `200` + şema (`meta.duration_ms` eklenir). Hatalar: token yok/yanlış `401 UNAUTHORIZED`; `PdfError` → `NOT_PDF` 400, `TOO_LARGE` 413, `ENCRYPTED_PDF` 422; `LLMError` → 502 kendi koduyla. Gövde `MAX_BYTES`'tan büyükse okumadan `413` (`Content-Length` + akış sınırı).
  - `GET /healthz` → `{"status":"ok"}` (token istemez).
- Loglama: istek başına tek satır `extract pages=%d bytes=%d ocr=%d ms=%d code=%s`; metin/alıntı asla.

- [ ] **Step 1: Write the failing tests**

```python
@pytest.mark.parametrize("url,ok", [("http://ollama:11434", True), ("http://localhost:11434", True),
    ("http://127.0.0.1:11434", True), ("http://10.0.3.4", True), ("http://192.168.1.2", True),
    ("https://api.openai.com", False), ("https://8.8.8.8", False), ("", False)])
def test_llm_url_gate(url, ok): ...   # load({"AI_SERVICE_TOKEN": "t"*32, "LLM_BASE_URL": url}) → Settings veya ValueError
def test_short_token_rejected(): ...  # "t"*31 → ValueError

def test_extract_ok(client, text_pdf):           # client = TestClient(create_app(settings, llm=FakeLLM([good_answer]), ocr=None))
    r = client.post("/v1/extract", content=text_pdf, headers=AUTH | {"Content-Type": "application/pdf"})
    assert r.status_code == 200 and r.json()["draft"]["contract_no"] == "SZL-2026-014"
    assert isinstance(r.json()["meta"]["duration_ms"], int)
def test_requires_token(client, text_pdf): ...    # header yok → 401; yanlış token → 401
@pytest.mark.parametrize("body,status,code", [(b"hello", 400, "NOT_PDF")])
def test_pdf_errors(client, body, status, code): ...
def test_oversize_413(client): ...                # 20 MiB + 1 bayt → 413 TOO_LARGE
def test_llm_down_502(text_pdf): ...              # FakeLLM LLMError("LLM_UNAVAILABLE") → 502 {"code": "LLM_UNAVAILABLE"}
def test_no_content_in_logs(client, text_pdf, caplog):
    client.post(...)
    assert "SZL-2026-014" not in caplog.text and "185,50" not in caplog.text
def test_healthz_no_auth(client): ...
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `.venv/Scripts/python -m pytest tests/test_app.py tests/test_config.py -q`
Expected: FAIL, `ImportError`.

- [ ] **Step 3: Implement `config.py`, `app.py`**

- [ ] **Step 4: Run tests to verify they pass**

Run: `.venv/Scripts/python -m pytest -q`
Expected: tümü PASS.

- [ ] **Step 5: Write `Dockerfile`**

`python:3.12-slim`; `apt-get install --no-install-recommends tesseract-ocr tesseract-ocr-tur`; `pip install -r requirements.txt`; root olmayan kullanıcı; `CMD ["uvicorn", "matchwatch_ai.app:factory", "--factory", "--host", "0.0.0.0", "--port", "8090"]`.

Run (Docker varsa): `docker build -t matchwatch-ai ai-service && docker run --rm matchwatch-ai python -m pytest -q` — bunun için test dosyaları imaja girmez; bunun yerine `docker run --rm -v ${PWD}/ai-service:/src -w /src matchwatch-ai sh -c "pip install pytest reportlab && python -m pytest -q"`.
Expected: tümü PASS, `test_real_ocr` dahil (Tesseract imajda).

- [ ] **Step 6: Commit**

```bash
git add ai-service
git commit -m "feat(ai): FastAPI extract endpoint with token auth and private-LLM gate"
```

---

### Task 5: Migration 005 + `contract_drafts` repository

**Files:**
- Create: `backend-go/migrations/005_contract_drafts.sql`, `backend-go/internal/repository/drafts.go`
- Modify: `backend-go/internal/transport/http/api_integration_test.go` (cleanup listesine `contract_drafts`'ı `contracts`'tan önce ekle), repository entegrasyon testlerinin cleanup'ı da aynı şekilde
- Test: `backend-go/internal/repository/drafts_integration_test.go`

**Interfaces:**
- Produces (`repository`):
  - `type DraftStatus = string`; sabitler `DraftExtracting = "EXTRACTING"`, `DraftReady = "READY"`, `DraftFailed = "FAILED"`, `DraftApproved = "APPROVED"`.
  - `type Draft struct { ID uuid.UUID; Status string; PDFPath, PDFSHA256 string; Draft, Warnings, Meta json.RawMessage; ErrorCode string; ContractID *uuid.UUID; CreatedAt, UpdatedAt time.Time }`
  - `func (tx *Tx) InsertDraft(ctx, id uuid.UUID, pdfPath, sha256hex string) error` — durum `EXTRACTING`.
  - `func (tx *Tx) GetDraft(ctx, id uuid.UUID) (Draft, error)` — yoksa `ErrNotFound`.
  - `func (tx *Tx) LockDraft(ctx, id uuid.UUID) (Draft, error)` — `FOR UPDATE`; yoksa `ErrNotFound`.
  - `func (tx *Tx) SetDraftResult(ctx, id uuid.UUID, draft, warnings, meta json.RawMessage) error` — `READY`, `error_code=''`.
  - `func (tx *Tx) SetDraftFailed(ctx, id uuid.UUID, code string) error` — `FAILED`.
  - `func (tx *Tx) SetDraftExtracting(ctx, id uuid.UUID) error`
  - `func (tx *Tx) ApproveDraft(ctx, id, contractID uuid.UUID) error` — `APPROVED`, `contract_id`.
  - `var ErrDraftNotReady = errors.New("draft not ready")`, `var ErrDraftApproved = errors.New("draft already approved")`

Migration (kolonlar tasarım §6.2): `id UUID PRIMARY KEY`, `tenant_id UUID NOT NULL REFERENCES tenants(id)`, `status TEXT NOT NULL CHECK (status IN (...))`, `pdf_path TEXT NOT NULL`, `pdf_sha256 TEXT NOT NULL`, `draft_json JSONB`, `warnings_json JSONB NOT NULL DEFAULT '[]'`, `meta_json JSONB NOT NULL DEFAULT '{}'`, `error_code TEXT NOT NULL DEFAULT ''`, `contract_id UUID`, `created_at`/`updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`, `UNIQUE (tenant_id, id)`, `FOREIGN KEY (tenant_id, contract_id) REFERENCES contracts(tenant_id, id)`, `CHECK ((status = 'APPROVED') = (contract_id IS NOT NULL))`. RLS ve politika `001`'deki `contracts` bloğunun aynısı; `GRANT SELECT, INSERT, UPDATE ON contract_drafts TO matchwatch_app`. `updated_at` her `Set*`/`Approve` güncellemesinde `now()`.

- [ ] **Step 1: Write the failing integration tests**

`TestDraftLifecycle`: Insert → Get (`EXTRACTING`, `Draft == nil`) → SetDraftResult(`{"contract_no":"X"}`, `[]`, `{"pages":2}`) → Get (`READY`, JSON eşit) → sözleşme ekle → ApproveDraft → Get (`APPROVED`, `ContractID` eşit). `TestDraftRLS`: tenant A'nın taslağını tenant B `GetDraft` → `ErrNotFound`. `TestDraftApproveRequiresContract`: `contract_id` olmadan `status='APPROVED'` güncellemesi CHECK hatası verir (doğrudan SQL ile). `TestDraftCrossTenantContractFK`: tenant B'nin sözleşme id'siyle tenant A taslağını onaylamak FK hatası verir.

- [ ] **Step 2: Run tests to verify they fail**

Run (`backend-go/`, URL'ler `.env`'den): `MATCHWATCH_TEST_DATABASE_URL=... MATCHWATCH_TEST_MIGRATE_URL=... go test ./internal/repository/ -run TestDraft -count=1`
Expected: FAIL (derleme: `InsertDraft` yok).

- [ ] **Step 3: Write migration and `drafts.go`**

- [ ] **Step 4: Run tests to verify they pass**

Run: aynı komut, sonra `go test ./... -count=1` (env'li) ve `go vet ./... && gofmt -l .`
Expected: tümü PASS; `gofmt` çıktısı boş.

- [ ] **Step 5: Commit**

```bash
git add backend-go
git commit -m "feat(repo): contract_drafts table with RLS and draft lifecycle"
```

---

### Task 6: Go `aiclient` + ayarlar

**Files:**
- Create: `backend-go/internal/aiclient/client.go`
- Modify: `backend-go/internal/config/config.go`, `backend-go/internal/config/config_test.go`, `.env.example`
- Test: `backend-go/internal/aiclient/client_test.go`

**Interfaces:**
- Produces:
  - `config.Config` yeni alanlar: `AIServiceURL string` (`AI_SERVICE_URL`, varsayılan boş = kapalı), `AIServiceToken string` (`AI_SERVICE_TOKEN`), `AIExtractTimeout time.Duration` (`AI_EXTRACT_TIMEOUT`, `time.ParseDuration`, varsayılan `5m`). URL doluysa token < 32 bayt → `Load` hata döner; süre ayrıştırılamazsa hata.
  - `aiclient.Result struct { Draft, Warnings, Meta json.RawMessage }`
  - `aiclient.Error struct { Code string; Status int }` + `Error() string`; ağ hatası/zaman aşımı → `Code: "AI_UNAVAILABLE"`, servis hata gövdesi → onun `code`'u.
  - `aiclient.New(baseURL, token string, timeout time.Duration) *Client`; `(*Client).Extract(ctx context.Context, pdf []byte) (Result, error)` — `POST {baseURL}/v1/extract`, `Content-Type: application/pdf`, `Authorization: Bearer`, yanıt ≤ 8 MiB okunur.
- `.env.example`'a eklenir: `AI_SERVICE_URL=` (boş), `AI_SERVICE_TOKEN=dev-only-ai-token-change-me-0123456789`, `AI_EXTRACT_TIMEOUT=5m`, `LLM_BASE_URL=http://ollama:11434`, `LLM_MODEL=qwen2.5:7b-instruct`.

- [ ] **Step 1: Write the failing tests**

`client_test.go` (`httptest.Server`): başarı → `Result.Draft` içinde `contract_no`; istek başlıkları doğru; `502 {"code":"LLM_UNAVAILABLE"}` → `*Error{Code:"LLM_UNAVAILABLE", Status:502}`; sunucu kapalı → `AI_UNAVAILABLE`; sunucu `timeout`'tan uzun bekler → `AI_UNAVAILABLE`. `config_test.go`: URL boşken token gerekmez; URL doluyken kısa token hata; `AI_EXTRACT_TIMEOUT=abc` hata; varsayılan `5*time.Minute`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/aiclient/ ./internal/config/ -count=1`
Expected: FAIL (derleme).

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/aiclient/ ./internal/config/ -count=1 && go vet ./... && gofmt -l .`
Expected: PASS, `gofmt` çıktısı boş.

- [ ] **Step 5: Commit**

```bash
git add backend-go .env.example
git commit -m "feat(aiclient): HTTP client and config for the extraction service"
```

---

### Task 7: Taslak REST uçları + onayda `draft_id`

**Files:**
- Create: `backend-go/internal/transport/http/drafts.go`
- Modify: `backend-go/internal/transport/http/router.go` (`NewRouter`'a `ai Extractor` ve `extractTimeout time.Duration` parametreleri; `nil` = kapalı), `backend-go/internal/transport/http/handlers.go` (`postContract`: `draft_id`), `backend-go/internal/repository/contract_json.go` (`contractJSON.DraftID *uuid.UUID` `json:"draft_id"`, `ContractDoc.DraftID *uuid.UUID`), `backend-go/cmd/api/main.go` (`AIServiceURL` doluysa `aiclient.New`; boşsa `var ai transporthttp.Extractor` sıfır değeri geçirilir — `(*aiclient.Client)(nil)` geçirmek typed-nil yüzünden `ai == nil` kontrolünü bozar), `backend-go/internal/transport/http/dto.go`, mevcut testlerdeki `NewRouter` çağrıları (`nil, 0` eklenir)
- Test: `backend-go/internal/transport/http/drafts_integration_test.go`

**Interfaces:**
- Consumes: Task 5 repository fonksiyonları, Task 6 `aiclient.Result`/`*aiclient.Error`.
- Produces:
  - `transporthttp.Extractor` interface: `Extract(ctx context.Context, pdf []byte) (aiclient.Result, error)`.
  - `draftResp struct { ID uuid.UUID "id"; Status string "status"; Draft json.RawMessage "draft"; Warnings json.RawMessage "warnings"; Meta json.RawMessage "meta"; ErrorCode string "error_code,omitempty"; ContractID *uuid.UUID "contract_id,omitempty" }`
  - Uçlar:
    - `POST /api/v1/contract-drafts` — `multipart/form-data`, alan `file`; `http.MaxBytesReader` 20 MiB + 1 MiB pay; içerik `%PDF-` ile başlamıyorsa `400 NOT_PDF`; aşım `413`. Akış: `id := uuid.New()` → PDF'i `filepath.Join(storageDir, tid, "contract", id+".pdf")`'e yaz → `InTenant{InsertDraft}` (hata olursa dosyayı sil) → `extractInto(ctx, tid, id, pdf)` → `201 draftResp`.
    - `extractInto`: `context.WithTimeout(ctx, extractTimeout)`; `ai.Extract`; başarıda `InTenant{SetDraftResult}`, `*aiclient.Error`'da `InTenant{SetDraftFailed(code)}`, diğer hatada `SetDraftFailed("AI_UNAVAILABLE")`. Sonucu yazmak için istek bağlamı değil `context.WithoutCancel(ctx)` kullanılır (istemci koptuysa da sonuç yazılır).
    - Handler başında `http.NewResponseController(w).SetWriteDeadline(time.Now().Add(extractTimeout + 30*time.Second))` (hata yok sayılır; httptest yazıcısı desteklemez).
    - `GET /api/v1/contract-drafts/{id}` → `200 draftResp` / `404`.
    - `POST /api/v1/contract-drafts/{id}/extract` → `InTenant{LockDraft; APPROVED ise ErrDraftApproved; SetDraftExtracting}` → PDF'i diskten `PDFPath` ile oku → `extractInto` → `200 draftResp`.
    - `ai == nil` iken üç uç da `503 AI_DISABLED`.
    - `POST /api/v1/contracts` gövdesinde `draft_id` varsa aynı transaction'da: `LockDraft` (yoksa `404`), `APPROVED` → `409 DRAFT_APPROVED`, `READY` değil → `409 DRAFT_NOT_READY`; `doc.FilePath = draft.PDFPath`; `InsertContract`; `ApproveDraft`.
  - `fail()`'e eklenir: `ErrDraftNotReady` → `409 DRAFT_NOT_READY`, `ErrDraftApproved` → `409 DRAFT_APPROVED`.

- [ ] **Step 1: Write the failing integration tests**

Sahte `Extractor` (testte tanımlı struct): sabit `Result`, veya `*aiclient.Error`, veya `delay`.

```go
func TestDraftHappyPath(t *testing.T)        // POST pdf → 201 READY, draft.contract_no; GET aynı; POST /contracts {...,"draft_id"} → 201; GET → APPROVED, contract_id; diskte <storage>/<tid>/contract/<id>.pdf; contracts.file_path == pdf yolu
func TestDraftNotPDF(t *testing.T)           // "hello" → 400 NOT_PDF, contract_drafts satırı yok
func TestDraftRecoverAfterFailure(t *testing.T) // Extractor önce *aiclient.Error{Code:"LLM_UNAVAILABLE"} → 201 FAILED error_code; POST /contracts draft_id → 409 DRAFT_NOT_READY; Extractor başarıya çevrilir; POST …/extract → 200 READY
func TestDraftApproveTwice(t *testing.T)     // ikinci onay → 409 DRAFT_APPROVED; …/extract → 409 DRAFT_APPROVED
func TestDraftCrossTenant(t *testing.T)      // tenant B GET → 404; tenant B /contracts draft_id=A'nınki → 404, B'de sözleşme yok
func TestDraftDisabled(t *testing.T)         // NewRouter(..., nil, 0) → 503 AI_DISABLED
func TestDraftSlowExtraction(t *testing.T)   // gerçek http.Server{Handler: router, WriteTimeout: time.Second} + httptest listener; Extractor 2 s bekler; extractTimeout 10 s → 201 READY
func TestContractWithoutDraftUnchanged(t *testing.T) // draft_id'siz mevcut akış aynen 201 (regresyon)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/transport/http/ -run 'TestDraft|TestContractWithoutDraft' -count=1` (env'li)
Expected: FAIL (derleme).

- [ ] **Step 3: Implement** (kurallar Interfaces'ta)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -count=1` (env'li) `&& go vet ./... && gofmt -l .`
Expected: tümü PASS, `gofmt` boş. `scripts/smoke.sh` beklenen `"caught_minor":21412` değişmez (Task 8'de çalıştırılır).

- [ ] **Step 5: Commit**

```bash
git add backend-go
git commit -m "feat(api): contract draft upload, re-extract and draft-linked approval"
```

---

### Task 8: Compose profili, uçtan uca smoke, belgeler

**Files:**
- Modify: `docker-compose.yml`, `CLAUDE.md` (Commands'a ai-service komutları; Architecture'a `ai-service` ve `aiclient`; Current status), `README.md` (varsa çalıştırma bölümüne bir paragraf)
- Create: `scripts/smoke-ai.sh`

**Interfaces:**
- Consumes: Task 4 imajı, Task 7 uçları, Task 2 fikstürü.

Compose ekleri (`profiles: ["ai"]`, port dışarı açılmaz):
- `ollama`: `image: ollama/ollama:<sabit sürüm>`, volume `ollama:/root/.ollama`.
- `ai`: `build: ./ai-service`, env `AI_SERVICE_TOKEN`, `LLM_BASE_URL=http://ollama:11434`, `LLM_MODEL`; `depends_on: [ollama]`; Go API compose'da değil, yerelde çalışır → geliştirmede erişim için `ai` servisi yalnız `127.0.0.1:8090:8090` bağlar.

`smoke-ai.sh` (LF; `set -euo pipefail`): `docker compose --profile ai up -d --build` → `docker compose exec ollama ollama pull $LLM_MODEL` → ai `healthz` bekle → Go API'yi `AI_SERVICE_URL=http://127.0.0.1:8090` ile başlat (mevcut `smoke.sh`'in API başlatma desenini kullan) → `devtoken` (sözleşme satıcıya bağlanır, tenant VKN'si kontrol edilmez; mevcut tenant kurulumu yeterli) → `curl -F file=@ai-service/tests/fixtures/contract_text.pdf .../contract-drafts` → `status == "READY"` ve `draft.contract_no == "SZL-2026-014"` → taslağı `jq` ile alıp eksik alanları (`qty_tolerance_pct` null ise `"0"`) doldurup `draft_id` ile `POST /contracts` → `201` → taslak `APPROVED`. Son satırda `smoke-ai OK`.

- [ ] **Step 1: Write compose profile and `smoke-ai.sh`**

- [ ] **Step 2: Verify Faz 1 flow unchanged**

Run: `docker compose config --services` → `postgres` (profil kapalıyken yalnız o); `docker compose --profile ai config --services` → `postgres`, `ollama`, `ai`; `bash scripts/smoke.sh`
Expected: `smoke.sh` çıktısında `"caught_minor":21412`.

- [ ] **Step 3: Run AI smoke**

Run: `bash scripts/smoke-ai.sh`
Expected: `smoke-ai OK`. (Model indirmesi ~5 GB; makinede Docker/disk yoksa bu adım atlanır ve rapora "çalıştırılmadı" yazılır — sahte başarı yazılmaz.)

- [ ] **Step 4: Update `CLAUDE.md`**

Commands: `cd ai-service && python -m venv .venv && .venv/Scripts/python -m pip install -r requirements-dev.txt && .venv/Scripts/python -m pytest -q`; `docker compose --profile ai up -d`. Architecture: `ai-service/` ve `internal/aiclient` birer madde; taslak → onay akışı bir cümle. Current status: Faz 2a tamam, sıradaki Faz 2b (embedding/semantik) ve 2c (TCMB).

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml scripts/smoke-ai.sh CLAUDE.md README.md
git commit -m "chore: ai compose profile, AI smoke test and docs for Faz 2a"
```
