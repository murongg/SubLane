import { expect, it } from 'vitest'
import { usagePace, type UsageWindow } from './usage'

const snapshot = {
  updated_at: 1800,
  server_time: 2100,
  expires_at: 2400,
  stale: false,
}
const window: UsageWindow = {
  kind: 'primary',
  used_percent: 70,
  window_seconds: 3600,
  reset_at: 3600,
}

it.each([
  [70, 'ahead', 20],
  [30, 'behind', 20],
  [50, 'even', 0],
  [50.04, 'even', 0],
  [50.06, 'ahead', 0.1],
  [0, 'behind', 50],
  [100, 'ahead', 50],
] as const)(
  'compares %s percent usage against elapsed time at the observation',
  (used_percent, direction, points) => {
    expect(usagePace({ ...window, used_percent }, snapshot)).toEqual({
      direction,
      points,
    })
  },
)

it('does not project the old percentage forward as the response clock advances', () => {
  expect(usagePace(window, snapshot, 2399)).toEqual({
    direction: 'ahead',
    points: 20,
  })
})

it.each([
  ['unknown usage', { used_percent: null }],
  ['usage above 100%', { used_percent: 101 }],
  ['negative usage', { used_percent: -1 }],
  ['non-finite usage', { used_percent: Number.NaN }],
  ['missing duration', { kind: 'model', window_seconds: null }],
  ['zero duration', { window_seconds: 0 }],
  ['negative duration', { window_seconds: -1 }],
  ['non-finite duration', { window_seconds: Infinity }],
  ['missing reset', { reset_at: null }],
  ['non-finite reset', { reset_at: Number.NaN }],
  ['reset at observation', { reset_at: 1800 }],
  ['already reset at server time', { reset_at: 2100 }],
  ['observation before window start', { reset_at: 5500 }],
  ['window start before epoch', { window_seconds: 4000 }],
] satisfies [string, Partial<UsageWindow>][])(
  'hides pace for %s',
  (_reason, invalid) => {
    expect(usagePace({ ...window, ...invalid }, snapshot)).toBeNull()
  },
)

it.each([
  ['stale observation', { stale: true }],
  ['expired snapshot', { expires_at: 2100 }],
  ['future observation', { updated_at: 2101 }],
  ['missing observation time', { updated_at: Number.NaN }],
  ['zero observation time', { updated_at: 0 }],
  ['missing server clock', { server_time: Number.NaN }],
  ['missing expiry', { expires_at: Number.NaN }],
])('hides pace for %s', (_reason, invalid) => {
  expect(usagePace(window, { ...snapshot, ...invalid })).toBeNull()
})

it('hides pace once the browser clock reaches expiry or reset', () => {
  expect(usagePace(window, snapshot, snapshot.expires_at)).toBeNull()
  expect(
    usagePace(window, { ...snapshot, expires_at: 4000 }, window.reset_at!),
  ).toBeNull()
  expect(usagePace(window, snapshot, Number.NaN)).toBeNull()
})
