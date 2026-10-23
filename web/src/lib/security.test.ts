import { expect, it, vi } from 'vitest'
import {
  addPasskey,
  passkeyAssertion,
  passkeySupported,
  securityError,
} from './security'

class SyntheticCredential {
  static parseCreationOptionsFromJSON(value: unknown) {
    return value
  }
  static parseRequestOptionsFromJSON(value: unknown) {
    return value
  }
  toJSON() {
    return { id: 'synthetic-credential', type: 'public-key', response: {} }
  }
}
function browser() {
  vi.stubGlobal('isSecureContext', true)
  vi.stubGlobal('PublicKeyCredential', SyntheticCredential)
  const create = vi.fn().mockResolvedValue(new SyntheticCredential())
  const get = vi.fn().mockResolvedValue(new SyntheticCredential())
  vi.stubGlobal('navigator', { credentials: { create, get } })
  return { create, get }
}
it('sends registration proof only to begin and serializes the browser credential to finish', async () => {
  const { create } = browser()
  expect(passkeySupported()).toBe(true)
  const fetch = vi.fn((url: string) =>
    Promise.resolve(
      url.endsWith('begin')
        ? new Response(
            JSON.stringify({
              publicKey: {
                challenge: 'synthetic-challenge',
                user: { id: 'synthetic-user' },
                rp: { id: 'example.test' },
                pubKeyCredParams: [],
              },
            }),
          )
        : new Response(null, { status: 204 }),
    ),
  )
  vi.stubGlobal('fetch', fetch)
  await addPasskey('Synthetic device', {
    current_password: 'synthetic-password',
    code: '123456',
  })
  expect(create).toHaveBeenCalledOnce()
  expect(fetch.mock.calls.map(([url]) => url)).toEqual([
    '/api/me/passkeys/begin',
    '/api/me/passkeys/finish',
  ])
  const calls = vi.mocked(globalThis.fetch).mock.calls
  expect(JSON.parse(String(calls[0][1]?.body))).toEqual({
    current_password: 'synthetic-password',
    code: '123456',
  })
  expect(JSON.parse(String(calls[1][1]?.body))).toEqual({
    name: 'Synthetic device',
    credential: new SyntheticCredential().toJSON(),
  })
})
it('does not complete passkey login when the device prompt is cancelled', async () => {
  const { get } = browser()
  get.mockRejectedValue(
    new DOMException('synthetic cancellation', 'NotAllowedError'),
  )
  const fetch = vi
    .fn()
    .mockResolvedValue(
      new Response(
        JSON.stringify({ publicKey: { challenge: 'synthetic-challenge' } }),
      ),
    )
  vi.stubGlobal('fetch', fetch)
  await expect(passkeyAssertion()).rejects.toMatchObject({
    name: 'NotAllowedError',
  })
  expect(fetch).toHaveBeenCalledOnce()
  expect(securityError(new DOMException('', 'NotAllowedError'))).toBe(
    'passkeyCancelled',
  )
})
it('rejects a missing browser credential without submitting completion', async () => {
  const { create } = browser()
  create.mockResolvedValue(null)
  const fetch = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        publicKey: {
          challenge: 'synthetic-challenge',
          user: {},
          rp: {},
          pubKeyCredParams: [],
        },
      }),
    ),
  )
  vi.stubGlobal('fetch', fetch)
  await expect(
    addPasskey('Synthetic', {
      current_password: 'synthetic-password',
      code: '',
    }),
  ).rejects.toMatchObject({ code: 'passkey_failed' })
  expect(fetch).toHaveBeenCalledOnce()
})
