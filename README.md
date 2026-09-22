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
  <a href="https://sublane-website.vercel.app/docs/roadmap"><img src="https://img.shields.io/badge/stage-internal%20testing-92400E?style=flat-square&amp;labelColor=555555" alt="Stage: internal testing" height="20"></a>
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a> · <a href="https://github.com/murongg/SubLane">Repository</a> · <a href="brand/README.md">Brand materials</a>
</p>

SubLane brings your team's AI subscriptions into one self-hosted gateway. It runs as a single Go service with SQLite and a built-in web dashboard.

## Features

- Codex, Claude and Antigravity accounts through OAuth or credential import.
- OpenAI-compatible APIs, [native Claude Messages and Gemini generation](https://sublane-website.vercel.app/docs/protocols), with HTTP/SSE and Responses WebSocket support.
- Team members, personal API keys and account-group permissions.
- Account pooling, concurrency limits, cooldowns and Codex quota awareness.
- Request diagnostics, usage charts, and backup and restore.
- English and Simplified Chinese, with light and dark themes.

## Quick start

With Docker, Compose, curl and jq installed, run:

```sh
curl -fsSL https://raw.githubusercontent.com/murongg/SubLane/main/scripts/install.sh | bash
```

Open http://127.0.0.1:8080, create the administrator, connect a subscription and add it to an account pool, then create a personal key. Follow [First request](https://sublane-website.vercel.app/docs/quickstart) to complete the connection; team sharing and usage limits can wait until you need them.

The script installs the latest stable release, or the newest prerelease if no stable release exists, in `./sublane`. Data stays in a Docker volume; existing directories are left unchanged. For options, manual Compose deployment, HTTPS and upgrades, see the [deployment guide](https://sublane-website.vercel.app/docs/deployment).

## Development

Requires Go 1.26+, Node.js 24 and pnpm 9.12.2.

```sh
git clone https://github.com/murongg/SubLane.git
cd SubLane
make setup
make dev
```

Open http://127.0.0.1:5173. Run `make check` to test and build the project.

## Documentation

Follow the [step-by-step tutorial](https://sublane-website.vercel.app/docs/guide/installation). Guides are maintained in the [documentation repository](https://github.com/murongg/sublane-website).

- [Deployment](https://sublane-website.vercel.app/docs/deployment) · [Backup and restore](https://sublane-website.vercel.app/docs/backup)
- [Providers](https://sublane-website.vercel.app/docs/providers) · [API keys](https://sublane-website.vercel.app/docs/api-keys) · [Account groups](https://sublane-website.vercel.app/docs/groups)
- [Development](https://sublane-website.vercel.app/docs/development) · [Architecture](https://sublane-website.vercel.app/docs/architecture)
- [Releases](https://sublane-website.vercel.app/docs/releases) · [Changelog](CHANGELOG.md)
- [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md)

## License

[AGPL-3.0-only](LICENSE). See [third-party notices](THIRD_PARTY_NOTICES.md) for dependencies and adapted components.
