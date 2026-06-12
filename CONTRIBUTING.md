# Contributing

KoalaBye welcomes focused contributions that preserve its privacy-first, self-hostable scope.

## Development

Install Go 1.26.4, then:

```bash
cp .env.example .env
export KOALABYE_DATABASE_PATH=./data/koalabye.db
make dev
```

Before submitting:

```bash
make fmt
make check
```

`make check` performs a non-mutating formatting check, verifies generated templ and sqlc output, runs all tests, and runs `go vet`. CI runs the same command and builds the Docker image.

## Database Changes

Add a new sequential file under `migrations/`; never rewrite an applied migration. Add or update SQL under `queries/`, then run `make sqlc`. Keep multi-step security-sensitive writes transactional.

## Code Style

Use ordinary Go conventions, narrow packages, explicit errors, and small interfaces only where they reduce coupling. Prefer the standard library and existing dependencies. Keep templates accessible, responsive, and functional without client-side JavaScript.

Every change should be reviewed for data minimization, tenant isolation, authorization, CSRF, session impact, and accidental external network requests. Update architecture and security documentation whenever their claims or boundaries change.

Organization actions must preserve role boundaries and the final-owner invariant. Instance Owner overrides, status changes, settings changes, and safety-limit changes require audit events. Invite implementations must store only hashes and must not introduce email as a requirement.

Campaign changes must preserve organization tenancy, explicit-owner invariants, documented implicit organization access, and lifecycle transition rules. Campaign slugs are organization-scoped; URLs use public IDs. Privacy changes must never add IP storage, fingerprinting, or raw install-token persistence.

Public campaign changes must remain cookie-free, work without JavaScript, and avoid external assets. Hash optional install tokens with HMAC-SHA256 and the instance secret, keep tokens out of responses and logs, reduce referrers to hostnames, and discard raw user agents after coarse classification. Treat raw and first-seen-token visits as separate counters, and use UTC month boundaries for visit safety limits.

Form and submission changes must keep field content plain text, preserve answer snapshots, validate values server-side, and enforce request-size and UTC monthly submission limits. Never add IP, raw user-agent, or raw-token columns. Public honeypot hits must not write data. Response access requires actual organization membership; do not inherit the general Instance Owner campaign shortcut for private response contents.

Analytics must aggregate existing minimized data with UTC boundaries and remain understandable through tables even when charts are present. Do not add external analytics or chart assets. Archived field summaries must use stored public IDs and label snapshots.

Exports are sensitive reads. Keep their permission boundary aligned with responses, audit every completed export without answer contents, and expose no internal IDs or token hashes. CSV changes must use `encoding/csv`; JSON changes must preserve typed values and a documented format version.

Retention and manual deletion are hard-delete paths. Restrict them to campaign owners, require CSRF and typed confirmation where specified, keep multi-table behavior transactional, preserve submissions when visits are deleted through `ON DELETE SET NULL`, and audit counts without deleted content.

KoalaBye is 100% free forever. Safety limits are operational abuse-prevention controls, not plans. Do not add billing, subscription, payment, premium, or upgrade concepts to code, copy, schema, or documentation.

## Translations

All visible strings belong in the dotted-key JSON catalogs under `internal/i18n/locales/`. English, German, and Spanish must retain exact key parity. Add keys before adding a page or handler message, use natural language rather than literal machine translation, and test locale routing when behavior changes. Legal pages currently require English and German only.

New handlers must have explicit authentication and permission decisions. New tables require migrations and tests; new queries require sqlc regeneration. Security-sensitive work and permission changes must include both success and denial tests.
