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

The data flow is: parse XML → persist → load audit input → match → run rules → save findings. Invoice ingest commits in its own tenant transaction; the audit (load, match, rules, save) then runs in a second one.

- `internal/ubl`: XML → parser structs. Struct tags match on the **namespace URI, never the prefix**, and paths are nested structs. ETTN is stored canonical (`uuid.Parse(...).String()`). Parse errors are `*ValidationError{Field, Reason}` or `ErrNot<Doc>` and must never panic.
- `internal/domain`: shared types and money helpers. `ToMinor` is the **only** decimal→kuruş conversion (half-away-from-zero). Rule, severity and status constants live here (`RulePriceMismatch`, `AuditRed`, `ResolutionDisputed`, …).
- `internal/migrate` + `migrations/*.sql` (embedded): applied in lexical order, one transaction per file, tracked in `schema_migrations`.
- `internal/repository`: `Store.InTenant`, ingest, audit load/save (`DespatchCandidates` locks lines in one statement, then reads availability in a second), contract JSON parsing, queries.
- `internal/matcher` and `internal/anomaly`: pure functions with no DB or network access. PRICE/QTY findings need a decimal impact above half a kuruş.
- `internal/audit`: the idempotent re-audit service, plus `Resolve`, which recomputes `audit_status` after a PATCH.
- `internal/auth`: HS256 JWT carrying `tenant_id`.
- `internal/transport/http`: REST under `/api/v1`. Raw XML is stored at `STORAGE_DIR/<tenant>/<invoice|despatch>/<ettn>.xml`.
- `cmd/api`, `cmd/devtoken`, `cmd/migrate`. `scripts/smoke.sh` is the end-to-end check (expects `"caught_minor":21412`).

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

## Current status (2026-11-03)

Faz 1 is complete on branch `faz1-go-core`: all 12 tasks, the final whole-branch review and its fix wave. It is submitted as a PR to `main`. Faz 2 and Faz 3 are not started.

- **Rulings that deviate from the plan text:**
  - The RLS policy wraps the setting in `NULLIF(...)`.
  - `CreateTenant` takes a `financingRate`.
  - FKs are composite.
  - `withholding_codes.percent` is nullable, and the loader skips NULL rows.
  - Ingest checks reject a line with more than one withholding subtotal.
  - PRICE/QTY findings need a decimal impact above 0.005.
  - The raw XML path includes the document kind.
  - Migration `004` adds the payment-term page and clause, and `CHECK (qty > 0)`.
  - Parser bounds: amounts and quantities below 1e15, prices below 1e13, currency `^[A-Z]{3}$`.
  - The `go` directive is 1.25.0 (required by pgx).
- **Known open minors:**
  - A PATCH racing a re-audit can leave the status one step stale.
  - FX currency codes are not format-checked.
  - Summed allowance and tax totals are not bounded.
  - A post-commit audit failure leaves the invoice PENDING; recover with `POST /invoices/{id}/audit`.
- **Open for the user:**
  - Withholding code 619's rate (70 vs "75/10").
  - Whether a GİB code list newer than V1.31 exists.
  - KVKK data residency.

## Conventions

- Integration tests read `MATCHWATCH_TEST_DATABASE_URL` (app role) and `MATCHWATCH_TEST_MIGRATE_URL` (owner role). They `t.Skip` when these are unset, and they must be re-runnable against the same DB: use unique tenants and ETTNs per run.
- Commit messages must not include `Co-Authored-By: Claude` or any Claude attribution.
