import { z } from 'zod'
import { request } from './request'

const windowSchema = z.object({
  kind: z.enum(['primary', 'secondary']),
  used_percent: z.number().nonnegative().nullable(),
  window_seconds: z.number().int().positive().nullable(),
  reset_at: z.number().int().nonnegative().nullable(),
})
const usageSchema = z.object({
  limits: z
    .array(
      z.object({
        name: z.string(),
        allowed: z.boolean().nullable(),
        limit_reached: z.boolean().nullable(),
        windows: z.array(windowSchema).max(2),
      }),
    )
    .max(33),
  updated_at: z.number().int().positive(),
})
export type UsageWindow = z.infer<typeof windowSchema>
export function usageOptions(id: string) {
  return {
    queryKey: ['accounts', 'usage', id],
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      request(`/api/accounts/${encodeURIComponent(id)}/usage`, usageSchema, {
        signal,
      }),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  }
}
