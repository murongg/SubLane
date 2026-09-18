# Security

## Current scope

SubLane provides one local administrator, first-run account creation, Argon2id password storage, persisted revocable sessions, same-origin JSON mutations, and bounded login attempts. See [the authentication contract](docs/authentication.md) for exact behavior and limits.

Complete initialization before exposing the instance to other users. The setup page is open while no administrator exists, and closes after the first successful creation.

Member keys, encrypted upstream credential storage, upstream OAuth, model forwarding, MFA, and password recovery are not implemented. The default service address and Compose host port bind to loopback. Use HTTPS and an explicit public origin for network deployments. This is still an early-development project, not a complete subscription gateway.

## Reporting

A private reporting channel and security contact have not been configured. Before a public release, maintainers should enable private vulnerability reporting or publish a contact here. Do not put credentials, account identifiers, private prompts, database files, or exploit-sensitive details in public issues. Share a sanitized description first and arrange a private channel with the maintainer for sensitive details.

No support window or response-time commitment is currently published.

## Development expectations

- Use synthetic test data and isolated temporary databases.
- Keep generated data, environment files, and credentials out of source control.
- Do not log authorization headers, OAuth tokens, or model request/response bodies.
- Preserve the separation between browser sessions, member gateway keys, and upstream credentials as those features are introduced.
- Validate forwarded URLs and protect administrator actions before enabling upstream integrations.
- Review upstream SDK upgrades against the compatibility scenarios in `docs/architecture.md`.

Read [LICENSE](LICENSE) and [third-party notices](THIRD_PARTY_NOTICES.md) for licensing information.
