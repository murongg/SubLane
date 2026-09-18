# Changelog

## Unreleased

### Added

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

Upstream account authorization and model forwarding are not part of this milestone.
