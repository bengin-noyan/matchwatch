# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

matchwatch is an internal-audit engine for Turkish companies. It cross-checks purchase contracts, e-İrsaliye (`DespatchAdvice`) and e-Fatura (`Invoice`) documents in GİB UBL-TR 1.2 format, a "3-way match", and stores anomaly findings together with their evidence.

- **Spec:** `docs/spec.md` is the authority. Its §4 "Kararlar" (D1–D15) deliberately deviate from the original "LeakZero" spec, and the D-decisions win where they conflict.
- **Phases:** Faz 1 is the Go core and is in progress. Faz 2 (the Python AI/PDF/embedding service) and Faz 3 (the Vue 3 web app) are not started.
- **Faz 1 plan:** `docs/superpowers/plans/2026-11-02-faz1-go-core.md` lists tasks, interfaces and the exact expected test values.

## Commands

All Go code lives in `backend-go/` (module `github.com/bengin-noyan/matchwatch/backend-go`). Compose and the Makefile are at the repo root.

```bash
cp .env.example .env              # then adjust; .env overrides .env.example in the Makefile
docker compose up -d              # Postgres 16 + pgvector; deploy/postgres/00-init.sh creates roles on first boot
docker compose down -v            # reset DB (init script only runs on an empty volume)

cd backend-go
go test ./...                     # unit tests; integration tests t.Skip without env
go test ./internal/ubl/ -run TestParseInvoiceBasic -v      # single test
go vet ./... && gofmt -l .

# migrations / integration tests (equivalent of `make migrate` / `make test-integration`)
MIGRATE_DATABASE_URL=... go run ./cmd/migrate
MATCHWATCH_TEST_DATABASE_URL=... MATCHWATCH_TEST_MIGRATE_URL=... go test ./... -count=1
```

- `make` is not installed on the primary dev machine (Windows), so run the commands above directly. Take the URLs from `.env`.
- Locally, the matchwatch Postgres runs on port **5433**, because another project holds 5432. The git-ignored `.env` already uses 5433.
- `.gitattributes` forces LF line endings. Shell scripts break inside the container if they get CRLF.

## Architecture

The data flow is: parse XML → persist → load audit input → match → run rules → save findings. All of it happens in one tenant transaction.

- `internal/ubl`: XML → parser structs. Struct tags match on the **namespace URI, never the prefix**, and paths are nested structs. ETTN is stored canonical (`uuid.Parse(...).String()`). Parse errors are `*ValidationError{Field, Reason}` or `ErrNot<Doc>` and must never panic.
- `internal/domain`: shared types and money helpers. `ToMinor` is the **only** decimal→kuruş conversion (half-away-from-zero). Rule, severity and status constants live here (`RulePriceMismatch`, `AuditRed`, `ResolutionDisputed`, …).
- `internal/migrate` + `migrations/*.sql` (embedded): applied in lexical order, one transaction per file, tracked in `schema_migrations`.
- Planned in the Faz 1 plan, not yet present:
  - `repository`: `Store.InTenant`, ingest, audit load/save, queries.
  - `matcher` and `anomaly`: pure functions with no DB or network access.
  - `audit`: the idempotent re-audit service.
  - `auth`: HS256 JWT carrying `tenant_id`.
  - `transport/http`: REST under `/api/v1`.
  - `cmd/api`, `cmd/devtoken`.

### Multi-tenancy (easy to break)

- **Two DB roles.**
  - `matchwatch_owner` runs migrations.
  - The app connects only as `matchwatch_app`: not the owner, and `NOBYPASSRLS`.
- **Row-level security.** Every tenant table has `ENABLE` + `FORCE ROW LEVEL SECURITY`. The policy uses `NULLIF(current_setting('app.current_tenant_id', true), '')::uuid`.
  - The `NULLIF` matters. After a transaction-local `set_config`, a pooled session keeps the GUC as `''`, and a bare `::uuid` cast would error.
- **Setting the tenant.** It is set per transaction with `SELECT set_config('app.current_tenant_id', $1, true)`.
  - Every tenant query must run inside that transaction (`Store.InTenant`).
  - Test setup has to do the same, even as the owner, because `FORCE` applies to the owner too.
- **Foreign keys are composite** (`(tenant_id, x_id) REFERENCES parent(tenant_id, id)`). FK checks and `ON DELETE CASCADE` bypass RLS, so single-column FKs would let one tenant link to, and cascade-delete, another tenant's rows. Keep new FKs composite.

### Money and rules

- **Never use `float32`/`float64`** for money, quantities or rates, and that includes tests.
  - In Go, use `shopspring/decimal`.
  - In the DB:
    - unit prices are `NUMERIC(19,6)` (D1: sub-kuruş prices exist);
    - quantities are `NUMERIC(19,4)`;
    - rates are `NUMERIC(5,2)`;
    - totals and `financial_impact_minor` are `BIGINT` kuruş.
  - In JSON, decimals are strings and kuruş values are integers.
- Price comparisons use the **net** unit price, `LineExtension / Qty` (D2).
- Invoice↔despatch is n:m through `despatch_allocations` (D3). The same delivered quantity cannot be invoiced twice.
- Every finding's `evidence_json` carries the invoice `line_id`. Contract-based findings also carry `contract_no`, `source_page` and `source_clause_text`.
- An LLM never computes amounts or makes approve/reject decisions.
- `withholding_codes` is seeded from GİB "UBL-TR Kod Listeleri" V1.31 (2023). Code 619's rate is marked unverified in `002_withholding_codes.sql`.

## Current status (2026-11-03, end of day)

Branch `faz1-go-core` (not pushed, not merged). The Faz 1 plan is executed with subagent-driven development: one implementer and one reviewer per task. The progress ledger lives at `.superpowers/sdd/2026-11-02-faz1-go-core/progress.md` (git-ignored). It records every completed task, fix round, deferred minor and ruling. Read it first when resuming.

- **Done and reviewed, Tasks 1–8:**
  - scaffold
  - domain
  - Invoice parser
  - Despatch/Receipt parser
  - schema + forced RLS + composite FKs + withholding seed
  - repository ingest, plus `003_document_paths.sql`
  - matcher
  - anomaly rules: price/qty/maturity/uncontracted/uom/fx
- **Next: Task 9.** Duplicate, withholding and price-creep rules. It was stopped before any code was written; re-dispatch it from HEAD `ff04b53`. After that come Task 10 (audit service), Task 11 (auth + REST API) and Task 12 (smoke test + README), then the final whole-branch review.
- **Rulings made during execution** that deviate from the plan text:
  - The RLS policy wraps the setting in `NULLIF(...)`.
  - `CreateTenant` takes a `financingRate`.
  - FKs are composite.
  - `withholding_codes.percent` is nullable.
  - Ingest checks reject a line with more than one withholding subtotal.
  - PRICE/QTY findings are emitted only when the impact is at least 1 kuruş.
  - UOM/FX findings carry contract evidence.
  - The `go` directive is 1.25.0 (required by pgx).
- **Open for the user:** check withholding code 619's rate (70 vs "75/10"), and check for a GİB code list newer than V1.31.

## Conventions

- Integration tests read `MATCHWATCH_TEST_DATABASE_URL` (app role) and `MATCHWATCH_TEST_MIGRATE_URL` (owner role). They `t.Skip` when these are unset, and they must be re-runnable against the same DB: use unique tenants and ETTNs per run.
- Commit messages must not include `Co-Authored-By: Claude` or any Claude attribution.
