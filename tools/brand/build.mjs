import { readFileSync, writeFileSync, mkdirSync, copyFileSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createHash } from 'node:crypto'
import * as fontkit from 'fontkit'
import { Resvg } from '@resvg/resvg-js'

const root = fileURLToPath(new URL('../../', import.meta.url))
const standalone = process.argv.includes('--standalone')
const out = standalone ? root : resolve(root, 'dist/brand')
const source = resolve(root, standalone ? 'source' : 'brand/source')
const approved = readFileSync(resolve(root, standalone ? 'source/mark.svg' : 'docs/assets/logo.svg'), 'utf8')
const paths = [...approved.matchAll(/<path d="([^"]+)"/g)].map((m) => m[1])
if (paths.length !== 2) throw new Error('The approved mark must contain exactly two paths.')
const markPaths = paths.map((d) => '<path d="' + d + '"/>').join('')
const baseFont = fontkit.openSync(resolve(source, 'Inter.ttf'))
const fonts = new Map()
const assets = []
const drawings = new Map()
let textBounds = []
const colors = {
  ink: '#171717', paper: '#FFFFFF', canvas: '#F7F7F7', night: '#101010',
  surface: '#191919', gray: '#616161', silver: '#A3A3A3', line: '#E5E5E5',
  success: '#15803D', warning: '#92400E', error: '#B91C1C', info: '#1D4ED8',
}
const esc = (s) => String(s).replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('"', '&quot;')
const num = (n) => Number(n.toFixed(4))

function font(weight) {
  if (!fonts.has(weight)) fonts.set(weight, baseFont.getVariation({ wght: weight, opsz: 32 }))
  return fonts.get(weight)
}

function line(value, size, weight = 500, tracking = -0.02) {
  const face = font(weight)
  const run = face.layout(value)
  const scale = size / face.unitsPerEm
  let cursor = 0
  const bounds = { minX: Infinity, maxX: -Infinity, minY: Infinity, maxY: -Infinity }
  const shapes = run.glyphs.map((glyph, index) => {
    const p = run.positions[index]
    const x = cursor + p.xOffset * scale
    const box = glyph.bbox
    if (Number.isFinite(box.minX)) {
      bounds.minX = Math.min(bounds.minX, x + box.minX * scale)
      bounds.maxX = Math.max(bounds.maxX, x + box.maxX * scale)
      bounds.minY = Math.min(bounds.minY, -p.yOffset * scale - box.maxY * scale)
      bounds.maxY = Math.max(bounds.maxY, -p.yOffset * scale - box.minY * scale)
    }
    cursor += p.xAdvance * scale + tracking * size
    return '<path transform="translate(' + num(x) + ' ' + num(-p.yOffset * scale) + ') scale(' + num(scale) + ' ' + num(-scale) + ')" d="' + glyph.path.toSVG() + '"/>'
  }).join('')
  return { width: cursor - tracking * size, shapes, bounds }
}

function text(value, x, y, size, weight = 500, fill = colors.ink, align = 'left', tracking = -0.02) {
  const shaped = line(value, size, weight, tracking)
  const offset = align === 'center' ? shaped.width / 2 : align === 'right' ? shaped.width : 0
  textBounds.push({ value, minX: x - offset + shaped.bounds.minX, maxX: x - offset + shaped.bounds.maxX, minY: y + shaped.bounds.minY, maxY: y + shaped.bounds.maxY })
  return '<g aria-label="' + esc(value) + '" fill="' + fill + '" transform="translate(' + num(x - offset) + ' ' + num(y) + ')">' + shaped.shapes + '</g>'
}

function rect(x, y, w, h, fill, radius = 0) {
  return '<rect x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '" rx="' + radius + '" fill="' + fill + '"/>'
}

function mark(x, y, size, fill = colors.ink) {
  return '<g fill="' + fill + '" transform="translate(' + num(x) + ' ' + num(y) + ') scale(' + num(size / 256) + ')">' + markPaths + '</g>'
}

