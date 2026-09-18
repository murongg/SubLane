import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const tooling = fileURLToPath(new URL('./', import.meta.url))
const project = resolve(tooling, '../..')

test('exports a complete portable kit without overwriting maintained source documentation', { timeout: 120_000 }, (t) => {
  const fixture = mkdtempSync(resolve(tmpdir(), 'synthetic-brand-'))
  t.after(() => rmSync(fixture, { recursive: true, force: true }))
  const tool = resolve(fixture, 'tools/brand')
  const source = resolve(fixture, 'brand/source')
  for (const path of [tool, source, resolve(fixture, 'docs/assets')]) mkdirSync(path, { recursive: true })
  for (const name of ['build.mjs', 'check.mjs', 'package.json', 'pnpm-lock.yaml']) copyFileSync(resolve(tooling, name), resolve(tool, name))
  symlinkSync(resolve(tooling, 'node_modules'), resolve(tool, 'node_modules'), 'dir')
  // The font is a rendering dependency; the mark and documents are synthetic fixtures.
  for (const name of ['Inter.ttf', 'OFL.txt']) copyFileSync(resolve(project, 'brand/source', name), resolve(source, name))
  const mark = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><path d="M16 16h80v80H16Z"/><path d="M144 144h80v80h-80Z"/></svg>'
  writeFileSync(resolve(fixture, 'docs/assets/logo.svg'), mark)
  writeFileSync(resolve(fixture, 'LICENSE'), 'Synthetic license fixture')
  writeFileSync(resolve(fixture, 'brand/README.md'), 'Maintained synthetic source guide')
  writeFileSync(resolve(tool, 'guide.md'), 'Synthetic distribution guide')
  writeFileSync(resolve(tool, 'guide.zh-CN.md'), 'Synthetic translated distribution guide')

  execFileSync(process.execPath, ['build.mjs'], { cwd: tool, timeout: 60_000 })
  const output = resolve(fixture, 'dist/brand')
  assert.ok(existsSync(resolve(output, 'manifest.json')), 'complete exports belong under dist/brand')
  assert.equal(readFileSync(resolve(fixture, 'brand/README.md'), 'utf8'), 'Maintained synthetic source guide')
  assert.equal(readFileSync(resolve(fixture, 'docs/assets/logo.svg'), 'utf8'), mark)
  assert.equal(readFileSync(resolve(output, 'README.md'), 'utf8'), 'Synthetic distribution guide')
  assert.equal(readFileSync(resolve(output, 'source/Inter.ttf')).compare(readFileSync(resolve(source, 'Inter.ttf'))), 0)
  execFileSync(process.execPath, ['check.mjs'], { cwd: tool, timeout: 60_000 })

  const before = JSON.parse(readFileSync(resolve(output, 'manifest.json'), 'utf8')).assets
  const portable = resolve(output, 'source/generator')
  symlinkSync(resolve(tooling, 'node_modules'), resolve(portable, 'node_modules'), 'dir')
  execFileSync(process.execPath, ['build.mjs', '--standalone'], { cwd: portable, timeout: 60_000 })
  execFileSync(process.execPath, ['check.mjs', '--standalone'], { cwd: portable, timeout: 60_000 })
  const after = JSON.parse(readFileSync(resolve(output, 'manifest.json'), 'utf8')).assets
  assert.deepEqual(after.map((a) => [a.path, a.sha256]), before.map((a) => [a.path, a.sha256]))
})
