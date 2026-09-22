import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import { authenticated } from '@/test/fixtures'

const syntheticMember = {
  id: 2,
  username: 'member-test',
  role: 'member',
  enabled: true,
  created_at: 1900000000,
}
const response = (data: unknown, status = 200) =>
  new Response(JSON.stringify(data), { status })

it('updates shared member limits and resets a member password', async () => {
  const policy = {
    user_id: 2,
    requests_per_minute: 0,
    max_concurrency: 0,
    in_flight: 0,
    requests_this_minute: 0,
    reset_at: 1900000000,
  }
  const fetch = vi
    .fn()
    .mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url.endsWith('/limits')) return Promise.resolve(response(policy))
      if (url.endsWith('/password') && init?.method === 'POST')
        return Promise.resolve(new Response(null, { status: 204 }))
      return Promise.resolve(
        response({ members: [syntheticMember], next_cursor: 0 }),
      )
    })
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  open()
  await user.click(
    await screen.findByRole('button', { name: 'More actions for member-test' }),
  )
  await user.click(screen.getByRole('menuitem', { name: 'Request limits' }))
  const limits = await screen.findByRole('dialog', {
    name: 'Request limits for member-test',
  })
  const rate = await within(limits).findByLabelText('Requests per minute')
  await user.clear(rate)
  await user.type(rate, '60')
  const concurrency = within(limits).getByLabelText('Concurrent requests')
  await user.clear(concurrency)
  await user.type(concurrency, '2')
  await user.click(within(limits).getByRole('button', { name: 'Save changes' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  const call = fetch.mock.calls.find(
    ([url, init]) => url.endsWith('/limits') && init?.method === 'PATCH',
  )
  expect(JSON.parse(String(call?.[1]?.body))).toEqual({
    requests_per_minute: 60,
    max_concurrency: 2,
  })
  await user.click(
    screen.getByRole('button', { name: 'More actions for member-test' }),
  )
  await user.click(screen.getByRole('menuitem', { name: 'Reset password' }))
  const reset = await screen.findByRole('dialog', {
    name: 'Reset password for member-test',
  })
  expect(within(reset).queryByLabelText('Current password')).toBeNull()
  await user.type(
    within(reset).getByLabelText('New password'),
    'synthetic-reset',
  )
  await user.type(
    within(reset).getByLabelText('Confirm password'),
    'synthetic-reset',
  )
  await user.click(within(reset).getByRole('button', { name: 'Save password' }))
  await screen.findByText('Password reset for member-test.')
  expect(screen.queryByDisplayValue('synthetic-reset')).toBeNull()
})

function open() {
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/members'] }),
      )}
    />,
  )
}

it('creates a member and toggles their access using the management API', async () => {
  let members: (typeof syntheticMember)[] = []
  const fetchMock = vi
    .fn()
    .mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (init?.method === 'POST') {
        members = [syntheticMember]
        return Promise.resolve(response(syntheticMember, 201))
      }
      if (init?.method === 'PATCH') {
        members = [{ ...syntheticMember, ...JSON.parse(String(init.body)) }]
        return Promise.resolve(response(members[0]))
      }
      return Promise.resolve(response({ members, next_cursor: 0 }))
    })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  open()
  await screen.findByText('No members yet')
  await user.click(screen.getByRole('button', { name: 'Add member' }))
  const drawer = await screen.findByRole('dialog', { name: 'Add member' })
  await user.type(within(drawer).getByLabelText('Username'), 'member-test')
  await user.type(
    within(drawer).getByLabelText('Password', { exact: true }),
    'member pass 42',
  )
  await user.type(
    within(drawer).getByLabelText('Confirm password'),
    'member pass 42',
  )
  await user.click(
    within(drawer).getByRole('button', { name: 'Create member' }),
  )
  await screen.findByRole('button', { name: 'Disable member-test' })
  const created = fetchMock.mock.calls.find(
    ([, init]) => init?.method === 'POST',
  )
  expect(JSON.parse(String(created?.[1]?.body))).toEqual({
    username: 'member-test',
    password: 'member pass 42',
  })
  await user.click(screen.getByRole('button', { name: 'Disable member-test' }))
  await screen.findByRole('button', { name: 'Enable member-test' })
  await user.click(screen.getByRole('button', { name: 'Enable member-test' }))
  await screen.findByRole('button', { name: 'Disable member-test' })
})

it('keeps the create form usable after a duplicate username and validates confirmation', async () => {
  const create = vi
    .fn()
    .mockResolvedValue(response({ error: 'username_taken' }, 409))
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (init?.method === 'POST') return create()
      return Promise.resolve(response({ members: [], next_cursor: 0 }))
    }),
  )
  const user = userEvent.setup()
  open()
  await screen.findByText('No members yet')
  await user.click(screen.getByRole('button', { name: 'Add member' }))
  const drawer = await screen.findByRole('dialog', { name: 'Add member' })
  await user.type(within(drawer).getByLabelText('Username'), 'member-test')
  await user.type(
    within(drawer).getByLabelText('Password', { exact: true }),
    'member pass 42',
  )
  await user.type(
    within(drawer).getByLabelText('Confirm password'),
    'different pass 42',
  )
  await user.click(
    within(drawer).getByRole('button', { name: 'Create member' }),
  )
  expect((await within(drawer).findByRole('alert')).textContent).toBe(
    'Passwords do not match.',
  )
  expect(create).not.toHaveBeenCalled()
  await user.clear(within(drawer).getByLabelText('Confirm password'))
  await user.type(
    within(drawer).getByLabelText('Confirm password'),
    'member pass 42',
  )
  await user.click(
    within(drawer).getByRole('button', { name: 'Create member' }),
  )
  expect((await within(drawer).findByRole('alert')).textContent).toBe(
    'This username is already in use.',
  )
  await waitFor(() =>
    expect(
      within(drawer)
        .getByRole('button', { name: 'Create member' })
        .hasAttribute('disabled'),
    ).toBe(false),
  )
})
