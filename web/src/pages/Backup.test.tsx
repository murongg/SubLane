import { QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { createQueryClient } from '@/lib/query'
import { authKey } from '@/lib/auth'
import { authenticated, memberAuthenticated } from '@/test/fixtures'
import { Backup } from './Backup'

const settings = {
  max_upload_bytes: 268435456,
  restore_directory: '/synthetic/restore-ready',
  destination_exists: false,
}
const info = {
  created_at: '2030-01-01T00:00:00Z',
  version: 'synthetic-version',
  schema_version: 18,
  database_bytes: 262144,
}
const response = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), { status })
function mount() {
  const client = createQueryClient()
  client.setQueryData(authKey, authenticated)
  render(
    <QueryClientProvider client={client}>
      <Backup />
    </QueryClientProvider>,
  )
  return client
}

it('validates the selected archive before preparing an independent restore', async () => {
  const fetch = vi
    .fn()
    .mockImplementation((url: string) =>
      Promise.resolve(
        response(
          url.endsWith('/verify')
            ? info
            : url.endsWith('/restore')
              ? { ...info, restore_directory: settings.restore_directory }
              : settings,
        ),
      ),
    )
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  mount()
  const restore = await screen.findByRole('button', { name: 'Prepare restore' })
  expect(restore.hasAttribute('disabled')).toBe(true)
  const file = new File(
    ['synthetic-archive'],
    'synthetic.sublane-backup.tar.gz',
    { type: 'application/gzip' },
  )
  await user.upload(screen.getByLabelText('Backup file'), file)
  await user.click(screen.getByRole('button', { name: 'Verify backup' }))
  expect(await screen.findByText('Backup verified')).toBeTruthy()
  await user.click(restore)
  expect(await screen.findByText('Restored data is ready')).toBeTruthy()
  expect(screen.getByText(settings.restore_directory)).toBeTruthy()
  expect(
    fetch.mock.calls.filter(([url]) => url.endsWith('/restore')),
  ).toHaveLength(1)
  const request = fetch.mock.calls.find(([url]) => url.endsWith('/restore'))
  expect(request?.[1]).toMatchObject({ method: 'POST' })
  expect(request?.[1]?.body).toBeInstanceOf(FormData)
  expect((request?.[1]?.body as FormData).get('file')).toBe(file)
})

it('does not download a backup that finishes after the administrator identity changes', async () => {
  let resolve!: (response: Response) => void
  const createURL = vi.fn(() => 'blob:synthetic')
  vi.stubGlobal(
    'URL',
    class extends URL {
      static createObjectURL = createURL
      static revokeObjectURL = vi.fn()
    },
  )
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) =>
      url.endsWith('/export')
        ? new Promise<Response>((done) => {
            resolve = done
          })
        : Promise.resolve(response(settings)),
    ),
  )
  const user = userEvent.setup()
  const client = mount()
  await screen.findByRole('button', { name: 'Download backup' })
  await user.type(
    screen.getByLabelText('Export password'),
    'synthetic backup passphrase',
  )
  await user.type(
    screen.getByLabelText('Confirm backup password'),
    'synthetic backup passphrase',
  )
  await user.click(screen.getByRole('button', { name: 'Download backup' }))
  await act(async () => {
    client.setQueryData(authKey, memberAuthenticated)
    resolve(
      new Response('synthetic-backup', {
        headers: { 'Content-Type': 'application/octet-stream' },
      }),
    )
  })
  await waitFor(() => expect(client.isMutating()).toBe(0))
  expect(createURL).not.toHaveBeenCalled()
})

it('requires a new successful verification after a repeat check fails', async () => {
  let checks = 0
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url.endsWith('/verify'))
        return Promise.resolve(
          ++checks === 1
            ? response(info)
            : response({ error: 'invalid_backup' }, 400),
        )
      return Promise.resolve(response(settings))
    }),
  )
  const user = userEvent.setup()
  mount()
  await user.upload(
    await screen.findByLabelText('Backup file'),
    new File(['synthetic-archive'], 'synthetic.tar.gz', {
      type: 'application/gzip',
    }),
  )
  await user.click(screen.getByRole('button', { name: 'Verify backup' }))
  await screen.findByText('Backup verified')
  await user.click(screen.getByRole('button', { name: 'Verify backup' }))
  await screen.findByRole('alert')
  expect(
    screen
      .getByRole('button', { name: 'Prepare restore' })
      .hasAttribute('disabled'),
  ).toBe(true)
})

