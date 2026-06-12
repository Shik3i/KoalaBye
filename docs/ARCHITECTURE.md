# Architecture

## System Shape

```text
Browser -> Caddy (TLS/reverse proxy) -> KoalaBye Go process -> SQLite file
```

KoalaBye is one stateless HTTP process except for its SQLite database and in-memory login limiter. Migrations and browser assets are embedded in the binary. Caddy is recommended for TLS and forwarding, but the app has no hard dependency on it.

## Technology Choices

Go provides a small deployment artifact, predictable resource use, strong concurrency primitives, and an approachable standard library. SQLite keeps self-hosting operationally small while WAL mode and a busy timeout support the expected workload. Server-rendered templ components keep the security boundary on the server. HTMX is vendored locally for future progressive enhancement.

## Internationalization

Translation catalogs are flat dotted-key JSON files embedded from `internal/i18n/locales/`. Startup validates that German and Spanish contain exactly the English baseline keys. Requests receive locale context before authentication and rendering.

Resolution order is explicit `?lang=xx`, future authenticated user preference, the `koalabye_lang` cookie, `Accept-Language`, then English. Explicit selection writes a SameSite=Lax cookie for one year. Templates resolve all visible strings through the request catalog and set `<html lang>` correctly. Unsupported locales and missing translations fall back safely to English; a completely unknown key renders a visible marker instead of crashing.

Legal routes are intentionally narrower: `/legal/privacy` and `/legal/imprint` support English and German. A Spanish request renders English with a visible availability note. Future public survey pages must avoid setting locale cookies unless the visitor explicitly changes language.

## Data and Tenancy

Users are global identities. Organizations are tenant boundaries, connected through `organization_members`. Organization roles are `owner`, `admin`, `member`, and `viewer`. Instance roles are separate and global; only `instance_owner` is active in the current UI. Prepared roles include admin, moderator, and support.

Organization URLs use random public IDs rather than integer primary keys. Owners control all organization settings and owner memberships. Admins can manage non-owner members and invite codes. Members and viewers have read access in this phase. An organization cannot lose its final owner. Disabled users and organizations are denied normal access.

Future campaigns belong to organizations. Campaign permissions must be derived from active membership and checked in handlers, not inferred from URLs or navigation visibility.

## Registration and Invites

Username/password registration is controlled by persisted instance settings. Public registration, invite-only mode, and invite-based registration are separate switches. Email remains optional; there is no SMTP, verification, or password-reset dependency.

Manual invite codes are long random bearer values. Only their SHA-256 hashes are stored. The raw value is rendered once after creation. Acceptance checks revocation, expiry, maximum uses, existing membership, and the organization's member limit in a transaction.

## Safety Limits

Instance defaults seed per-organization limits. Organization and active-invite limits are enforced now; member limits are enforced when invites are accepted. Campaign, visit, and submission limits are stored for the future modules that own those operations. These are abuse-prevention controls for free instances, never product plans or monetization boundaries.

Instance Owners can adjust defaults and per-organization limits. Sensitive overrides, status changes, settings changes, and organization membership administration produce audit events.

## First-Run Setup

On every relevant request, the app checks for an active Instance Owner. With none, `/` and login redirect to `/setup`. Setup creates the user, owner role, default organization, owner membership, initial settings, and audit event in one transaction. Once an owner exists, setup redirects to login. Optional environment bootstrap follows the same transaction and never overwrites data.

## Authentication

Passwords use Argon2id with per-password random salts. Sessions use 256-bit random bearer tokens; only SHA-256 hashes are stored. The cookie is HttpOnly, SameSite=Lax, scoped to `/`, and optionally Secure. Logout revokes the row. CSRF uses a signed, HttpOnly, SameSite=Strict cookie matched against a hidden form value.

The in-memory login limiter is intentionally small and username-keyed so it does not persist IP addresses. A distributed public deployment will need a privacy-preserving shared limiter.

## Quality Gates

Go tests exercise authentication, session revocation, permissions, locale resolution, HTTP headers/assets, migrations, and core queries. `make check` verifies formatting, generated templ/sqlc output, tests, and vet. GitHub Actions runs the same gate on pushes and pull requests, then builds the production Docker image.

## Permissions and Audit

Authentication middleware loads identity. Authorization remains explicit in protected handlers through the permissions service and denies by default. The audit log records setup, bootstrap, login success/failure, logout, instance settings and limit changes, user and organization status changes, and sensitive membership administration without passwords, session tokens, or raw invite codes.

## Privacy Model

The foundation stores account and organization data only. It has no analytics SDK, external browser asset, IP column, user-agent column, or fingerprinting code. Future public pages must remain cookie-free and collect only fields declared by the campaign owner.

## Planned Modules

- Campaigns and campaign roles
- Form builder
- Public uninstall and survey pages
- Privacy-preserving visit counting
- Submission storage and retention controls
- Aggregate analytics and exports
- Passkeys
- Optional email integration

These modules should extend organization ownership and the permission service rather than create parallel tenancy models.
