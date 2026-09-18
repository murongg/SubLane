import { afterEach, describe, expect, it, vi } from 'vitest'
import { system } from '@/test/fixtures'
import { getSystem } from './api'

afterEach(() => vi.unstubAllGlobals())

describe('getSystem', () => {
  it('validates real response shape and forwards cancellation', async () => {
    const signal = new AbortController().signal
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify(system)))
    vi.stubGlobal('fetch', fetchMock)
    expect(await getSystem(signal)).toEqual(system)
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/system',
      expect.objectContaining({ signal }),
    )
  })
  it('rejects service errors without exposing response bodies', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response('synthetic private diagnostic', { status: 503 }),
        ),
    )
    await expect(getSystem()).rejects.toThrow('unavailable')
  })
  it('rejects an incompatible API instead of showing a healthy state', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('{"status":"ok"}')),
    )
    await expect(getSystem()).rejects.toThrow('invalid_response')
  })
})
