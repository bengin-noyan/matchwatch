# matchwatch — Ürün ve Mimari Spesifikasyonu

Kaynak: proje sahibinin "LeakZero" spesifikasyonu (2026-11-02), ürün adı **matchwatch** olarak değiştirildi.
Aşağıdaki "Kararlar" bölümü orijinal spec'ten bilinçli sapmaları içerir ve onlara önceliklidir.

## 1. Problem

Türk işletmeleri GİB UBL-TR e-Fatura ve e-İrsaliye kullanır. Entegratörler yalnızca şema/iletimi denetler, ticari
içeriği (sözleşmeye uygunluk, teslimat doğruluğu) denetlemez. Satın alma bütçesinin %2–5'i hatalı faturaya, gizli
zamlara, teslim alınmayan mala ve mükerrer faturaya kaybolur. matchwatch ERP/entegratör üzerine oturan otonom iç
denetim katmanıdır:

1. **Sözleşme Zekası:** PDF sözleşmeden VKN, birim fiyat, iskonto, vade günü, döviz kuralı, miktar toleransı çıkarımı.
2. **3-Way Matching:** Sözleşme ↔ e-İrsaliye (`DespatchAdvice`) ↔ e-Fatura (`Invoice`), UBL-TR 1.2.
3. **Kalem Eşleştirme:** Önce deterministik (SKU/barkod), sonra semantik (pgvector, kosinüs ≥ 0.86, aynı uom).
4. **Anomali Motoru:** Aşağıdaki kurallar.

## 2. Teknoloji

- Çekirdek: Go 1.23+ (API, UBL parser, kural motoru), `shopspring/decimal`, `pgx/v5`.
- AI servisi: Python 3.12+ FastAPI (PDF/OCR, LLM yapılandırılmış çıkarım, embedding). **Faz 2.**
- DB: PostgreSQL 16 + pgvector. Tüm tenant tablolarında `tenant_id UUID` + Row-Level Security.
- Frontend: Vue 3 + TS + Tailwind + Pinia + TanStack Table. **Faz 3.** Profesyonel, şık, dinamik web uygulaması.
- Altyapı: Docker Compose.

### İhlal edilemez kurallar

1. Para/miktar/oran için asla `float32/float64`. Go'da `decimal.Decimal`, DB'de `BIGINT` (kuruş) veya `NUMERIC`.
2. Hesaplama ve onay/red kararları LLM'e bırakılmaz. LLM yalnızca çıkarım ve embedding üretir.
3. Her bulgu kanıtıyla saklanır (fatura XML satır referansı + sözleşme sayfa no + madde metni).

## 3. Anomali Kuralları

| Kod | Koşul | Seviye | Finansal etki |
|---|---|---|---|
| PRICE_MISMATCH | fatura net birim fiyatı > sözleşme birim fiyatı | CRITICAL | (fatura − sözleşme) × fatura miktarı |
| QTY_EXCEEDS_DELIVERY | faturalanan > net_teslim × (1 + tolerans%/100); net_teslim = teslim − red | CRITICAL | (faturalanan − net_teslim) × net birim fiyat |
| MATURITY_VIOLATION | (due_date − issue_date) < sözleşme payment_term_days | WARNING | erken ödeme finansman maliyeti |
| DUPLICATE_SUSPECT | son 90 gün, aynı tedarikçi, aynı tutar + benzer kalem açıklaması (veya aynı fatura no) | CRITICAL | fatura ödenecek tutarı |
| WITHHOLDING_ERROR | beklenen tevkifat oranı ≠ faturadaki | CRITICAL | (beklenen − faturadaki tevkifat tutarı) |
| PRICE_CREEP | sözleşmesiz kalemde birim fiyat > son 6 ay EMA × 1.10 | WARNING | (fiyat − EMA) × miktar |
| UNCONTRACTED_ITEM | tedarikçinin aktif sözleşmesi var ama kalem yok | WARNING | 0 |

Fatura durumu: bulgu yok → GREEN; yalnızca WARNING (INFO yok sayılır) → YELLOW; en az bir CRITICAL → RED.

## 4. Kararlar (orijinal spec'ten sapmalar — bunlar geçerlidir)

- **D1 Birim fiyat `NUMERIC(19,6)`**, `BIGINT` kuruş değil. Alt-kuruş birim fiyatlar (0,0375 TL) yuvarlamayla
  kaybolur. Satır/fatura toplamları ve `financial_impact_minor` kuruş `BIGINT` kalır. Kuruşa çevirme: half-away-from-zero.
