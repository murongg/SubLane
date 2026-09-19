import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from '@/App'
import { createAppRouter } from '@/router'
import { memberAuthenticated, anonymous } from '@/test/fixtures'

it('validates a personal password change then clears the authenticated session', async () => {
  const fetch = vi
    .fn()
    .mockImplementation((url: string) =>
      Promise.resolve(
        new Response(
          JSON.stringify(
            url === '/api/auth/state' ? memberAuthenticated : anonymous,
          ),
        ),
      ),
    )
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  render(
    <App
      router={createAppRouter(
        createMemoryHistory({ initialEntries: ['/preferences'] }),
      )}
    />,
  )
  await user.click(
    await screen.findByRole('button', { name: 'Change password' }),
  )
  const dialog = await screen.findByRole('dialog', { name: 'Change password' })
  await user.type(
    within(dialog).getByLabelText('Current password'),
    'synthetic-old-pass',
  )
  await user.type(
    within(dialog).getByLabelText('New password'),
    'synthetic-new-pass',
  )
  await user.type(
    within(dialog).getByLabelText('Confirm password'),
    'different-pass',
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Save password' }),
  )
  expect(fetch.mock.calls.some(([url]) => url === '/api/me/password')).toBe(
    false,
  )
  await user.clear(within(dialog).getByLabelText('Confirm password'))
  await user.type(
    within(dialog).getByLabelText('Confirm password'),
    'synthetic-new-pass',
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Save password' }),
  )
  await screen.findByRole('heading', { name: 'Sign in to SubLane' })
  await waitFor(() =>
    expect(
      fetch.mock.calls.find(([url]) => url === '/api/me/password'),
    ).toBeTruthy(),
  )
  const [, init] = fetch.mock.calls.find(([url]) => url === '/api/me/password')!
  expect(JSON.parse(init.body)).toEqual({
    current_password: 'synthetic-old-pass',
    new_password: 'synthetic-new-pass',
  })
  expect(screen.queryByDisplayValue('synthetic-new-pass')).toBeNull()
})
