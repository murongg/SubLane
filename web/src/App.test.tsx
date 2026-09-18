import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { App } from './App'
import { createAppRouter } from './router'
import { system } from './test/fixtures'

it('loads service data, navigates accounts, and persists the theme', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(new Response(JSON.stringify(system))),
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
  await user.selectOptions(screen.getByLabelText('Language'), 'zh')
  expect(await screen.findByRole('heading', { name: '订阅账号' })).toBeTruthy()
  expect(localStorage.getItem('sublane-language')).toBe('zh')
  await user.selectOptions(screen.getByLabelText('界面主题'), 'dark')
  await waitFor(() =>
    expect(document.documentElement.classList.contains('dark')).toBe(true),
  )
  expect(localStorage.getItem('sublane-theme')).toBe('dark')
})

it('shows a failed connection and allows recovery', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(new Response('{}', { status: 503 }))
    .mockResolvedValue(new Response(JSON.stringify(system)))
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
  expect(fetchMock).toHaveBeenCalledTimes(2)
})
