# Architecture

## System Shape

```text
Browser -> Caddy (TLS/reverse proxy) -> KoalaBye Go process -> SQLite file
```

KoalaBye is one stateless HTTP process except for its SQLite database and in-memory login limiter. Migrations and browser assets are embedded in the binary. Caddy is recommended for TLS and forwarding, but the app has no hard dependency on it.

## Technology Choices

Go provides a small deployment artifact, predictable resource use, strong concurrency primitives, and an approachable standard library. SQLite keeps self-hosting operationally small while WAL mode and a busy timeout support the expected workload. Server-rendered templ components keep the security boundary on the server. HTMX is vendored locally for future progressive enhancement.

## Data and Tenancy

Users are global identities. Organizations are tenant boundaries, connected through `organization_members`. Organization roles are `owner`, `admin`, `member`, and `viewer`. Instance roles are separate and global; only `instance_owner` is active in the current UI. Prepared roles include admin, moderator, and support.

Future campaigns belong to organizations. Campaign permissions must be derived from active membership and checked in handlers, not inferred from URLs or navigation visibility.

## First-Run Setup

On every relevant request, the app checks for an active Instance Owner. With none, `/` and login redirect to `/setup`. Setup creates the user, owner role, default organization, owner membership, initial settings, and audit event in one transaction. Once an owner exists, setup redirects to login. Optional environment bootstrap follows the same transaction and never overwrites data.

## Authentication

Passwords use Argon2id with per-password random salts. Sessions use 256-bit random bearer tokens; only SHA-256 hashes are stored. The cookie is HttpOnly, SameSite=Lax, scoped to `/`, and optionally Secure. Logout revokes the row. CSRF uses a signed, HttpOnly, SameSite=Strict cookie matched against a hidden form value.

The in-memory login limiter is intentionally small and username-keyed so it does not persist IP addresses. A distributed public deployment will need a privacy-preserving shared limiter.

## Permissions and Audit

Authentication middleware loads identity. Authorization remains explicit in protected handlers through the permissions service and denies by default. The audit log records setup, bootstrap, login success/failure, and logout without passwords or session tokens.

## Privacy Model

The foundation stores account and organization data only. It has no analytics SDK, external browser asset, IP column, user-agent column, or fingerprinting code. Future public pages must remain cookie-free and collect only fields declared by the campaign owner.

## Planned Modules

- Campaigns and campaign roles
- Form builder
- Public uninstall and survey pages
- Privacy-preserving visit counting
- Submission storage and retention controls
- Aggregate analytics and exports
- Invite codes and registration workflows
- Passkeys
- Optional email integration

These modules should extend organization ownership and the permission service rather than create parallel tenancy models.
