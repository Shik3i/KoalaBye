# Changelog

## v0.1.14 - 2026-06-13

### Added
- Privacy-preserving funnel analytics for visits, first form interactions, and submissions.
- Period, campaign, app, extension, platform, browser, and operating-system comparisons.
- Configurable CSV columns and filter-aware CSV/JSON exports.
- Campaign list filters, sorting by real activity, global campaign search, and a guided onboarding checklist.
- Local action icons, active navigation states, unsaved-change warnings, and request loading states.

### Changed
- Analytics now include equal-length previous-period deltas and field-value trends.
- Response and analytics filters expose only values present in authorized campaign data.
- Responsive navigation, touch targets, reduced-motion behavior, and keyboard focus handling were refined.

### Privacy
- A form start is stored once per anonymous visit after the first form interaction; no field value, draft, cookie, IP address, or new visitor identifier is collected.
- Diagnostic filters and export columns remain restricted by the campaign's enabled collection settings.

### Accessibility
- Added keyboard-operable global search, explicit current-page navigation, accessible table alternatives for charts, and improved mobile overflow behavior.
- Documented the release accessibility review in `docs/audits/V0.1.14_ACCESSIBILITY_AUDIT.md`.

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
