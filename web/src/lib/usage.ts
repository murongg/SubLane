import { queryOptions } from '@tanstack/react-query'
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
  server_time: z.number().int().positive(),
  expires_at: z.number().int().positive(),
  stale: z.boolean(),
  refreshing: z.boolean(),
  refresh_failed: z.boolean(),
  retry_after_seconds: z.number().int().nonnegative(),
})
export type UsageWindow = z.infer<typeof windowSchema>
export function usageOptions(id: string) {
  return queryOptions({
    queryKey: ['accounts', 'usage', id],
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      request(`/api/accounts/${encodeURIComponent(id)}/usage`, usageSchema, {
        signal,
      }),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    // Poll only while the server has a bounded refresh in flight; stop on network failures.
    refetchInterval: (query) =>
      query.state.status !== 'error' && query.state.data?.refreshing
        ? 1000
        : false,
  })
}

export function refreshUsage(id: string) {
  return request(
    `/api/accounts/${encodeURIComponent(id)}/usage/refresh`,
    usageSchema,
    { method: 'POST', body: '{}' },
  )
}