function lockup(x, y, size, fill = colors.ink) {
  return mark(x, y, size, fill) + text('SubLane', x + size * 1.19, y + size * 0.773, size * 0.80, 650, fill, 'left', -0.025)
}

function document(w, h, body, title) {
  return '<svg xmlns="http://www.w3.org/2000/svg" width="' + w + '" height="' + h + '" viewBox="0 0 ' + w + ' ' + h + '" role="img"><title>' + esc(title) + '</title>' + body + '</svg>\n'
}

function record(path, data, metadata) {
  const file = resolve(out, path)
  mkdirSync(dirname(file), { recursive: true })
  writeFileSync(file, data)
  assets.push({ path, bytes: Buffer.byteLength(data), sha256: createHash('sha256').update(data).digest('hex'), ...metadata })
}

function asset(name, w, h, body, options = {}) {
  const { transparent = false, hasMark = true, png = true, title = 'SubLane' } = options
  for (const box of textBounds) {
    if (box.minX < 0 || box.minY < 0 || box.maxX > w || box.maxY > h) throw new Error(name + ': clipped text: ' + box.value)
  }
  textBounds = []
  const svg = document(w, h, body, title)
  drawings.set(name, { w, h, body })
  record(name + '.svg', svg, { format: 'svg', width: w, height: h, transparent, mark: hasMark })
  if (png) record(name + '.png', new Resvg(svg).render().asPng(), { format: 'png', width: w, height: h, transparent })
}

// Every logo appearance reuses the canonical paths, including tiny raster exports.
for (const [theme, fg] of [['black', colors.ink], ['white', colors.paper]]) {
  asset('logos/mark-' + theme, 1024, 1024, mark(0, 0, 1024, fg), { transparent: true })
  asset('logos/lockup-' + theme, 720, 176, lockup(16, 16, 144, fg), { transparent: true })
  asset('logos/stacked-' + theme, 640, 600, mark(176, 44, 288, fg) + text('SubLane', 320, 476, 118, 650, fg, 'center', -0.025), { transparent: true })
  asset('logos/wordmark-' + theme, 960, 240, text('SubLane', 480, 183, 206, 650, fg, 'center', -0.025), { transparent: true, hasMark: false })
}

