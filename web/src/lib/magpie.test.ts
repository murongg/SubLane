import { expect, it, vi } from 'vitest'
import { magpieLink, openMagpie } from './magpie'

const secret = 'sl_' + 'x'.repeat(43)

it('encodes a provider with all native gateway protocols and automatic model discovery', () => {
  const link = new URL(
    magpieLink('https://gateway.example.test', 'Synthetic & 测试', secret),
  )
  expect(link.protocol).toBe('magpie:')
  expect(link.host).toBe('import')
  expect(Object.fromEntries(link.searchParams)).toEqual({
    name: 'SubLane · Synthetic & 测试',
    chat: 'https://gateway.example.test/v1',
    responses: 'https://gateway.example.test/v1',
    anthropic: 'https://gateway.example.test',
    gemini: 'https://gateway.example.test/v1beta',
    key: secret,
  })
})

it('launches only the local Magpie import protocol', () => {
  const assign = vi.fn()
  vi.stubGlobal('window', { location: { assign } })
  const link = magpieLink('http://127.0.0.1:8080', 'Synthetic key', secret)
  openMagpie(link)
  expect(assign).toHaveBeenCalledExactlyOnceWith(link)
  for (const invalid of [
    'https://example.test/import?key=synthetic',
    'magpie://other?key=synthetic',
    'magpie://import/other?key=synthetic',
  ]) {
    expect(() => openMagpie(invalid)).toThrow()
  }
  expect(assign).toHaveBeenCalledTimes(1)
})
