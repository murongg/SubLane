import { z } from 'zod'
import { request } from './request'
import { gatewayStatusSchema } from './connection'

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
    status: gatewayStatusSchema,
  }),
})

export async function getSystem(signal?: AbortSignal) {
  return request('/api/system', systemSchema, { signal })
}