- **D2 Fiyat karşılaştırması net birim fiyatla** yapılır: `line_extension / invoiced_qty` (iskonto düşülmüş).
- **D3 İrsaliye–fatura n:m.** `matched_despatch_line_id` yerine `despatch_allocations(invoice_line_id,
  despatch_line_id, qty)` tablosu. Bir irsaliye satırının miktarı birden çok faturaya bölünebilir, aynı miktar iki kez
  faturalanamaz.
- **D4 Döviz:** fatura ve sözleşme para birimi aynıysa doğrudan karşılaştır. Farklıysa ve faturada sözleşme para
  birimine giden `PricingExchangeRate` varsa onunla çevir ve kuru kanıta yaz. Hiçbiri yoksa fiyat kuralı atlanır ve
  `FX_UNVERIFIED` (INFO) bulgusu yazılır. TCMB kur beslemesi Faz 2.
- **D5 Uom uyuşmazlığı:** SKU eşleşti ama uom farklıysa fiyat/miktar karşılaştırılmaz, `UOM_MISMATCH` (WARNING).
- **D6 Vade finansman maliyeti:** `tenants.financing_rate_annual NUMERIC(7,4)` (ör. 0.4500 = %45/yıl).
  etki = payable × oran × erken_gün / 365.
- **D7 Tevkifat:** global `withholding_codes(code, name, percent)` tablosu GİB UBL-TR Kod Listeleri'nden seed edilir.
  Beklenen kod sözleşme kaleminde (`contract_items.withholding_code`). Faturadaki kodun oranı tabloyla da tutarlı olmalı.
- **D8 Embedding kolonları Faz 2'de eklenir** (boyut seçilen modele bağlı). Faz 1'de pgvector eklentisi kurulur,
  semantik eşleştirme ve hizmet faturası semantik mükerrerlik Faz 2. Faz 1 açıklama benzerliği: normalize token
  Jaccard ≥ 0.80.
- **D9 RLS:** migration'lar `matchwatch_owner` ile, uygulama `matchwatch_app` (owner değil, BYPASSRLS yok) ile çalışır.
  Tüm tenant tablolarında `ENABLE` + `FORCE ROW LEVEL SECURITY`. Tenant her transaction'da
  `set_config('app.current_tenant_id', $1, true)` ile ayarlanır (transaction-local).
- **D10 Aynı ETTN ikinci kez yüklenirse** 409 döner (unique `(tenant_id, ettn)`); bu mükerrerlik bulgusu değil reddir.
- **D11 Yeniden denetim idempotenttir:** OPEN bulgular ve faturanın allocation'ları silinip yeniden hesaplanır;
  DISPUTED / APPROVED_EXCEPTION bulgular korunur ve aynı (rule_code, invoice_line_id) tekrar üretilmez.
- **D12 Faz 1'de sözleşmeler JSON** ile yüklenir; Faz 2'deki PDF çıkarımı aynı şemayı doldurur.
- **D13 Kullanıcı/login Faz 3.** Faz 1'de API, HS256 JWT (`tenant_id`, `sub` claim) doğrular; `cmd/devtoken` geliştirme
  token'ı üretir.

- **D14 Hizmet kalemleri:** `contract_items.requires_delivery` (varsayılan true). false ise miktar/irsaliye kuralı
  çalışmaz. Sözleşmesiz satırda miktar kuralı yalnız fatura bir irsaliyeye referans veriyorsa çalışır.
- **D15 Kur ve bulgu para birimi:** `audit_findings.impact_currency` eklenir; ROI özeti para birimine göre gruplanır.
- **D16 Semantik eşleşme öneridir (Faz 2b):** §1.3'teki "kosinüs ≥ 0.86 ile eşleştir" uygulanmaz; yerel modelde
  benzer ama farklı ürünler (A4/A3, M8/M10) 0.86'yı geçti, aynı ürünün farklı yazımları çoğunlukla geçemedi. Embedding
  aynı uom'daki kalemlerden en fazla 3 öneri üretir (≥ 0.50); insan onaylarsa (tedarikçi, normalize ad, uom) takma adı
  kaydedilir ve sonraki eşleşmeler deterministik olur. Ayrıntı: `docs/superpowers/specs/2026-11-04-faz2b-semantic-matching-design.md`.

## 5. Fazlar

- **Faz 1:** Go çekirdek: UBL parser, SKU eşleştirme, tüm kurallar, PostgreSQL + RLS, sözleşme JSON import, REST API, testler.
- **Faz 2:** Python AI servisi: PDF sözleşme çıkarımı, embedding, semantik eşleştirme, TCMB kurları.
- **Faz 3:** Vue 3 web uygulaması (AuditGrid, SplitDiffViewer, RoiSummaryCards), kullanıcılar, yayına alma.
