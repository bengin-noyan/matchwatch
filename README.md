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
| POST | `/api/v1/despatches` | ingest a DespatchAdvice (XML) |
| POST | `/api/v1/receipts` | ingest a ReceiptAdvice (XML) |
| POST | `/api/v1/invoices` | ingest an Invoice (XML) and audit it |
| GET | `/api/v1/invoices` | list invoices (`status`, `limit` filters) |
| GET | `/api/v1/invoices/{id}` | invoice with lines and findings |
| POST | `/api/v1/invoices/{id}/audit` | re-run the audit (idempotent) |
| PATCH | `/api/v1/findings/{id}` | set a finding's resolution |
| GET | `/api/v1/summary` | caught/disputed totals per currency |

## Docs

- [`docs/spec.md`](docs/spec.md): the authoritative spec (its §4 decisions D1–D15 win).
- Phases: Faz 1 Go core (this repo, see `docs/superpowers/plans/2026-11-02-faz1-go-core.md`); Faz 2 Python AI/PDF service and Faz 3 Vue 3 web app are not started.
