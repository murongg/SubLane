import { StrictMode } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { Turnstile } from './Turnstile'

it('keeps login unverified on script failure and allows retrying the widget', async () => {
  const publish = vi.fn()
  vi.stubGlobal('turnstile', undefined)
  const view = render(<Turnstile siteKey="synthetic-site" onToken={publish} />)
  const script = document.querySelector(
    'script[src*="challenges.cloudflare.com"]',
  )!
  expect(script).toBeTruthy()
  fireEvent.error(script)
  expect(await screen.findByRole('alert')).toBeTruthy()
  expect(publish).toHaveBeenLastCalledWith('')
  const remove = vi.fn()
  const widget = vi.fn().mockReturnValue('synthetic-widget')
  vi.stubGlobal('turnstile', { render: widget, remove })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }))
  expect(widget).toHaveBeenCalledOnce()
  expect(document.contains(script)).toBe(false)
  act(() => widget.mock.calls[0][1].callback('synthetic-token'))
  expect(publish).toHaveBeenLastCalledWith('synthetic-token')
  view.unmount()
  expect(remove).toHaveBeenCalledWith('synthetic-widget')
  act(() => widget.mock.calls[0][1].callback('stale-token'))
  expect(publish).toHaveBeenLastCalledWith('synthetic-token')
})

it('uses the compact challenge on narrow screens', async () => {
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockReturnValue({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }),
  )
  const widget = vi.fn().mockReturnValue('synthetic-widget')
  vi.stubGlobal('turnstile', { render: widget, remove: vi.fn() })
  render(<Turnstile siteKey="synthetic-site" onToken={vi.fn()} />)
  await waitFor(() => expect(widget).toHaveBeenCalledOnce())
  expect(widget.mock.calls[0][1].size).toBe('compact')
})

it('loads one script across Strict Mode remounts and renders only the active widget', async () => {
  vi.stubGlobal('turnstile', undefined)
  const append = vi.spyOn(document.head, 'append')
  const view = render(
    <StrictMode>
      <Turnstile siteKey="synthetic-site" onToken={vi.fn()} />
    </StrictMode>,
  )
  const scripts = append.mock.calls
    .flat()
    .filter(
      (node) =>
        node instanceof HTMLScriptElement &&
        node.src.includes('challenges.cloudflare.com'),
    )
  expect(scripts).toHaveLength(1)
  const renderWidget = vi.fn().mockReturnValue('synthetic-widget')
  const remove = vi.fn()
  vi.stubGlobal('turnstile', { render: renderWidget, remove })
  fireEvent.load(scripts[0] as HTMLScriptElement)
  await waitFor(() => expect(renderWidget).toHaveBeenCalledOnce())
  view.unmount()
  expect(remove).toHaveBeenCalledOnce()
})
