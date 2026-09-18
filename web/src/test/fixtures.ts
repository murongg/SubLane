export const system = {
  name: 'SubLane',
  version: 'synthetic-version',
  status: 'ok',
  uptime_seconds: 42,
  storage: { engine: 'sqlite', status: 'ready' },
  gateway: { provider: 'codex', status: 'not_configured' },
}

export const authenticated = {
  initialized: true,
  user: { username: 'admin-test' },
}

export const anonymous = { initialized: true, user: null }
