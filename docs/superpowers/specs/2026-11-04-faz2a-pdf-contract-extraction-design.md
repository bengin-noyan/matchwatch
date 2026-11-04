# Faz 2a — PDF Sözleşme Çıkarımı: Tasarım

Tarih: 2026-11-04. Durum: mimar kararıyla onaylandı (kullanıcı soruları atlayıp kararı Claude'a bıraktı).
Üst belge: `docs/spec.md` (D1–D15 geçerli). Bu belge D12'yi ("Faz 2'deki PDF çıkarımı aynı şemayı doldurur") uygular.

## 1. Amaç ve neden

Faz 1'de sözleşmeler elle yazılmış JSON ile yükleniyor. Gerçek müşteride sözleşme PDF'tir ve kimse JSON yazmaz.
Faz 2a, bir PDF sözleşmeden Faz 1'in `ContractDoc` JSON şemasını **taslak** olarak doldurur; bir insan taslağı
inceleyip onaylayınca sözleşme mevcut yoldan (`ParseContractJSON` → `InsertContract`) aktif olur.

Neden taslak + insan onayı: sözleşme fiyatı CRITICAL `PRICE_MISMATCH` bulgularının referansıdır. Yanlış okunmuş bir
fiyat ya sahte kırmızı üretir ya gerçek kaçağı gizler. Spec kuralı 2 ("onay/red kararları LLM'e bırakılmaz") gereği
LLM çıktısı doğrudan aktif sözleşme olamaz.

## 2. Hedefler

- G1. Metin katmanlı ve taranmış (görüntü) PDF'lerden `ContractDoc` taslağı üretmek.
- G2. Her çıkarılan değerin kanıtı: kalemler için `source_page` + `source_clause_text`, vade için
  `payment_term_source_page` + `payment_term_clause_text` (spec kuralı 3).
- G3. Her değer kaynak metne **dayandırılmış** olmalı: LLM'in döndürdüğü alıntı, belirttiği sayfanın metninde
  geçmiyorsa değer atılır ve uyarı yazılır (halüsinasyon koruması).
- G4. Sayılar deterministik Türkçe sayı ayrıştırıcıyla `Decimal`'e çevrilir; LLM sayı hesaplamaz, yalnızca metinde
  yazan ifadeyi alıntılar.
- G5. KVKK: belge TR altyapımızdan çıkmaz. LLM kendi sunucumuzda çalışan açık ağırlıklı bir modeldir.

## 3. Hedef dışı (Faz 2a'da yok)

- Embedding, semantik kalem eşleştirme, hizmet faturası semantik mükerrerlik → **Faz 2b**.
- TCMB kur beslemesi → **Faz 2c**.
- Yabancı LLM API'si (OpenAI/Anthropic vb.) ve müşteri bazlı opt-in. KVKK m.9 dayanağı ve tenant ayarı olmadan
  yapılmaz; kod yolu da yazılmaz.
- İskonto/kademeli fiyat/endeksli fiyat hesaplama. `ContractDoc`'ta iskonto alanı yok; sözleşmede iskonto maddesi
  görülürse yalnızca uyarı yazılır, fiyata uygulanmaz.
- Tevkifat kodu tahmini. Kod yalnızca sözleşmede açıkça yazıyorsa (`601`–`825`) doldurulur; "yemek hizmeti → 604"
  gibi eşleme bir vergi kararıdır, insana bırakılır.
- Web arayüzü (Faz 3). Faz 2a yalnızca API.
- Asenkron iş kuyruğu (aşağıda §6.4).

## 4. Anti-hedefler (çalışsa bile başarısızlık sayılır)

- Kaynak metinde olmayan bir fiyatın taslağa girmesi.
- Float ile fiyat taşınması (Python'da `float`, JSON'da sayı). Fiyatlar her yerde string/`Decimal`.
- Python servisinin veritabanına veya tenant verisine kalıcı erişimi. Servis durumsuzdur.
- Belgenin veya metnin loglara yazılması (KVKK). Loglarda yalnızca boyut, sayfa sayısı, süre, hata kodu.

## 5. Mimari

```
Client ──multipart PDF──▶ Go API ──PDF bytes──▶ Python ai-service ──prompt──▶ yerel LLM (Ollama, OpenAI-uyumlu)
   ▲                        │  (RLS, DB, depolama)      │  (durumsuz)
   │                        ▼                           ▼
   └── taslak JSON ◀── contract_drafts ◀── taslak + uyarılar
   └── onaylı JSON ──▶ POST /api/v1/contracts (mevcut) ──▶ contracts (+ draft APPROVED)
```

- **Go API sahibidir:** kimlik doğrulama, tenant, RLS, PDF depolama, taslak kaydı, onay. Python yalnızca
  `PDF bytes → taslak` fonksiyonudur.
- **Python `ai-service/`** (repo kökünde, `backend-go/` kardeşi): Python 3.12, FastAPI, tek uç `POST /v1/extract`.
- **LLM**: Ollama üzerinden OpenAI-uyumlu `/v1/chat/completions`, JSON şema kısıtlı çıktı. Varsayılan model
  `qwen2.5:7b-instruct` (Apache-2.0, Türkçe yeterli, tek GPU/CPU'da çalışır). Model adı ve URL ayardır.

## 6. Ayrıntılar

### 6.1 Python servisi (`ai-service/`)

Boru hattı, sayfa sayfa:

1. **Metin çıkarma:** `pypdfium2` ile her sayfanın metin katmanı. (PyMuPDF AGPL olduğu için seçilmedi; SaaS'ta lisans
   riski.)
2. **OCR geri dönüşü:** sayfa metni boşsa/çok kısaysa (< 20 anlamlı karakter) sayfa `pypdfium2` ile 300 DPI
   render edilir, `pytesseract` + Tesseract `tur` dil paketiyle okunur. Taslağa `ocr_pages` listesi yazılır.
3. **LLM çağrısı:** sayfa numaralı metin (`=== SAYFA n ===`) + JSON şema. LLM'den **alıntı** istenir, değer değil:
   her alan için `{ "text": "<sözleşmede aynen geçen ifade>", "page": n }`. Örn. birim fiyat için `"12,50 TL"`.
   Uzun belgeler sayfa gruplarına bölünür (bağlam sınırı); kalemler birleştirilir.
4. **Dayandırma (grounding):** her alıntı, boşluk/büyük-küçük harf normalize edilerek belirtilen sayfanın metninde
   aranır. Bulunmazsa alan `null`, uyarı `UNGROUNDED:<alan>`.
5. **Deterministik ayrıştırma** (LLM'siz, birim testli):
   - Türkçe sayı: `1.234,56` / `1234,56` / `12,5` → `Decimal`; belirsiz `1.234` (binlik mi ondalık mı) →
     binlik kabul edilir ve `AMBIGUOUS_NUMBER` uyarısı. `float` yasak.
   - Para birimi: `TL`, `₺`, `TRY`, `Türk Lirası` → `TRY`; `USD`, `$`, `Dolar` → `USD`; `EUR`, `€`, `Avro`/`Euro` → `EUR`.
     Diğerleri ISO kodu yazıyorsa o; yoksa `null` + uyarı.
   - Tarih: `gg.aa.yyyy`, `gg/aa/yyyy`, `g Ay yyyy` (Türkçe ay adları) → `YYYY-MM-DD`.
   - Vade: "60 gün", "altmış (60) gün" → rakamla yazılan sayı alınır; yalnızca yazıyla yazılmışsa `null` + uyarı.
   - Yüzde: `%5`, `% 5`, `yüzde 5` → `5`.
   - VKN/TCKN: 10/11 hane; VKN için GİB kontrol hanesi algoritması doğrulanır, geçmezse uyarı (değer korunur,
     insan karar verir).
   - Birim: ham metin bırakılır (`adet`, `kg`); Go'daki `domain.NormalizeUOM` zaten normalize eder. Python'da
     yalnızca bilinen Türkçe adlar UBL koduna çevrilir (`adet→C62`, `kg/kilogram→KGM`, `lt/litre→LTR`, `m→MTR`,
     `ton→TNE`, `saat→HUR`, `ay→MON`); bilinmeyen birim ham kalır + uyarı.
6. **İskonto/özel madde taraması:** metinde `iskonto`, `indirim`, `fiyat farkı`, `endeks` geçerse
   `PRICE_CLAUSE_REVIEW:<sayfa>` uyarısı (deterministik kelime taraması).

Çıktı (`200`):

```json
{
  "draft": { /* ContractDoc şeması, eksik alanlar null; tüm decimal'ler string */ },
  "warnings": [{ "code": "UNGROUNDED", "field": "items[2].unit_price", "page": 3, "message": "..." }],
  "meta": { "pages": 7, "ocr_pages": [5, 6], "model": "qwen2.5:7b-instruct", "duration_ms": 41230 }
}
```

Hatalar: `400 NOT_PDF`, `413 TOO_LARGE` (> 20 MiB veya > 200 sayfa), `422 ENCRYPTED_PDF`, `502 LLM_UNAVAILABLE`,
`502 LLM_BAD_OUTPUT` (şemaya uymayan çıktı, 1 tekrar denemeden sonra).

Güvenlik/KVKK:
- Servis yalnızca iç ağda dinler; Go her istekte `Authorization: Bearer <AI_SERVICE_TOKEN>` gönderir (sabit zamanlı
  karşılaştırma). Token yoksa servis başlamaz.
- `LLM_BASE_URL` başlangıçta çözümlenir; loopback/özel ağ (RFC1918, `localhost`, compose servis adı) değilse servis
  **başlamaz**. Yabancı API'ye yanlışlıkla gönderimi yapısal olarak engeller.
- PDF ve metin diske yazılmaz (bellek içi), loglanmaz.

### 6.2 Go tarafı

- **Migration `005_contract_drafts.sql`:**
  `contract_drafts(id, tenant_id, status, pdf_path, pdf_sha256, draft_json JSONB, warnings_json JSONB, meta_json JSONB,
  error_code, contract_id, created_at, updated_at)`; `status IN ('EXTRACTING','READY','FAILED','APPROVED')`;
  `ENABLE`+`FORCE` RLS, `NULLIF` politikası, `UNIQUE(tenant_id,id)`, `contract_id` için composite FK
  `(tenant_id, contract_id) REFERENCES contracts(tenant_id, id)`. `matchwatch_app`'e SELECT/INSERT/UPDATE.
- **`POST /api/v1/contract-drafts`** (multipart, alan `file`, ≤ 20 MiB, `%PDF-` imzası kontrolü):
  1. PDF'i `STORAGE_DIR/<tenant>/contract/<draft_id>.pdf`'e yaz (mevcut `rawPath` deseni, tür `contract`).
  2. Taslak satırı `EXTRACTING` olarak kendi transaction'ında commit et.
  3. Python'u çağır (zaman aşımı `AI_EXTRACT_TIMEOUT`, varsayılan 5 dk).
  4. Sonucu `READY` (+ draft/uyarılar/meta) ya da `FAILED` (+ `error_code`) olarak yaz.
  5. `201` + taslak gövdesi döndür (FAILED olsa da `201`, `status` alanı söyler; istemci GET ile tekrar sorabilir).
- **`GET /api/v1/contract-drafts/{id}`**: taslağı döndürür.
- **`POST /api/v1/contract-drafts/{id}/extract`**: `FAILED`/`READY`/takılı `EXTRACTING` taslağı yeniden çıkarır
  (Faz 1'deki `POST /invoices/{id}/audit` kurtarma deseni). `APPROVED` → `409`.
- **Onay = mevcut `POST /api/v1/contracts`**, gövdeye isteğe bağlı `"draft_id"` eklenir. Gövde (insanın düzelttiği
  JSON) olduğu gibi `ParseContractJSON`'dan geçer; aynı transaction'da taslak `READY` ise `APPROVED` +
  `contract_id` yazılır, değilse `409 DRAFT_NOT_READY`. Yeni doğrulama yolu yazılmaz.
- **Ayar:** `AI_SERVICE_URL`, `AI_SERVICE_TOKEN`, `AI_EXTRACT_TIMEOUT`. `AI_SERVICE_URL` boşsa
  `/contract-drafts` uçları `503 AI_DISABLED` döner; Faz 1 davranışı değişmez.

### 6.3 Docker Compose

- `ai` servisi (`ai-service/Dockerfile`, `python:3.12-slim` + `tesseract-ocr` + `tesseract-ocr-tur`), port dışarı
  açılmaz. Geliştirmede Go API host üzerinde çalıştığı için `ai` yalnızca `127.0.0.1:8090`'da (loopback) yayımlanır
  (kontrolcü kararı); üretimde hiçbir port açılmaz.
- `ollama` servisi (`ollama/ollama`), model volume'da; ikisi de `profiles: ["ai"]` altında — Faz 1 geliştirme akışı
  (`docker compose up -d`) etkilenmez, model indirmesi istenmeden yapılmaz.

### 6.4 Senkron çıkarım kararı

Çıkarım istek içinde senkron yapılır; taslak satırı çağrıdan **önce** commit edildiği için istemci zaman aşımına
uğrasa bile sonuç kaybolmaz, `GET` ile alınır. Kuyruk/worker eklenmez. Yükseltme koşulu: Faz 3 arayüzünde eşzamanlı
yükleme hacmi Go HTTP zaman aşımlarını zorlarsa iş kuyruğuna geçilir (kodda `shortcut:` yorumu).

## 7. Test stratejisi

- **Python birim testleri (pytest):** sayı/tarih/para/vade/yüzde/VKN/birim ayrıştırıcıları; dayandırma; tablo
  testleri. `float` kullanılmaz (testlerde de; bir test kaynak ağacında `float(` arar ve başarısız olur).
- **Python boru hattı testleri:** sahte LLM istemcisi (sabit JSON döner) + fikstür PDF'leri. `pypdfium2` PDF
  yazamadığı için fikstürler `tests/fixtures/make_fixtures.py` ile (yalnızca geliştirme bağımlılığı `reportlab`,
  BSD; taranmış sürüm için sayfa `pypdfium2` ile render edilip Pillow ile görüntü-PDF'e çevrilir) bir kez üretilir ve
  küçük ikili dosyalar olarak commit edilir: metin katmanlı 2 sayfalık sözleşme + aynı sözleşmenin taranmış sürümü. Halüsinasyon senaryosu: sahte LLM metinde olmayan fiyat döndürür →
  alan `null` + `UNGROUNDED`.
- **OCR testi** Tesseract kurulu değilse `skip` (Windows geliştirme makinesi); Docker içinde çalışır.
- **Gerçek LLM testi** yalnızca `MATCHWATCH_TEST_LLM_URL` varsa (Go'daki entegrasyon testi deseni).
- **Go:** `httptest` ile sahte ai-service; taslak yaşam döngüsü (EXTRACTING→READY→APPROVED, FAILED→extract→READY,
  APPROVED→409), RLS (başka tenant taslağı görünmez/onaylanamaz), `AI_SERVICE_URL` boş → 503.
- **Uçtan uca:** `scripts/smoke.sh`'e dokunulmaz; ayrı `scripts/smoke-ai.sh` (profil `ai` açıkken) fikstür PDF'i
  yükler, taslağı onaylar, sözleşmenin oluştuğunu doğrular.

## 8. Mimar kararları (Claude'un verdiği, kullanıcının değil)

| # | Karar | Gerekçe |
|---|---|---|
| R1 | Faz 2 → 2a (PDF), 2b (embedding/semantik), 2c (TCMB) | Bağımsız; 2a en çok değer, diğerleri ona bağlı değil |
| R2 | Taslak + insan onayı zorunlu | Spec kuralı 2; fiyat CRITICAL bulguların referansı |
| R3 | LLM alıntı döndürür, sayıları Python ayrıştırır | LLM hesap yapmaz; kanıt metni bedavaya gelir |
| R4 | Dayandırılamayan değer atılır | Halüsinasyon fiyatı en kötü hata |
| R5 | Ollama + `qwen2.5:7b-instruct`, OpenAI-uyumlu API | KVKK, açık lisans, model değişimi tek ayar |
| R6 | `pypdfium2` + Tesseract, PyMuPDF değil | AGPL riski yok |
| R7 | Python durumsuz, DB'ye dokunmaz | RLS tek yerde (Go) kalır |
| R8 | Onay mevcut `POST /contracts` + `draft_id` | Doğrulama tek yerde |
| R9 | Senkron çıkarım, taslak önce commit | Kuyruk yok; sonuç kaybolmaz |
| R10 | `LLM_BASE_URL` özel ağ değilse servis başlamaz | Yurt dışı aktarımı yapısal olarak engelli |
| R11 | Tevkifat kodu tahmin edilmez | Vergi kararı insana |
| R12 | `ai`/`ollama` compose profili | Faz 1 akışı ve test süresi değişmez |

## 9. Builder'a bırakılanlar

- Prompt metni ve sayfa grubu boyutu (bağlam sınırına göre).
- FastAPI proje düzeni, bağımlılık sabitleme aracı (`requirements.txt` yeterli).
- Uyarı mesajlarının Türkçe metinleri.
- Fikstür sözleşmenin içeriği (gerçekçi ama uydurma şirketler; gerçek VKN kullanılmaz).
