# Contributing to SubLane

SubLane is at the foundation stage. Read [PRODUCT.md](PRODUCT.md) and [the roadmap](docs/roadmap.md) before expanding scope. Small improvements to a reliable Codex-first workflow are more useful than broad provider coverage at this stage.

## Local workflow

1. Install Go 1.26+, Node.js 24, pnpm 9.12.2, and jq.
2. Run `make setup`, then `make dev`.
3. Write a failing test for a behavior change; implement the change and rerun the test.
4. Run `make check` before submitting your work.
5. Update relevant documentation and both UI languages when behavior changes.

See [development notes](docs/development.md) for commands and troubleshooting.

## Change boundaries

- Keep changes focused on one problem and describe the concrete before/after behavior.
- Use synthetic fixtures. Do not include auth files, subscription identifiers, real request payloads, or personal information in tests, screenshots, logs, or issue reports.
- Add SQLite migrations rather than rewriting migrations that have shipped.
- Keep provider SDK types behind their integration boundary.
- Avoid adding dependencies for small tasks already handled by the standard library or installed packages.
- Document validation actually performed, plus any untested client or platform.

## Interface changes

Follow [DESIGN.md](DESIGN.md). Check loading, empty, error, and success states where applicable. Verify keyboard navigation, narrow screens, light/dark themes, and English/Chinese copy. Use the existing semantic tokens and component conventions.

## Pull requests

Explain the problem, resulting behavior, relevant tradeoffs, and verification. Include UI screenshots for visible changes and migration notes when persisted data changes. Do not include unrelated cleanup or local planning documents.

No contribution agreement or public support process is configured yet. Contributions must be compatible with the project's AGPL-3.0-only license and preserve third-party attribution.
