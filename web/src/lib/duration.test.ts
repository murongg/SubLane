import { expect, it } from 'vitest'
import { formatDuration } from './duration'

it.each([
  [0, '0 sec'],
  [59, '59 sec'],
  [60, '1 min'],
  [3661, '1 hr 1 min'],
  [90061, '1 day 1 hr'],
])('formats %i seconds into at most two useful units', (seconds, expected) => {
  expect(formatDuration(seconds, 'en')).toBe(expected)
})

it('uses Chinese duration units', () => {
  expect(formatDuration(90061, 'zh')).toBe('1天 1小时')
})
