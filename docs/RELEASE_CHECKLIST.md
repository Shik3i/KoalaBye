# Release Checklist

## Code and Tests

- [ ] Run `go fmt ./...`.
- [ ] Run `go test ./...`.
- [ ] Run `go vet ./...`.
- [ ] Run `make check`.
- [ ] Run `git diff --check`.
- [ ] Build the Docker image.
- [ ] Confirm the working tree contains only intended changes.

## Database and Upgrade

- [ ] Review new migrations for forward-only safety.
- [ ] Start from an empty database and complete `/setup`.
- [ ] Start with a copy of the previous release database and verify automatic migration.
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
- [ ] Verify HTTPS deployment uses `KOALABYE_SECURE_COOKIES=true`.
- [ ] Verify `/healthz` and the Docker healthcheck.
- [ ] Test a SQLite backup and restore.
- [ ] Review deployment, security, and release documentation.

## Publish

- [ ] Update release notes.
- [ ] Tag the release.
- [ ] Publish a container image when the registry process is ready.
- [ ] Record known limitations, especially manual-only retention deletion.

