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
| `make build` | Build frontend and embed it into `bin/sublane` |
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

## Tests

Backend tests cover environment validation, SQLite initialization/reopening, migration tracking, readiness failure, and API/static asset boundaries. Frontend tests cover response validation, request cancellation propagation, error recovery, navigation, and persistent theme/language selection. Use synthetic fixtures and temporary databases only.

Run focused tests during development:

```sh
go test ./internal/server -run TestRoutesAndAssetBoundary -count=1
pnpm --dir web test src/App.test.tsx
```

Browser verification should cover desktop and narrow layouts, mobile navigation, direct route reload, language changes, light/dark/system themes, and a disconnected backend. Unit tests do not replace these checks.

## Adding translations

1. Add an English key in `web/src/locales/en.ts`.
2. Add the same key in `web/src/locales/zh.ts`.
3. Use `useTranslation()` and `t()` in components, including ARIA labels.
4. Run `pnpm --dir web typecheck` and inspect longer copy in both languages.

UI preferences are local to the browser. They are not team settings and are not written to SQLite.

## Adding migrations

Create the next numbered `.sql` file in `internal/storage/migrations/`. Never alter a released migration. Keep migration changes transactional and include an upgrade/persistence test. Go embeds the migration files automatically.

## Troubleshooting

- **Backend root returns `frontend_not_built`:** use Vite on port 5173 for development, or run `make build` and start `bin/sublane`.
- **Frontend shows a connection failure:** confirm the Go process is running and Vite's proxy points to the configured address.
- **Port already in use:** stop the previous local process or change the configured port and proxy together. Do not terminate unrelated processes.
- **Go downloads a toolchain:** this is expected when the installed Go is older than `go.mod` requires.
- **Docker build cannot connect:** start your Docker engine, then run `docker compose config --quiet` and `docker build -t sublane:local .`.
- **Language or theme stays changed:** preferences are deliberately stored in browser local storage. Change them in Preferences to restore defaults.

## Maintenance

Keep `go.sum` and `web/pnpm-lock.yaml` under version control. Run dependency upgrades as separate changes and preserve third-party license notices. Do not copy a newer Shadcn Admin tree over local components without reviewing localization, naming, and accessibility changes.
