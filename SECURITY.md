# Security

## Current scope

SubLane is a local-development scaffold. It does not yet implement administrator authentication, member keys, encrypted upstream credential storage, OAuth, or request forwarding. The default service address and Compose host port bind to loopback. Do not treat this version as a production access gateway.

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
