# matchwatch

Satın alma sözleşmeleri, e-İrsaliye ve e-Faturaları (UBL-TR) çapraz denetleyen, 3-Way Matching ve anomali tespitiyle kurumsal harcama kaçaklarını önleyen AI iç denetim motoru.

matchwatch cross-checks purchase contracts, e-Despatch and e-Invoice documents (GİB UBL-TR 1.2) and stores anomaly findings with their evidence. Faz 1 (this Go core) is rule-based and deterministic; amounts are never computed by an LLM.

## Requirements

Go 1.25+ and Docker (Postgres 16 + pgvector via `docker compose`).

## Quick start

```bash
cp .env.example .env     # adjust; on a machine where 5432 is taken, set POSTGRES_PORT and the URLs
make up migrate run
```

Without `make` (Windows), from the repo root, with the `.env` values exported:

```bash
docker compose up -d
cd backend-go
go run ./cmd/migrate      # needs MIGRATE_DATABASE_URL
go run ./cmd/devtoken -name Demo -vkn 9876543210 -rate 0.45   # creates a tenant, prints a JWT
go run ./cmd/api          # needs DATABASE_URL, JWT_SECRET, HTTP_ADDR, STORAGE_DIR
```

### AI extraction (optional)

`docker compose --profile ai up -d` adds Ollama and the Python `ai-service` (127.0.0.1:8090). Set `AI_SERVICE_URL=http://127.0.0.1:8090` and `AI_SERVICE_TOKEN` for the API, then upload a contract PDF to `POST /api/v1/contract-drafts`, review the draft and approve it with `POST /api/v1/contracts` carrying `draft_id`. `bash scripts/smoke-ai.sh` runs this end to end.

Invoice lines that match no contract item by code or barcode get up to three embedding-ranked suggestions in their `UNCONTRACTED_ITEM` finding (`evidence.suggestions`). Suggestions never match on their own (D16): confirm one with `POST /api/v1/invoice-lines/{id}/match` `{"contract_item_id": "..."}`, which stores an alias for that vendor's item name and re-audits the invoice.

When an invoice's currency differs from the contract's and the invoice carries no usable `PricingExchangeRate`, the price rule uses TCMB's forex buying rate from the last bulletin before the invoice date (D17), fetched from `TCMB_BASE_URL` (default `https://www.tcmb.gov.tr/kurlar`) and cached in the database. Set `TCMB_BASE_URL=` (empty) to turn this off; findings then fall back to `FX_UNVERIFIED`.

When the invoice carries its own `PricingExchangeRate`, that rate is compared with the same TCMB rate; more than 1% off gives `FX_RATE_DEVIATION`, and the price rule then converts at TCMB instead (D18).

`EXTRACT_DEADLINE_S` (ai-service, default 270) bounds the whole extraction, retries included, and must stay below `AI_EXTRACT_TIMEOUT` (API, default 5m) so the service answers before the API gives up. On slow CPU hosts raise both together (and `LLM_TIMEOUT_S`).

## Tests

```bash
cd backend-go && go test ./...                      # unit tests (make test)
# integration tests need MATCHWATCH_TEST_DATABASE_URL and MATCHWATCH_TEST_MIGRATE_URL (make test-integration)
cd backend-go && go test ./... -count=1
bash scripts/smoke.sh                               # end-to-end: DB, API, happy-path flow
```

## API

All routes under `/api/v1` need `Authorization: Bearer <tenant JWT>`.

| Method | Path | Purpose |
|---|---|---|
| GET | `/healthz` | liveness (no auth) |
| POST | `/api/v1/contracts` | load a contract (JSON) |
| GET | `/api/v1/contracts/pdf?contract_no=` | the PDF an approved contract was read from |
| POST | `/api/v1/despatches` | ingest a DespatchAdvice (XML) |
| POST | `/api/v1/receipts` | ingest a ReceiptAdvice (XML) |
| POST | `/api/v1/invoices` | ingest an Invoice (XML) and audit it |
| GET | `/api/v1/invoices` | list invoices: `q` (invoice no, vendor, ETTN), `status`, `vendor_id`, `from`/`to` (issue date), `open=true`, `sort` + `dir`, `limit`/`offset`; returns `counts` per status |
| GET | `/api/v1/invoices/{id}` | invoice with lines and findings; each finding carries its `decisions` (who, when, note), oldest first |
| POST | `/api/v1/invoices/{id}/audit` | re-run the audit (idempotent) |
| GET | `/api/v1/aliases` | confirmed item aliases with their contract item and who confirmed them, plus `removed` (the last 50 removals: what, who, when); optional `vendor_id` |
| DELETE | `/api/v1/aliases/{id}` | remove a wrong alias and re-audit the invoices that used it; returns `reaudited` |
| GET | `/api/v1/findings` | findings across invoices, largest impact first; `resolution`, `rule`, `vendor_id`, `limit` (≤ 200), `offset`; `counts` per rule |
| PATCH | `/api/v1/findings/{id}` | set a finding's resolution; also appends it to the decision history |
| POST | `/api/v1/findings/decisions` | one decision for 1–200 findings, `{finding_ids, resolution_status, note}`; all or nothing |
| GET | `/api/v1/summary` | caught/disputed totals per currency |

## Docs

- [`docs/spec.md`](docs/spec.md): the authoritative spec (its §4 decisions D1–D15 win).
- Phases: Faz 1 Go core (this repo, see `docs/superpowers/plans/2026-11-02-faz1-go-core.md`); Faz 2 Python AI/PDF service and Faz 3 Vue 3 web app are not started.

## Web app

`web/` is the Vue 3 app (login, invoice list, findings with contract and invoice side by side, uploads, contract
draft review, summary). Open an account with `cmd/onboard` (owner role; password on stdin), then either run
`npm run dev` in `web/` against the API on `:8080`, or build it (`npm run build`) and let the API serve it with
`WEB_DIR=../web/dist`. Set `COOKIE_SECURE=false` only for plain-http local use.
