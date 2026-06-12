# Project Guidelines

These rules apply to human contributors and coding agents.

## Product Boundaries

- Keep KoalaBye 100% free, open source, and self-hostable.
- Do not add billing, paid tiers, cloud-only assumptions, or hidden monetization.
- Do not add mandatory email, SaaS, analytics, or other third-party dependencies without explicit project approval.
- Do not introduce React, Next.js, Vue, an npm build pipeline, or external CDNs without explicit approval.
- Prefer server-rendered HTML, small HTMX enhancements, simple Go code, and minimal dependencies.

## Privacy and Security

- Store no IP addresses in the database.
- Do not fingerprint users or retain raw user-agent strings by default.
- Future public survey pages must not set cookies.
- Make collected data transparent and avoid dark patterns or guilt-inducing uninstall copy.
- Treat tenancy boundaries, role checks, session handling, and public submissions as security-critical.
- Check every permission server-side. Hidden links are not authorization.
- Deny access by default when identity, role, or tenancy is uncertain.

## Engineering

- Keep the Docker image and memory footprint small.
- Use sqlc queries for normal database access and goose migrations for every schema change.
- Never edit an applied migration; add a new numbered migration.
- Keep packages aligned with product ownership boundaries.
- Add focused tests for security-sensitive behavior.
- Document architectural or privacy changes in `docs/ARCHITECTURE.md`, `SECURITY.md`, or both.
- Vendor browser assets locally and record their version.

## Internationalization

- Every user-facing string, including titles, navigation, forms, validation errors, flash messages, empty states, and accessibility labels, must use an i18n key.
- Do not hardcode user-facing text in templates or handlers.
- English is the baseline and fallback locale. The supported UI locales are English (`en`), German (`de`), and Spanish (`es`).
- Every new UI feature must add natural translations for all supported UI locales.
- Locale files use flat dotted keys under `internal/i18n/locales/`. Keep every locale in exact key parity with English.
- Legal pages initially support only English and German. Spanish legal-page requests must visibly fall back to English.
- Future public survey and uninstall pages remain cookie-free by default. A language cookie is acceptable only after an explicit language choice.

## Testing

- Every feature needs focused automated tests.
- Security-sensitive code, permission checks, locale selection, and migration behavior require tests.
- Test both allowed and denied authorization paths.
- Test translation fallback and locale-file parity when changing i18n behavior.
- Run `make check` before committing.

## Accessibility Baseline

- Use real labels for every form field and semantic HTML landmarks.
- Keep all controls keyboard-accessible with visible focus states.
- Maintain sufficient text and control contrast.
- Associate errors with fields where practical and expose page-level errors with appropriate roles.
- Render the selected locale in the document's `<html lang>` attribute.

## Contributor and Agent Checklist

- Before adding a page, add its translation keys in every supported UI locale.
- Before adding a handler, decide and test its authentication and permission requirements.
- Before adding a table, add a goose migration and migration tests.
- Before adding a query, update sqlc and commit the generated output.
- Before changing architecture, update `docs/ARCHITECTURE.md`.
- Do not add external CDNs, mandatory email, billing, analytics, or external services unless explicitly requested.
