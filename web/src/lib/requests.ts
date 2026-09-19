import { z } from 'zod'
import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { authKey, type AuthState } from './auth'
import { request } from './request'
import { providers } from './accounts'

export const outcomes = [
  'success',
  'incomplete',
  'error',
  'canceled',
  'rejected',
] as const
export const outcomeKeys = {
  success: 'requestSuccess',
  incomplete: 'requestIncomplete',
  error: 'requestFailed',
  canceled: 'requestCanceled',
  rejected: 'requestRejected',
} as const
const tokens = z.number().int().nonnegative().nullable()
const recordSchema = z.object({
  id: z.number().int().positive(),
  user_id: z.number().int(),
  key_id: z.number().int(),
  group_id: z.number().int(),
  account_id: z.string(),
  provider: z.union([z.enum(providers), z.literal('')]),
  model: z.string(),
  transport: z.enum(['http', 'websocket']),
  operation: z.enum(['responses', 'chat', 'compact']),
  started_at: z.number().int(),
  duration_ms: z.number().int().nonnegative(),
  outcome: z.enum(outcomes),
  error_code: z.string(),
  upstream_status: z.number().int().nullable(),
  input_tokens: tokens,
  output_tokens: tokens,
  cached_tokens: tokens,
  username: z.string(),
  key_name: z.string(),
  group_name: z.string(),
  account_name: z.string(),
})
export function requestOptions(
  client: QueryClient,
  userID: number | undefined,
  scope: 'personal' | 'all',
  cursor: number,
  accountID: string,
  outcome: string,
) {
  const query = new URLSearchParams({
    cursor: String(cursor),
    outcome,
  })
  if (scope === 'all') query.set('account_id', accountID)
  return queryOptions({
    queryKey: ['request-history', userID, scope, cursor, accountID, outcome],
    // An old observer must stop before a role/identity change finishes rerendering the route.
    enabled: () => {
      const user = client.getQueryData<AuthState>(authKey)?.user
      return (
        userID !== undefined &&
        user?.id === userID &&
        (scope === 'personal' || user.role === 'admin')
      )
    },
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      request(
        `${scope === 'personal' ? '/api/me/requests' : '/api/requests'}?${query}`,
        z.object({
          requests: z.array(recordSchema).max(50),
          next_cursor: z.number().int().nonnegative(),
        }),
        { signal },
      ),
  })
}
