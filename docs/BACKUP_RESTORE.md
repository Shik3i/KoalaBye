# Backup and Restore

## What to Back Up

- The SQLite database, normally `/data/koalabye.db`.
- The `.env` file or equivalent secret/configuration store.
- The Compose and reverse-proxy configuration used by the deployment.

Protect backups as sensitive data. They can contain account information, free-text feedback, and optional sanitized campaign diagnostics or URL context.

## Safe SQLite Backups

When `sqlite3` is installed, the repository helper performs an online-safe backup and integrity check:

```bash
scripts/backup-sqlite.sh /data/koalabye.db backups/koalabye-$(date +%F-%H%M%S).db
```

The safest simple method is to stop writes before copying the database:

```bash
mkdir -p backups
docker compose -f docker-compose.example.yml stop koalabye
docker compose -f docker-compose.example.yml cp \
  koalabye:/data/koalabye.db \
  backups/koalabye-$(date +%F-%H%M%S).db
docker compose -f docker-compose.example.yml start koalabye
```

For online backups, use SQLite's backup API or a trusted tool such as `sqlite3 .backup`; do not copy only the main database while writes are active.

SQLite may create `koalabye.db-wal` and `koalabye.db-shm`. A raw filesystem copy taken during active writes must capture a consistent set of these files. Stopping the container or using the SQLite backup API avoids that ambiguity.

Validate a backup before accepting it:

```bash
sqlite3 backups/koalabye-YYYY-MM-DD-HHMMSS.db "PRAGMA integrity_check;"
```

The expected result is `ok`.

## Restore

1. Stop KoalaBye.
2. Preserve the current `/data` contents separately.
3. Replace `/data/koalabye.db` with the verified backup.
4. Remove stale `koalabye.db-wal` and `koalabye.db-shm` files only while KoalaBye is stopped.
5. Restore the matching environment and proxy configuration.
6. Ensure the database is writable by the non-root container user.
7. Start KoalaBye and inspect logs for migration or permission errors.
8. Verify `/healthz`, login, organizations, campaigns, responses, and a public campaign page.

For a restore into a fresh path:

```bash
scripts/restore-sqlite.sh backups/koalabye-YYYY-MM-DD-HHMMSS.db /data/koalabye-restored.db
```

Run a restore drill on a separate volume or test host before relying on the procedure in production. Never test a restore by overwriting the only production copy.
