# Changelog

## Unreleased

### Changed

- Upgrade the public CLIProxyAPI translation SDK to v7.3.7 while retaining SubLane-owned authorization, credential storage, refresh, and forwarding.

### Added

- Version-tag release automation for Linux amd64/arm64 binaries and GHCR images, with checksums, native container smoke checks and deployment/upgrade documentation.
- Online backups and verification through the CLI and administrator UI, with isolated-directory restore and explicit restart switching.
- Account groups and grants, member request limits, durable personal/team usage charts, account model catalogs, and Codex client version settings.
- Request IDs, first-output latency, diagnostic filtering and fresh Codex quota-aware account selection with persisted observations.

- Subscription quota windows, remaining percentages, reset times, and manual refresh on the accounts page, with durable SQLite snapshots, bounded memory caching, shared refreshes, and stale/failure feedback.
- Codex OAuth and auth.json import, encrypted subscription credentials, serialized refresh, account management, and member-key HTTP/SSE/WebSocket forwarding with pinned account affinity.
- A lightweight public CLIProxyAPI translation adapter and opt-in synthetic Codex CLI protocol tests.

- chi routing and sqlc-generated database queries, with pinned generation tooling and CI drift checks.
- Personal API key creation, owner-only copying of encrypted recoverable keys, expiry/pause controls, owner-scoped metadata, revocation, and a separate gateway authentication boundary.
- Administrator-created member accounts, per-user sessions, immediate account revocation, and role-based menu, route, and API access.
- Workspace readiness overview with client availability, localized uptime, and preserved readings after a failed refresh.
- First-run administrator creation, login/logout, persisted 12-hour sessions, and default-protected management routes.
- Argon2id password hashing, bounded login/hash work, origin checks, and bilingual authentication pages.

- Initial Go and SQLite application foundation with health/readiness endpoints.
- Embedded React frontend based on selected Shadcn Admin components.
- Live system overview, account-connection empty state, preferences, and client-side routing.
- English and Simplified Chinese localization; English is the default.
- Black/white primary palette, semantic status colors, and light/dark/system themes.
- Unit tests, race checks, build commands, container configuration, and CI.
- AGPL-3.0-only license, third-party attribution, and contributor documentation.

Live subscription and desktop acceptance remain pending; resource measurements and advanced token budgets remain planned.
