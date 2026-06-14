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

Resolution order in the authenticated application is explicit `?lang=xx`, future authenticated user preference, the `koalabye_lang` cookie, `Accept-Language`, then English. Explicit selection writes a SameSite=Lax cookie for one year. Public campaign pages instead use the campaign default and an explicit `?lang=en|de|es` override without writing a cookie. Templates resolve all visible strings through the request catalog and set `<html lang>` correctly. Unsupported locales and missing translations fall back safely to English; a completely unknown key renders a visible marker instead of crashing.

Legal routes are intentionally narrower: `/legal/privacy` and `/legal/imprint` support English and German. A Spanish request renders English with a visible availability note.

## Data and Tenancy

Users are global identities. Organizations are tenant boundaries, connected through `organization_members`. Organization roles are `owner`, `admin`, `member`, and `viewer`. Instance roles are separate and global; only `instance_owner` is active in the current UI. Prepared roles include admin, moderator, and support.

Organization URLs use random public IDs rather than integer primary keys. Owners control all organization settings and owner memberships. Admins can manage non-owner members and invite codes. Members and viewers have read access in this phase. An organization cannot lose its final owner. Disabled users and organizations are denied normal access.

Campaigns belong to exactly one organization and use random public IDs in management URLs. Slugs are unique only within their organization. All campaigns, including archived and instance-disabled campaigns, count toward the organization's campaign safety limit because the MVP does not delete them.

Organization owners and admins receive implicit campaign-owner rights. Explicit campaign roles are `owner`, `editor`, `analyst`, and `viewer`; only existing organization members may receive them. Creation adds the creator as an explicit campaign owner, and the final explicit owner cannot be removed. Permission functions distinguish viewing, basic editing, privacy changes, access management, and archival and deny by default.

Campaign status transitions are `draft -> active`, `active -> paused`, `paused -> active`, and any non-archived state to `archived`. Archived campaigns are read-only. Instance-disabled campaigns remain visible to authorized users but reject normal writes.

## Registration and Invites

Username/password registration is controlled by persisted instance settings. Public registration, invite-only mode, and invite-based registration are separate switches. Email remains optional; there is no SMTP, verification, or password-reset dependency.

Manual invite codes are long random bearer values. Only their SHA-256 hashes are stored. The raw value is rendered once after creation. Acceptance checks revocation, expiry, maximum uses, existing membership, and the organization's member limit in a transaction.

## Safety Limits

Instance defaults seed per-organization limits. Organization, member, active-invite, campaign, monthly visit, and monthly submission limits are enforced. Visit and submission months use UTC boundaries. These are abuse-prevention controls for free instances, never product plans or monetization boundaries.

Instance Owners can adjust defaults and per-organization limits. Sensitive overrides, status changes, settings changes, and organization membership administration produce audit events.

## Campaign Privacy and Links

Each campaign has a privacy-settings row from creation. Strict privacy disables optional coarse referrer, browser, operating-system, and URL-context data. Balanced enables those coarse fields but never IP storage or fingerprinting. Public pages work without mandatory JavaScript, load only local assets, require no session, set no cookies, and always disclose enabled collection.

Canonical public links are `/c/{campaignPublicID}`. A readable `/u/{orgSlug}/{campaignSlug}` form resolves to the same page. Only active, public-link-enabled campaigns in enabled organizations are available; every other state returns a generic response.

Visits store a raw-count flag and a first-seen-token-count flag separately. Optional opaque install tokens are limited to 256 characters, HMAC-SHA256 hashed with `KOALABYE_SECRET`, and never persisted or rendered raw. Requests with the same campaign and token hash inside a 30-minute window reuse the original visit instead of storing or counting another row. Across longer periods, the token can produce additional raw visits but still counts as unique only once. Tokenless requests cannot be deduplicated without adding a cookie or another visitor identifier, so they remain anonymous page views. Referrers are reduced to lowercase hostnames. User agents are reduced to documented browser (`Chrome`, `Firefox`, `Safari`, `Edge`, `Other`, `Unknown`) and OS (`Windows`, `macOS`, `Linux`, `Android`, `iOS`, `Other`, `Unknown`) families; raw user agents are discarded.

Optional URL context is extracted from a fixed allowlist and stored as a small JSON object. Values are length- and character-limited, unknown keys are ignored, and the raw query string is never stored. Context can flow to authorized submission exports only through the existing visit link.

## Forms and Submissions

Campaign forms are ordered rows in `campaign_form_fields`, with type-specific JSON limited to plain text-block bodies and textarea lengths. Checkbox and radio choices live in `campaign_form_options`. Fields and options are soft-archived; there is no raw HTML, conditional logic, multi-page state, upload, or custom JavaScript.

