<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="brand/logos/lockup-white.svg">
    <img src="brand/logos/lockup-black.svg" alt="SubLane" width="420">
  </picture>
</h1>

<p align="center">
  A minimal, self-hosted subscription gateway for internal teams. Codex first.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0--only-171717?style=flat-square&amp;labelColor=555555" alt="License: AGPL-3.0-only" height="20"></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.26%2B-171717?style=flat-square&amp;logo=go&amp;logoColor=white&amp;labelColor=555555" alt="Go 1.26+" height="20"></a>
  <a href="web/package.json"><img src="https://img.shields.io/badge/React-19-171717?style=flat-square&amp;logo=react&amp;logoColor=white&amp;labelColor=555555" alt="React 19" height="20"></a>
  <a href="internal/storage/storage.go"><img src="https://img.shields.io/badge/SQLite-embedded-171717?style=flat-square&amp;logo=sqlite&amp;logoColor=white&amp;labelColor=555555" alt="SQLite embedded" height="20"></a>
  <a href="docs/roadmap.md"><img src="https://img.shields.io/badge/stage-internal%20testing-92400E?style=flat-square&amp;labelColor=555555" alt="Stage: internal testing" height="20"></a>
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a> · <a href="https://github.com/murongg/SubLane">Repository</a> · <a href="brand/README.md">Brand materials</a>
</p>

**Status: internal testing.** SubLane provides local administrator/member access, personal gateway keys, account pools and permissions, Codex/Claude/Antigravity subscription management, model discovery, HTTP/SSE/WebSocket forwarding, request diagnostics, usage charts and backup/restore. Codex pools can avoid accounts with fresh exhausted-quota observations. Live provider/desktop compatibility and resource baselines still require explicit acceptance evidence.

Version-tag release automation builds Linux binaries and multi-platform GHCR images. Published downloads become available after the first successful release; see [deployment](docs/deployment.md) and [release operations](docs/releases.md).

## Stack

