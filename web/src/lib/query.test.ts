import { expect, it, vi } from 'vitest'
import { QueryObserver } from '@tanstack/react-query'
import { requestOptions } from './requests'
import { authKey, authOptions } from './auth'
import { createQueryClient } from './query'
import { authenticated, memberAuthenticated, system } from '@/test/fixtures'

it.each(['personal', 'all'] as const)(
  'stops a mounted %s history observer when the active identity changes',
  async (scope) => {
    const client = createQueryClient()
    client.setQueryData(authKey, authenticated)
    const fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ requests: [], next_cursor: 0 })),
      )
    vi.stubGlobal('fetch', fetch)
    const options = requestOptions(
      client,
      authenticated.user.id,
      scope,
      0,
      '',
      '',
    )
    await client.fetchQuery(options)
    const observer = new QueryObserver(client, options)
    const unsubscribe = observer.subscribe(() => {})
    try {
      client.setQueryData(authKey, memberAuthenticated)
      await client.invalidateQueries({ queryKey: ['request-history'] })
      expect(fetch).toHaveBeenCalledTimes(1)
    } finally {
      unsubscribe()
      client.clear()
    }
  },
)

it('removes administrator data when session refresh detects another identity', async () => {
  const client = createQueryClient()
  let state = authenticated as typeof authenticated | typeof memberAuthenticated
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation(() =>
        Promise.resolve(new Response(JSON.stringify(state))),
      ),
  )
  await client.fetchQuery(authOptions())
  client.setQueryData(['system'], system)
  client.setQueryData(['members', 0], { members: [], next_cursor: 0 })
  state = memberAuthenticated
  await client.fetchQuery({ ...authOptions(), staleTime: 0 })
  expect(client.getQueryData(authKey)).toEqual(memberAuthenticated)
  expect(client.getQueryData(['system'])).toBeUndefined()
  expect(client.getQueryData(['members', 0])).toBeUndefined()
  client.clear()
})
