import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it } from 'vitest'
import { ModelPicker } from './ModelPicker'

function Picker() {
  const [value, setValue] = useState('')
  return (
    <ModelPicker
      id="test-model"
      value={value}
      onChange={setValue}
      models={['synthetic-basic', 'synthetic-premium']}
    />
  )
}
it('filters discovered model IDs and supports keyboard selection', async () => {
  const user = userEvent.setup()
  render(<Picker />)
  const input = screen.getByRole('combobox', { name: 'Model ID' })
  await user.click(input)
  expect(screen.getAllByRole('option')).toHaveLength(2)
  await user.type(input, 'premium')
  expect(screen.getAllByRole('option')).toHaveLength(1)
  await user.keyboard('{ArrowDown}{Enter}')
  expect((input as HTMLInputElement).value).toBe('synthetic-premium')
  expect(screen.queryByRole('listbox')).toBeNull()
})
it('keeps the empty state distinct from a matching model', async () => {
  const user = userEvent.setup()
  render(<Picker />)
  await user.type(screen.getByRole('combobox'), 'unknown')
  expect(screen.queryByRole('option')).toBeNull()
  expect(screen.getByText('No matching models.')).toBeTruthy()
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('listbox')).toBeNull()
})
