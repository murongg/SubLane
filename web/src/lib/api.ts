import { z } from 'zod'

const systemSchema = z.object({
  name: z.literal('SubLane'),
  version: z.string(),
  status: z.literal('ok'),
  uptime_seconds: z.number().int().nonnegative(),
  storage: z.object({
    engine: z.literal('sqlite'),
    status: z.literal('ready'),
  }),
  gateway: z.object({
    provider: z.literal('codex'),
    status: z.literal('not_configured'),
  }),
})

export async function getSystem(signal?: AbortSignal) {
  const response = await fetch('/api/system', {
    signal,
    headers: { Accept: 'application/json' },
  })
  if (!response.ok) throw new Error('unavailable')
  const result = systemSchema.safeParse(await response.json())
  if (!result.success) throw new Error('invalid_response')
  return result.data
}
