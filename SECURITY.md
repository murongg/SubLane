# Security

## Current scope

SubLane provides one local administrator and administrator-created members, first-run account creation, persisted roles, Argon2id password storage, per-user revocable sessions, same-origin JSON mutations, and bounded login attempts. Management APIs default to administrator-only access. Disabling a member revokes all of that member’s sessions in the same transaction. See [the authentication contract](https://sublane-website.vercel.app/docs/authentication) for exact behavior and limits.

Complete initialization before exposing the instance to other users. The setup page is open while no administrator exists, and closes after the first successful creation.

Personal gateway keys use 256-bit random secrets and SHA-256 digests at rest. Keys are shown once, scoped to their owner, and cannot access session-based management endpoints. Suspended members cannot authenticate gateway requests; explicit key revocation is permanent.

Codex credentials are encrypted with AES-256-GCM using a private instance key. OAuth state is PKCE-protected, session-bound, short-lived, and single-use. Upstream tokens are refreshed and persisted under one owner; model requests and WebSocket history/concurrency are bounded. See [Codex security and verification boundaries](https://sublane-website.vercel.app/docs/codex). MFA and password recovery are not implemented. The default service address and Compose host port bind to loopback. Use HTTPS and an explicit public origin for network deployments. This is still an early-development project, not a complete subscription gateway.

## Reporting

A private reporting channel and security contact have not been configured. Before a public release, maintainers should enable private vulnerability reporting or publish a contact here. Do not put credentials, account identifiers, private prompts, database files, or exploit-sensitive details in public issues. Share a sanitized description first and arrange a private channel with the maintainer for sensitive details.

No support window or response-time commitment is currently published.

## Development expectations

- Use synthetic test data and isolated temporary databases.
- Keep generated data, environment files, and credentials out of source control.
- Do not log authorization headers, OAuth tokens, or model request/response bodies.
- Preserve the separation between browser sessions, member gateway keys, and upstream credentials as those features are introduced.
- Validate forwarded URLs and protect administrator actions before enabling upstream integrations.
- Review upstream SDK upgrades against the compatibility scenarios in [the architecture guide](https://sublane-website.vercel.app/docs/architecture).

Read [LICENSE](LICENSE) and [third-party notices](THIRD_PARTY_NOTICES.md) for licensing information.
