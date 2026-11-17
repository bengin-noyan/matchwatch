#!/usr/bin/env bash
# Uçtan uca duman testi: gerçek Postgres, gerçek API programı, her şeyin yolunda gittiği akış. make gerekmez.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

# Makefile'daki öncelik sırası: .env, .env.example'ın üstüne yazar.
set -a
. ./.env.example
[ -f .env ] && . ./.env
set +a

tmp="$(mktemp -d)"
if command -v cygpath >/dev/null; then tmp="$(cygpath -m "$tmp")"; fi # Windows'taki Go, Windows yolu ister
api_pid=""
cleanup() { [ -n "$api_pid" ] && kill "$api_pid" 2>/dev/null || true; rm -rf "$tmp"; }
trap cleanup EXIT

port="${SMOKE_PORT:-18089}"
export HTTP_ADDR="127.0.0.1:$port" STORAGE_DIR="$tmp/storage"
base="http://127.0.0.1:$port"
fix="$root/backend-go/testdata"

docker compose up -d
cd backend-go
for i in $(seq 1 60); do
  go run ./cmd/migrate >"$tmp/migrate.log" 2>&1 && break
  [ "$i" = 60 ] && { cat "$tmp/migrate.log"; echo "migrate failed" >&2; exit 1; }
  sleep 2
done

token="$(go run ./cmd/devtoken -name Demo -vkn 9876543210 -rate 0.45 | tail -n 1)"
go build -o "$tmp/api" ./cmd/api
"$tmp/api" >"$tmp/api.log" 2>&1 &
api_pid=$!
for i in $(seq 1 30); do
  curl -sSf "$base/healthz" >/dev/null 2>&1 && break
  [ "$i" = 30 ] && { cat "$tmp/api.log"; echo "API did not start" >&2; exit 1; }
  sleep 1
done

# post <yol> <content-type> <dosya>: cevap 2xx değilse cevabın gövdesiyle hata verir.
post() {
  local code
  code="$(curl -sS -o "$tmp/out" -w '%{http_code}' -X POST "$base$1" \
    -H "Authorization: Bearer $token" -H "Content-Type: $2" --data-binary "@$3")"
  case "$code" in 2??) ;; *) echo "POST $1 -> $code" >&2; cat "$tmp/out" >&2; exit 1 ;; esac
  echo "POST $1 -> $code"
}
post /api/v1/contracts application/json "$fix/contract_basic.json"
post /api/v1/despatches application/xml "$fix/despatch_basic.xml"
post /api/v1/receipts application/xml "$fix/receipt_basic.xml"
post /api/v1/invoices application/xml "$fix/invoice_basic.xml"

summary="$(curl -sSf "$base/api/v1/summary" -H "Authorization: Bearer $token")"
echo "$summary"
case "$summary" in *'"caught_minor":21412'*) echo "SMOKE OK" ;; *) echo "SMOKE FAIL: caught_minor 21412 not found" >&2; exit 1 ;; esac
