# Third-party notices

SubLane is licensed under AGPL-3.0-only. Third-party components retain their own licenses.

## Shadcn Admin

- Project: https://github.com/satnaing/shadcn-admin
- Revision: `e16c87f213a5ba5e45964e9b67c792105ec74d26`
- Copyright: (c) 2024 Sat Naing
- License: MIT; full text in `licenses/shadcn-admin.txt`.
- Adapted files: `web/src/components/ui/`, `web/src/hooks/use-mobile.ts`, and the `cn` function in `web/src/lib/cn.ts`. The application shell and `Session` menu follow the upstream header, sidebar, and NavUser patterns.
- Changes: component filename conventions, local import paths, bilingual accessibility labels, menu checkmarks and touch targets, and removal of the unused randomized sidebar skeleton. SubLane uses its own administrator session and business routes.

Go and JavaScript dependencies are pinned in `go.mod`, `go.sum`, `web/package.json`, and `web/pnpm-lock.yaml`. Their licenses remain available in their respective source packages. CLIProxyAPI is a planned integration and is not linked into this foundation build.

The Radix dropdown menu is pinned to 2.1.16 alongside Dialog 1.1.15 so they share focus-scope and dismissable-layer dependencies. Keep those primitives aligned when upgrading and verify keyboard focus in the account menu inside the mobile navigation sheet.

## Inter

- Project: https://github.com/rsms/inter
- Copyright: (c) 2020 The Inter Project Authors
- License: SIL Open Font License 1.1; full text in `brand/source/OFL.txt`.
- Font source: Google Fonts, revision `1edf95b4328bc5997ca93d2c0c7205272ec7347f`, `ofl/inter/Inter[opsz,wght].ttf`.
- The unmodified variable font is included as `brand/source/Inter.ttf`. Brand graphics use outlined glyphs generated at the documented weights and optical size.
- Brand export tooling is isolated in `tools/brand`; it does not add a font or graphics renderer to the application runtime.
