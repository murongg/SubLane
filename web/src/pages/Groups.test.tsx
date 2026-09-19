import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import { authenticated } from '@/test/fixtures'

const base = {
  id: 1,
  name: 'Default',
  is_default: true,
  enabled: true,
  account_count: 1,
  member_count: 1,
  created_at: 1,
  updated_at: 1,
}
const account = {
  id: 'synthetic-account',
  provider: 'codex',
  name: 'Synthetic account',
  email: 'member@example.test',
  plan: '',
  enabled: true,
  status: 'ready',
  expires_at: 0,
  created_at: 1,
  updated_at: 1,
}
const response = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), { status })
it('creates a pool with only selected accounts', async () => {
  const groups = [base]
  const fetch = vi
    .fn()
    .mockImplementation((url: string, init?: RequestInit) => {
      if (url === '/api/auth/state')
        return Promise.resolve(response(authenticated))
      if (url === '/api/accounts')
        return Promise.resolve(response({ accounts: [account] }))
      if (url === '/api/groups' && init?.method === 'POST') {
        expect(JSON.parse(String(init.body))).toEqual({
          name: 'Project alpha',
          enabled: true,
          account_ids: ['synthetic-account'],
        })
        const group = {
          ...base,
          id: 2,
          name: 'Project alpha',
          is_default: false,
          member_count: 0,
        }
        groups.push(group)
        return Promise.resolve(
          response({ ...group, account_ids: ['synthetic-account'] }, 201),
        )
      }
      if (url === '/api/groups') return Promise.resolve(response({ groups }))
      return Promise.reject(new Error('Unexpected request'))
    })
  vi.stubGlobal('fetch', fetch)
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/groups'] }),
      )}
    />,
  )
  await screen.findByRole('heading', { name: 'Account groups' })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Create group' }))
  const dialog = await screen.findByRole('dialog')
  await user.type(within(dialog).getByLabelText('Group name'), 'Project alpha')
  await user.click(
    await within(dialog).findByRole('checkbox', { name: /Synthetic account/ }),
  )
  await user.click(within(dialog).getByRole('button', { name: 'Save group' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  await screen.findByText('Project alpha')
})
