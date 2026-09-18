import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from './App'
import { createAppRouter } from './router'
import { anonymous, authenticated, system } from './test/fixtures'

const response = (data: unknown, status = 200) =>
  new Response(JSON.stringify(data), { status })

function open(path = '/') {
  render(
    <App
      router={createAppRouter(createMemoryHistory({ initialEntries: [path] }))}
    />,
  )
}

it('welcomes a fresh instance, creates the administrator, and enters the workspace', async () => {
  const calls: Array<{ url: string; body: unknown }> = []
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response({ initialized: false, user: null }))
      if (url === '/api/auth/setup') {
        calls.push({ url, body: JSON.parse(String(init?.body)) })
        return Promise.resolve(response(authenticated, 201))
      }
      return Promise.resolve(response(system))
    }),
  )
  const user = userEvent.setup()
  open('/accounts')
  await screen.findByRole('heading', { name: 'Welcome to SubLane' })
  expect(screen.queryByLabelText('Username')).toBeNull()
  expect(calls).toHaveLength(0)
  await user.click(screen.getByRole('link', { name: 'Start setup' }))
  expect(
    await screen.findByRole('heading', { name: 'Create administrator' }),
  ).toBeTruthy()
  expect(document.activeElement).toBe(
    screen.getByRole('heading', { name: 'Create administrator' }),
  )
  expect(screen.queryByRole('link', { name: 'Accounts' })).toBeNull()
  await user.type(screen.getByLabelText('Username'), 'admin-test')
  await user.type(
    screen.getByLabelText('Password', { exact: true }),
    'fake password 42',
  )
  await user.type(
    screen.getByLabelText('Confirm password'),
    'different passphrase',
  )
  await user.click(screen.getByRole('button', { name: 'Create administrator' }))
  expect(await screen.findByText('Passwords do not match.')).toBeTruthy()
  expect(calls).toHaveLength(0)
  await user.clear(screen.getByLabelText('Confirm password'))
  await user.type(screen.getByLabelText('Confirm password'), 'fake password 42')
  await user.click(screen.getByRole('button', { name: 'Create administrator' }))
  expect(
    await screen.findByRole('heading', { name: 'Workspace overview' }),
  ).toBeTruthy()
  expect(calls[0].body).toEqual({
    username: 'admin-test',
    password: 'fake password 42',
  })
  expect(localStorage.getItem('sublane_session')).toBeNull()
})

it.each([
  { name: 'below minimum', password: 'x'.repeat(7), valid: false },
  { name: 'minimum', password: 'x'.repeat(8), valid: true },
  { name: 'maximum', password: 'x'.repeat(20), valid: true },
  { name: 'above maximum', password: 'x'.repeat(21), valid: false },
  { name: 'short Unicode', password: '🙂'.repeat(7), valid: false },
  { name: 'Unicode maximum', password: '🙂'.repeat(20), valid: true },
])('validates setup password length: $name', async ({ password, valid }) => {
  const setupCalls = vi.fn()
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response({ initialized: false, user: null }))
      if (url === '/api/auth/setup') {
        setupCalls(JSON.parse(String(init?.body)))
        return Promise.resolve(response(authenticated, 201))
      }
      return Promise.resolve(response(system))
    }),
  )
  const user = userEvent.setup()
  open('/setup/admin')
  await screen.findByRole('heading', { name: 'Create administrator' })
  await user.type(screen.getByLabelText('Username'), 'admin-test')
  await user.type(screen.getByLabelText('Password', { exact: true }), password)
  await user.type(screen.getByLabelText('Confirm password'), password)
  await user.click(screen.getByRole('button', { name: 'Create administrator' }))
  if (valid) {
    await screen.findByRole('heading', { name: 'Workspace overview' })
    expect(setupCalls).toHaveBeenCalledExactlyOnceWith({
      username: 'admin-test',
      password,
    })
  } else {
    expect((await screen.findByRole('alert')).textContent).toBe(
      'Use a password with 8–20 characters.',
    )
    expect(setupCalls).not.toHaveBeenCalled()
    expect(document.activeElement).toBe(
      screen.getByLabelText('Password', { exact: true }),
    )
  }
})

it('supports returning to the welcome page and keeps the chosen language', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(response({ initialized: false, user: null }))
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  open('/setup')
  await screen.findByRole('heading', { name: 'Welcome to SubLane' })
  await user.click(screen.getByRole('button', { name: 'Language' }))
  await user.click(screen.getByRole('menuitemradio', { name: '简体中文' }))
  await user.click(screen.getByRole('link', { name: '开始设置' }))
  await screen.findByRole('heading', { name: '创建管理员' })
  await user.click(screen.getByRole('button', { name: '返回欢迎页' }))
  const heading = await screen.findByRole('heading', {
    name: '欢迎使用 SubLane',
  })
  expect(document.activeElement).toBe(heading)
  expect(screen.queryByLabelText('用户名')).toBeNull()
  expect(fetchMock.mock.calls.every(([url]) => url === '/api/auth/state')).toBe(
    true,
  )
})

