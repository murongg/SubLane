export function magpieLink(origin: string, name: string, secret: string) {
  const params = new URLSearchParams({
    name: `SubLane · ${name}`,
    chat: `${origin}/v1`,
    responses: `${origin}/v1`,
    anthropic: origin,
    gemini: `${origin}/v1beta`,
    key: secret,
  })
  // Leave models unset so Magpie discovers the current catalog allowed by this key.
  return `magpie://import?${params.toString()}`
}

export function openMagpie(link: string) {
  const url = new URL(link)
  if (
    url.protocol !== 'magpie:' ||
    url.host !== 'import' ||
    url.pathname !== ''
  ) {
    throw new Error('invalid_magpie_link')
  }
  // Keep the credential-bearing link in the local app protocol, outside web relays and history APIs.
  window.location.assign(link)
}
