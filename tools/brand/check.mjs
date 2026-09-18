import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFileSync, existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { resolve } from 'node:path'
import { Resvg } from '@resvg/resvg-js'

const root = fileURLToPath(new URL('../../', import.meta.url))
const standalone = process.argv.includes('--standalone')
const pack = standalone ? root : resolve(root, 'dist/brand')
assert.ok(existsSync(resolve(pack, 'manifest.json')), 'Build the brand assets before checking them.')
const manifest = JSON.parse(readFileSync(resolve(pack, 'manifest.json'), 'utf8'))
const source = readFileSync(resolve(root, standalone ? 'source/mark.svg' : 'docs/assets/logo.svg'), 'utf8')
const approved = [...source.matchAll(/<path d="([^"]+)"/g)].map((m) => m[1])
assert.equal(approved.length, 2)
assert.equal(manifest.markSha256, createHash('sha256').update(approved.join('\n')).digest('hex'))

for (const item of manifest.assets) {
  const file = resolve(pack, item.path)
  assert.ok(existsSync(file), item.path)
  const data = readFileSync(file)
  assert.equal(createHash('sha256').update(data).digest('hex'), item.sha256, item.path + ': checksum')
  if (item.format === 'svg') {
    const svg = data.toString()
    assert.ok(!/<image|<script|<foreignObject|<text[\s>]/.test(svg), item.path + ': must be pure vectors with outlined type')
    assert.ok(!/(?:href|src)=["']https?:/.test(svg), item.path + ': external dependency')
    if (item.mark) for (const d of approved) assert.ok(svg.includes('d="' + d + '"'), item.path + ': changed mark')
    const rendered = new Resvg(svg).render()
    assert.equal(rendered.width, item.width)
    assert.equal(rendered.height, item.height)
    assert.equal(rendered.pixels[3], item.transparent ? 0 : 255, item.path + ': background opacity')
  }
  if (item.format === 'png') {
    assert.equal(data.subarray(1, 4).toString(), 'PNG', item.path)
    assert.equal(data.readUInt32BE(16), item.width, item.path + ': width')
    assert.equal(data.readUInt32BE(20), item.height, item.path + ': height')
    if (item.path.startsWith('social/github-')) assert.ok(data.length < 1_000_000, 'GitHub upload exceeds 1 MB')
  }
}

const ico = readFileSync(resolve(pack, 'icons/favicon.ico'))
assert.equal(ico.readUInt16LE(2), 1)
assert.equal(ico.readUInt16LE(4), 4)
for (let i = 0; i < 4; i++) {
  const offset = ico.readUInt32LE(6 + i * 16 + 12)
  assert.equal(ico.subarray(offset + 1, offset + 4).toString(), 'PNG')
}
console.log('Verified ' + manifest.assets.length + ' assets: approved mark, outlined type, dimensions, alpha, checksums, SVG rendering, ICO, and GitHub size limit.')
