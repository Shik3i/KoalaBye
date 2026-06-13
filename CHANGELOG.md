# Changelog

## v0.1.3 - 2026-06-13

### Added
- transparent setting-driven public collection notices
- opt-in allowlisted URL context with sanitized export support
- structured privacy documentation and operator legal placeholders
- shared flag-backed language controls for English, German, and Spanish
- responsive authenticated navigation and clearer instance administration shortcuts
- campaign preview and copy controls for public links and browser snippets

### Changed
- improved dark-theme contrast, responsive layout, status badges, and form-builder clarity
- expanded README quick start, project badges, privacy summary, and asset attribution

### Privacy
- partial responses, text drafts, browser major version, device class, and UTC offset remain uncollected
- raw query strings, unknown URL parameters, IP addresses, raw user agents, and raw install tokens remain unstored
- flag artwork and UI behavior are served locally without external asset requests

## v0.1.0 - Experimental initial release

### Added
- setup without default credentials
- username/password auth
- organizations and invite codes
- instance admin and safety limits
- campaigns and roles
- public campaign links
- anonymous visit counting
- install-token HMAC hashing
- form builder
- submissions
- response inbox
- analytics
- CSV/JSON exports
- retention/manual deletion
- i18n EN/DE/ES
- Docker/selfhost docs
- no email requirement
- no billing/payment code
- no external CDN

### Security
- Initial experimental release

### Privacy
- Initial experimental release

### Operations
- Initial experimental release

### Known limitations
- retention is manual, no scheduler yet
- legal pages are placeholders and must be replaced before public production
- no email/password reset
- no passkeys/OAuth yet
- no custom domains
- no automated backup scheduler
- Docker build must pass in CI/release environment
