# Contributing

KoalaBye welcomes focused contributions that preserve its privacy-first, self-hostable scope.

## Development

Install Go 1.24+, then:

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

## Translations

All visible strings belong in the dotted-key JSON catalogs under `internal/i18n/locales/`. English, German, and Spanish must retain exact key parity. Add keys before adding a page or handler message, use natural language rather than literal machine translation, and test locale routing when behavior changes. Legal pages currently require English and German only.

New handlers must have explicit authentication and permission decisions. New tables require migrations and tests; new queries require sqlc regeneration. Security-sensitive work and permission changes must include both success and denial tests.
