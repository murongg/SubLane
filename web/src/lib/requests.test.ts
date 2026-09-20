import { expect, it } from 'vitest'
import { cacheHitRate } from './requests'

it.each([
  [1250, 1070, 0.856],
  [1000, 0, 0],
  [1000, 1000, 1],
  [0, 0, null],
  [null, 100, null],
  [1000, null, null],
  [100, 101, null],
  [-1, 0, null],
  [100, -1, null],
])(
  'calculates cache hit rate for input=%s, cached=%s',
  (input, cached, expected) => {
    expect(cacheHitRate(input, cached)).toBe(expected)
  },
)
