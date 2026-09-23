import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { WorkspaceSelect } from './WorkspaceSelect'

it('shows the selected workspace and disables suspended workspaces', async () => {
  sessionStorage.setItem('sublane.workspace', '2')
  const client = new QueryClient()
  client.setQueryData(['auth'], {
    initialized: true,
    user: { id: 2, username: 'synthetic-member', role: 'admin' },
    workspace_count: 2,
  })
  client.setQueryData(['tenants'], {
    tenants: [
      { id: 1, name: 'First workspace', status: 'suspended' },
      { id: 2, name: 'Second workspace', status: 'active' },
    ],
  })
  try {
    render(
      <QueryClientProvider client={client}>
        <WorkspaceSelect />
      </QueryClientProvider>,
    )
    await userEvent
      .setup()
      .click(
        screen.getByRole('button', { name: 'Workspace: Second workspace' }),
      )
    expect(
      screen
        .getByRole('menuitem', { name: 'First workspace' })
        .getAttribute('aria-disabled'),
    ).toBe('true')
    expect(
      screen
        .getByRole('menuitem', { name: 'Second workspace' })
        .getAttribute('aria-current'),
    ).toBe('true')
  } finally {
    sessionStorage.removeItem('sublane.workspace')
  }
})

it('shows the workspace name even when there is only one workspace', async () => {
  const client = new QueryClient()
  client.setQueryData(['auth'], {
    initialized: true,
    user: { id: 1, username: 'synthetic-admin', role: 'admin' },
    workspace_count: 1,
  })
  client.setQueryData(['tenants'], {
    tenants: [{ id: 1, name: 'Synthetic studio', status: 'active' }],
  })
  render(
    <QueryClientProvider client={client}>
      <WorkspaceSelect />
    </QueryClientProvider>,
  )
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Workspace: Synthetic studio' }))
  expect(
    screen.getByRole('menuitem', { name: 'Synthetic studio' }),
  ).toBeTruthy()
  await userEvent
    .setup()
    .click(screen.getByRole('menuitem', { name: 'Create workspace' }))
  expect(
    await screen.findByRole('dialog', { name: 'Create workspace' }),
  ).toBeTruthy()
})

it('loads the global workspace list for a multi-workspace user', async () => {
  sessionStorage.setItem('sublane.workspace', '2')
  const client = new QueryClient()
  client.setQueryData(['auth'], {
    initialized: true,
    user: { id: 2, username: 'synthetic-member', role: 'admin' },
    workspace_count: 2,
  })
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        tenants: [
          { id: 1, name: 'First workspace', status: 'active' },
          { id: 2, name: 'Second workspace', status: 'active' },
        ],
      }),
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
  try {
    render(
      <QueryClientProvider client={client}>
        <WorkspaceSelect />
      </QueryClientProvider>,
    )
    expect(
      await screen.findByRole('button', {
        name: 'Workspace: Second workspace',
      }),
    ).toBeTruthy()
    expect(fetchMock.mock.calls[0][0]).toBe('/api/workspaces')
  } finally {
    sessionStorage.removeItem('sublane.workspace')
  }
})

it('reports a failed workspace list instead of showing a single workspace', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['auth'], {
    initialized: true,
    user: { id: 2, username: 'synthetic-member', role: 'admin' },
    workspace_count: 2,
  })
  vi.stubGlobal(
    'fetch',
    vi.fn().mockRejectedValue(new Error('synthetic network error')),
  )
  render(
    <QueryClientProvider client={client}>
      <WorkspaceSelect />
    </QueryClientProvider>,
  )
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Workspace' }))
  expect((await screen.findByRole('alert')).textContent).toContain(
    'Could not load workspaces',
  )
})