for (const dark of [false, true]) {
  const theme = dark ? 'dark' : 'light'
  const bg = dark ? colors.night : colors.paper
  const fg = dark ? colors.paper : colors.ink
  const muted = dark ? colors.silver : colors.gray
  asset('icons/avatar-' + theme, 1024, 1024, rect(0, 0, 1024, 1024, bg) + mark(192, 192, 640, fg))
  asset('icons/app-' + theme, 1024, 1024, rect(0, 0, 1024, 1024, bg, 224) + mark(192, 192, 640, fg), { transparent: true })

  asset('banners/readme-' + theme, 1600, 420,
    rect(0, 0, 1600, 420, bg) + lockup(64, 56, 216, fg) +
    text('A minimal subscription gateway for internal teams.', 84, 346, 38, 450, muted) +
    text('Codex first.', 1516, 346, 27, 550, muted, 'right'))

  asset('banners/profile-' + theme, 1500, 500,
    rect(0, 0, 1500, 500, bg) + lockup(92, 86, 208, fg) +
    text('Subscription access. One team gateway.', 108, 389, 34, 450, muted) +
    text('github.com/murongg/SubLane', 1392, 452, 22, 450, muted, 'right'))

  for (const [kind, w, h] of [['github', 1280, 640], ['opengraph', 1200, 630]]) {
    asset('social/' + kind + '-' + theme, w, h,
      rect(0, 0, w, h, bg) + lockup(60, 60, 176, fg) +
      text('A minimal subscription gateway', 72, 369, 47, 450, fg) +
      text('for internal teams.', 72, 431, 47, 450, fg) +
      text('Open source · Codex first', 72, h - 65, 23, 450, muted) +
      text('github.com/murongg/SubLane', w - 72, h - 65, 23, 450, muted, 'right'))
  }

  asset('social/announcement-' + theme, 1080, 1080,
    rect(0, 0, 1080, 1080, bg) + lockup(64, 64, 112, fg) +
    text('Building', 80, 430, 132, 600, fg, 'left', -0.035) +
    text('SubLane.', 80, 575, 132, 600, fg, 'left', -0.035) +
    text('A minimal subscription gateway', 80, 761, 42, 450, muted) +
    text('for internal teams.', 80, 821, 42, 450, muted) +
    text('Foundation stage', 80, 976, 25, 500, fg) +
    text('Open source', 1000, 976, 25, 450, muted, 'right'))

  asset('presentation/cover-' + theme, 1920, 1080,
    rect(0, 0, 1920, 1080, bg) + lockup(128, 132, 304, fg) +
    text('A minimal subscription gateway', 150, 705, 72, 450, fg) +
    text('for internal teams.', 150, 800, 72, 450, fg) +
    text('github.com/murongg/SubLane', 150, 985, 30, 450, muted) +
    text('Project overview', 1770, 985, 30, 450, muted, 'right'))

  asset('wallpapers/desktop-' + theme, 3840, 2160,
    rect(0, 0, 3840, 2160, bg) + mark(1656, 648, 528, fg) +
    text('SubLane', 1920, 1390, 216, 650, fg, 'center', -0.025) +
    text('github.com/murongg/SubLane', 1920, 2056, 32, 450, muted, 'center'))

  asset('stickers/round-' + theme, 1024, 1024,
    '<circle cx="512" cy="512" r="488" fill="' + bg + '"/>' +
    mark(272, 176, 480, fg) + text('SubLane', 512, 790, 108, 650, fg, 'center', -0.025),
    { transparent: true })
}

const tile = rect(0, 0, 256, 256, colors.ink, 48) + mark(16, 16, 224, colors.paper)
const icoFrames = []
for (const size of [16, 32, 48, 64, 128, 256]) {
  const png = new Resvg(document(256, 256, tile, 'SubLane'), { fitTo: { mode: 'width', value: size } }).render().asPng()
  record('icons/favicon-' + size + '.png', png, { format: 'png', width: size, height: size, transparent: true })
  if (size <= 64) icoFrames.push({ size, png })
}
for (const size of [180, 192, 512]) {
  const body = rect(0, 0, size, size, colors.ink) + mark(size * 0.18, size * 0.18, size * 0.64, colors.paper)
  const name = size === 180 ? 'icons/apple-touch-icon' : 'icons/android-' + size
  record(name + '.png', new Resvg(document(size, size, body, 'SubLane')).render().asPng(), { format: 'png', width: size, height: size, transparent: false })
}
asset('icons/favicon', 256, 256,
  '<style>:root{color:#171717}@media(prefers-color-scheme:dark){:root{color:#FFFFFF}}</style>' + mark(0, 0, 256, 'currentColor'),
  { transparent: true, png: false })

const icoHeader = Buffer.alloc(6 + 16 * icoFrames.length)
icoHeader.writeUInt16LE(1, 2)
icoHeader.writeUInt16LE(icoFrames.length, 4)
let icoOffset = icoHeader.length
icoFrames.forEach(({ size, png }, index) => {
  const at = 6 + index * 16
  icoHeader[at] = size
  icoHeader[at + 1] = size
  icoHeader.writeUInt16LE(1, at + 4)
  icoHeader.writeUInt16LE(32, at + 6)
  icoHeader.writeUInt32LE(png.length, at + 8)
  icoHeader.writeUInt32LE(icoOffset, at + 12)
  icoOffset += png.length
})
record('icons/favicon.ico', Buffer.concat([icoHeader, ...icoFrames.map((f) => f.png)]), { format: 'ico', sizes: icoFrames.map((f) => f.size) })

