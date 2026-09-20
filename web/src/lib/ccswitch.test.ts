import { describe, expect, it } from 'vitest'
import { ccSwitchLink, validImportSettings } from './ccswitch'
const secret = 'sl_' + 'x'.repeat(43)
const settings = {
  origin: 'https://gateway.example.test:8443',
  name: 'SubLane · 研发 "A&B"',
  model: 'codex/synthetic-model(high)',
}
describe('CC Switch provider import contract', () => {
  it('encodes a Codex provider with an exact model and no automatic activation', () => {
    const link = new URL(ccSwitchLink({ ...settings, secret }))
    expect(link.protocol).toBe('ccswitch:')
    expect(link.host).toBe('v1')
    expect(link.pathname).toBe('/import')
    expect(Object.fromEntries(link.searchParams)).toEqual({
      resource: 'provider',
      app: 'codex',
      name: settings.name,
      endpoint: settings.origin + '/v1',
      apiKey: secret,
      model: settings.model,
      homepage: settings.origin,
      enabled: 'false',
    })
  })
  it('supports localhost and normalizes whitespace without changing the key', () => {
    const link = new URL(
      ccSwitchLink({
        ...settings,
        origin: 'http://localhost:18850/',
        name: '  Synthetic key  ',
        model: '  codex/synthetic-model  ',
        secret,
      }),
    )
    expect(link.searchParams.get('endpoint')).toBe('http://localhost:18850/v1')
    expect(link.searchParams.get('name')).toBe('Synthetic key')
    expect(link.searchParams.get('model')).toBe('codex/synthetic-model')
    expect(link.searchParams.get('apiKey')).toBe(secret)
  })
  it('rejects incomplete or injected configuration before a secret is requested', () => {
    for (const input of [
      { ...settings, name: '' },
      { ...settings, name: 'injected\nname' },
      { ...settings, model: '' },
      { ...settings, model: 'model\nkey="value"' },
      { ...settings, model: 'x'.repeat(161) },
      { ...settings, origin: 'javascript:alert(1)' },
      { ...settings, origin: 'https://user:password@example.test' },
      { ...settings, origin: 'https://gateway.example.test/?key=secret' },
      { ...settings, origin: 'https://gateway.example.test/other' },
    ]) {
      expect(validImportSettings(input)).toBe(false)
      expect(() => ccSwitchLink({ ...input, secret })).toThrow()
    }
    expect(() =>
      ccSwitchLink({ ...settings, secret: 'not-a-sublane-key' }),
    ).toThrow()
  })
})
