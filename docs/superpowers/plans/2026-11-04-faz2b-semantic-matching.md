# Faz 2b Semantik Kalem Eşleştirme Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Kod/barkod tutmayan fatura satırına embedding ile sözleşme kalemi önerisi üret, insan onayını takma ad olarak sakla, sonraki eşleşmeleri takma adla deterministik yap.

**Architecture:** ai-service `POST /v1/embed` (Ollama `bge-m3`). Go denetimi, eşleşmeyen satırları transaction dışında embed eder, sözleşme kalemi vektörlerini pgvector'da tutar, önerileri `UNCONTRACTED_ITEM` kanıtına yazar. `POST /invoice-lines/{id}/match` takma ad kaydeder ve yeniden denetler.

**Tech Stack:** Python 3.12/FastAPI/httpx, Go 1.25/pgx/shopspring decimal, Postgres 16 + pgvector.

**Spec:** `docs/superpowers/specs/2026-11-04-faz2b-semantic-matching-design.md` (D16, R1–R7).

## Global Constraints

- Go'da ve testlerde `float32/float64` yok; benzerlik `decimal`, vektör `json.RawMessage`.
- Yeni tenant tablosu: `ENABLE` + `FORCE ROW LEVEL SECURITY`, `NULLIF(current_setting('app.current_tenant_id', true), '')::uuid` politikası, composite FK.
- Kalem adı/vektör loglanmaz (KVKK). Hata mesajları sabit.
- Commit mesajında Claude atfı yok.
- Öneri tabanı `0.50`, en fazla `3`; embed bağlam zaman aşımı `60s`; boyut `1024`; model varsayılanı `bge-m3`.

## Review Focus

- AI kapalı/hatalı: denetim aynı Faz 1 sonucunu vermeli (öneri yok, hata yok). → Task 5 testi.
- Farklı uom'daki kalem önerilmemeli; başka modelle embed edilmiş kalem karşılaştırılmamalı. → Task 3 testi.
- Başka tedarikçinin kalemine takma ad: 422. → Task 6 testi.
- Takma ad hedefi aktif sözleşmede değilse yok sayılmalı. → Task 2 testi.
- Takma adla eşleşen kodsuz satır irsaliyeden tahsis alabilmeli (sözleşme kalemi kodu). → Task 5 testi.

---

### Task 1: ai-service `/v1/embed`

**Files:** Modify `ai-service/matchwatch_ai/llm.py`, `config.py`, `app.py`; Test `ai-service/tests/test_embed.py`.

- `Settings.embed_model = "bge-m3"` (`EMBED_MODEL`).
- `OpenAICompatEmbedder(base_url, model, timeout_s, transport).embed(texts) -> list[list]`: POST `/v1/embeddings` `{"model", "input"}`, `data` sırası `index`'e göre; sayı/tip uyuşmazsa `LLMError("LLM_BAD_OUTPUT")`, bağlantı/HTTP hata `LLM_UNAVAILABLE`.
- `create_app(settings, llm=None, ocr=..., embedder=None)`; `/v1/embed`: 401, gövde doğrulama 400 `BAD_REQUEST` (liste değil, 0 veya >256, boş/>512 metin, string değil), 502 LLM hataları, 200 `{"model", "vectors"}`. Log: `embed n=%d ms=%d code=%s`.
- Testler: yetki, sınırlar, sıra korunur, upstream hata → 502, embedder sınıfı httpx MockTransport ile.

### Task 2: Go saf katman

**Files:** `backend-go/internal/domain/namekey.go` (+test), `domain/types.go`, `internal/matcher/matcher.go` (+test).

- `domain.NameKey(s string) string`; test: `"AYÇİÇEK YAĞI 5LT/TNK"` → `"aycicek yagi 5lt tnk"`, `"  Işık  "` → `"isik"`.
- `domain.AliasKey{NameKey, UOM string}`; `domain.Suggestion{ContractItemID uuid.UUID; ItemName string; Similarity decimal.Decimal}`.
- `matcher.MatchAlias(line, items, aliases map[domain.AliasKey]uuid.UUID) *domain.ContractItem` — hedef `items` içinde değilse nil.
- `matcher.PickSuggestions(c []domain.Suggestion) []domain.Suggestion` — `>= 0.50`, sırayı korur, en fazla 3.

### Task 3: Migration 006 + repository

**Files:** `backend-go/migrations/006_semantic_matching.sql`, `internal/repository/semantic.go`, `semantic_integration_test.go`.

- `tx.Aliases(ctx, vendorID) (map[domain.AliasKey]uuid.UUID, error)`
- `tx.ItemsNeedingEmbedding(ctx, contractID, model) ([]domain.ContractItem, error)` (yalnız ID, Name)
- `tx.SetItemEmbedding(ctx, itemID, model string, vec json.RawMessage) error`
- `tx.Suggestions(ctx, contractID, uom, model string, vec json.RawMessage) ([]domain.Suggestion, error)` — en yakın 3, benzerlik `round(...,4)::text`.
- `tx.UpsertAlias(ctx, lineID, itemID) (invoiceID uuid.UUID, err)` — satır yoksa `ErrNotFound`, kalem yoksa `ErrNotFound`, tedarikçi farklıysa `ErrVendorMismatch`.
- Test: sahte 1024 vektörler; sıralama, uom filtresi, model filtresi, RLS ile başka tenant görünmez.

### Task 4: `aiclient.Embed`

**Files:** `internal/aiclient/client.go`, `client_test.go`.

- `Embed(ctx, texts []string) (model string, vecs []json.RawMessage, err error)`; JSON gövde, 256'lık parçalar, sayı uyuşmazlığı `AI_BAD_RESPONSE`, hata gövdesi kodu taşınır, 8 MiB sınırı.

### Task 5: Denetim entegrasyonu + kanıt

**Files:** `internal/audit/service.go`, `semantic.go`, test; `internal/anomaly/run.go`, `price.go`, `qty.go`, `withholding.go` (+testler); `cmd/api/main.go`; çağıranlar.

- `audit.Embedder` arayüzü (`Embed`); `audit.New(store, emb Embedder)` (nil = kapalı).
- `LineContext.MatchMethod string` (`"ALIAS"` ya da boş), `LineContext.Suggestions []domain.Suggestion`.
- `uncontractedItem`: öneri varsa `evidence["suggestions"]`. ALIAS ise fiyat/miktar/tevkifat kanıtına `match_method`.
- Takma ad eşleşmesinde satır kodu/barkodu boşsa `Allocate` için kalemin kodu/barkodu.
- Test: sahte embedder ile öneri; AI hatası → öneri yok, aynı durum; takma ad → `PRICE_MISMATCH` + `ALIAS`.

### Task 6: HTTP uç noktası

**Files:** `internal/transport/http/router.go`, `match.go`, `match_integration_test.go`.

- `POST /api/v1/invoice-lines/{id}/match` → 400 geçersiz id/gövde, 404, 422 `vendor mismatch`, 200 `{invoice_id, audit_status}`.

### Task 7: Compose, smoke, belgeler

- `scripts/smoke-ai.sh`: `EMBED_MODEL` çek; kodsuz satırlı fatura fixture'ı ile öneri → onay → `PRICE_MISMATCH` + `ALIAS`.
- `docker-compose.yml` `EMBED_MODEL`; `.env.example`; `CLAUDE.md` durum/mimari; `README.md`.
