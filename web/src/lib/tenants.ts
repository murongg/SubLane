import { queryOptions } from '@tanstack/react-query'
import { z } from 'zod'
import { request } from './request'

const tenantSchema = z.object({
  id: z.number().int().positive(),
  name: z.string().min(1).max(64),
  status: z.enum(['active', 'suspended']),
})

export const tenantsOptions = queryOptions({
  queryKey: ['tenants'],
  // Workspace recovery must work even when the selected workspace has no active membership.
  enabled: true,
  queryFn: ({ signal }) =>
    request(
      '/api/workspaces',
      z.object({ tenants: z.array(tenantSchema).max(100) }),
      {
        signal,
      },
    ),
})

export function createWorkspace(input: { name: string }) {
  return request('/api/workspaces', tenantSchema, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
