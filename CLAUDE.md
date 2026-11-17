# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

matchwatch is an internal-audit engine for Turkish companies. It cross-checks purchase contracts, e-İrsaliye (`DespatchAdvice`) and e-Fatura (`Invoice`) documents in GİB UBL-TR 1.2 format, a "3-way match", and stores anomaly findings together with their evidence.

- **Spec:** `docs/spec.md` is the authority. Its §4 "Kararlar" (D1–D15) deliberately deviate from the original "LeakZero" spec, and the D-decisions win where they conflict.
- **Phases:** Faz 1 is the Go core and is done (merged to `main`). Faz 2a (PDF contract extraction via `ai-service/`) is merged to `main` (PR #2). Faz 2b (semantic item suggestions + human-confirmed aliases, D16) is merged (PR #3). Faz 2c (TCMB rates, D17) is merged (PR #4). Faz 3 (login, the Vue 3 web app in `web/`, dashboard, vendor pages, contract PDF viewer) is merged (PR #5). Faz 3e (invoice search, filters, paging) is merged (PR #6). Faz 3f (decision history) is merged (PR #7). Faz 3g (bulk decisions) is merged (PR #8). Faz 3h (alias management) is merged (PR #9). Faz 3i (public landing page on the login screen) is merged (PR #10). Faz 3j/3k (landing split from login, self sign-up) are merged (PR #12). The planned web work is complete. Faz 4 prep (the provider-agnostic production setup) is merged (PR #13). Faz 5a (user invites and password reset by one-time link) is merged (PR #14). The `FX_RATE_DEVIATION` rule (D18) is merged (PR #15), and the code comments were rewritten in plain Turkish (PR #16).
- **Faz 1 plan:** `docs/superpowers/plans/2026-11-02-faz1-go-core.md` lists tasks, interfaces and the exact expected test values.

## Commands

All Go code lives in `backend-go/` (module `github.com/bengin-noyan/matchwatch/backend-go`). Compose and the Makefile are at the repo root.

```bash
cp .env.example .env              # then adjust; .env overrides .env.example in the Makefile
docker compose up -d              # Postgres 16 + pgvector; deploy/postgres/00-init.sh creates roles on first boot
docker compose down -v            # reset DB (init script only runs on an empty volume)

docker compose --profile ai up -d  # + Ollama and the Python ai-service on 127.0.0.1:8090 (needs AI_SERVICE_TOKEN)
bash scripts/smoke-ai.sh          # AI end-to-end: PDF -> draft -> contract, then suggestion -> alias -> PRICE_MISMATCH (pulls ~6 GB of models)
cd ai-service && python -m venv .venv && .venv/Scripts/python -m pip install -r requirements-dev.txt && .venv/Scripts/python -m pytest -q

cd web && npm install && npm run dev  # SPA on :5173, proxies /api to the Go API on :8080
cd web && npm test && npm run build   # Vitest, then vue-tsc + vite build into web/dist
# one API serving the built SPA on one origin: WEB_DIR=../web/dist COOKIE_SECURE=false go run ./cmd/api
printf 'a-long-password\n' | MIGRATE_DATABASE_URL=... go run ./cmd/onboard -name "Firma" -vkn 9876543210 -rate 0.45 -email user@firma.com.tr
python -X utf8 -I scripts/seed-demo.py http://127.0.0.1:8080 user@firma.com.tr a-long-password  # dev: months of demo documents

# production (docs/runbook.md): one image (Dockerfile) + Postgres + Caddy TLS; secrets in deploy/.env
docker compose -f deploy/compose.prod.yml --env-file deploy/.env up -d --build
bash scripts/backup.sh && bash scripts/restore-check.sh <backup folder>   # locally: COMPOSE="docker compose" STORAGE_PATH=backend-go/data

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
- `ai-service/` (Python, FastAPI, port 8090): `POST /v1/extract` turns a contract PDF into a structured draft (text layer, OCR fallback, local Ollama LLM, values grounded in the page text) `POST /v1/embed` embeds item names (Ollama `bge-m3`, 1024 dims) and `GET /healthz`. Bearer auth via `AI_SERVICE_TOKEN`; `LLM_BASE_URL` must be a private host (KVKK).
- `internal/aiclient`: HTTP client for the ai-service. Draft flow: `POST /contract-drafts` stores the PDF and extracts into a `contract_drafts` row, a human reviews the draft, then `POST /contracts` with `draft_id` approves it (draft becomes `APPROVED`, frozen). Re-extract via `POST /contract-drafts/{id}/extract`.
- Semantic matching (D16, Faz 2b): matching order is code → barcode → `item_aliases` (vendor, `domain.NameKey`, uom). Before the audit transaction, `audit.Service.suggest` embeds the still-unmatched lines (and any contract items lacking a vector for that model) and ranks same-uom items with pgvector; up to 3 suggestions ≥ 0.50 go into the `UNCONTRACTED_ITEM` evidence. Embeddings **never** auto-match: `POST /invoice-lines/{id}/match` records the human's choice as an alias and re-audits. AI errors only drop suggestions. Go carries vectors as raw JSON (`$n::text::vector`), never floats.
- TCMB rates (D17, Faz 2c): `internal/tcmb` fetches `kurlar/YYYYMM/DDMMYYYY.xml` (404 = no bulletin). `audit.Service.fxFallback` runs before the audit transaction: when the invoice's own `PricingExchangeRate` does not reach a contract item currency, it walks back from the day before the issue date (≤ 10 days) through the global `fx_days`/`fx_rates` cache, fetching missing days, and hands TRY-crossed forex-buying rates to `priceRules` (`anomaly.Input.FXFallback`). The invoice's rate still wins; no rate → `FX_UNVERIFIED`. D18: when the invoice has its own rate, `fxFallback` also fetches TCMB for that pair; more than 1% off gives a header `FX_RATE_DEVIATION` (WARNING, impact payable × rate difference, 0 when in the buyer's favour) and the price rule then converts at TCMB instead, so the gap is neither hidden nor counted twice (migration `013` extends the `rule_code` CHECK). `TCMB_BASE_URL` set but empty disables it. Only a date leaves our infrastructure.
- Login (Faz 3a): `users` has **no RLS and no app-role privileges**; the API reads it only through the `SECURITY DEFINER` functions `auth_lookup(email)` and `auth_user(id, tenant)`, and users are created only by `cmd/onboard` (owner role), by sign-up or by redeeming an invite (below). `POST /auth/login` sets the JWT in an `HttpOnly; SameSite=Strict; Path=/api` cookie (`COOKIE_SECURE=false` for local http); the middleware also still accepts `Authorization: Bearer`. Failed logins lock an e-mail for 15 min after 10 tries (in-process).
- `web/` (Faz 3b): Vue 3 + TS + Vite + Tailwind 4 + Pinia + TanStack Table v9 SPA. Design rules in `docs/superpowers/specs/2026-11-05-faz3-web-design.md`: red pencil = what the system found, blue ballpoint = what a person decided; fonts self-hosted via `@fontsource` (no Google CDN, KVKK). Money is never computed in the browser (`src/money.ts` only re-formats digits). `WEB_DIR` makes the Go API serve `web/dist` on the same origin (CSP, SPA fallback). Screens: Masa (dashboard, `GET /dashboard`), Faturalar (server-side search over invoice no, vendor and ETTN with Turkish i's folded in SQL, status/vendor/date/open-only filters, sorting and 50-row pages; all of it lives in the URL query), Tedarikçiler (`GET /vendors`, `/vendors/{id}` with per-item price history), uploads, drafts. "Sözleşmede göster" on a finding opens its source page (`GET /contracts/pdf?contract_no=`, served from `contracts.file_path`; 404 for contracts entered without a PDF) rendered by lazily loaded pdf.js, with the clause boxed in red pencil (`src/pdf/clause.ts` matches like the ai-service grounding: case- and whitespace-blind, Turkish i's folded). The Go web server forces `.mjs` to `text/javascript`, because the OS mime table can say `text/plain` and the pdf.js worker then fails. Charts are hand-built SVG following the `dataviz` skill (single red-pencil series, one axis, hover + table view; status-bar colours validated with its palette checker).
- Decision history (Faz 3f): every `PATCH /findings/{id}` also appends to `finding_decisions` in the same statement (status, note, user id, an e-mail snapshot via `auth_user`; both NULL for dev tokens). The app role has only SELECT/INSERT on it (append-only). Rows are keyed by the finding's identity (invoice, line, rule), not its id, because a re-audit deletes and recreates OPEN findings; `GET /invoices/{id}` attaches them to each finding as `decisions`, oldest first. Decisions made before migration `009` have no trail.
- Bulk decisions (Faz 3g): `audit.Service.Resolve` takes a list of finding ids (PATCH passes one). It is all or nothing: an unknown id or another tenant's id fails the whole batch with 404. It first locks the findings' invoices in id order (`LockFindingInvoices`), the same lock `ResetAudit` takes first, so decisions cannot deadlock with a re-audit or with each other, and a decision can no longer leave the status stale behind a racing re-audit. Then it decides on each finding (each gets its own history row) and recomputes every touched invoice's status. `POST /findings/decisions` takes 1..200 ids. `GET /findings` lists findings across invoices (resolution/rule/vendor filters, largest impact first, counts per rule); the web screen Bulgular (`/bulgular`) ticks rows on a page and decides them with one note.
- Alias management (Faz 3h): `GET /aliases` lists the confirmed item aliases with their contract item; `DELETE /aliases/{id}` (migration `010` grants DELETE) removes one and re-audits, after the delete commits, every invoice of that vendor with a line matched to that item, uom and name key, so the line falls back to `UNCONTRACTED_ITEM` with fresh suggestions. Findings a person already decided stay (normal re-audit rule). Web screen: Kalem eşleşmeleri (`/eslesmeler`), removal asks for a second click. Alias log (migration `014`): confirming and removing both append to `alias_events` in the same statement (who, with an e-mail snapshot via `auth_user`, plus a copy of vendor, name key, uom, contract no and item name, since the alias row itself is deleted). Like `finding_decisions` it is append-only for the app role and has no backfill. `GET /aliases` also returns `confirmed_by_email` per alias and `removed`, the last 50 removals; the screen shows them under "Kaldırılan eşleşmeler".
- Landing (Faz 3i, split in 3j): guests opening `/` land on `/tanitim` (`LandingView.vue`), a deep link goes to `/giris?next=`. `/giris` (`LoginView.vue`) is only the login: a dark showcase panel (hidden on phones) beside the form. The landing has the hero, an animated SVG desk scene (`components/landing/AuditScene.vue`: contract and e-İrsaliye checked against an e-Fatura, red-pencil loop on the price, blue-ballpoint decision), facts, how it works, all ten rules, KVKK/trust. No external images or fonts (CSP and KVKK): everything is inline SVG and CSS. Motion stops under `prefers-reduced-motion`; in-page links scroll with `scrollIntoView` because the router's `scrollBehavior` would jump to the top on a hash change. Every number on the page is a product fact, not a marketing claim. 3k added the auto-rotating screen tour (`components/landing/ProductTour.vue`, sample data labelled as such), FAQ, closing call to action and footer.
- Sign-up (Faz 3k): `/kayit` (`SignupView.vue`; it and `/giris` share `components/AuthLayout.vue` with a Giriş yap | Kaydol switch). `POST /auth/signup` {company, vkn, email, password} opens a **new, empty tenant** (financing rate 0) and its first user, then sets the session cookie (201). VKN/TCKN check digits are verified (`domain.ValidTaxID`); VKNs stay non-unique, so nobody can claim another company's VKN and lock it out. The user row goes in through `auth_signup(tenant, email, hash)` (migration `011`, `SECURITY DEFINER`), which only adds a user to a tenant that has none. Taken e-mail = 409 `EMAIL_TAKEN`; 5 attempts per client IP per hour (in-process). The IP is the TCP peer; with `TRUST_PROXY=true` (set in the image) `BehindProxy` takes it from the right-most `X-Forwarded-For` entry, the one Caddy writes, so the API must be reachable only through Caddy. `/auth/me`'s 401 does not trigger the expiry redirect (the router guard handles guests).
- Invites and password reset (Faz 5a): both are one-time links **mailed only to the address they are for**, never returned by the API or shown in the UI, so no colleague or inviter can use a link meant for someone else. `internal/mail` speaks SMTP with STARTTLS: `MAIL_URL` is `smtp://user:pass@host:587`, or `log` (dev: mails, links included, go to the API log), or empty (invites and "Şifremi unuttum" answer 503 `MAIL_DISABLED`). `MAIL_FROM` may carry a name. `APP_BASE_URL` builds the links and is never taken from the request's Host header. The provider must be in Türkiye (KVKK). `auth_tokens` (migration `012`) is like `users`: no RLS, no app-role privileges, reached only through `SECURITY DEFINER` functions, and it stores only the SHA-256 of a token. `GET /users` lists logins and open invites. `POST /invites` {email} mails a 7-day invite (a new one replaces the earlier unused one; an e-mail that has a login gets 409). `DELETE /invites/{id}` revokes. The public `POST /auth/forgot` {email} always answers 202 at once and mails a 1-hour reset link afterwards only if the account exists, so neither the answer nor its timing reveals accounts; it is limited to 5 an hour per IP and per address. The public `POST /auth/link` {token} says what a link is for. `POST /auth/redeem` {token, password} uses it once: an invite creates the login in the inviter's tenant, a reset sets the password and sets `users.sessions_valid_after`. The middleware checks every user session through `auth_session_ok` (the user exists and the JWT's `iat` is not older than the last reset), so a reset ends every older session; dev tokens skip the check. The web links are `/davet#<token>` and `/sifre#<token>`, with the token in the fragment, out of server logs. Screens: Kullanıcılar (`/kullanicilar`) and `/sifremi-unuttum`. `cmd/resetlink` (owner role) prints a reset link when mail is down. There are no roles yet: every user may invite.
- `cmd/api`, `cmd/devtoken`, `cmd/migrate`, `cmd/onboard`, `cmd/resetlink`. `scripts/smoke.sh` is the end-to-end check (expects `"caught_minor":21412`).

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
- `withholding_codes` is seeded from GİB "UBL-TR Kod Listeleri" V1.31 (2023). Code 619 is 70% (7/10, KDVGUT; V1.28–V1.31 printed "75/10" as a typo, fixed by V1.43). Verified unchanged against V1.43 (2026-11-04).

## Current status (2026-11-17)

- **Merged to `main`:**
  - Faz 1: PR #1, merge commit `0ceeca3`.
  - Faz 2a, PDF extraction: PR #2, merge commit `aebc5d1`.
  - Faz 2b, D16: PR #3, merge commit `94e56e8`. Its smoke also fixed a Faz 1 bug: invoices without despatch refs failed to audit.
  - Faz 2c, D17: PR #4, merge commit `797b4bb`.
  - Faz 3 (3a login, 3b SPA, 3c dashboard/vendors/seed, 3d contract PDF viewer): PR #5, merge commit `82aa524`.
  - Faz 3e, invoice search/filters/paging: PR #6, merge commit `06bee0d`.
  - Faz 3f, decision history: PR #7, merge commit `7a0d469`.
  - Faz 3g, bulk decisions: PR #8, merge commit `2f2892e`.
  - Faz 3h, alias management: PR #9, merge commit `6138aa0`.
  - Faz 3i, landing page on `/giris`: PR #10, merge commit `40739bc`.
  - Docs status: PR #11, merge commit `1d45df2`.
  - Faz 3j/3k, landing split from login and self sign-up: PR #12, merge commit `c3b068c`.
  - Faz 4 prep, the provider-agnostic production setup: PR #13, squash commit `42a57a7`. It contains:
  - `Dockerfile`: a single distroless image with api, migrate, onboard and the built SPA.
  - `deploy/compose.prod.yml`: Postgres, a one-shot migrate, the API, Caddy for TLS, and AI behind `--profile ai`.
  - `scripts/backup.sh` and `scripts/restore-check.sh`.
  - `docs/runbook.md`.
  - `TRUST_PROXY`.

  The whole stack was verified locally with `DOMAIN=localhost`: TLS redirect, HSTS, onboard, login, an upload written to storage, the sign-up limit holding against forged `X-Forwarded-For`, and backup → restore-check.
  - Faz 5a, user invites and password reset: PR #14, merge commit `1cb87c5`.
  - `FX_RATE_DEVIATION` rule (D18): PR #15, merge commit `764d3ae`.
  - Code comments rewritten in plain Turkish: PR #16, merge commit `4e85f47`.
- **CI:** `.github/workflows/ci.yml` runs on every PR and on `main`: Go (gofmt, vet, tests with the integration tests against a compose Postgres), web (Vitest + build) and ai-service (pytest; the OCR test skips without tesseract).
- **Next steps, in order:**
  1. Finish Faz 4 once the user picks the provider, the domain and the off-host backup target (TR region, encrypted at rest): provision the host, DNS, the first deploy following the runbook, the backup cron and the off-host sync.
  - Faz 4 also picks the SMTP provider (TR region, KVKK) for `MAIL_URL`; until then production has no invites or "Şifremi unuttum".
  - Optional, not requested yet: a compact mobile nav (7 menu items overflow on phones), a log of who removed an alias, a contact/pricing section on the landing page.
  - KVKK applies throughout (see the rulings below), and an LLM never computes amounts.
- **Merging** is the user's call (`gh pr merge N --merge --delete-branch`). Claude merges only when the user explicitly asks (as with #11/#12 on 2026-11-10).
- **Local demo:** run `cmd/onboard` once, then `scripts/seed-demo.py`, and serve with `WEB_DIR=../web/dist COOKIE_SECURE=false go run ./cmd/api`.
- **Working mode:** Claude acts as Staff Architect / Principal Engineer. It decides technical and domain questions itself, records them as rulings and reports in Turkish. It does not hand these questions back to the user.
- **Housekeeping:** delete each feature branch, locally and on GitHub, after its PR merges (all earlier ones are gone as of 2026-11-17).

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
  - The `go` directive is 1.26.0 (pgx needs 1.25; `golang.org/x/crypto`, added for login in Faz 3a, raised it to 1.26).
- **Known open minors:**
  - Summed allowance and tax totals are not bounded.
  - A post-commit audit failure leaves the invoice PENDING; recover with `POST /invoices/{id}/audit`.
- **Architect rulings (2026-11-03):**
  - Code 619 = 70.00 is verified against KDVGUT I/C-2.1.3.3.2.
  - GİB withholding list re-checked (2026-11-04) against "UBL-TR Kod Listeleri" V1.43 (27.07.2026, live 14.09.2026, from `UBLTR_1.2.1_Kilavuzlar.zip` on ebelge.gib.gov.tr). All 52 codes and rates match the V1.31 seed, and the V1.32–V1.43 changelog has no withholding entry, so no migration was needed. V1.43 also fixes the 619 typo (now 7/10). Future GİB changes still ship as a new migration, never as an edit to `002`.
  - KVKK: all personal and commercial data stays in Türkiye, either on a self-hosted or a TR-region Postgres plus storage, with no cross-border transfer by default. In Faz 2, documents must not go to a foreign LLM API unless there is a KVKK Art. 9 transfer basis (a standard contract filed with the Board) and the customer opts in. The default is local extraction.
  - Delivery model (2026-11-03): matchwatch is a multi-tenant cloud SaaS, not a desktop app. All tenants share one Postgres and are isolated by RLS. It runs in a TR-region cloud (per KVKK above). "Local extraction" means the Faz 2 service runs on our own TR infrastructure, not on the customer's machine. Faz 3 ships as a Vue SPA served from the same infrastructure. `cmd/devtoken` is dev-only; real login and tenant onboarding are a Faz 3 prerequisite.

## Conventions

- Integration tests read `MATCHWATCH_TEST_DATABASE_URL` (app role) and `MATCHWATCH_TEST_MIGRATE_URL` (owner role). They `t.Skip` when these are unset, and they must be re-runnable against the same DB: use unique tenants and ETTNs per run.
- Commit messages must not include `Co-Authored-By: Claude` or any Claude attribution.
