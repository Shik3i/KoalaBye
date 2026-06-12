# First Deployment Test

You can test on any temporary domain or local host before pointing real users at the instance.

## Recommended Configuration

- Start with `docker-compose.example.yml`.
- Copy `.env.example` to `.env`.
- Set `KOALABYE_BASE_URL=https://bye.koalastuff.net` (or your actual domain).
- Set `KOALABYE_SECURE_COOKIES=true` behind HTTPS.
- Keep public registration disabled and use invitation-based access.

For local HTTP-only testing, use `KOALABYE_SECURE_COOKIES=false`. Do not carry that value into HTTPS production.

## Acceptance Drill

1. Complete `/setup` on a clean database.
2. Create a test organization and campaign.
3. Configure and activate a feedback form.
4. Set a browser extension uninstall URL to the campaign with an optional random `t` token.
5. Verify duplicate token visits count once as unique.
6. Submit anonymous feedback and verify the response inbox and analytics.
7. Export CSV and JSON and confirm no raw token or token hash is present.
8. Run a backup and restore into a fresh path or volume.
9. Start against the restored database and verify `/healthz`, `/version`, and test data.
10. Check key pages at 360px in English, German, and Spanish.

Legal pages are placeholders and Spanish intentionally falls back to English. Replace the legal content before a public production deployment.
