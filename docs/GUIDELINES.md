# Project Guidelines

These rules apply to human contributors and coding agents.

## Product Boundaries

- Keep KoalaBye 100% free, open source, and self-hostable.
- Do not add billing, plans, subscriptions, payments, upgrade prompts, cloud-only assumptions, or hidden monetization.
- Treat quotas only as safety limits for abuse prevention and accidental overload. Trusted users may receive higher limits manually without payment.
- Do not add mandatory email, SaaS, analytics, or other third-party dependencies without explicit project approval.
- Do not introduce React, Next.js, Vue, an npm build pipeline, or external CDNs without explicit approval.
- Prefer server-rendered HTML, small HTMX enhancements, simple Go code, and minimal dependencies.

## Privacy and Security

- Store no IP addresses in the database.
- Do not fingerprint users or retain raw user-agent strings by default.
- Public survey pages must not set session, language, or tracking cookies. Language changes use `?lang=`.
- Make collected data transparent and avoid dark patterns or guilt-inducing uninstall copy.
- Treat tenancy boundaries, role checks, session handling, and public submissions as security-critical.
- Check every permission server-side. Hidden links are not authorization.
- Deny access by default when identity, role, or tenancy is uncertain.
- Never store raw invite codes. Show a newly generated invite only once and store its hash.
- Preserve at least one owner per organization and one active Instance Owner per instance.
- Preserve at least one explicit owner per campaign. Organization owners and admins retain documented implicit campaign-owner access.
- Install-token processing must use HMAC-SHA256 with the instance secret and must never store, render, audit, or log raw tokens.
- Store only referrer hostnames and coarse documented browser/OS families when their campaign settings permit it.
- Keep public pages functional without JavaScript and free of external assets or analytics.
- Store form content as plain text and rely on escaped rendering; never permit raw HTML or custom JavaScript.
- Validate public answers against active fields and options, cap request bodies, and silently avoid storage for honeypot hits.
- Response contents require organization membership plus an owner, editor, or analyst campaign role. Instance Owner status alone is not private-response access.
- Analytics and exports inherit the private-response membership boundary. Never expose answer contents or exports through global Instance Owner moderation access alone.
- Built-in charts must be local, accessible, and based only on minimized stored data. Keep an HTML table fallback.
- Exports must omit internal IDs, IP addresses, raw user agents, raw install tokens, and install-token hash values. Audit format and counts, not content.
- Retention and manual deletion actions are owner-only, CSRF-protected, transactional, and permanently destructive.

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
- Audit sensitive Instance Owner overrides, status changes, role changes, and safety-limit changes.
- Campaign routes use public IDs, enforce organization-scoped slugs, and keep archived or disabled records counted against safety limits.
- Public visit limits use UTC month boundaries. Raw visits and first-seen token visits are distinct counters.
- Public submission limits use UTC month boundaries. Submission rows contain no IP, raw user agent, or raw install token.
- Analytics date ranges and daily grouping use UTC. Metadata summaries render only when the corresponding collection setting is enabled.
- Do not add external CDNs, mandatory email, billing, analytics, or external services unless explicitly requested.