Public submissions are capped at 128 KiB and validated against the current active form. Unknown fields are ignored. Required values, active option membership, ratings from 1 through 5, and textarea limits are enforced server-side. A hidden honeypot returns the same thank-you page without writing a submission.

Partial response collection is not implemented. No field value is stored before final submission. The documented future boundary is an explicit `structured_only` mode for validated checkbox, radio, and rating values; hidden text-draft autosave remains outside the current architecture.

`campaign_submissions` stores a public ID, campaign, optional visit link, optional copied HMAC install-token hash, and UTC timestamp. It has no IP or user-agent columns. Answers store field public ID, type, label snapshot, and JSON value so later form edits do not erase historical meaning. Templ escapes all labels and free text during rendering.

Owners and editors edit forms. Owners, editors, and analysts read response contents; viewers and non-members cannot. Instance Owner moderation rights do not imply private response access: response handlers require actual organization membership and a response-capable campaign role.

## Analytics and Exports

Analytics query the existing visit, submission, answer, field, and option tables directly. There is no tracking SDK or secondary analytics store. Preset ranges are 7, 30, 90 days, and all time; finite ranges begin at UTC midnight and daily trends group ISO timestamps by UTC date.

Overview totals remain campaign-wide while trends and field summaries honor the selected range. Field summaries parse stored JSON values: ratings show average and counts 1-5, radio and checkbox fields show option counts and percentages, and textareas show only answer counts with a link to the inbox. Archived fields remain visible when historical answers reference their public ID and label snapshot.

Optional referrer, browser, and OS lists render only when their campaign collection setting is enabled. Charts are inline SVG with an HTML table fallback and local CSS; no chart library or external asset is used.

CSV exports create one column per historical answer field using its public ID and a sanitized label. Checkbox arrays are semicolon-separated and standard CSV quoting protects commas, quotes, and newlines. JSON export version 1 preserves typed answer values and field snapshots. Both formats expose only public identifiers and the boolean presence of an install-token hash. Export audit metadata contains format, campaign public ID, and approximate submission count, never answers.

Analytics, responses, and exports require actual organization membership plus campaign owner, editor, or analyst access. Viewer and non-member access is denied. Instance Owner status alone does not grant private analytics or export access.

## Retention and Deletion

Campaign settings can enable a 30, 90, 180, or 365 day retention threshold. This phase deliberately has no background scheduler. A campaign owner manually deletes visits and submissions older than the UTC cutoff in one transaction. Answers cascade with deleted submissions; deleting a visit sets surviving submission links to `NULL`.

Campaign owners can also hard-delete every response or every visit after typed campaign-slug confirmation. These actions are CSRF-protected and audited with counts but no deleted content. Analysts, editors, viewers, non-members, and Instance Owners without organization access cannot run deletion actions.

## First-Run Setup

On every relevant request, the app checks for an active Instance Owner. With none, `/` and login redirect to `/setup`. Setup creates the user, owner role, default organization, owner membership, initial settings, and audit event in one transaction. Once an owner exists, setup redirects to login. Optional environment bootstrap follows the same transaction and never overwrites data.

## Authentication

Passwords use Argon2id with per-password random salts. Sessions use 256-bit random bearer tokens; only SHA-256 hashes are stored. The cookie is HttpOnly, SameSite=Lax, scoped to `/`, and optionally Secure. Logout revokes the row. CSRF uses a signed, HttpOnly, SameSite=Strict cookie matched against a hidden form value.

The in-memory login limiter is intentionally small and username-keyed so it does not persist IP addresses. A distributed public deployment will need a privacy-preserving shared limiter.

## Quality Gates

Go tests exercise authentication, session revocation, permissions, locale resolution, HTTP headers/assets, migrations, and core queries. `make check` verifies formatting, generated templ/sqlc output, tests, and vet. GitHub Actions runs the same gate on pushes and pull requests, then builds the production Docker image.

## Permissions and Audit

Authentication middleware loads identity. Authorization remains explicit in protected handlers through the permissions service and denies by default. The audit log records setup, authentication, instance settings and limits, user/organization/campaign moderation, campaign creation and lifecycle, privacy changes, and membership/access administration without passwords, session tokens, raw invite codes, or future install tokens.

## Privacy Model

KoalaBye has no analytics SDK, external browser asset, IP column, raw user-agent column, raw-query column, or fingerprinting code. Public pages are cookie-free, disclose enabled collection, and collect only fields enabled by the campaign owner. Campaign analytics expose aggregates and local charts, never visitor profiles.

## Planned Modules

- Optional retention scheduling
- Additional privacy-friendly aggregate views
- Passkeys
- Optional email integration

These modules should extend organization ownership and the permission service rather than create parallel tenancy models.
