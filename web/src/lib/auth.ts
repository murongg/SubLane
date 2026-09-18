import { queryOptions } from '@tanstack/react-query'
import { z } from 'zod'
import { request } from './request'

const authSchema = z
  .object({
    initialized: z.boolean(),
    user: z.object({ username: z.string().min(1).max(32) }).nullable(),
  })
  .refine((value) => value.initialized || value.user === null)

export type AuthState = z.infer<typeof authSchema>
export const authKey = ['auth'] as const

export const authOptions = () =>
  queryOptions({
    queryKey: authKey,
    queryFn: ({ signal }) => request('/api/auth/state', authSchema, { signal }),
    staleTime: 60_000,
    refetchOnWindowFocus: true,
    refetchInterval: 60_000,
  })

export function signIn(input: { username: string; password: string }) {
  return request('/api/auth/login', authSchema, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function setup(input: { username: string; password: string }) {
  return request('/api/auth/setup', authSchema, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function signOut() {
  return request('/api/auth/logout', authSchema, { method: 'POST', body: '{}' })
}
