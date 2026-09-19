import { z } from 'zod'
import { request } from './request'

const memberSchema = z.object({
  id: z.number().int().positive(),
  username: z.string().min(3).max(32),
  role: z.literal('member'),
  enabled: z.boolean(),
  created_at: z.number().int().nonnegative(),
})
const pageSchema = z.object({
  members: z.array(memberSchema).max(50),
  next_cursor: z.number().int().nonnegative(),
})
export type Member = z.infer<typeof memberSchema>

export function getMembers(cursor: number, signal?: AbortSignal) {
  return request(`/api/members?cursor=${cursor}`, pageSchema, { signal })
}
export function createMember(input: { username: string; password: string }) {
  return request('/api/members', memberSchema, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
export function setMemberEnabled(input: { id: number; enabled: boolean }) {
  return request(`/api/members/${input.id}`, memberSchema, {
    method: 'PATCH',
    body: JSON.stringify({ enabled: input.enabled }),
  })
}

export function resetMemberPassword(id: number, password: string) {
  return request(`/api/members/${id}/password`, z.undefined(), {
    method: 'POST',
    body: JSON.stringify({ new_password: password }),
  })
}
