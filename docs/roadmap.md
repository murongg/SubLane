# Roadmap

This is product direction, not a release-date commitment. The foundation milestone creates a testable place to implement the first usable Codex workflow.

## Foundation

- Go HTTP service, environment validation, and graceful shutdown.
- SQLite migrations and persistent local storage.
- Shadcn Admin-based navigation and page primitives.
- Real service status, honest unavailable/empty states, and retry handling.
- English by default, Simplified Chinese, and light/dark/system themes.
- Embedded binary builds, container files, CI, and project documentation.

## First usable release

1. **Implemented:** administrator bootstrap, local member creation and enable/disable controls, shared login/logout, per-user sessions, and role-protected management routes.
2. **M2 implemented:** pinned public CLIProxyAPI executors behind the SubLane lifecycle adapter, encrypted credentials, session-bound OAuth state and provider-supported PKCE, and one refresh owner.
3. **M3 implementation ready; live acceptance pending:** add/import, verify, enable/disable, remove, and reauthorize Codex, Claude, and Antigravity accounts; forward HTTP/SSE, compaction, and WebSocket requests. Codex CLI protocol tests pass with synthetic upstreams; real subscription and desktop flows still require explicit validation.
4. **Implemented:** local member accounts, personal gateway keys with expiry/rename/pause, ownership isolation, revocation, password changes, administrator member resets and local administrator recovery. Invitation links remain a later addition.
5. **Implemented:** per-account concurrency, durable transient cooldown with request-driven recovery, and bounded recent request metadata. Member request limits and durable aggregate usage reports are implemented. Token budgets and advanced routing remain planned.
6. **Backup/restore implemented:** online SQLite snapshots, CLI and administrator web export/verification, isolated-directory recovery and explicit restart switching. Version-tag release automation, Linux amd64/arm64 archives and native container verification are configured. A measured resource baseline and systematic live-client acceptance remain. See [release operations](releases.md). See [backup and restore](backup.md).

Ship and use this scope with a real internal team before expanding it. Build account and policy behavior with synthetic tests; real compatibility checks require explicitly supplied test accounts and separate evidence.

Management mutations also have a bounded administrator audit with transactional success records and sanitized failed attempts. See [management audit](audit.md).

## Later candidates

- OIDC through an existing identity provider.
- Advanced quota accounting and reporting.
- Official API key upstreams, additional providers, and Claude/Antigravity quota readers.
- More detailed operational diagnostics.

Direct LDAP integration, billing, reseller workflows, and mandatory external databases are outside the initial scope.

## Account pool groups

Implemented default-group migration, administrator-managed pools, member grants, immutable API-key group bindings, group-scoped model discovery and conversation affinity, exact model allowlists, and permission rechecks on every WebSocket turn. Groups retain the subscription-first internal-team scope; billing and per-group quotas remain outside this iteration. See [account groups](groups.md).
