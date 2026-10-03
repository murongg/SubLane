import { spawn } from 'node:child_process'
import { createServer } from 'node:net'

const address = process.env.SUBLANE_ADDR || '127.0.0.1:8080'
try {
  const target = new URL(`http://${address.startsWith(':') ? `127.0.0.1${address}` : address}`)
  const port = Number(target.port || 80)
  if (
    !/:\d+$/.test(address) || port < 1 || target.pathname !== '/' ||
    target.search || target.hash || target.username || target.password
  ) {
    throw new Error('SUBLANE_ADDR must be host:port with a port between 1 and 65535')
  }
  // Air survives a server bind failure; reject conflicts before Vite can reach another instance.
  await new Promise((resolve, reject) => {
    const listener = createServer()
    listener.once('error', reject)
    const host = address.startsWith(':') ? undefined : target.hostname.replace(/^\[|\]$/g, '')
    listener.listen({ host, port, exclusive: true }, () => {
      listener.close((error) => error ? reject(error) : resolve())
    })
  })
} catch (error) {
  console.error(error.code === 'EADDRINUSE'
    ? `Development backend address ${address} is already in use. Choose a free port with SUBLANE_ADDR=127.0.0.1:9090 make dev.`
    : `Cannot start the development backend: ${error.message}`)
  process.exit(1)
}
const env = { ...process.env, SUBLANE_ADDR: address }

const children = [
  spawn('make', ['dev-api'], { env, stdio: 'inherit', detached: process.platform !== 'win32' }),
  spawn('pnpm', ['--dir', 'web', 'dev', ...process.argv.slice(2)], { env, stdio: 'inherit', detached: process.platform !== 'win32' }),
]
let stopping = false
const stop = (exitCode = 0) => {
  if (stopping) return
  stopping = true
  for (const child of children) {
    if (child.pid) {
      try {
        // Air and Vite spawn descendants; signal the group to avoid orphaned servers.
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
