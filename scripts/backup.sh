#!/usr/bin/env bash
# Postgres'i (pg_dump -Fc) ve belge deposunu $BACKUP_DIR/<UTC zamanı>/ altına yedekler, sonra $KEEP_DAYS günden eski
# yedekleri siler. Repo kök dizininden çalıştır (cron için docs/runbook.md'ye bak). Klasörü sunucunun dışına kopyalamak
# bundan sonraki, sağlayıcıya göre değişen adım; hedef Türkiye'de olmalı ve veriyi şifreli saklamalı (KVKK).
set -euo pipefail
BACKUP_DIR=${BACKUP_DIR:-/srv/matchwatch/backups}
STORAGE_PATH=${STORAGE_PATH:-/srv/matchwatch/storage}
KEEP_DAYS=${KEEP_DAYS:-14}
COMPOSE=${COMPOSE:-docker compose -f deploy/compose.prod.yml --env-file deploy/.env}

out="$BACKUP_DIR/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$out.partial"
# Önce veritabanı: dökümden sonra yazılan dosyalar zararsız fazlalıktır; tersi olsaydı dosyası olmayan satırlar kalırdı.
$COMPOSE exec -T postgres pg_dump -U postgres -d matchwatch -Fc > "$out.partial/db.dump"
tar -C "$STORAGE_PATH" -cz . > "$out.partial/storage.tar.gz"
(cd "$out.partial" && sha256sum db.dump storage.tar.gz > SHA256SUMS)
mv "$out.partial" "$out" # adında .partial olmayan klasör tamamlanmış demektir

find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name '*Z' -mtime +"$KEEP_DAYS" -exec rm -rf {} +
echo "backup: $out ($(du -sh "$out" | cut -f1))"
