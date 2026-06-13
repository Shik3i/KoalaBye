# Release Checklist

## Code and Tests

- [ ] Confirm `go version` reports Go 1.26.4.
- [ ] Run `go fmt ./...`.
- [ ] Run `go test ./...`.
- [ ] Run `go vet ./...`.
- [ ] Run `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`.
- [ ] Run `make check`.
- [ ] If GNU Make is unavailable, run `go run ./cmd/devcheck`.
- [ ] Run `git diff --check`.
- [ ] Build the Docker image.
- [ ] Confirm Docker Desktop or another Docker daemon is running before a local build.
- [ ] If Docker is unavailable locally, confirm the CI Docker job passes before tagging.
- [ ] Verify `/version` matches the intended version, commit, and build date.
- [ ] Confirm the working tree contains only intended changes.

## Database and Upgrade

- [ ] Review new migrations for forward-only safety.
- [ ] Start from an empty database and complete `/setup`.
- [ ] Start with a copy of the previous release database and verify automatic migration.
- [ ] Restart after migration and confirm already-applied migrations are idempotent.
- [ ] Confirm backup and restore instructions still match the image and volume layout.

## Product QA

- [ ] Log out and log in again.
- [ ] Create or open an organization.
- [ ] Create, configure, activate, and publicly test a campaign.
- [ ] Verify duplicate optional tokens count once as unique.
- [ ] Submit feedback and verify the thank-you page, inbox, and analytics.
- [ ] Verify CSV and JSON exports contain no raw token or token hash.
- [ ] Verify retention and permanent deletion require authorization and confirmation.
- [ ] Check setup, app, campaign, analytics, and public pages at 360px width.
- [ ] Check English, German, and Spanish rendering for important pages.

## Privacy and Operations

- [ ] Verify there are no external CDN, font, analytics, or script requests.
- [ ] Verify public pages store no IP address, raw user agent, or raw install token.
- [ ] Verify public pages always disclose enabled diagnostics and set no cookies.
- [ ] Verify URL context accepts only allowlisted keys and never stores the raw query string.
- [ ] Verify partial responses and text drafts are not saved before final submission.
- [ ] Search for missing i18n markers and legal-compliance overclaims.
- [ ] Verify HTTPS deployment uses `KOALABYE_SECURE_COOKIES=true`.
- [ ] Verify HTTPS deployment uses `KOALABYE_SECURE_COOKIES=true`.
- [ ] Verify `/healthz` and the Docker healthcheck.
- [ ] Test a SQLite backup and restore.
- [ ] Start against the restored database and verify expected sample data.
- [ ] Review deployment, security, and release documentation.
- [ ] Review `docs/PRIVACY.md` and remaining operator legal placeholders.

## Publish

- [ ] **CRITICAL**: Obtain explicit permission and approval from the user before committing, tagging, or pushing any release.
- [ ] Update release notes.
- [ ] Confirm legal placeholders have been replaced for any public production deployment.
- [ ] Tag the release.
- [ ] Publish a container image when the registry process is ready.
- [ ] Record known limitations, especially manual-only retention deletion.

## Release Candidate Record

- [ ] Ensure working tree clean.
- [ ] Run local checks.
- [ ] Ensure `govulncheck` clean.
- [ ] Ensure Docker build passes locally or in CI.
- [ ] Ensure legal placeholders are acceptable for staging.
- [ ] Backup current DB if upgrading.
- [ ] Create tag only after CI green.
- [ ] Verify release workflow.
- [ ] Deploy to staging.
- [ ] Run end-to-end smoke test.
- [ ] Only then consider production.
- [ ] Record the commit SHA and working-tree status.
- [ ] Record test, vet, formatting, generated-code, vulnerability-scan, and Docker results.
- [ ] Record clean-install, migration, and backup/restore drill results.
- [ ] Record remaining blockers and the person accepting each known limitation.
