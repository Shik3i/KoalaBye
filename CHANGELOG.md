# Changelog

## v0.4.3 - 2026-06-18

### Security
- Login rate limiter no longer trusts `X-Forwarded-For`/`X-Real-IP` unless the
  peer is in the new `KOALABYE_TRUSTED_PROXIES` allowlist, closing a brute-force
  bypass when the app is exposed directly (removed unconditional `RealIP` middleware)
- Login attempt map is now swept of expired entries, preventing unbounded
  memory growth from failed logins and rotated IPs
- Added per-IP fixed-window rate limiting to public submission endpoints
  (10/minute) to prevent flooding and denial-of-feedback abuse

### Performance
- Active-session `last_seen_at` is now written at most once per 10 minutes
  instead of on every authenticated request, reducing SQLite write contention

## v0.4.0 - 2026-06-16

### Design System
- Added CSS custom properties for typography scale (`--text-xs` through `--text-4xl`)
- Added CSS custom properties for spacing system (`--space-xs` through `--space-xl`)
- Replaced hardcoded font-size values with semantic CSS variables throughout
- Created reusable icon component library (`icons.templ`): Sun, Moon, EyeOpen, EyeClosed, GitHub, Info, Close
- Unified icon rendering across theme toggle, password toggle, and footer
- Consolidated `.compact-button` duplicate CSS rules

### Features
- Added server-side Toast/Flash API: `web.SetFlashCookie()`, `FlashMiddleware`, `FlashToasts` component
- Added Custom CSS field to Campaign Branding (migration 00013)
- Campaign owners can now inject custom CSS into public campaign pages
- Campaign public pages render custom CSS via `<style>` tag in `<head>`
- Added HSTS header (`Strict-Transport-Security`) to security headers
- Added `Cache-Control: public, max-age=31536000, immutable` for static assets

### Security
- Changed `KOALABYE_SECURE_COOKIES` default from `false` to `true`
- Updated `.env.example` to reflect secure defaults
- Decoupled `isLocalDevelopment()` from `SecureCookies` flag

### Housekeeping
- Removed 22 unused CSS classes (icon, drag-handle, command-dialog, etc.)
- Fixed `.skip-link` to use `:focus-visible` alongside `:focus`
- Added `will-change: transform` to hover-animated cards
- Added all i18n keys for custom CSS in EN, DE, ES

## v0.3.1 - 2026-06-15

### UI & Design
- Fixed `.grid.four` CSS bug (rendered 3 columns instead of 4)
- Added disabled form input styling (`opacity: .55`)
- Added skeleton shimmer animation for `[aria-busy]` loading states
- Added password show/hide toggle on login form
- Added pause-on-hover for toast notifications
- Added `<hr>` theme styling and removed duplicate CSS rules
- Added password-wrapper CSS for toggle button layout
- Consolidated inline styles into CSS utility classes

### QoL
- Added `docs/audits/` to `.gitignore` to prevent audit bloat

## v0.2.10 - 2026-06-14

### Added
- Professional favicon set, web app manifest, and reproducible favicon generation script.
- Configurable public contact email for the privacy and imprint pages.
- Shared responsive footer with project, legal, support, repository, and build links.

### Changed
- Polished the legal pages and localized footer content in English, German, and Spanish.
- Improved footer consistency and restored a clearly visible light/dark mode toggle icon.
- Documented production, privacy, operations, and release requirements more precisely.

### Privacy
- Deduplicate repeated campaign visits for the same privacy-preserving install-token hash within 30 minutes.
- Continue to store no IP addresses, raw user agents, unknown URL parameters, or other direct identifiers.
- Reuse the original public visit identifier for duplicate requests without consuming campaign quotas.

### Security
- Validate configured public contact email addresses before storing them.
- Keep all icons, fonts, and frontend assets self-hosted.

## v0.1.12 - 2026-06-13

### Added
- Dynamic campaigns list on the main dashboard showing recent campaigns.
- Inline campaigns list inside organization details for direct access.
- Dynamic conditional field visibility in Form Builder (Add Field form).
- Keyboard accessibility for tooltip info-icons (tabindex="0").

### Changed
- Refined empty states for campaigns and organizations with a premium dashed card design.
- Reorganized Campaign Detail page layout with adaptive visual states (Onboarding Mode vs. Active Mode).
- Made browser extension uninstall URL registration persistent on startup (MV3 service worker compliance).

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
