import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, it } from 'vitest'
import { RegexInput } from './RegexInput'

function Editor({ initial = '' }: { initial?: string }) {
  const [value, setValue] = useState(initial)
  return (
    <>
      <label htmlFor="synthetic-regex">Synthetic expression</label>
      <RegexInput
        id="synthetic-regex"
        value={value}
        onChange={setValue}
        maxLength={1024}
      />
    </>
  )
}

it('highlights regex tokens while preserving native editing and exact characters', async () => {
  const pattern = String.raw`^(DEMO_TOKEN|SYNTHETIC)_[A-Z0-9]{8}\.$`
  const { container } = render(<Editor />)
  const input = screen.getByLabelText(
    'Synthetic expression',
  ) as HTMLTextAreaElement
  await userEvent.setup().type(input, 'DEMO_TOKEN')
  expect(input.value).toBe('DEMO_TOKEN')
  fireEvent.change(input, { target: { value: pattern } })
  expect(input.value).toBe(pattern)
  expect(input.maxLength).toBe(1024)
  expect(container.querySelector('[aria-hidden="true"]')?.textContent).toBe(
    pattern,
  )
  expect(
    container.querySelector('[data-token="char-class"]')?.textContent,
  ).toBe('[A-Z0-9]')
  expect(
    container.querySelector('[data-token="quantifier"]')?.textContent,
  ).toBe('{8}')
  expect(container.querySelector('[data-token="anchor"]')?.textContent).toBe(
    '^',
  )
})

it.each([
  '[unfinished',
  String.fromCharCode(92),
  String.raw`^研发_\p{Han}{1,8}$`,
  'x'.repeat(1024),
])(
  'preserves incomplete and Unicode expressions while editing: %s',
  (pattern) => {
    const { container } = render(<Editor initial={pattern} />)
    expect(
      (screen.getByLabelText('Synthetic expression') as HTMLTextAreaElement)
        .value,
    ).toBe(pattern)
    expect(container.querySelector('[aria-hidden="true"]')?.textContent).toBe(
      pattern,
    )
  },
)

it('renders untrusted expressions as text and synchronizes both scroll directions', () => {
  const pattern = '<img src=x onerror=synthetic()>[A-Z]{4}\n^SYNTHETIC$'
  const { container } = render(<Editor initial={pattern} />)
  const input = screen.getByLabelText(
    'Synthetic expression',
  ) as HTMLTextAreaElement
  expect(container.querySelector('img')).toBeNull()
  expect(container.querySelector('[aria-hidden="true"]')?.textContent).toBe(
    pattern,
  )
  input.scrollLeft = 56
  input.scrollTop = 24
  fireEvent.scroll(input)
  expect(container.querySelector('pre')?.style.transform).toBe(
    'translate(-56px, -24px)',
  )
  expect(input.value).toBe(pattern)
})

it('keeps empty saved values hidden and respects fieldset disabling', () => {
  render(
    <fieldset disabled>
      <label htmlFor="saved-regex">Saved expression</label>
      <RegexInput
        id="saved-regex"
        value=""
        onChange={() => {}}
        placeholder="Keep saved content"
        required
      />
    </fieldset>,
  )
  const input = screen.getByLabelText('Saved expression') as HTMLTextAreaElement
  expect(input.value).toBe('')
  expect(input.placeholder).toBe('Keep saved content')
  expect(input.required).toBe(true)
  expect(input.matches(':disabled')).toBe(true)
})
