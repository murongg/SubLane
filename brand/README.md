# SubLane brand assets

[简体中文](README.zh-CN.md)

This directory keeps the assets used by repository documentation, color values, and the sources required to regenerate the full brand kit.

## Kept in the repository

| Files | Purpose |
| --- | --- |
| `logos/lockup-black.svg`, `logos/lockup-white.svg` | The horizontal logo used in both READMEs |
| `social/github-dark.svg`, `social/github-dark.png` | The selected GitHub social preview |
| `source/Inter.ttf`, `source/OFL.txt` | The original font and its license for reproducible exports |
| `tokens.json` | Brand colors, typography, and spacing rules |

The canonical symbol remains [`docs/assets/logo.svg`](../docs/assets/logo.svg), with a light-fill counterpart beside it. Its two paths must stay unchanged. The sole maintained generator lives in `tools/brand/`.

## Usage

- Use the black lockup on light backgrounds and the white lockup on dark backgrounds.
- Preserve proportions and keep at least one quarter of the mark canvas clear in ordinary layouts.
- Use the symbol at 24px or larger in ordinary interfaces. Export the dedicated favicon files for smaller browser sizes.
- The wordmark uses outlined Inter 650, optical size 32. The application retains its platform sans-serif font.
- Keep primary branding monochrome; use semantic colors only for status.

SubLane assets follow the repository's [AGPL-3.0-only license](../LICENSE). The included Inter font retains its separate [SIL OFL 1.1 license](source/OFL.txt).

## Generate the full kit

From the repository root:

```sh
make brand
```

The complete set is written to **`dist/brand/`**, which is ignored by Git. The command also refreshes only the two README lockups, the selected social preview pair, and `tokens.json` in this directory.

Open `dist/brand/preview.html` to browse all exports. The export includes PNG variants, avatars, app/browser icons, banners, social cards, presentation covers, wallpapers, stickers, usage guides, and a portable generator. Distribution guide templates live in `tools/brand/guide.md` and `guide.zh-CN.md`.

Keep complete export archives as release attachments or separate downloads. Do not commit `dist/` or copy its generated files back into this directory. No release attachment is published by the build command.

The generator's integration test uses synthetic assets:

```sh
pnpm --dir tools/brand test
```
