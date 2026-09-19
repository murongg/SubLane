import { expect, it, vi } from 'vitest'
import { z } from 'zod'
import { request } from './request'

it('accepts a successful empty response only when the endpoint schema permits it', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation(() =>
        Promise.resolve(new Response(null, { status: 204 })),
      ),
  )
  await expect(
    request('/api/accounts/synthetic', z.undefined(), {
      method: 'DELETE',
      body: '{}',
    }),
  ).resolves.toBeUndefined()
  await expect(
    request('/api/accounts/synthetic', z.object({ id: z.string() })),
  ).rejects.toMatchObject({ code: 'invalid_response' })
})
