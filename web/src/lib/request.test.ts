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

it('sends the selected workspace with browser management requests', async () => {
  sessionStorage.setItem('sublane.workspace', '2')
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ status: 'ok' }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
  vi.stubGlobal('fetch', fetchMock)
  try {
    await request('/api/connection', z.object({ status: z.string() }))
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/connection',
      expect.objectContaining({
        headers: expect.objectContaining({ 'X-SubLane-Workspace': '2' }),
      }),
    )
  } finally {
    sessionStorage.removeItem('sublane.workspace')
  }
})
