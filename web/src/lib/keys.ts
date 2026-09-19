import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { authKey, type AuthState } from './auth'
import { request } from './request'

const keySchema = z.object({
  id: z.number().int().positive(),
  group_id: z.number().int().positive(),
  group_name: z.string().min(1),
  group_access: z.enum(['allowed', 'blocked']),
  name: z.string().min(1).max(256),
  prefix: z.string().min(1).max(20),
  created_at: z.number().int().nonnegative(),
  last_used_at: z.number().int().nullable(),
  revoked_at: z.number().int().nullable(),
})
const pageSchema = z.object({
  keys: z.array(keySchema).max(50),
  next_cursor: z.number().int().nonnegative(),
})
const createdSchema = z.object({
  key: keySchema,
  secret: z.string().regex(/^sl_[A-Za-z0-9_-]{43}$/),
})
export type APIKey = z.infer<typeof keySchema>

export function keyOptions(
  client: QueryClient,
  userID: number,
  cursor: number,
) {
  return queryOptions({
    queryKey: ['keys', userID, cursor],
    queryFn: ({ signal }) =>
      request(`/api/keys?cursor=${cursor}`, pageSchema, { signal }),
    // These personal queries opt into member access and stop immediately if the active identity changes.
    enabled: () =>
      userID > 0 &&
      client.getQueryData<AuthState>(authKey)?.user?.id === userID,
  })
}
export function createKey(input: { name: string; group_id: number }) {
  return request('/api/keys', createdSchema, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
export function revokeKey(id: number) {
  return request(`/api/keys/${id}/revoke`, keySchema, {
    method: 'POST',
    body: '{}',
  })
}
