import { expect, it, vi } from 'vitest'
import { exportBackup } from './backup'

it('keeps backup export scoped to the selected workspace', async () => {
  sessionStorage.setItem('sublane.workspace', '2')
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(new Uint8Array([1]), {
      status: 200,
      headers: { 'Content-Type': 'application/gzip' },
    }),
  )
  vi.stubGlobal('fetch', fetchMock)
  try {
    await exportBackup(new AbortController().signal)
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
