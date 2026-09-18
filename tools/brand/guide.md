# SubLane brand materials

[简体中文](README.zh-CN.md)

A complete digital asset set based on the approved original mark: two offset lane segments forming an S. The symbol's two vector paths are preserved exactly across the set.

Open `preview.html` locally to browse and filter the artwork. Use `overview.png` for a quick view of the system.

## Pick the right file

| Use | Recommended file | Size / format |
| --- | --- | --- |
| Standalone symbol | `logos/mark-black.svg` or `mark-white.svg` | Transparent SVG + 1024px PNG |
| Horizontal logo | `logos/lockup-black.svg` or `lockup-white.svg` | Transparent SVG + 720 × 176 PNG |
| Stacked logo | `logos/stacked-black.svg` or `stacked-white.svg` | Transparent SVG + 640 × 600 PNG |
| Wordmark only | `logos/wordmark-black.svg` or `wordmark-white.svg` | Transparent SVG + 960 × 240 PNG |
| GitHub / community avatar | `icons/avatar-dark.png` or `avatar-light.png` | 1024 × 1024; safe for circular cropping |
| Rounded app icon artwork | `icons/app-dark.svg` or `app-light.svg` | SVG + transparent 1024px PNG |
| Browser favicon | `icons/favicon.svg` and `icons/favicon.ico` | Adaptive SVG; ICO contains 16/32/48/64px |
| Raster favicon | `icons/favicon-16.png` through `favicon-256.png` | 16/32/48/64/128/256px |
| Apple touch icon | `icons/apple-touch-icon.png` | Opaque 180 × 180 PNG |
| Android / web app icon | `icons/android-192.png`, `icons/android-512.png` | Opaque square PNG |
| README header | `banners/readme-light.svg` or `readme-dark.svg` | SVG + 1600 × 420 PNG |
| Project profile banner | `banners/profile-light.png` or `profile-dark.png` | SVG + 1500 × 500 PNG |
| GitHub social preview | `social/github-light.png` or `github-dark.png` | SVG + 1280 × 640 PNG, under 1 MB |
| Open Graph card | `social/opengraph-light.png` or `opengraph-dark.png` | SVG + 1200 × 630 PNG |
| Square announcement | `social/announcement-light.png` or `announcement-dark.png` | SVG + 1080 × 1080 PNG |
| Presentation cover | `presentation/cover-light.png` or `cover-dark.png` | SVG + 1920 × 1080 PNG |
| Desktop wallpaper | `wallpapers/desktop-light.png` or `desktop-dark.png` | SVG + 3840 × 2160 PNG |
| Circular sticker artwork | `stickers/round-light.svg` or `round-dark.svg` | SVG + transparent 1024px PNG |
| Clear-space diagram | `guidelines/clear-space.svg` | SVG + PNG |
| Color values | `tokens.json` | Brand neutrals and semantic colors |

`black` and `white` describe the artwork color; those logo files have transparent backgrounds. `light` and `dark` describe the complete surface treatment. Use the white logo on dark surfaces and the black logo on light surfaces.

## Logo rules

- Keep the original geometry. Do not rotate, stretch, add gradients, or alter the two path segments.
- Leave at least one quarter of the square SVG canvas clear on every side in ordinary layouts. Precomposed lockups and compact icon containers have their own spacing.
- Use the standalone symbol at 24px or larger in ordinary interfaces. Use the supplied favicon files for the 16px browser case.
- Scale the horizontal and stacked lockups as a whole; do not independently scale the wordmark.
- The primary artwork is monochrome. Green, amber, red, and blue are for success, warnings, errors, and information, as recorded in the application design system.
- Use opaque social cards for predictable contrast in link previews. Transparent logos are for placement onto a known surface.

## Typography

The wordmark uses Inter at weight 650, optical size 32, with restrained tracking. All text in exported artwork is converted to vector paths; another computer does not need the font installed to display it correctly.

The variable font and its SIL Open Font License are included in `source/`. The font is unmodified. The application UI continues to use its existing platform sans-serif stack; this asset set does not change the application's typography.

## Copy and current product status

The principal descriptor is “A minimal subscription gateway for internal teams.” The supporting line is “Open source · Codex first.”

The square announcement says “Building SubLane” and “Foundation stage.” It does not claim that account authorization or model forwarding is already available. Update product-stage copy deliberately when those capabilities ship.

## README

The repository's English and Chinese READMEs use the transparent horizontal logo pair with compact license, stack, and project-stage badges. The full-width README banner pair remains available as an alternative. When copying the example into another directory, update both relative paths.

```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="brand/logos/lockup-white.svg">
  <img src="brand/logos/lockup-black.svg" alt="SubLane" width="420">
</picture>
```

## Browser icons

Copy the desired icon files into the application's public directory before adding these declarations. The paths below assume the copied files are served from the web root.

```html
<link rel="icon" type="image/svg+xml" href="/favicon.svg">
<link rel="icon" type="image/x-icon" href="/favicon.ico">
<link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon.png">
```

The adaptive SVG follows `prefers-color-scheme`. The ICO fallback uses a white mark on a dark tile. Opaque square app icons are supplied for platform masking; the rounded app artwork is a separate presentation asset.

## GitHub social preview

Use `social/github-dark.png` or its light counterpart in the repository's **Settings → Social preview**. Generating these files does not upload them or change repository settings.

The export follows [GitHub's documented recommendation](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/customizing-your-repositorys-social-media-preview): 1280 × 640 pixels and a PNG under 1 MB.

## Editing and rebuilding

The project's canonical symbol remains `docs/assets/logo.svg`. A portable copy is included here as `source/mark.svg`. The artwork generator reads the canonical file, shapes text using the included Inter font, and exports pure SVG paths and PNGs using resvg.

From the project root:

```sh
make brand
```

This installs the isolated tools under `tools/brand`, generates the complete kit in Git-ignored `dist/brand/`, and checks mark consistency, text bounds, PNG dimensions, SVG rendering, transparency, ICO frames, checksums, and GitHub upload size. Only the maintained README lockups, selected social preview, and color values are refreshed in `brand/`. These dependencies are not included in the Go service or the frontend runtime.

To change copy or compositions, edit `tools/brand/build.mjs` and rebuild. SVG paths remain directly editable in vector design tools; outlined copy is easiest to change through the generator.

The downloadable bundle also includes a portable generator in `source/generator/`. From that folder, run `pnpm install --frozen-lockfile`, `pnpm build`, and `pnpm check`. It reads the bundled `source/mark.svg` and font, so it does not require the application repository. Node.js 22.12+ and pnpm 9.12.2 are required. Keep the font license with the source.

`manifest.json` lists the exported files, dimensions, transparency, and SHA-256 checksums. Sticker files are artwork, not printer-specific production files; obtain bleed, cut-line, and color-profile requirements from the chosen printer.

## Sources and license

- Symbol: the user-approved original SubLane SVG, preserved without geometry changes.
- Composition, lockups, icons, and exports: deterministic vector construction from that SVG.
- Font: Inter, from Google Fonts revision `1edf95b4328bc5997ca93d2c0c7205272ec7347f`; see `source/OFL.txt`.
- SubLane artwork and source files follow the repository's AGPL-3.0-only license in `LICENSE`. The included font retains its separate SIL OFL 1.1 license.
