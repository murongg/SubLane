# Development

## Requirements

- Go 1.26 or newer; the module pins the minimum toolchain.
- Node.js 24 (see `.node-version`).
- pnpm 9.12.2, matching the frontend `packageManager` field.
- Docker is optional for local development and required only for container verification.

Run all Make targets from the repository root.

## Commands

| Command | Purpose |
| --- | --- |
| `make setup` | Download pinned Go and frontend dependencies |
| `make dev` | Build the development backend and launch it with Vite |
| `make dev-api` | Run only the Go backend |
| `make dev-web` | Run only Vite |
| `make release VERSION=0.0.0-test` | Build Linux amd64/arm64 archives and SHA-256 checksums locally |
| `make build` | Build frontend and embed it into `bin/sublane` |
| `make generate` | Regenerate typed database access using pinned sqlc |
| `make generate-check` | Verify generated Go matches the SQL and sqlc configuration |
| `make test` | Run Go race tests and frontend tests |
| `make lint` | Vet, formatting, typecheck, and lint |
| `make check` | Run all checks and the production build |
| `make brand` | Export and check the complete brand kit under ignored `dist/brand/` |
| `make clean` | Remove generated binaries, frontend output, and brand exports |

`make clean` does not remove the database or dependencies. Restart the backend after editing Go code. Vite updates frontend code through HMR. Use `make dev WEB_PORT=5174` when the default frontend port is occupied.

## Configuration

Copying `.env.example` does not activate it. Export variables in your shell or supply them to a single command:

```sh
SUBLANE_LOG_LEVEL=debug make dev
SUBLANE_ADDR=127.0.0.1:9090 ./bin/sublane
```

Vite's development proxy expects port 8080 by default. Change its target alongside `SUBLANE_ADDR` if you move the development backend.

The API proxy must preserve Host (`changeOrigin: false`) for same-origin authentication. Start with an empty test data directory to exercise setup and create an administrator through the form using synthetic credentials. Never reset a working instance by deleting its database.

## Tests

Backend tests cover environment validation, SQLite upgrades preserving administrator sessions, one-time setup/races/rollback, member lifecycle, per-user session limits, disable-time revocation, role-based API access, origin/body checks, rate limits, readiness failure, and API/static asset boundaries. Frontend tests cover role-aware routing and menus, member creation/status changes, setup validation, login errors, logout and cross-identity cache clearing, expiry, response validation, navigation, and persistent theme/language selection. Use synthetic fixtures and temporary databases only.

Run focused tests during development:

```sh
go test ./internal/server -run TestRoutesAndAssetBoundary -count=1
pnpm --dir web test src/App.test.tsx
```

Browser verification should cover desktop and narrow layouts, mobile navigation, direct route reload, language changes, light/dark/system themes, and a disconnected backend. Unit tests do not replace these checks.

## Codex adapter tests

The adapter tests never use a real account. They inject local fake upstreams through a transport seam while production endpoint selection stays fixed. To exercise an installed Codex CLI with an isolated temporary configuration and synthetic model output:

```sh
SUBLANE_TEST_CODEX=1 go test ./internal/server -run TestCodexCLIProtocol -count=1 -v
```

The normal test suite skips this opt-in external-client test. Do not read a developer's existing `auth.json` or keychain for tests. Live provider and desktop acceptance require an explicitly authorized account; record those results separately from protocol coverage.

## Adding translations

1. Add an English key in `web/src/locales/en.ts`.
2. Add the same key in `web/src/locales/zh.ts`.
3. Use `useTranslation()` and `t()` in components, including ARIA labels.
4. Run `pnpm --dir web typecheck` and inspect longer copy in both languages.

UI preferences are local to the browser. They are not team settings and are not written to SQLite.

## HTTP routes and database queries

HTTP routes use chi with standard `net/http` handlers. Resources register their own `Get`, `Head`, `Post`, and `Patch` handlers inside `Route` groups. Add management routes to the administrator-protected router in `internal/server/server.go`. Public authentication and personal-key routes are explicit exceptions. Session, role, and gateway bearer authentication run through router-level `Use`, protecting method errors and unknown paths as well as matching endpoints. Public login/setup use `With` for origin validation and throttling. Do not reintroduce method or path dispatch switches in handlers.

Each subrouter installs JSON `NotFound` and `MethodNotAllowed` handlers. The 405 handler derives `Allow` from chi's registered routes through `Match`, without running application handlers. Health checks, system status, and static assets explicitly register HEAD; other APIs retain their declared methods. The SPA is registered only for GET/HEAD at the root and cannot handle reserved API fallbacks.

Write named SQL queries in `internal/storage/queries/`, grouped by domain. `sqlc.yaml` uses the existing migration directory as its schema and generates `internal/storage/db/`. Run `make generate` after changing queries, migrations, or generator configuration. Commit SQL and generated Go together; never edit generated files manually. Domain services own validation and transactions, bind queries with `WithTx(tx)`, and convert rows to public response types.

The Makefile pins sqlc to v1.31.1 and runs it through `go run`; the first invocation downloads and builds the tool. Go may download a compatible toolchain for the generator. No global sqlc installation, cloud service, or database connection is needed for generation. Generator dependencies stay outside the application's module dependency graph. Generated files are versioned, so ordinary Go builds and production containers do not need sqlc. `make check`, including CI, runs `make generate-check` to reject stale generated code without rewriting it.

## Adding migrations

Create the next numbered `.sql` file in `internal/storage/migrations/`. Never alter a released migration. Keep migration changes transactional and include an upgrade/persistence test. Go embeds the migration files automatically. Run `make generate` so sqlc checks queries against the resulting schema; sqlc does not apply database migrations.

## Troubleshooting

- **Backend root returns `frontend_not_built`:** use Vite on port 5173 for development, or run `make build` and start `bin/sublane`.
- **Frontend shows a connection failure:** confirm the Go process is running and Vite's proxy points to the configured address.
- **Port already in use:** stop the previous local process or change the configured port and proxy together. Do not terminate unrelated processes.
- **Go downloads a toolchain:** this is expected when the installed Go is older than `go.mod` requires.
- **Docker build cannot connect:** start your Docker engine, then run `docker compose -f docker.compose.yaml config --quiet` and `docker compose -f docker.compose.yaml -f docker.compose.build.yaml build`.
- **Language or theme stays changed:** preferences are deliberately stored in browser local storage. Change them in Preferences to restore defaults.

## Maintenance

Keep `go.sum` and `web/pnpm-lock.yaml` under version control. Run dependency upgrades as separate changes and preserve third-party license notices. Do not copy a newer Shadcn Admin tree over local components without reviewing localization, naming, and accessibility changes.

## Packaging and release checks

See [deployment](deployment.md) for container smoke tests and [release operations](releases.md) for the tag-triggered pipeline. `make test` includes the built-in Node release metadata tests; `make lint` checks release/smoke script syntax. CI builds and tests containers on both native Linux architectures, then the release workflow publishes those exact tested images and extracts their binaries. No registry credentials are used in pull-request checks.
