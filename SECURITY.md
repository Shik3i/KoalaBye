# Security Policy

## Reporting a Vulnerability

Please do not open a public issue for an exploitable vulnerability. Contact the maintainers privately through the security contact published in the repository profile. Include affected versions, reproduction steps, and impact. The project will acknowledge reports and coordinate disclosure as maintainers become available.

## Current Controls

- No default administrator credentials. The first-run flow creates the Instance Owner.
- Argon2id password hashing with random salts and a 12-character minimum.
- Cryptographically random sessions with only token hashes stored server-side.
- HttpOnly, SameSite session cookies and configurable Secure cookies.
- Signed CSRF tokens on setup, login, logout, and future state-changing forms.
- Server-side permission checks with deny-by-default behavior.
- Hashed, expiring, revocable organization invite codes with bounded use counts.
- Owner invariants for organizations and the instance; the final active owner cannot be removed or disabled.
- Transactional enforcement of organization, member, and active-invite safety limits.
- Transactional campaign quota and final-explicit-owner enforcement.
- Campaign privacy settings make token hashing mandatory and keep IP storage and fingerprinting outside the data model.
- Cookie-free public campaign pages require no session and use URL-based language selection.
- Optional install tokens are length-bounded and HMAC-SHA256 hashed with the instance secret; raw values are never stored or rendered.
- Referrers are reduced to hostnames and user agents to coarse browser/OS families before storage.
- Monthly visit safety limits are enforced before a visit row is inserted.
- Public feedback bodies are capped at 128 KiB and validated against active fields and options.
- Submission inserts and UTC monthly safety limits are enforced transactionally.
- Submission rows store no IP address, raw user agent, or raw install token; linked visits contribute only their existing HMAC hash.
- A honeypot returns generic success without storing a submission.
- Response contents require organization membership plus an owner, editor, or analyst campaign role. Instance Owner status alone is insufficient.
- CSP, frame denial, MIME sniffing prevention, no-referrer, and restrictive permissions headers.
- SQLite foreign keys, WAL mode, busy timeout, transactions, and migrations.
- No external CDN requests, IP database storage, fingerprinting, or raw user-agent retention.
- A non-root runtime container and persistent `/data` volume.
- Automated tests cover setup lockout, authentication, registration policies, session revocation, tenant isolation, invite lifecycle, owner invariants, instance administration, CSRF-backed flows, security headers, translations, and migrations.

Use HTTPS in production, set `KOALABYE_SECURE_COOKIES=true`, protect the database file and backups, and replace the example secret with at least 32 random characters.

## MVP Limitations

- Login rate limiting is per-process and username-keyed. It is not coordinated across replicas and resets at restart.
- There is no password reset, MFA, passkey, account recovery, or session management UI.
- Audit retention and export policies are not configurable yet.
- Invite links are bearer credentials and must be shared through a trusted channel.
- Submission retention controls, exports, and detailed aggregate analytics are not implemented yet.
- Security contact and signed release procedures must be finalized before a public hosted launch.
- Dependency and container scanning are not yet automated in CI.
- SQLite backups and restore verification remain an operator responsibility.

The authenticated application may store a non-sensitive language preference cookie after an explicit language choice. Public uninstall pages remain cookie-free; language selection is carried in the URL.
