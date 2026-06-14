# Privacy

KoalaBye is designed for useful uninstall feedback without building visitor profiles. This document describes the software's data model and defaults. It is not legal advice, and each operator remains responsible for reviewing the laws, notices, retention periods, and campaign questions that apply to their deployment.

## Campaign Modes

### Strict

Strict is the default for new campaigns. It can count visits and submissions, store submitted answers, and HMAC-hash an optional install token. It does not collect referrer domains, coarse browser or operating-system families, URL context, partial responses, IP addresses, raw user agents, raw tokens, full referrer URLs, cookies, or fingerprints.

When an optional install token is present, repeated requests for the same campaign and token hash within 30 minutes reuse the first minimized visit record. This prevents reloads and browser retries from inflating visit analytics without introducing cookies, IP-based identifiers, or fingerprinting. Tokenless requests remain independent anonymous page views because KoalaBye deliberately has no privacy-invasive identifier available to link them.

### Balanced Diagnostics

Balanced is optional per campaign. The owner can independently enable:

- referrer hostname only
- coarse browser family
- coarse operating-system family
- allowlisted URL context

Raw User-Agent values are parsed in memory and discarded. Unknown values become `Unknown`. The current implementation does not store browser major versions, device class, UTC offset, or exact timezone names.

### Partial Responses

Partial response collection is not implemented and is effectively `off`. KoalaBye does not autosave text drafts or non-text answers before final submission.

A future `structured_only` mode must be explicit, publicly disclosed, body-limited, option-validated, cookie-free, and restricted to checkbox, radio, rating, last-touched-field, and completion-state data. Text and textarea drafts must remain excluded unless a separate, clearly warned setting is deliberately designed and reviewed.

## URL Context

Campaign owners may opt into these keys:

- `app_version`
- `extension_version`
- `platform`
- `source`
- `channel`
- `utm_source`
- `utm_medium`
- `utm_campaign`
- `utm_content`
- `utm_term`

Values are limited to 128 characters and the characters `A-Z`, `a-z`, `0-9`, `.`, `_`, `:`, and `-`. Values containing URL schemes or failing validation are ignored. Unknown keys are ignored. The raw query string is never stored, and `t` is handled only as input to HMAC hashing.

Do not put names, email addresses, account IDs, advertising IDs, or other personal data in campaign URLs. Custom context keys are not supported in this release.

## Public Disclosure

Every public campaign page displays a collection notice. The notice distinguishes Strict mode from enabled coarse diagnostics and separately identifies URL-context collection. Public pages do not set cookies. An explicit theme choice can be stored in browser `localStorage`.

## Exports

Authorized CSV and JSON exports can include sanitized URL context attached to the linked visit. They never include raw install tokens, install-token hashes, raw query strings, IP addresses, raw user agents, full referrer URLs, or internal integer IDs. Exports can contain submitted free text and must be handled as sensitive operator data.

## Privacy Promises

- no external analytics
- no third-party scripts
- no fingerprinting
- no IP address storage
- no raw User-Agent storage
- no raw full referrer URL storage
- no raw query-string storage
- no cross-campaign user tracking
- no paid-tier data lock-in
- no custom JavaScript on public forms
- no hidden autosave of text fields
- privacy controls visible to campaign owners
- enabled diagnostics disclosed on public pages

## Retention and Responsibility

Campaign owners can configure manual retention thresholds and hard-delete visits or responses. Automated scheduling is not implemented. Operators must protect the SQLite database and backups, choose suitable retention periods, configure and legally review the bundled operator notices, and ensure campaign questions and URL parameters do not request unnecessary personal data.
