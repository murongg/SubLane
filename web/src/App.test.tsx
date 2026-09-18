import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from './App'
import { createAppRouter } from './router'
import { anonymous, authenticated, system } from './test/fixtures'

it('loads service data, navigates accounts, and persists the theme', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation((url: string) =>
        Promise.resolve(
          new Response(
            JSON.stringify(url === '/api/auth/state' ? authenticated : system),
          ),
        ),
      ),
  )
  const user = userEvent.setup()
  render(
    <App
      router={createAppRouter(createMemoryHistory({ initialEntries: ['/'] }))}
    />,
  )
  expect(await screen.findByText('synthetic-version')).toBeTruthy()
  await user.click(screen.getByRole('link', { name: 'Accounts' }))
  expect(
    await screen.findByRole('heading', { name: 'Subscription accounts' }),
  ).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Language' }))
  await user.click(screen.getByRole('menuitemradio', { name: '简体中文' }))
  expect(await screen.findByRole('heading', { name: '订阅账号' })).toBeTruthy()
  expect(localStorage.getItem('sublane-language')).toBe('zh')
  await user.click(screen.getByRole('button', { name: '界面主题' }))
  await user.click(screen.getByRole('menuitemradio', { name: '深色' }))
  await waitFor(() =>
    expect(document.documentElement.classList.contains('dark')).toBe(true),
  )
  expect(localStorage.getItem('sublane-theme')).toBe('dark')
  await user.click(screen.getByRole('link', { name: '偏好设置' }))
  expect(await screen.findByRole('heading', { name: '偏好设置' })).toBeTruthy()
  expect(screen.queryByRole('menu')).toBeNull()
})

it('shows a failed connection and allows recovery', async () => {
  let systemCalls = 0
  const fetchMock = vi.fn().mockImplementation((url: string) => {
    if (url === '/api/auth/state')
      return Promise.resolve(new Response(JSON.stringify(authenticated)))
    systemCalls++
    return Promise.resolve(
      systemCalls === 1
        ? new Response('{}', { status: 503 })
        : new Response(JSON.stringify(system)),
    )
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(
    <App
      router={createAppRouter(createMemoryHistory({ initialEntries: ['/'] }))}
    />,
  )
  expect(await screen.findByRole('alert')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Reconnect' }))
  expect(await screen.findByText('synthetic-version')).toBeTruthy()
  expect(systemCalls).toBe(2)
})

it('keeps keyboard focus in the account menu opened inside the mobile sidebar', async () => {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: query === '(max-width: 767px)',
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }))
  let state = authenticated as typeof authenticated | typeof anonymous
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url === '/api/auth/logout') {
        state = anonymous
        return Promise.resolve(new Response(JSON.stringify(state)))
      }
      return Promise.resolve(
        new Response(
          JSON.stringify(url === '/api/auth/state' ? state : system),
        ),
      )
    }),
  )
  const user = userEvent.setup()
  render(
    <App
      router={createAppRouter(createMemoryHistory({ initialEntries: ['/'] }))}
    />,
  )
  await screen.findByRole('heading', { name: 'Workspace overview' })
  await user.click(screen.getByRole('button', { name: 'Toggle navigation' }))
  await screen.findByRole('dialog', { name: 'Navigation' })
  await user.click(screen.getByRole('button', { name: 'Account menu' }))
  const signOut = await screen.findByRole('menuitem', { name: 'Sign out' })
  await user.keyboard('{ArrowDown}')
  expect(document.activeElement).toBe(signOut)
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole('button', { name: 'Account menu' }),
    ),
  )
  expect(screen.getByRole('dialog', { name: 'Navigation' })).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Account menu' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Sign out' }))
  await screen.findByRole('heading', { name: 'Sign in to SubLane' })
  await user.click(screen.getByRole('button', { name: 'Language' }))
  expect(
    await screen.findByRole('menuitemradio', { name: 'English' }),
  ).toBeTruthy()
})
