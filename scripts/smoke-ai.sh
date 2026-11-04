#!/usr/bin/env bash
# End-to-end AI smoke: real Postgres, Ollama + ai-service containers, real API binary, PDF -> draft -> approved contract.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

set -a
. ./.env.example
[ -f .env ] && . ./.env
set +a

tmp="$(mktemp -d)"
if command -v cygpath >/dev/null; then tmp="$(cygpath -m "$tmp")"; fi
api_pid=""
cleanup() { [ -n "$api_pid" ] && kill "$api_pid" 2>/dev/null || true; rm -rf "$tmp"; }
trap cleanup EXIT

# CPU inference is slow: generous timeouts.
export LLM_TIMEOUT_S=900 EXTRACT_DEADLINE_S=870 AI_EXTRACT_TIMEOUT=15m AI_SERVICE_URL=http://127.0.0.1:8090
port="${SMOKE_PORT:-18089}"
export HTTP_ADDR="127.0.0.1:$port" STORAGE_DIR="$tmp/storage"
base="http://127.0.0.1:$port"

docker compose --profile ai up -d --build
docker compose exec -T ollama ollama pull "$LLM_MODEL"
for i in $(seq 1 60); do
  curl -sSf http://127.0.0.1:8090/healthz >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { docker compose logs ai >&2; echo "ai-service did not start" >&2; exit 1; }
  sleep 2
done

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

pdf="$root/ai-service/tests/fixtures/contract_text.pdf"
if command -v cygpath >/dev/null; then pdf="$(cygpath -m "$pdf")"; fi # native curl needs a Windows path
code="$(curl -sS --max-time 1200 -o "$tmp/draft.json" -w '%{http_code}' -X POST "$base/api/v1/contract-drafts" \
  -H "Authorization: Bearer $token" -F "file=@$pdf;type=application/pdf")"
[ "$code" = 201 ] || { echo "upload -> $code" >&2; cat "$tmp/draft.json" >&2; exit 1; }

# Prints status/contract_no/warnings, writes the approval body ($tmp/contract.json), fails on missing required fields.
python -X utf8 -I - "$tmp/draft.json" "$tmp/contract.json" <<'PY'
import json, sys
r = json.load(open(sys.argv[1], encoding="utf-8"))
d = r.get("draft") or {}
print("status:", r.get("status"), "contract_no:", d.get("contract_no"))
print("warnings:", [w.get("code") for w in r.get("warnings") or []])
print(json.dumps(d, ensure_ascii=False, indent=1))
if r.get("status") != "READY" or d.get("contract_no") != "SZL-2026-014":
    sys.exit("smoke-ai FAIL: draft not READY / contract_no != SZL-2026-014")
if d.get("qty_tolerance_pct") is None:
    d["qty_tolerance_pct"] = "0"
missing = [k for k in ("vendor_vkn", "vendor_name", "valid_from", "valid_until", "currency") if not d.get(k)]
missing += [f"items[{i}].unit_price" for i, it in enumerate(d.get("items") or []) if it.get("unit_price") is None]
if not d.get("items"):
    missing.append("items")
if missing:
    sys.exit("smoke-ai FAIL: draft lacks required fields: " + ", ".join(missing))
d["draft_id"] = r["id"]
json.dump(d, open(sys.argv[2], "w", encoding="utf-8"), ensure_ascii=False)
PY

code="$(curl -sS -o "$tmp/out" -w '%{http_code}' -X POST "$base/api/v1/contracts" -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" --data-binary "@$tmp/contract.json")"
[ "$code" = 201 ] || { echo "POST /contracts -> $code" >&2; cat "$tmp/out" >&2; exit 1; }
id="$(python -X utf8 -I -c "import json,sys;print(json.load(open(sys.argv[1]))['id'])" "$tmp/draft.json")"
st="$(curl -sSf "$base/api/v1/contract-drafts/$id" -H "Authorization: Bearer $token" | python -X utf8 -I -c "import json,sys;print(json.load(sys.stdin)['status'])")"
[ "$st" = APPROVED ] || { echo "draft status $st, want APPROVED" >&2; exit 1; }
echo "smoke-ai OK"
