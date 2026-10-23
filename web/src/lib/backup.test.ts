import { expect, it, vi } from 'vitest'
import { exportBackup, verifyBackup } from './backup'

it('keeps backup export scoped to the selected workspace', async () => {
  sessionStorage.setItem('sublane.workspace', '2')
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(new Uint8Array([1]), {
      status: 200,
      headers: { 'Content-Type': 'application/octet-stream' },
    }),
  )
  vi.stubGlobal('fetch', fetchMock)
  try {
    await exportBackup(
      'synthetic backup passphrase',
      new AbortController().signal,
    )
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/settings/backup/export',
      expect.objectContaining({
        headers: expect.objectContaining({ 'X-SubLane-Workspace': '2' }),
      }),
    )
  } finally {
    sessionStorage.removeItem('sublane.workspace')
  }
})

it('keeps backup passwords in the multipart body and lets the browser set its boundary', async () => {
  const fetch = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        created_at: '2030-01-01T00:00:00Z',
        version: 'synthetic',
        schema_version: 20,
        database_bytes: 256,
        encrypted: true,
      }),
    ),
  )
  vi.stubGlobal('fetch', fetch)
  const file = new File(['synthetic ciphertext'], 'synthetic.age')
  await verifyBackup(
    file,
    'synthetic backup passphrase',
    new AbortController().signal,
  )
  const options = fetch.mock.calls[0][1]
  expect(options.body).toBeInstanceOf(FormData)
  expect(options.body.get('password')).toBe('synthetic backup passphrase')
  expect(options.headers['Content-Type']).toBeUndefined()
})