const labels = {
  'logos': 'Logo system', 'icons': 'Avatars and app icons', 'banners': 'Headers and banners',
  'social': 'Social graphics', 'presentation': 'Presentation covers', 'wallpapers': 'Desktop wallpapers', 'stickers': 'Sticker artwork',
}

function inset(name, x, y, w, h) {
  const d = drawings.get(name)
  return '<svg x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '" viewBox="0 0 ' + d.w + ' ' + d.h + '">' + d.body + '</svg>'
}

let board = rect(0, 0, 2400, 1740, '#EEEEEE')
board += text('SubLane', 40, 57, 34, 650) + text('Brand materials / Original mark', 2360, 57, 24, 450, colors.gray, 'right')
board += rect(40, 92, 1460, 500, colors.night) + lockup(132, 186, 256, colors.paper)
board += text('A minimal subscription gateway for internal teams.', 152, 523, 30, 450, colors.silver)
board += rect(1520, 92, 840, 500, colors.paper)
board += text('One mark. Clear at every size.', 1564, 150, 28, 500)
board += mark(1590, 240, 240) + mark(1900, 288, 160) + mark(2180, 344, 80)
board += text('Original SVG geometry', 1564, 547, 23, 450, colors.gray)
board += rect(40, 612, 760, 466, colors.paper) + text('Profile and application', 76, 668, 27, 500)
board += inset('icons/app-dark', 94, 723, 216, 216) + rect(364, 721, 220, 220, '#EEEEEE', 49) + inset('icons/app-light', 366, 723, 216, 216)
board += text('Light / dark · Circle-safe avatars included', 76, 1030, 21, 450, colors.gray)
board += inset('social/github-dark', 820, 612, 960, 466)
board += rect(1800, 612, 560, 466, colors.paper) + text('Wordmark', 1840, 668, 27, 500)
board += text('SubLane', 1840, 825, 106, 650, colors.ink, 'left', -0.025)
board += text('Inter 650 · Outlined for portability', 1840, 1019, 21, 450, colors.gray)
board += rect(40, 1098, 1510, 300, colors.paper) + inset('banners/readme-light', 64, 1120, 1462, 250)
board += rect(1570, 1098, 790, 300, colors.night)
board += text('Monochrome by default.', 1614, 1160, 32, 550, colors.paper)
board += text('Color only when it carries meaning.', 1614, 1210, 25, 450, colors.silver)
for (const [i, value] of ['#FFFFFF', '#A3A3A3', '#616161', '#171717'].entries()) board += rect(1614 + i * 175, 1252, 151, 62, value, 4)
for (const [i, [label, fill]] of [['Success', '#86EFAC'], ['Warning', '#FCD34D'], ['Error', '#FCA5A5'], ['Info', '#93C5FD']].entries()) {
  board += rect(1614 + i * 175, 1338, 16, 16, fill, 3) + text(label, 1640 + i * 175, 1355, 22, 450, colors.silver)
}
board += rect(40, 1418, 760, 278, colors.night) + text('Browser-ready', 76, 1474, 27, 500, colors.paper)
for (const [i, size] of [32, 48, 64, 96].entries()) board += mark(90 + i * 166, 1527 + (96 - size) / 2, size, colors.paper)
board += text('SVG · PNG · ICO', 76, 1651, 21, 450, colors.silver)
board += rect(820, 1418, 1540, 278, colors.paper) + lockup(884, 1480, 136)
board += text('github.com/murongg/SubLane', 2320, 1615, 27, 450, colors.gray, 'right')
board += text('Open source. Codex first.', 885, 1652, 25, 450, colors.gray)
asset('overview', 2400, 1740, board, { title: 'SubLane brand materials overview' })

