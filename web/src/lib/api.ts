import { z } from 'zod'
import { request } from './request'

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
  return request('/api/system', systemSchema, { signal })
}
