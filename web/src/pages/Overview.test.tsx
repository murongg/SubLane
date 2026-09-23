import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import { authenticated, system, workspaces } from '@/test/fixtures'

function open() {
  render(
    <App
      router={createAppRouter(createMemoryHistory({ initialEntries: ['/'] }))}
    />,
  )
}

it('shows connection prerequisites and readable live instance data', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation((url: string) =>
        Promise.resolve(
          new Response(
            JSON.stringify(
              url === '/api/auth/state'
                ? authenticated
                : { ...system, uptime_seconds: 90061 },
            ),
          ),
        ),
      ),
  )
  const user = userEvent.setup()
  open()
  expect(await screen.findByText('synthetic-version')).toBeTruthy()
  expect(screen.getByText('1 day 1 hr')).toBeTruthy()
  expect(screen.getByRole('heading', { name: 'Gateway setup' })).toBeTruthy()
  expect(screen.getByRole('heading', { name: 'Client access' })).toBeTruthy()
  expect(screen.getByText('Codex CLI')).toBeTruthy()
  expect(screen.getByText('Codex desktop')).toBeTruthy()
  await user.click(screen.getByRole('link', { name: 'View accounts' }))
  expect(
    await screen.findByRole('heading', { name: 'Subscription accounts' }),
  ).toBeTruthy()
})

it('preserves the last successful status when refresh fails and recovers on retry', async () => {
  let reads = 0
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url === '/api/auth/state')
        return Promise.resolve(new Response(JSON.stringify(authenticated)))
      if (url === '/api/workspaces')
        return Promise.resolve(new Response(JSON.stringify(workspaces)))
      reads++
      return Promise.resolve(
        reads === 2
          ? new Response('{}', { status: 503 })
          : new Response(
              JSON.stringify({ ...system, version: `synthetic-${reads}` }),
            ),
      )
    }),
  )
  const user = userEvent.setup()
  open()
  await screen.findByText('synthetic-1')
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect((await screen.findByRole('alert')).textContent).toContain(
    'Showing the last successful check',
  )
  expect(screen.getByText('synthetic-1')).toBeTruthy()
  expect(screen.getAllByText('Status unknown').length).toBeGreaterThan(0)
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(await screen.findByText('synthetic-3')).toBeTruthy()
  expect(screen.queryByRole('alert')).toBeNull()
  expect(screen.getByText('Running')).toBeTruthy()
})

it('uses the verified account state for gateway readiness', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      const data =
        url === '/api/auth/state'
          ? authenticated
          : {
              ...system,
              gateway: {
                provider: 'codex',
                status: 'ready',
                has_usable_key: true,
              },
            }
      return Promise.resolve(new Response(JSON.stringify(data)))
    }),
  )
  open()
  expect(
    await screen.findByText(
      'Your gateway is ready. Connect a client using a personal API key.',
    ),
  ).toBeTruthy()
  expect(screen.queryByText('Not configured')).toBeNull()
})

it('does not mark gateway keys ready before an active key has a ready pool', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) =>
      Promise.resolve(
        new Response(
          JSON.stringify(
            url === '/api/auth/state'
              ? authenticated
              : {
                  ...system,
                  gateway: {
                    provider: 'codex',
                    status: 'ready',
                    has_usable_key: false,
                  },
                },
          ),
        ),
      ),
    ),
  )
  open()
  expect(await screen.findByText('No usable key')).toBeTruthy()
  expect(screen.getByRole('link', { name: 'Create API key' })).toBeTruthy()
  expect(
    screen.queryByText(
      'Your gateway is ready. Connect a client using a personal API key.',
    ),
  ).toBeNull()
})
