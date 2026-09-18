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
  user: { id: 1, username: 'admin-test', role: 'admin' as const },
}

export const memberAuthenticated = {
  initialized: true,
  user: { id: 2, username: 'member-test', role: 'member' as const },
}

export const anonymous = { initialized: true, user: null }
