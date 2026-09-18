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
2. **M2 implemented:** a pinned public CLIProxyAPI translation adapter, encrypted credentials, session-bound OAuth PKCE/state, and one refresh owner.
3. **M3 implementation ready; live acceptance pending:** add/import, verify, enable/disable, remove, and reauthorize Codex accounts; forward HTTP/SSE, compaction, and WebSocket requests. Codex CLI protocol tests pass with synthetic upstreams; real subscription and desktop flows still require explicit validation.
4. **Implemented:** local member accounts, personal gateway keys, ownership isolation, and revocation. Invitation links remain a later addition.
5. Per-member quotas/usage and actionable error records. A global concurrency bound already protects the gateway; advanced scheduling and accounting remain planned.
6. Deployment, migrations, backup/restore, and a measured resource baseline.

Ship and use this scope with a real internal team before expanding it. Build account and policy behavior with synthetic tests; real compatibility checks require explicitly supplied test accounts and separate evidence.

## Later candidates

- OIDC through an existing identity provider.
- Advanced quota accounting and reporting.
- Official API key upstreams and additional providers.
- More detailed operational diagnostics.

Direct LDAP integration, billing, reseller workflows, and mandatory external databases are outside the initial scope.
