# KoalaBye

KoalaBye is a privacy-focused, 100% free, open-source, self-hostable platform for uninstall feedback and lightweight anonymous surveys. It is designed for browser extensions, apps, and small developer tools that need honest feedback without tracking people.

> **Status:** early foundation. Authentication, first-run setup, organizations, permissions, audit logging, and deployment packaging are present. Campaigns and public survey pages are intentionally not implemented yet.

## Principles

- 100% free software under the MIT License.
- Self-hosting is first-class; the same codebase can operate a public multi-user instance.
- No external CDNs, fonts, analytics, fingerprinting, or mandatory third-party services.
- No IP address storage in the database and no raw user-agent storage by default.
- Email is optional and is not required for the MVP.
- The official KoalaStuff instance, if offered, is intended to remain 100% free forever.
- Safety limits exist only to prevent abuse and accidental overload. They are not paid tiers.

## Stack

Go, chi, templ, SQLite, sqlc, goose, server-rendered HTML, and locally vendored HTMX. The application is a single binary with embedded migrations and static assets.

## Languages

The application UI supports English (`en`), German (`de`), and Spanish (`es`). English is the default and fallback language. Locale selection follows `?lang=xx`, the `koalabye_lang` cookie, `Accept-Language`, then English. The language switcher preserves the current path.

Legal placeholders under `/legal/privacy` and `/legal/imprint` currently support English and German only. Spanish requests clearly fall back to English.

## Docker Quick Start

```bash
cp .env.example .env
# Replace KOALABYE_SECRET in .env with: openssl rand -base64 48
docker compose -f docker-compose.example.yml up --build
```

Open <http://localhost:8080/setup>. The first account becomes the global Instance Owner and receives a default organization. There are **no default admin credentials**.

For HTTPS, place Caddy on the `koalabye` Docker network and adapt `Caddyfile.example`. Set the public base URL and `KOALABYE_SECURE_COOKIES=true`.

## Local Development

Requirements: Go 1.24+ and Docker only if testing the image.

```bash
cp .env.example .env
mkdir -p data
export KOALABYE_DATABASE_PATH=./data/koalabye.db
export KOALABYE_SECRET=change-me-long-random-secret
make dev
```

The insecure example secret is accepted only for a local HTTP URL. Run `make check` before submitting changes. It checks Go and templ formatting, generated templ/sqlc code, tests, and vet. `make sqlc` regenerates typed queries.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `KOALABYE_BASE_URL` | `http://localhost:8080` | Public origin |
| `KOALABYE_LISTEN_ADDR` | `:8080` | HTTP listen address |
| `KOALABYE_DATABASE_PATH` | `./data/koalabye.db` | SQLite file |
| `KOALABYE_SECRET` | insecure dev value | CSRF signing secret; required and strong in production |
| `KOALABYE_MODE` | `selfhost` | `selfhost` or `cloud` |
| `KOALABYE_REGISTRATION_ENABLED` | `false` | Global registration policy |
| `KOALABYE_INVITE_ONLY` | `true` | Invitation policy |
| `KOALABYE_INVITE_REGISTRATION_ENABLED` | `true` | Allow account creation through valid invites |
| `KOALABYE_SECURE_COOKIES` | `false` | Require HTTPS cookies |
| `KOALABYE_INSTANCE_NAME` | `KoalaBye` | Display name |

The `KOALABYE_DEFAULT_MAX_*` variables in `.env.example` seed abuse-prevention limits for organizations, members, active invites, and future campaigns, visits, and submissions. Instance Owners can raise organization limits manually. There is no billing, subscription, payment, or upgrade path.

## Organizations and Invites

Users may belong to multiple organizations with `owner`, `admin`, `member`, or `viewer` roles. Owners and admins manage members and create manual invite codes; only owners may change owner memberships. Every organization must retain at least one owner.

Invite codes require no email. KoalaBye stores only a hash, displays the raw code once after creation, and enforces its role, expiry, use count, member limit, and revocation state. Instance Owners manage users, organizations, registration settings, safety limits, and audit events under `/instance`.

The optional bootstrap admin variables may create the first owner only when no owner exists. They never overwrite users and the password is never logged.

## Roadmap

The next layers are campaigns, a form builder, cookie-free public uninstall pages, privacy-preserving visit counts, response storage, exports, and aggregate analytics. Passkeys and optional email may follow. Billing, paid tiers, payments, and hidden monetization are permanently out of scope.

See [Architecture](docs/ARCHITECTURE.md), [Guidelines](docs/GUIDELINES.md), [Security](SECURITY.md), and [Contributing](CONTRIBUTING.md).
