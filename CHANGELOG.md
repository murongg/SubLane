# Changelog

## Unreleased

### Added

- Subscription quota windows, remaining percentages, reset times, and manual refresh on the accounts page.
- Codex OAuth and auth.json import, encrypted subscription credentials, serialized refresh, account management, and member-key HTTP/SSE/WebSocket forwarding with pinned account affinity.
- A lightweight public CLIProxyAPI translation adapter and opt-in synthetic Codex CLI protocol tests.

- chi routing and sqlc-generated database queries, with pinned generation tooling and CI drift checks.
- Personal API key creation, one-time secret display, owner-scoped metadata, revocation, and a separate gateway authentication boundary.
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

Live subscription and desktop acceptance remain pending; usage accounting and advanced quotas remain planned.
