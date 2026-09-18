import { expect, it, vi } from 'vitest'
import { authKey, authOptions } from './auth'
import { createQueryClient } from './query'
import { authenticated, memberAuthenticated, system } from '@/test/fixtures'

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