- Go 1.26+, chi routing over `net/http`, sqlc-generated queries, and pure-Go SQLite in WAL mode.
- React, TypeScript, Vite, TanStack Router and Query.
- Selected components from [Shadcn Admin](https://github.com/satnaing/shadcn-admin), with a neutral palette and semantic status colors.
- One binary for production; Node.js is only needed to build the frontend.
- CLIProxyAPI v7 built-in executors behind SubLane’s encrypted credential lifecycle and chi gateway; no Redis or separate proxy process required.

## Run with Docker

For a published version, download `docker.compose.yaml` from [GitHub Releases](https://github.com/murongg/SubLane/releases), select the image version in a Compose `.env` file, then run:

```sh
docker compose -f docker.compose.yaml pull
docker compose -f docker.compose.yaml up -d
```

The default image is `ghcr.io/murongg/sublane:latest`; set `SUBLANE_IMAGE` to a fixed published version or digest for upgrades you control. Images support Linux amd64 and arm64. The host port binds to `127.0.0.1:8080` and a named volume preserves the database and encryption key. See [deployment and upgrades](docs/deployment.md) for HTTPS, backups, recovery and the standalone binary.

Before the first image release, build locally with `docker compose -f docker.compose.yaml -f docker.compose.build.yaml up --build -d` from a source checkout.

## Development quick start

Install Go 1.26+, Node.js 24, and pnpm 9.12.2. An older Go with automatic toolchain downloads enabled can read `go.mod` and fetch the required toolchain.

```sh
git clone git@github.com:murongg/SubLane.git
cd SubLane
make setup
make dev
```

Open http://127.0.0.1:5173. The backend listens on http://127.0.0.1:8080. Vite proxies backend requests; no CORS setup is needed. Ctrl+C stops both processes. Frontend changes use HMR; restart `make dev` after backend changes.

Alternatively, run `make dev-api` and `make dev-web` in separate terminals.

If port 5173 is occupied, use `make dev WEB_PORT=5174`.

On first startup, open the UI and select **Start setup** on the welcome page. Create the administrator with a username and password to enter the workspace. Complete initialization locally before opening the instance to other users. See [administrator authentication](docs/authentication.md) for session behavior and HTTPS deployment.

## Build and run

```sh
make build
./bin/sublane
```

Open http://127.0.0.1:8080. The binary includes the frontend and SQLite driver. `./bin/sublane --version` prints its version. Pass `VERSION=0.1.0` to `make build` to override the build version.

```sh
docker compose -f docker.compose.yaml -f docker.compose.build.yaml up --build -d
```

The source-build override creates `sublane:local`. Compose binds the host port to loopback and stores the database in a named volume. For network access, use an HTTPS reverse proxy and set the external `SUBLANE_PUBLIC_URL` in the container environment.

## Subscription providers

Open **Accounts** as the administrator, select **Codex**, **Claude**, or **Antigravity**, and authorize a subscription or import its credential JSON. Verify the connection, then use a personal API key and the configuration guide on **API keys**. See [provider setup and model selection](docs/providers.md) and [the complete Codex guide](docs/codex.md) for the manual OAuth callback, credential backups, client setup, and current limits.

## Account groups

Administrators can organize accounts into pools and grant member access. Each personal key is bound to one group; only accounts in that pool can serve its requests. Existing accounts and keys migrate to the default group. Groups can optionally restrict exact model IDs. Keys support rename, reversible pause, optional expiry and owner-only copying of newly created keys, with encrypted storage. See [account groups](docs/groups.md) and [personal keys](docs/api-keys.md) for access behavior.

## Pool operation

Accounts expose current concurrency and durable cooldown state, with administrator-configurable limits. Each user can inspect their own model-call metadata under **Requests**; administrators can inspect **All requests**. Neither view records prompt or response bodies. See [pool runtime](docs/pool-runtime.md) for recovery behavior and request history. [Team controls](docs/team-controls.md) cover password management, member request limits and independent usage summaries. Administrators can inspect manual changes through the bounded [management audit](docs/audit.md).

## Configuration

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `SUBLANE_ADDR` | `127.0.0.1:8080` | HTTP listen address |
| `SUBLANE_DATA_DIR` | `./data` | Directory containing `sublane.db` |
| `SUBLANE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `SUBLANE_PUBLIC_URL` | Unset | External origin for proxy deployments; HTTPS enables Secure cookies |

The server reads process environment variables. It does not load `.env` automatically. `.env.example` documents the available settings. If you change the backend address during development, also update Vite's proxy target.

SQLite migrations run at startup and are recorded transactionally. The connection pool uses a single connection. Use the [backup command or administrator page](docs/backup.md) for a consistent online snapshot including `credentials.key`; copying just the live `.db` file may omit WAL data. The key is required to decrypt subscription credentials. Restore a pre-upgrade backup into a separate directory if reverting across a schema change.

## Project layout

```text
cmd/sublane/           Process entrypoint and graceful shutdown
internal/config/      Environment parsing and validation
internal/auth/        Local users, roles, member lifecycle, and sessions
internal/apikey/      Personal gateway keys and bearer authentication
internal/server/      HTTP routes and SPA asset handling
internal/storage/     SQLite initialization, migrations, SQL queries, generated access
web/src/components/   Application shell and adapted UI components
web/src/pages/        Workspace, accounts, groups, keys, requests, usage and settings
web/src/lib/          API boundary, localization, and class names
web/src/locales/      English and Simplified Chinese strings
web/embed.go          Production frontend embedding
scripts/dev.mjs       Development process orchestration
```

Do not import provider SDK types into membership or storage code. The public SDK execution and translation packages are isolated in `internal/upstream`. Account and gateway lifecycle behavior stays owned by SubLane.

## Endpoints

- `GET /healthz`: process liveness.
- `GET /readyz`: SQLite readiness; returns 503 when unavailable.
- `GET /api/auth/state`: public initialization and session state; setup/login/logout use explicit POST endpoints.
- `GET /api/system`: authenticated instance version, uptime, storage, and gateway integration state.
- The management `/api/` subtree requires an administrator session by default. Unknown routes return JSON errors. `/v0` remains unimplemented. `/v1` requires a gateway API key; configured subscriptions support Responses HTTP/SSE/WebSocket, Codex compaction, and Chat Completions forwarding. See [Codex setup and verification](docs/codex.md). Missing static assets return 404 rather than the SPA document.

## Verification

```sh
make check
```

This checks sqlc-generated code consistency and runs Go vet/format checks, TypeScript, ESLint, Prettier, Go race tests, frontend tests, and the production build. Run `make generate` after editing query SQL or migrations. Tests use synthetic data and temporary SQLite databases. CI also builds and smoke-tests native amd64 and arm64 Docker images with synthetic data, read-only runtime paths, persistent volumes and backup/restore.

## Interface conventions

English is the default, regardless of the browser's language. Language and light/dark/system preferences are stored in the current browser. Keep user-facing strings in `web/src/locales/`; the Chinese dictionary is type-checked against the English keys. Keep `PRODUCT.md` and primary developer documentation in English.

The shell uses black, white, and neutral grays. Green, amber, red, and blue communicate success, warnings, errors, and information. Pair color with text and icons. Component filenames use PascalCase; rename new shadcn-generated files and imports to match this convention.

The repository keeps a small set of maintained brand assets. Run `make brand` to export the complete kit into Git-ignored `dist/brand/`; see the [brand guide](brand/README.md).

## Documentation

- [Product scope](PRODUCT.md)
- [Design system](DESIGN.md)
- [Brand materials and usage guide](brand/README.md)
- [Agent instructions](AGENTS.md)
- [Architecture](docs/architecture.md)
- [Administrator authentication](docs/authentication.md)
- [Member accounts and permissions](docs/members.md)
- [Personal API keys](docs/api-keys.md)
- [Management audit](docs/audit.md)
- [Backup and restore](docs/backup.md)
- [System settings and Codex version synchronization](docs/settings.md)
- [Account and group model catalogs](docs/models.md)
- [Deployment and upgrades](docs/deployment.md)
- [Release operations](docs/releases.md)
- [Development guide](docs/development.md)
- [Roadmap](docs/roadmap.md)
- [Contributing](CONTRIBUTING.md)
- [Security](SECURITY.md)
- [Changelog](CHANGELOG.md)

## License

Copyright (c) 2026 SubLane contributors. Licensed under **AGPL-3.0-only**; see [LICENSE](LICENSE). Preserve [third-party notices](THIRD_PARTY_NOTICES.md) and their license texts when distributing this project. Source repository: [murongg/SubLane](https://github.com/murongg/SubLane).
