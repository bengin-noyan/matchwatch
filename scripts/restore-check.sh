#!/usr/bin/env bash
# Proves a backup restores: checksums, the storage archive, and a pg_restore into a throwaway Postgres that is
# set up exactly like production (same init script), then prints row counts to compare with the live system.
#   bash scripts/restore-check.sh /srv/matchwatch/backups/20261110T020000Z
set -euo pipefail
dir=${1:?usage: restore-check.sh <backup folder>}
dir=$(cd "$dir" && pwd)
root=$(cd "$(dirname "$0")/.." && pwd)
name=matchwatch-restore-check-$$

(cd "$dir" && sha256sum -c --quiet SHA256SUMS)
files=$(tar -tz < "$dir/storage.tar.gz" | grep -c '\.\(xml\|pdf\)$' || true)

trap 'docker rm -f "$name" >/dev/null 2>&1 || true' EXIT
# MSYS_NO_PATHCONV: Git Bash on Windows would otherwise rewrite the container paths.
MSYS_NO_PATHCONV=1 docker run -d --name "$name" -e POSTGRES_PASSWORD=check \
  -e MATCHWATCH_OWNER_PASSWORD=check -e MATCHWATCH_APP_PASSWORD=check \
  -v "$root/deploy/postgres:/docker-entrypoint-initdb.d:ro" pgvector/pgvector:pg16 >/dev/null
# The entrypoint restarts Postgres after the init scripts; wait for the final server, not the temporary one.
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
