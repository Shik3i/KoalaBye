# Pre-v0.1.0 Audit Report

**Date:** 2026-06-12  
**Commit Audited:** `550f49c34d2c2c960ad7864163a2b903f1269bf5`  
**Checks Run:** `go fmt`, `go test`, `go vet`, `go run ./cmd/devcheck`, `docker build`, `git diff --check`, `govulncheck`  
**Verdict:** Ready for v0.1.0 with minor polishes applied.

## Issues Found

- **Blocker:** 0
- **High:** 0
- **Medium:** 0
- **Low:** 4
  - Formatting error in `cmd/smoke/main.go`
  - Legal placeholder copy was confusingly final.
  - Native dark/light mode compatibility was missing in CSS.
  - Verification of GitHub Actions push prevention on branches.

## Fixes Applied

- **Legal Text:** Updated `en.json` and `de.json` to explicitly label the legal pages as placeholders intended for self-hosted/test deployments.
- **Theme/CSS:** Added `color-scheme: light dark;` to `app.css` to allow browser-native dark mode compatibility without adding a heavy theme engine.
- **Go Formatting:** Ran `go fmt` on `cmd/smoke/main.go`.
- **Workflow Verification:** Confirmed that `ci.yml` strictly builds without pushing, while `release.yml` correctly targets only `v*.*.*` tags for publishing.

## Ideas for Post-v0.1.0

- True theme toggle (persisted choice) and more robust design presets.
- Full automated data retention jobs (background workers).
- Adding real generated legal texts or cookie banner configurations for public hosted instances.

## Release Readiness Checklist

- [x] Code formatting and linting pass
- [x] Tests pass
- [x] CSP and Privacy Headers are present and correct
- [x] Referrer-Policy is `no-referrer`
- [x] Release workflows are secure and publish correctly
- [x] Unsafe inline scripts/styles removed
- [x] No huge DB schema redesign necessary (indexes suffice for now)
