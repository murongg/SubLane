import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { RequestID } from './RequestID'

it('copies the complete ID even when the list display is compact', async () => {
  const user = userEvent.setup()
  const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
  const value = 'req_synthetic_request_with_a_long_identifier'
  render(<RequestID value={value} compact />)
  expect(screen.getByText(value).getAttribute('title')).toBe(value)
  await user.click(screen.getByRole('button', { name: 'Copy request ID' }))
  expect(copy).toHaveBeenCalledWith(value)
  expect(screen.getByRole('status').textContent).toBe('Copied')
})

it('retains the ID and explains how to copy it when clipboard access fails', async () => {
  const user = userEvent.setup()
  vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(
    new Error('Synthetic clipboard denial'),
  )
  render(<RequestID value="req_synthetic" compact />)
  await user.click(screen.getByRole('button', { name: 'Copy request ID' }))
  expect(screen.getByText('req_synthetic')).toBeTruthy()
  expect(screen.getByRole('status').textContent).toContain('Select the ID')
})

it('does not invent an ID or offer copying for older records', () => {
  render(<RequestID value="" compact />)
  expect(screen.getByText('—').getAttribute('title')).toBe(
    'No request ID was recorded for this call.',
  )
  expect(screen.queryByRole('button')).toBeNull()
})
