import { queryOptions } from '@tanstack/react-query'
import { z } from 'zod'
import { request } from './request'

const schema = z.object({
  enabled: z.boolean(),
  site_key: z.string(),
  secret_configured: z.boolean(),
})
export const turnstileOptions = (userID: number) =>
  queryOptions({
    queryKey: ['turnstile', userID],
    queryFn: ({ signal }) =>
      request('/api/settings/turnstile', schema, { signal }),
  })
export function saveTurnstile(
  input: { enabled: boolean; site_key: string; secret_key?: string },
  signal: AbortSignal,
) {
  return request('/api/settings/turnstile', schema, {
    method: 'PATCH',
    body: JSON.stringify(input),
    signal,
  })
}
