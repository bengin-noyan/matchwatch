# Faz 2b — Semantik Kalem Eşleştirme: Tasarım

Tarih: 2026-11-04. Durum: mimar kararıyla onaylandı (kullanıcı kararı Claude'a bıraktı).
Üst belge: `docs/spec.md`. Bu belge spec §1.3'ü ("sonra semantik, kosinüs ≥ 0.86, aynı uom") ve D8'i uygular,
ama §1.3'ten bilinçli olarak sapar (yeni **D16**, aşağıda).

## 1. Amaç ve neden

Faz 1'de fatura satırı sözleşme kalemine yalnızca `item_code` ya da barkod tutarsa bağlanır. Gerçek faturalarda
tedarikçi kodu sözleşmedekinden farklıdır ya da hiç yoktur ("AYCICEK YAGI 5LT TNK" ↔ "Ayçiçek Yağı 5 L Teneke").
Bu satırlar `UNCONTRACTED_ITEM` olur ve asıl kaçak (sözleşmeden yüksek fiyat) görünmez kalır.

## 2. Kalibrasyon bulgusu (karar bunun üstüne kurulu)

Yerel `bge-m3` (Ollama, 1024 boyut) ile 18 Türkçe kalem çifti ölçüldü (2026-11-04):

- Aynı ürün, farklı yazım: ham 0.59–0.92, normalize metinle 0.58–0.94.
- Farklı ürün, benzer ad: **A4↔A3 kâğıt 0.90, M8↔M10 civata 0.89, ayçiçek↔zeytinyağı 0.86**, Tip 550↔650 un 0.80.

İki dağılım iç içe: hiçbir eşik "otomatik eşleştir" için güvenli değil. 0.86'da hem gerçek eşleşmelerin çoğu
kaçar hem de yanlış ürüne bağlanma olur. Yanlış bağlanma, yanlış fiyatla **sahte CRITICAL `PRICE_MISMATCH`**
demektir. Buna karşılık **sıralama** güvenilir: 9/9 gerçek eşleşmede doğru kalem ilk sıradaydı.

## 3. D16 — Semantik eşleşme öneridir, karar insanındır

- Kod/barkod eşleşmeyen satır için embedding ile aynı uom'daki sözleşme kalemlerinden **en fazla 3 öneri** üretilir
  (benzerlik ≥ 0.50). Öneriler `UNCONTRACTED_ITEM` bulgusunun kanıtına yazılır; bulgu WARNING kalır.
- İnsan bir öneriyi onaylarsa **kalem takma adı** (`item_aliases`) kaydedilir: (tedarikçi, normalize ad, uom) →
  sözleşme kalemi. Fatura yeniden denetlenir.
- Sonraki denetimlerde eşleşme sırası: kod → barkod → takma ad. Takma ad eşleşmesi deterministiktir; aynı
  tedarikçinin aynı adlı satırı bir daha soru sormaz.
- Gerekçe: spec kuralı 2 (onay/red kararları LLM'e bırakılmaz). Embedding yalnızca aday sıralar.

## 4. Hedefler

- G1. Kod/barkod tutmayan satırda doğru sözleşme kalemi ilk üç önerideyse, bir onayla eşleşir; o andan sonra o
  satır adı kalıcı olarak eşleşir.
- G2. Takma adla eşleşen satırda bütün sözleşme kuralları (fiyat, miktar, tevkifat) çalışır; kanıtta
  `match_method: "ALIAS"` yazar.
- G3. AI servisi kapalı ya da hatalıysa denetim Faz 1 davranışıyla tamamlanır (öneri yok); denetim hiç bozulmaz.
- G4. KVKK: kalem adları TR altyapımızdan çıkmaz; embedding modeli yerel Ollama'da çalışır.

## 5. Hedef dışı

- Otomatik semantik eşleşme (D16 gereği yok).
- İrsaliye satırlarının semantik eşleşmesi. Takma adla eşleşen satırın kodu boşsa, irsaliye tahsisi için sözleşme
  kaleminin kodu/barkodu kullanılır; irsaliyede de kod yoksa Faz 1'deki gibi `no_despatch_found`.
- Hizmet faturası semantik mükerrerlik (spec Faz 2) → ayrı iş, Faz 2b sonrası.
- Takma ad silme/listeleme uç noktaları, öneri arayüzü → Faz 3. Yanlış onay, aynı satır için başka kalem onaylanarak
  düzeltilir (upsert).
- Sözleşme yenilenince takma adlar eski kalemi gösterir ve yok sayılır; yeni sözleşmede öneri akışı tekrar çalışır.

## 6. Ayrıntılar

### 6.1 Python servisi

- `POST /v1/embed`, bearer token (extract ile aynı). Gövde `{"texts": [...]}`: 1–256 metin, her biri boş olmayan ve
  ≤ 512 karakter; aşılırsa 400 `BAD_REQUEST`.
- Yanıt `{"model": "<ad>", "vectors": [[...], ...]}`, girişle aynı sıra ve sayıda. Ollama'nın OpenAI uyumlu
  `/v1/embeddings` ucu; `EMBED_MODEL` (varsayılan `bge-m3`), `trust_env=False`, `LLM_BASE_URL` ile aynı özel host.
- Hata: Ollama erişilemez/bozuk yanıt → 502 `LLM_UNAVAILABLE` / `LLM_BAD_OUTPUT`. Metin loglanmaz; yalnızca sayı ve süre.

### 6.2 Veri

Migration `006_semantic_matching.sql`:

- `contract_items.name_embedding vector(1024)`, `contract_items.embedding_model TEXT` (ikisi de NULL olabilir).
  Boyut modele bağlıdır; 1024 olmayan modele geçmek yeni migration ister.
- `item_aliases(id, tenant_id, vendor_id, name_key, uom, contract_item_id, created_at)`,
  `UNIQUE (tenant_id, vendor_id, name_key, uom)`, composite FK'ler (vendor, contract_item `ON DELETE CASCADE`),
  `ENABLE` + `FORCE ROW LEVEL SECURITY`, standart politika.
- Fatura satırı vektörü **saklanmaz**: yalnız eşleşmeyen satırlar için denetim anında hesaplanır.

### 6.3 Go

- `domain.NameKey(s)`: Türkçe küçük harf, aksan katlama (ç ğ ı ö ş ü → c g i o s u), harf/rakam dışı → boşluk,
  boşluk sıkıştırma. Hem takma ad anahtarı hem embedding girdisi budur (kalibrasyonda normalize metin daha iyi).
- `matcher.MatchAlias(line, items, aliases)` ve `matcher.PickSuggestions(cands)` (≥ 0.50, en fazla 3, decimal): saf.
- `aiclient.Embed(ctx, texts) (model string, vectors []json.RawMessage, err)`. Vektörler Go'da **ayrıştırılmaz**;
  ham JSON dizisi pgvector literal'i olarak `$n::vector`'a gider (Go'da float yok, boyutu Postgres doğrular).
- Denetim akışı (`audit.Service`, embedder nil değilse):
  1. Okuma transaction'ı: fatura, aktif sözleşme, takma adlar; kod/barkod/takma ad ile eşleşmeyen satırlar.
  2. Eşleşmeyen satır yoksa AI çağrılmaz. Varsa satır adları embed edilir (transaction dışında, 60 sn bağlam).
  3. Dönen modelle vektörü olmayan ya da farklı modelde olan sözleşme kalemleri embed edilip yazılır.
  4. Asıl denetim transaction'ı Faz 1'deki gibi; eşleşmeyen satır için pgvector sorgusu
     (`1 - (name_embedding <=> $v)`, aynı sözleşme, aynı uom, aynı model) → `PickSuggestions`.
  - Adım 1–3'te hata: log (içerik yok), öneri olmadan devam (G3).
- `POST /api/v1/invoice-lines/{id}/match` `{"contract_item_id": "..."}`: satır 404, kalem 404, kalem başka
  tedarikçinin sözleşmesindeyse 422. Takma adı upsert eder, faturayı yeniden denetler, `{invoice_id, audit_status}` döner.
- Kanıt: `UNCONTRACTED_ITEM` → `suggestions: [{contract_item_id, item_name, similarity}]` (benzerlik string,
  4 hane). Takma adla eşleşen satırın fiyat/miktar/tevkifat bulguları → `match_method: "ALIAS"`.

### 6.4 Compose ve smoke

`scripts/smoke-ai.sh` `bge-m3`'ü de çeker ve şu akışı ekler: kodsuz satırlı fatura → öneride doğru kalem ilk sırada →
onay → yeniden denetimde `PRICE_MISMATCH` + `match_method: "ALIAS"`.

## 7. Test stratejisi

- Python: `/v1/embed` sahte embedder ile (yetki, sınırlar, sıra, hata kodları); gerçek model testi env ile atlanır.
- Go birim: `NameKey`, `MatchAlias`, `PickSuggestions`, kanıt alanları, `aiclient.Embed` (httptest).
- Go entegrasyon: sabit 1024 boyutlu sahte vektörlerle öneri sorgusu ve sıralaması; denetim + onay + yeniden
  denetim; AI hatasında denetimin Faz 1 sonucunu vermesi; HTTP uç noktası (404/422/200).

## 8. Mimar kararları

- R1. D16: otomatik semantik eşleşme yok; öneri + insan onayı + takma ad (§2 ölçümü).
- R2. Model `bge-m3` (çok dilli, Ollama'da var, 1.2 GB, 1024 boyut).
- R3. Öneri tabanı 0.50, en fazla 3: kalibrasyonda gerçek eşleşmelerin en düşüğü 0.58, ilgisiz kalem en yüksek 0.46.
  Öneri zararsızdır; taban gürültüyü kısar, kararı vermez.
- R4. Fatura satırı vektörü saklanmaz (mükerrerlik işi gelince yeniden değerlendirilir).
- R5. Takma ad anahtarı (tedarikçi, NameKey, uom); hedef somut sözleşme kalemi.
- R6. Go vektörleri opak taşır; float Go'ya girmez.
- R7. Embed zaman aşımı sabit 60 sn (`shortcut:` yorumlu; ayar gerekirse config'e alınır).
