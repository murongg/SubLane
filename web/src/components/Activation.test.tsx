import { render, screen, within } from '@testing-library/react'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import { authenticated, system, workspaces } from '@/test/fixtures'

it('offers subscription and API channel setup as separate choices', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) =>
      Promise.resolve(
        new Response(
          JSON.stringify(
            url === '/api/auth/state'
              ? authenticated
              : url === '/api/workspaces'
                ? workspaces
                : {
                    ...system,
                    gateway: {
                      ...system.gateway,
                      status: 'not_configured',
                      setup: {
                        stage: 'account',
                        has_successful_request: false,
                      },
                    },
                  },
          ),
        ),
      ),
    ),
  )
  render(
    <App
      router={createAppRouter(createMemoryHistory({ initialEntries: ['/'] }))}
    />,
  )
  const guide = await screen.findByRole('region', {
    name: 'Complete your first request',
  })
  expect(
    within(guide)
      .getByRole('link', { name: 'API channels' })
      .getAttribute('href'),
  ).toBe('/channels')
})

it('directs an administrator with a verified unassigned account to pools', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) =>
      Promise.resolve(
        new Response(
          JSON.stringify(
            url === '/api/auth/state'
              ? authenticated
              : url === '/api/workspaces'
                ? workspaces
                : {
                    ...system,
                    gateway: {
                      ...system.gateway,
                      setup: { stage: 'pool', has_successful_request: false },
                    },
                  },
          ),
        ),
      ),
    ),
  )
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/admin/instance'] }),
      )}
    />,
  )
  expect(
    (
      await screen.findByRole('link', { name: 'Set up a resource group' })
    ).getAttribute('href'),
  ).toBe('/groups')
  expect(screen.getByText('Complete your first request')).toBeTruthy()
})