it.each(['/setup', '/setup/admin'])(
  'skips %s after initialization for a signed-in administrator',
  async (path) => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockImplementation((url: string) =>
          Promise.resolve(
            response(url === '/api/auth/state' ? authenticated : system),
          ),
        ),
    )
    open(path)
    await screen.findByRole('heading', { name: 'Workspace overview' })
    expect(screen.queryByRole('link', { name: 'Start setup' })).toBeNull()
  },
)

it.each(['/setup', '/setup/admin'])(
  'routes initialized instances from %s to login and supports Chinese',
  async (path) => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockImplementation((url: string) =>
          Promise.resolve(
            url === '/api/auth/state'
              ? response(anonymous)
              : response({ error: 'invalid_credentials' }, 401),
          ),
        ),
    )
    const user = userEvent.setup()
    open(path)
    expect(
      await screen.findByRole('heading', { name: 'Sign in to SubLane' }),
    ).toBeTruthy()
    await user.type(screen.getByLabelText('Username'), 'admin-test')
    await user.type(
      screen.getByLabelText('Password', { exact: true }),
      'wrong synthetic password',
    )
    await user.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(
      await screen.findByText('Username or password is incorrect.'),
    ).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Language' }))
    await user.click(screen.getByRole('menuitemradio', { name: '简体中文' }))
    expect(
      await screen.findByRole('heading', { name: '登录 SubLane' }),
    ).toBeTruthy()
    expect(screen.getByLabelText('用户名')).toBeTruthy()
  },
)

it('revokes access on logout and fetches fresh private data on a new login', async () => {
  let state = authenticated as typeof authenticated | typeof anonymous
  let reads = 0
  const trace: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      trace.push(url + ':' + Boolean(state.user))
      if (url === '/api/auth/state') return Promise.resolve(response(state))
      if (url === '/api/auth/logout') {
        state = anonymous
        return Promise.resolve(response(state))
      }
      if (url === '/api/auth/login') {
        state = authenticated
        return Promise.resolve(response(state))
      }
      reads++
      return Promise.resolve(
        response({ ...system, version: 'synthetic-' + reads }),
      )
    }),
  )
  const user = userEvent.setup()
  open()
  expect(await screen.findByText('synthetic-1')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Account menu' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Sign out' }))
  expect(
    await screen.findByRole('heading', { name: 'Sign in to SubLane' }),
  ).toBeTruthy()
  expect(screen.queryByText('synthetic-1')).toBeNull()
  await user.type(screen.getByLabelText('Username'), 'admin-test')
  await user.type(
    screen.getByLabelText('Password', { exact: true }),
    'synthetic passphrase 42',
  )
  await user.click(screen.getByRole('button', { name: 'Sign in' }))
  await waitFor(() => expect(screen.getByText(/synthetic-\d/)).toBeTruthy())
  expect(screen.getByText('synthetic-2')).toBeTruthy()
  expect(trace).not.toContain('/api/system:false')
})

it('returns to login when a protected request reports an expired session', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation((url: string) =>
        Promise.resolve(
          url === '/api/auth/state'
            ? response(authenticated)
            : response({ error: 'unauthorized' }, 401),
        ),
      ),
  )
  open()
  expect(
    await screen.findByRole('heading', { name: 'Sign in to SubLane' }),
  ).toBeTruthy()
  await waitFor(() =>
    expect(screen.queryByRole('link', { name: 'Accounts' })).toBeNull(),
  )
})

it('keeps the workspace and lets the account menu retry a failed sign-out', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/auth/logout') return Promise.resolve(response({}, 503))
      return Promise.resolve(response(system))
    }),
  )
  const user = userEvent.setup()
  open()
  await screen.findByRole('heading', { name: 'Workspace overview' })
  await user.click(screen.getByRole('button', { name: 'Account menu' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Sign out' }))
  expect((await screen.findByRole('alert')).textContent).toBe(
    'Could not sign out. Please try again.',
  )
  expect(
    screen.getByRole('heading', { name: 'Workspace overview' }),
  ).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Account menu' }))
  expect(
    (await screen.findByRole('menuitem', { name: 'Sign out' })).getAttribute(
      'aria-disabled',
    ),
  ).not.toBe('true')
})

it('handles unexpected server error codes without crashing the login form', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation((url: string) =>
        Promise.resolve(
          url === '/api/auth/state'
            ? response(anonymous)
            : response({ error: 'constructor' }, 500),
        ),
      ),
  )
  const user = userEvent.setup()
  open('/login')
  await screen.findByRole('heading', { name: 'Sign in to SubLane' })
  await user.type(screen.getByLabelText('Username'), 'admin-test')
  await user.type(
    screen.getByLabelText('Password', { exact: true }),
    'synthetic passphrase 42',
  )
  await user.click(screen.getByRole('button', { name: 'Sign in' }))
  expect((await screen.findByRole('alert')).textContent).toContain(
    'The service is unavailable.',
  )
})
