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
make test
make vet
```

## Database Changes

Add a new sequential file under `migrations/`; never rewrite an applied migration. Add or update SQL under `queries/`, then run `make sqlc`. Keep multi-step security-sensitive writes transactional.

## Code Style

Use ordinary Go conventions, narrow packages, explicit errors, and small interfaces only where they reduce coupling. Prefer the standard library and existing dependencies. Keep templates accessible, responsive, and functional without client-side JavaScript.

Every change should be reviewed for data minimization, tenant isolation, authorization, CSRF, session impact, and accidental external network requests. Update architecture and security documentation whenever their claims or boundaries change.
