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
