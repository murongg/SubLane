import { spawn } from 'node:child_process'
import { mkdirSync } from 'node:fs'

// Build first so shutdown targets the actual server, not a go-run wrapper process.
mkdirSync('bin', { recursive: true })
const build = spawn('go', ['build', '-o', 'bin/sublane-dev', './cmd/sublane'], { stdio: 'inherit' })
build.on('error', (error) => { console.error(error.message); process.exit(1) })
build.on('exit', (code) => {
  if (code !== 0) process.exit(code ?? 1)
  const children = [
    spawn('./bin/sublane-dev', [], { stdio: 'inherit', detached: process.platform !== 'win32' }),
    spawn('pnpm', ['--dir', 'web', 'dev', ...process.argv.slice(2)], { stdio: 'inherit', detached: process.platform !== 'win32' }),
  ]
  let stopping = false
  const stop = (exitCode = 0) => {
    if (stopping) return
    stopping = true
    for (const child of children) {
      if (child.pid) {
        try {
          if (process.platform === 'win32') child.kill('SIGTERM')
          else process.kill(-child.pid, 'SIGTERM')
        } catch (error) { if (error.code !== 'ESRCH') console.error(error.message) }
      }
    }
    process.exitCode = exitCode
  }
  for (const child of children) {
    child.on('error', (error) => { console.error(error.message); stop(1) })
    child.on('exit', (code) => stop(code ?? 0))
  }
  process.on('SIGINT', () => stop())
  process.on('SIGTERM', () => stop())
})
