#!/usr/bin/env bash
# Backs up Postgres (pg_dump -Fc) and the document storage into $BACKUP_DIR/<UTC time>/, then drops backups older
# than $KEEP_DAYS days. Run from the repo root (cron: see docs/runbook.md). Copying the folder off the host is the
# provider-specific step that follows; the target must be in Türkiye and encrypted at rest (KVKK).
set -euo pipefail
BACKUP_DIR=${BACKUP_DIR:-/srv/matchwatch/backups}
STORAGE_PATH=${STORAGE_PATH:-/srv/matchwatch/storage}
KEEP_DAYS=${KEEP_DAYS:-14}
COMPOSE=${COMPOSE:-docker compose -f deploy/compose.prod.yml --env-file deploy/.env}

out="$BACKUP_DIR/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$out.partial"
# Database first: files written after the dump are harmless extras; the reverse would leave rows without files.
$COMPOSE exec -T postgres pg_dump -U postgres -d matchwatch -Fc > "$out.partial/db.dump"
tar -C "$STORAGE_PATH" -cz . > "$out.partial/storage.tar.gz"
(cd "$out.partial" && sha256sum db.dump storage.tar.gz > SHA256SUMS)
mv "$out.partial" "$out" # a folder without .partial is complete

find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name '*Z' -mtime +"$KEEP_DAYS" -exec rm -rf {} +
echo "backup: $out ($(du -sh "$out" | cut -f1))"