asset('guidelines/clear-space', 960, 640,
  rect(0, 0, 960, 640, colors.paper) +
  '<path d="M80 80H632V632H80Z" fill="none" stroke="#A3A3A3" stroke-width="2" stroke-dasharray="8 8"/>' +
  mark(172, 172, 368) +
  text('Clear space', 56, 57, 30, 550) + text('At least 1/4', 675, 272, 27, 500) +
  text('of the mark canvas', 675, 312, 24, 450, colors.gray) + text('on each side.', 675, 350, 24, 450, colors.gray))

const manifest = {
  name: 'SubLane brand materials',
  markSource: 'source/mark.svg',
  projectMarkSource: 'docs/assets/logo.svg',
  markSha256: createHash('sha256').update(paths.join('\n')).digest('hex'),
  font: { name: 'Inter', weights: [450, 500, 550, 600, 650], opticalSize: 32, source: 'source/Inter.ttf', license: 'source/OFL.txt', revision: '1edf95b4328bc5997ca93d2c0c7205272ec7347f' },
  assets,
}
writeFileSync(resolve(out, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n')
writeFileSync(resolve(out, 'tokens.json'), JSON.stringify({ brand: 'SubLane', colors, semantic: { light: { success: '#15803D', warning: '#92400E', error: '#B91C1C', info: '#1D4ED8' }, dark: { success: '#86EFAC', warning: '#FCD34D', error: '#FCA5A5', info: '#93C5FD' } }, logo: { clearSpace: '0.25 × canvas', minimumDigitalSize: 24, faviconException: 16 }, typography: { wordmark: 'Inter 650', opticalSize: 32, tracking: '-0.025em', exportedAs: 'paths' } }, null, 2) + '\n')
if (!standalone) {
  mkdirSync(resolve(out, 'source'), { recursive: true })
  copyFileSync(resolve(root, 'docs/assets/logo.svg'), resolve(out, 'source/mark.svg'))
  copyFileSync(resolve(root, 'LICENSE'), resolve(out, 'LICENSE'))
  for (const name of ['Inter.ttf', 'OFL.txt']) copyFileSync(resolve(source, name), resolve(out, 'source', name))
  copyFileSync(resolve(root, 'tools/brand/guide.md'), resolve(out, 'README.md'))
  copyFileSync(resolve(root, 'tools/brand/guide.zh-CN.md'), resolve(out, 'README.zh-CN.md'))
  mkdirSync(resolve(out, 'source/generator'), { recursive: true })
  for (const name of ['build.mjs', 'check.mjs', 'pnpm-lock.yaml']) {
    copyFileSync(resolve(root, 'tools/brand', name), resolve(out, 'source/generator', name))
  }
  const packageJSON = JSON.parse(readFileSync(resolve(root, 'tools/brand/package.json'), 'utf8'))
  packageJSON.scripts = { build: 'node build.mjs --standalone', check: 'node check.mjs --standalone' }
  writeFileSync(resolve(out, 'source/generator/package.json'), JSON.stringify(packageJSON, null, 2) + '\n')
}

const cards = [...drawings.entries()].filter(([name]) => !name.startsWith('guidelines/') && name !== 'overview').map(([name, d]) => {
  const group = name.split('/')[0]
  const white = name.endsWith('-white')
  return '<article data-group="' + group + '"><div class="art' + (white ? ' dark' : '') + '"><img src="' + name + '.svg" alt="' + esc(name.replaceAll('/', ' / ').replaceAll('-', ' ')) + '" loading="lazy" width="' + d.w + '" height="' + d.h + '"></div><div class="meta"><div><strong>' + esc(name.split('/')[1].replaceAll('-', ' ')) + '</strong><small>' + d.w + ' × ' + d.h + '</small></div><div class="links"><a href="' + name + '.svg" download>SVG</a>' + (name !== 'icons/favicon' ? '<a href="' + name + '.png" download>PNG</a>' : '') + '</div></div></article>'
}).join('\n')
const html = '<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>SubLane · Brand materials</title><link rel="icon" type="image/svg+xml" href="icons/favicon.svg"><style>' +
  '*{box-sizing:border-box}body{margin:0;background:#f7f7f7;color:#171717;font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}header,main{max-width:1440px;margin:auto;padding:32px}header{padding-top:48px}h1{font-size:32px;letter-spacing:-.04em;margin:0 0 10px}p{color:#616161;margin:0}nav{display:flex;flex-wrap:wrap;gap:8px;margin:28px 0 0}button,a{font:inherit;color:inherit}button{border:1px solid #ccc;background:transparent;border-radius:6px;padding:7px 12px;cursor:pointer}button[aria-pressed=true]{background:#171717;color:#fff;border-color:#171717}button:focus-visible,a:focus-visible{outline:2px solid #616161;outline-offset:3px}.intro{display:flex;flex-wrap:wrap;gap:12px;justify-content:space-between;align-items:flex-start}.intro a{font-size:14px;text-underline-offset:4px}.grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:28px}article{min-width:0}.art{height:300px;background:#fff;border:1px solid #e5e5e5;display:flex;align-items:center;justify-content:center;overflow:hidden}.art.dark{background:#101010}.art img{display:block;max-width:100%;max-height:100%;width:auto;height:auto;object-fit:contain}.meta{display:flex;justify-content:space-between;gap:16px;padding:14px 0}.meta strong{display:block;font-size:14px;font-weight:500;text-transform:capitalize}.meta small{color:#616161}.links{display:flex;gap:14px;align-items:center}.links a{font-size:13px;text-underline-offset:3px}[hidden]{display:none!important}.count{font-size:13px;margin-top:12px}@media(max-width:660px){header,main{padding:24px 16px}.grid{grid-template-columns:1fr;gap:20px}.art{height:260px}h1{font-size:28px}}@media(pointer:coarse){button,a{min-height:44px;display:inline-flex;align-items:center}}</style></head><body>' +
  '<header><div class="intro"><div><h1>SubLane brand materials</h1><p>The original mark. Ready for your project, profiles, and presentations.</p></div><div class="links"><a href="icons/favicon.ico" download>Favicon ICO</a><a href="README.md">Usage guide</a></div></div><nav aria-label="Asset categories"><button type="button" data-filter="all" aria-pressed="true">All assets</button>' +
  Object.entries(labels).map(([key, label]) => '<button type="button" data-filter="' + key + '" aria-pressed="false">' + label + '</button>').join('') +
  '</nav><p class="count" aria-live="polite" id="count"></p></header><main><div class="grid">' + cards +
  '</div></main><script>const buttons=document.querySelectorAll("[data-filter]");const cards=document.querySelectorAll("[data-group]");function filter(value){let count=0;cards.forEach(card=>{card.hidden=value!=="all"&&card.dataset.group!==value;if(!card.hidden)count++});buttons.forEach(button=>button.setAttribute("aria-pressed",String(button.dataset.filter===value)));document.getElementById("count").textContent=count+" vector designs · PNG and ICO exports included"}buttons.forEach(button=>button.addEventListener("click",()=>filter(button.dataset.filter)));filter("all")</script></body></html>'
writeFileSync(resolve(out, 'preview.html'), html)
if (!standalone) {
  // Only promote the assets used by repository documentation. Full exports stay in dist.
  for (const name of ['logos/lockup-black.svg', 'logos/lockup-white.svg', 'social/github-dark.svg', 'social/github-dark.png', 'tokens.json']) {
    const target = resolve(root, 'brand', name)
    mkdirSync(dirname(target), { recursive: true })
    copyFileSync(resolve(out, name), target)
  }
}
console.log('Built ' + assets.length + ' assets from the two approved logo paths. Wordmarks and graphic text are outlined.')
