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