it('starts an authenticated backup download with the server filename', async () => {
  const createURL = vi.fn(() => 'blob:synthetic-backup')
  vi.stubGlobal(
    'URL',
    class extends URL {
      static createObjectURL = createURL
      static revokeObjectURL = vi.fn()
    },
  )
  const clicked = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(function (this: HTMLAnchorElement) {
      expect(this.download).toBe(
        'sublane-20300101-000000.sublane-backup.tar.gz.age',
      )
      expect(this.href).toBe('blob:synthetic-backup')
    })
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) =>
      Promise.resolve(
        url.endsWith('/export')
          ? new Response('synthetic-archive', {
              headers: {
                'Content-Type': 'application/octet-stream',
                'Content-Disposition':
                  'attachment; filename="sublane-20300101-000000.sublane-backup.tar.gz.age"',
              },
            })
          : response(settings),
      ),
    ),
  )
  const user = userEvent.setup()
  mount()
  await screen.findByRole('button', { name: 'Download backup' })
  await user.type(
    screen.getByLabelText('Export password'),
    'synthetic backup passphrase',
  )
  await user.type(
    screen.getByLabelText('Confirm backup password'),
    'synthetic backup passphrase',
  )
  await user.click(screen.getByRole('button', { name: 'Download backup' }))
  await waitFor(() => expect(clicked).toHaveBeenCalledTimes(1))
  expect(createURL).toHaveBeenCalledTimes(1)
  clicked.mockRestore()
})

it('requires matching export passwords and discards them after submission', async () => {
  const fetch = vi.fn((url: string) =>
    Promise.resolve(
      url.endsWith('/export')
        ? response({ error: 'backup_failed' }, 503)
        : response(settings),
    ),
  )
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  const client = mount()
  const button = await screen.findByRole('button', { name: 'Download backup' })
  expect(button.hasAttribute('disabled')).toBe(true)
  await user.type(
    screen.getByLabelText('Export password'),
    'synthetic backup passphrase',
  )
  await user.type(
    screen.getByLabelText('Confirm backup password'),
    'different passphrase',
  )
  expect(button.hasAttribute('disabled')).toBe(true)
  await user.clear(screen.getByLabelText('Confirm backup password'))
  await user.type(
    screen.getByLabelText('Confirm backup password'),
    'synthetic backup passphrase',
  )
  await user.click(button)
  await screen.findByRole('alert')
  expect(
    (screen.getByLabelText('Export password') as HTMLInputElement).value,
  ).toBe('')
  expect(
    (screen.getByLabelText('Confirm backup password') as HTMLInputElement)
      .value,
  ).toBe('')
  expect(
    JSON.parse(
      String(
        vi
          .mocked(globalThis.fetch)
          .mock.calls.find(([url]) => String(url).endsWith('/export'))?.[1]
          ?.body,
      ),
    ),
  ).toEqual({ password: 'synthetic backup passphrase' })
  expect(
    client
      .getMutationCache()
      .getAll()
      .every((mutation) => mutation.state.variables === undefined),
  ).toBe(true)
})

it('sends the password inside the upload body and invalidates verification when it changes', async () => {
  const fetch = vi.fn((url: string) =>
    Promise.resolve(
      response(
        url.endsWith('/verify') ? { ...info, encrypted: true } : settings,
      ),
    ),
  )
  vi.stubGlobal('fetch', fetch)
  const user = userEvent.setup()
  mount()
  const file = new File(['synthetic ciphertext'], 'synthetic.tar.gz.age', {
    type: 'application/octet-stream',
  })
  await user.upload(await screen.findByLabelText('Backup file'), file)
  await user.type(
    screen.getByLabelText('Backup password'),
    'synthetic backup passphrase',
  )
  await user.click(screen.getByRole('button', { name: 'Verify backup' }))
  await screen.findByText('Backup verified')
  const body = vi
    .mocked(globalThis.fetch)
    .mock.calls.find(([url]) => String(url).endsWith('/verify'))?.[1]?.body
  expect(body).toBeInstanceOf(FormData)
  expect((body as FormData).get('password')).toBe('synthetic backup passphrase')
  expect((body as FormData).get('file')).toBe(file)
  await user.type(screen.getByLabelText('Backup password'), ' changed')
  expect(
    screen
      .getByRole('button', { name: 'Prepare restore' })
      .hasAttribute('disabled'),
  ).toBe(true)
})
