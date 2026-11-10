# matchwatch production runbook

One Linux host in a TR region (KVKK: data, backups and the AI models stay in Türkiye). All commands run from the
repo checkout on the server, for example `/srv/matchwatch/app`. Shorthand used below:

```bash
alias dc='docker compose -f deploy/compose.prod.yml --env-file deploy/.env'
```

Still open, chosen by the owner: the cloud provider, the domain, and the off-host backup target.

## 1. First deploy

1. Host: Docker Engine with the compose plugin. Firewall: open 22 (admin IPs only), 80 and 443. Nothing else;
   Postgres and the API publish no ports.
2. DNS: an A/AAAA record for the domain pointing at the host. Caddy needs it before it can get a certificate.
3. Directories. The image runs as uid 65532:
   ```bash
   sudo mkdir -p /srv/matchwatch/storage /srv/matchwatch/backups
   sudo chown 65532:65532 /srv/matchwatch/storage
   ```
4. Secrets: `cp deploy/.env.prod.example deploy/.env && chmod 600 deploy/.env`, then fill it in. Generate each
   password with `openssl rand -hex 32`. Keep them hex, because they go into database URLs unescaped.
5. Start: `dc up -d --build`. The `migrate` service runs once and exits. `api` starts only if it succeeded, and
   Caddy fetches the certificate.
6. Check: `curl https://<domain>/healthz` returns `{"status":"ok"}`, and http:// redirects to https://.
7. First customer: either they sign up at `/kayit`, or you open the tenant yourself (with a financing rate):
   ```bash
   printf '%s\n' "$PASSWORD" | dc run --rm -T --entrypoint /app/onboard api \
     -name "Firma A.Ş." -vkn 1234567890 -rate 0.45 -email kisi@firma.com.tr
   ```

### With AI (contract extraction, item suggestions)

Set `AI_SERVICE_URL=http://ai:8090` and an `AI_SERVICE_TOKEN` of at least 32 characters in `deploy/.env`, then:

```bash
dc --profile ai up -d --build
dc --profile ai exec ollama ollama pull qwen2.5:7b-instruct
dc --profile ai exec ollama ollama pull bge-m3
```

The models take about 6 GB on disk. Plan for at least 16 GB RAM, or a GPU host, for acceptable extraction times.

## 2. Update

```bash
git pull
dc up -d --build    # rebuilds the image; migrate runs again (applied files are skipped), then api restarts
```

Migrations only move forward. Take a backup before an update that adds a migration (section 3).

## 3. Backups

`scripts/backup.sh` writes `db.dump` (pg_dump -Fc), `storage.tar.gz` (raw XML and contract PDFs) and `SHA256SUMS`
into `/srv/matchwatch/backups/<UTC time>/`, and keeps 14 days (`KEEP_DAYS`). A folder still named `*.partial` is
from a failed run: delete it. Nightly cron, as a user in the `docker` group:

```cron
30 2 * * * cd /srv/matchwatch/app && bash scripts/backup.sh >> /srv/matchwatch/backups/backup.log 2>&1
```

Off-host copy (provider-specific, still open): after the backup, sync the folder to object storage in a TR region
with encryption at rest. A backup that only lives on the same disk is not a backup.

**Restore check, at least monthly and after every restore-relevant change:**

```bash
bash scripts/restore-check.sh /srv/matchwatch/backups/<newest>
```

It verifies the checksums, restores into a throwaway Postgres set up like production, and prints row counts.
Compare them with the live system:

```bash
dc exec postgres psql -U postgres -d matchwatch -tAc \
  "select (select count(*) from tenants),(select count(*) from invoices),(select count(*) from audit_findings)"
```

## 4. Restore (disaster)

```bash
dc stop api caddy
dc down -v                       # deletes the database volume
dc up -d postgres                # empty database; the init script creates the roles again
dc exec -T postgres pg_restore -U postgres -d matchwatch --exit-on-error < /srv/matchwatch/backups/<t>/db.dump
sudo tar -C /srv/matchwatch/storage -xzf /srv/matchwatch/backups/<t>/storage.tar.gz
sudo chown -R 65532:65532 /srv/matchwatch/storage
dc up -d
```

`down -v` also deletes Caddy's certificate volume. Caddy requests a new certificate, but Let's Encrypt limits how
many it issues for the same domain per week, so do not run this repeatedly.

## 5. Operations

- Logs: `dc logs -f api`. Each container keeps at most 5 × 20 MB of logs.
- Invoice stuck in PENDING after an audit error: `POST /api/v1/invoices/{id}/audit`.
- Rotate `JWT_SECRET`: edit `deploy/.env`, then `dc up -d api`. Every user is logged out.
- Rotate a database password: run `ALTER ROLE matchwatch_app PASSWORD '...'` (or `matchwatch_owner`) through
  `dc exec postgres psql -U postgres`, update `deploy/.env`, then `dc up -d`. The `*_PASSWORD` values in `.env` are
  only read by the init script on an empty volume.
- Sign-up limit (5 per IP per hour) and login lockout are in-process. A restart resets them, and they assume a
  single `api` container.
