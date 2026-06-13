# Operations

KoalaBye is one Go service with one SQLite database. It does not require email, a queue, object storage, an external database, or analytics services.

Source builds use Go 1.26.4. Run `make check` before deployment, or use `go run ./cmd/devcheck` on Windows and other systems without GNU Make.

## First Deployment

1. Copy `.env.example` to a protected environment file.
2. Set `KOALABYE_BASE_URL` to the exact public HTTPS origin.
3. Generate a long random `KOALABYE_SECRET`, for example with `openssl rand -base64 48`.
4. Set `KOALABYE_SECURE_COOKIES=true`.
5. Keep `KOALABYE_REGISTRATION_ENABLED=false` for a private instance.
6. Start KoalaBye and Caddy with the production Compose example.
7. Open `/setup`; the first account becomes the Instance Owner. There are no default credentials.
8. Review privacy and legal-notice pages whenever hosting, jurisdiction, or the non-commercial project status changes.

## Routine Checks

- `GET /healthz` checks that the process can reach SQLite.
- `GET /version` reports only version, commit, build date, and Go version.
- Use `docker compose logs -f koalabye` to watch startup, migration, and request errors.
- Instance Owners can adjust organization safety limits under Instance Admin.
- Review campaign privacy settings and public collection notices after upgrades.
- Treat exported answers and URL context as sensitive operator data even though KoalaBye minimizes collection.

## Backups and Restores

Use `scripts/backup-sqlite.sh DATABASE_PATH BACKUP_PATH` when `sqlite3` is installed. It uses SQLite's online backup command and verifies the result. If `sqlite3` is unavailable, stop KoalaBye before copying the database; do not copy only the main file during active WAL writes.

Use `scripts/restore-sqlite.sh BACKUP_PATH RESTORE_PATH` only while KoalaBye is stopped. It refuses to overwrite an existing database and verifies integrity first. Follow [BACKUP_RESTORE.md](BACKUP_RESTORE.md) for the full drill.

## Upgrades

1. Back up and verify the current database.
2. Test the upgrade on a copy first.
3. Deploy the new image.
4. Watch logs while automatic migrations run.
5. Verify `/healthz`, `/version`, login, a private campaign, and a public submission.

Never edit a migration that has shipped. Add a new forward migration for every schema change.

Balanced diagnostics are per campaign. Do not add personal data to URL parameters, reverse-proxy logs, or external analytics around KoalaBye. The application itself never stores raw query strings, but operators must review proxy access-log configuration separately.

Docker image builds require Docker Desktop or another running Docker daemon. If no local daemon is available, use the CI Docker job as the image-build gate. A release tag must wait for a successful Docker build somewhere.

## Secret Rotation

Changing `KOALABYE_SECRET` invalidates existing CSRF signatures and can disrupt session continuity. It also changes the HMAC used for optional install-token hashes, so the same client token will no longer match its earlier unique-visit identity. Rotate only as a planned security event, expect users to sign in again, and document the analytics continuity boundary.
