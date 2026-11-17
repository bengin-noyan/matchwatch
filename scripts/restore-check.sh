#!/usr/bin/env bash
# Bir yedeğin gerçekten geri yüklenebildiğini kanıtlar: checksum'lar, depo arşivi ve canlıdakiyle birebir aynı kurulmuş
# (aynı init betiği) geçici bir Postgres'e pg_restore. Sonra canlı sistemle karşılaştırmak için satır sayılarını
# yazdırır.
#   bash scripts/restore-check.sh /srv/matchwatch/backups/20261110T020000Z
set -euo pipefail
dir=${1:?usage: restore-check.sh <backup folder>}
dir=$(cd "$dir" && pwd)
root=$(cd "$(dirname "$0")/.." && pwd)
name=matchwatch-restore-check-$$

(cd "$dir" && sha256sum -c --quiet SHA256SUMS)
files=$(tar -tz < "$dir/storage.tar.gz" | grep -c '\.\(xml\|pdf\)$' || true)

trap 'docker rm -f "$name" >/dev/null 2>&1 || true' EXIT
# MSYS_NO_PATHCONV: yoksa Windows'taki Git Bash konteyner içindeki yolları değiştirir.
MSYS_NO_PATHCONV=1 docker run -d --name "$name" -e POSTGRES_PASSWORD=check \
  -e MATCHWATCH_OWNER_PASSWORD=check -e MATCHWATCH_APP_PASSWORD=check \
  -v "$root/deploy/postgres:/docker-entrypoint-initdb.d:ro" pgvector/pgvector:pg16 >/dev/null
# Entrypoint, init betiklerinden sonra Postgres'i yeniden başlatır; geçici sunucuyu değil, son sunucuyu bekle.
for _ in $(seq 60); do
  docker logs "$name" 2>&1 | grep -q 'PostgreSQL init process complete' &&
    docker exec "$name" pg_isready -q -h 127.0.0.1 -U postgres && break
  sleep 1
done

docker exec -i "$name" pg_restore -U postgres -d matchwatch --exit-on-error < "$dir/db.dump"
q() { docker exec "$name" psql -U postgres -d matchwatch -tAc "$1" | tail -1; }
echo "migrations: $(q 'SELECT count(*) FROM schema_migrations')"
echo "tenants:    $(q 'SELECT count(*) FROM tenants')"
echo "invoices:   $(q 'SELECT count(*) FROM invoices')"
echo "findings:   $(q 'SELECT count(*) FROM audit_findings')"
echo "documents in storage: $files"
echo "restore-check: OK"
