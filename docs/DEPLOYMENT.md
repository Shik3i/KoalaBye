# Deployment

KoalaBye is a single Go service backed by SQLite. It does not require email, Redis, a queue, object storage, an external database, analytics, fonts, or other hosted services.

## Docker Compose

Use `docker-compose.staging.example.yml` for a source-built staging deployment and `docker-compose.production.example.yml` for a pinned release image. Both include Caddy, HTTPS-oriented cookie settings, persistent `/data`, restart policies, and healthchecks.

1. Copy the example configuration:

   ```bash
   cp .env.example .env
   ```

2. Generate a production secret and set it in `.env`:

   ```bash
   openssl rand -base64 48
   ```

3. Set the public HTTPS URL and secure cookies:

   ```dotenv
   KOALABYE_BASE_URL=https://bye.example.com
   KOALABYE_SECURE_COOKIES=true
   ```

4. Start the service:

   ```bash
   docker compose -f docker-compose.example.yml up -d --build
   ```

5. Open `https://bye.example.com/setup`. There are no default credentials. The first account becomes the Instance Owner.

The Compose example mounts the named `koalabye_data` volume at `/data`. The SQLite database is `/data/koalabye.db`.

## Caddy

The included `Caddyfile.example` expects Caddy to share the `koalabye` Docker network:

```caddyfile
bye.example.com {
	encode zstd gzip
	reverse_proxy koalabye:8080
}
```

Caddy obtains and renews HTTPS certificates when DNS and ports 80/443 are configured correctly. Keep `KOALABYE_BASE_URL` equal to the public origin and set `KOALABYE_SECURE_COOKIES=true` behind HTTPS.

## Environment Variables

| Variable | Production guidance |
| --- | --- |
| `KOALABYE_BASE_URL` | Exact public HTTPS origin |
| `KOALABYE_LISTEN_ADDR` | Keep `:8080` in the container |
| `KOALABYE_DATABASE_PATH` | Keep `/data/koalabye.db` |
| `KOALABYE_SECRET` | Required random secret, at least 32 characters |
| `KOALABYE_MODE` | Use `selfhost` |
| `KOALABYE_SECURE_COOKIES` | Set `true` behind HTTPS |
| `KOALABYE_INSTANCE_NAME` | Name shown in the UI |
| `KOALABYE_REGISTRATION_ENABLED` | Usually `false` for a private instance |
| `KOALABYE_INVITE_ONLY` | Usually `true` |
| `KOALABYE_INVITE_REGISTRATION_ENABLED` | Allows account creation from valid invite codes |
| `KOALABYE_DEFAULT_MAX_*` | Initial abuse-prevention limits |

Optional `KOALABYE_BOOTSTRAP_ADMIN_*` variables create the first owner only when none exists. Prefer interactive `/setup` unless automated provisioning is required.

## File Permissions

The image runs as the non-root `koalabye` user. `/data` must be writable by that user. With a bind mount, create the directory first and grant access to the container user rather than running the application as root. Keep the database and environment file readable only by the service operator; `chmod 600 .env` is a sensible default.

## Healthcheck

`GET /healthz` returns `200 OK` only when the application can reach SQLite. The Docker image includes a healthcheck against `http://127.0.0.1:8080/healthz`.

`GET /version` returns non-sensitive build metadata. Compare its version and commit to the intended deployment after every upgrade.

## Legal Pages

The bundled privacy and imprint pages are explicitly placeholders, available in English and German with an English fallback for Spanish. They are not final legal advice or deploy-ready legal text. Replace them before a public production launch.

## Backups

Back up the SQLite database and the deployment configuration needed to reproduce the instance. Follow [BACKUP_RESTORE.md](BACKUP_RESTORE.md). Test restores regularly; an untested backup is not release readiness.

## Updating

1. Create and test a backup.
2. Pull the desired source revision or image.
3. Rebuild and restart:

   ```bash
   docker compose -f docker-compose.example.yml up -d --build
   ```

4. Watch the service logs for migration errors.
5. Verify `/healthz`, login, one private campaign, and one public feedback page.

Database migrations run automatically at startup. Do not skip backups before an update.

