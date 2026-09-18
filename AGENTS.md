# Agent instructions

## Context

Read `PRODUCT.md` for product scope, `DESIGN.md` for interface rules, and `docs/architecture.md` for implementation boundaries before making significant changes. Local account access, Codex OAuth/import, encrypted credentials, and gateway forwarding are implemented. Live subscription and desktop compatibility require separate evidence; see `docs/codex.md`.

Use English for code comments, `PRODUCT.md`, and primary developer documentation. Keep the English and Simplified Chinese UI dictionaries complete. English is the default interface language.

## Work style

- Make the smallest complete change that meets the request. Prefer existing mechanisms to new dependencies or abstractions.
- Keep unrelated edits out of a task. Do not introduce speculative provider frameworks, shared packages, or runtime services.
- Explain assumptions and verification results concisely. Never report an untested build, endpoint, or client integration as working.
- Product plans and specifications in `docs/plans/` and `docs/specs/` are local working files; do not commit them unless explicitly requested.
- Do not create commits, publish a repository, or deploy without a user request.

## Boundaries

- `cmd/sublane` owns startup, process lifecycle, and dependency wiring.
- `internal/config` owns environment parsing and validation.
- `internal/auth` owns local user credentials, roles, member lifecycle, first-run initialization, and persisted sessions.
- `internal/apikey` owns personal gateway key generation, hashed storage, ownership, revocation, and bearer authentication. API keys must never authenticate browser management sessions.
- `internal/storage` owns SQLite initialization, migrations, query SQL, and sqlc-generated database access under `internal/storage/db`.
- `internal/server` owns chi routing and HTTP handling; it must not silently serve HTML for API errors.
- `web` is a client-rendered React app and an embedded Go asset package. It must not require a Node.js server in production.
- `internal/codex` owns the pinned public CLIProxyAPI translation SDK and Codex HTTP protocol. Use the lightweight adapter selected for this project; do not start the SDK service or import upstream `internal` packages.
- `internal/accounts` owns subscription metadata and serialized credential changes, `internal/vault` owns encryption, `internal/oauth` owns session-bound PKCE attempts, and `internal/gateway` owns account affinity and request admission. Membership, policy, and storage code must not depend on SDK types.
- Avoid adding packages solely for hypothetical reuse. Keep related code together and move it only when ownership or reuse justifies a boundary.

## Backend

- Use standard Go conventions and explicit dependency injection at test seams.
- Register management routes on the administrator-protected chi router. Keep handlers compatible with `net/http`; domain services must not depend on chi.
- Use chi `Route` and method-specific registrations for HTTP resources; do not dispatch methods or gateway paths manually inside handlers. Apply session, role, and gateway authentication with router-level `Use` so 404/405 responses remain protected. Reserve `With` for endpoint-specific checks such as login origin validation and throttling.
- Write application queries in `internal/storage/queries/` and run `make generate`. Never hand-edit `internal/storage/db/`; keep generated code with its SQL changes. Migration bootstrap SQL remains in storage.
- Keep transaction ownership in domain services and use `queries.WithTx(tx)` for every query inside a transaction. Keep database row types separate from public API responses.
- SQLite migrations are additive, ordered SQL files. Never edit an already released migration; add a new one.
- Do not hold a database transaction open during network IO or model generation. Persist rotated credentials before returning them to callers.
- Reauthorization must preserve upstream identity. Recheck gateway keys on every WebSocket turn, and never move an existing conversation to another account after disablement or deletion.
- Keep OAuth states, request bodies, stream events, WebSocket history, and concurrent operations bounded. Never read local Codex credentials automatically or use real credentials in tests.
- Preserve cancellation and graceful shutdown. Future model streaming routes need explicit timeout and resource policies rather than blanket response buffering.
- Do not add authentication bypasses or expose management endpoints as member APIs.
- Default to loopback listening. Keep management routes under the default administrator-only API subtree. Members must never inherit administrator API access; enforce roles on both direct routes and backend requests. First-run setup uses a username and password; preserve its atomic single-administrator guard and disable it after initialization. See `docs/authentication.md` for the current authentication contract.

## Frontend

- Do not use the JavaScript `void` operator, including to discard promises. Return or await promises where appropriate and handle failures explicitly. TypeScript `void` return types are allowed.
- Use PascalCase component filenames and lowercase domain folders. Follow TanStack Router conventions for routes.
- Reuse the adapted Shadcn Admin components under `web/src/components/ui/`.
- Use TanStack Query for remote state and validate API responses at the boundary.
- Keep user-facing text in `web/src/locales/en.ts` and `zh.ts`, including accessibility labels and error messages.
- Keep colors in semantic CSS tokens. Primary actions stay black/white; use status colors only for status or relevant feedback.
- Preserve keyboard access, visible focus, reduced-motion behavior, responsive navigation, and both themes.
- Render honest loading, failure, and empty states. Never add fake account or usage data to product screens.

## Tests and verification

- Behavior-changing code requires a failing test before implementation, then a passing test. Text-only, documentation, and nonfunctional configuration edits do not require unit tests.
- Use only synthetic fixtures, fake upstreams, and temporary SQLite databases. Never read actual credentials or use personal, customer, production, or identifiable data in tests.
- Keep meaningful tests close to the code they cover. Do not add tests that merely duplicate implementation details or assert deleted text is gone.
- Run `make check` before completing a broad implementation. For a small change, start with affected tests and run the relevant build or checks.
- For UI changes, verify the affected flows in the browser at desktop and narrow widths, in both themes, and with English and Chinese where relevant.
- For deployment changes, validate Compose and build the image when Docker is available. State clearly when the Docker daemon is unavailable.
- Inspect critical logic after verification. Add concise comments where ordering, invariants, compatibility assumptions, or edge cases would otherwise be easy to break.

## Credentials and licensing

- Never commit databases, credentials, `.env` files, auth caches, or upstream request bodies.
- Do not log tokens or model prompt/response content by default.
- Preserve AGPL-3.0-only licensing and all third-party notices. Adapted Shadcn Admin components retain their MIT attribution.
- Do not invent repository URLs, maintainer contact addresses, support promises, or performance benchmarks.
