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
  <a href="docs/roadmap.md"><img src="https://img.shields.io/badge/stage-foundation-92400E?style=flat-square&amp;labelColor=555555" alt="Stage: foundation" height="20"></a>
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a> · <a href="https://github.com/murongg/SubLane">Repository</a> · <a href="brand/README.md">Brand materials</a>
</p>

**Status: foundation scaffold.** The HTTP server, SQLite migrations, embedded frontend, live system status, navigation, themes, and English/Chinese localization work. Subscription authorization, model forwarding, member authentication, and usage reporting are not implemented yet. This version is intended for local development.

## Stack

- Go 1.26+, standard-library HTTP server, pure-Go SQLite in WAL mode.
- React, TypeScript, Vite, TanStack Router and Query.
- Selected components from [Shadcn Admin](https://github.com/satnaing/shadcn-admin), with a neutral palette and semantic status colors.
- One binary for production; Node.js is only needed to build the frontend.
- CLIProxyAPI SDK integration will live behind a dedicated backend adapter in the next milestone.

## Quick start

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

## Build and run

```sh
make build
./bin/sublane
```

Open http://127.0.0.1:8080. The binary includes the frontend and SQLite driver. `./bin/sublane --version` prints its version. Pass `VERSION=0.1.0` to `make build` to override the build version.

```sh
docker compose up --build -d
```

The Compose service binds the host port to loopback and stores the database in a named volume. Authentication is not implemented in this milestone; keep the instance local while developing.

## Configuration

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `SUBLANE_ADDR` | `127.0.0.1:8080` | HTTP listen address |
| `SUBLANE_DATA_DIR` | `./data` | Directory containing `sublane.db` |
| `SUBLANE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |

The server reads process environment variables. It does not load `.env` automatically. `.env.example` documents the available settings. If you change the backend address during development, also update Vite's proxy target.

SQLite migrations run at startup and are recorded transactionally. The connection pool uses a single connection. Back up the data directory after stopping the server; copying just the live `.db` file may omit WAL data.

## Project layout

```text
cmd/sublane/           Process entrypoint and graceful shutdown
internal/config/      Environment parsing and validation
internal/server/      HTTP routes and SPA asset handling
internal/storage/     SQLite initialization and SQL migrations
web/src/components/   Application shell and adapted UI components
web/src/pages/        Overview, accounts, preferences, and 404
web/src/lib/          API boundary, localization, and class names
web/src/locales/      English and Simplified Chinese strings
web/embed.go          Production frontend embedding
scripts/dev.mjs       Development process orchestration
```

Do not import provider SDK types into membership or storage code. Add CLIProxyAPI integration behind a narrow adapter when implementing the first real Codex account flow; this scaffold has no OAuth credentials or upstream model requests.

## Endpoints

- `GET /healthz`: process liveness.
- `GET /readyz`: SQLite readiness; returns 503 when unavailable.
- `GET /api/system`: instance version, uptime, storage, and gateway integration state.
- Other `/api`, `/v0`, and `/v1` routes return JSON 404 responses. Unknown static assets return 404 rather than the SPA document.

## Verification

```sh
make check
```

This runs Go vet/format checks, TypeScript, ESLint, Prettier, Go race tests, frontend tests, and the production build. Tests use synthetic data and temporary SQLite databases. CI also builds the Docker image.

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
- [Development guide](docs/development.md)
- [Roadmap](docs/roadmap.md)
- [Contributing](CONTRIBUTING.md)
- [Security](SECURITY.md)
- [Changelog](CHANGELOG.md)

## License

Copyright (c) 2026 SubLane contributors. Licensed under **AGPL-3.0-only**; see [LICENSE](LICENSE). Preserve [third-party notices](THIRD_PARTY_NOTICES.md) and their license texts when distributing this project. Source repository: [murongg/SubLane](https://github.com/murongg/SubLane).
