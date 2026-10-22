import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { authKey, type AuthState } from './auth'
import { request } from './request'

export type CatalogTarget =
  | { kind: 'account' | 'channel'; id: string }
  | { kind: 'group' | 'key'; id: number }
const snapshotSchema = z.object({
  models: z.array(z.string().min(1).max(128)).max(512),
  hidden_models: z.array(z.string().min(1).max(128)).max(512).default([]),
  updated_at: z.number().int().nonnegative(),
  server_time: z.number().int(),
  known: z.boolean(),
  stale: z.boolean(),
  usable: z.boolean(),
  refreshing: z.boolean(),
  refresh_failed: z.boolean(),
  retry_after_seconds: z.number().nonnegative(),
})
const groupSchema = z.object({
  hidden_models: z.array(z.string().min(1).max(160)).max(4096).default([]),
  models: z
    .array(
      z.object({
        id: z.string().min(1).max(160),
        object: z.literal('model'),
        owned_by: z.string(),
      }),
    )
    .max(4096),
  known_accounts: z.number().int().nonnegative(),
  unknown_accounts: z.number().int().nonnegative(),
  stale_accounts: z.number().int().nonnegative(),
  refreshing: z.boolean(),
  refresh_failed: z.boolean(),
  server_time: z.number().int(),
})
export type CatalogView = z.infer<typeof snapshotSchema> & { partial: boolean }
const modelCheckSchema = z.object({
  model: z.string().min(1).max(128),
  available: z.boolean(),
  duration_ms: z.number().int().nonnegative(),
  error_code: z.string().max(64),
})
export type ModelCheck = z.infer<typeof modelCheckSchema>
export function checkModel(
  target: Extract<CatalogTarget, { kind: 'account' | 'channel' }>,
  model: string,
  signal: AbortSignal,
) {
  return request(`${catalogPath(target)}/check`, modelCheckSchema, {
    method: 'POST',
    body: JSON.stringify({ model }),
    signal,
  })
}
export function catalogPath(target: CatalogTarget) {
  const collections = {
    account: 'accounts',
    channel: 'channels',
    group: 'groups',
    key: 'keys',
  }
  return `/api/${collections[target.kind]}/${encodeURIComponent(target.id)}/models`
}
async function readCatalog(
  target: CatalogTarget,
  signal: AbortSignal,
): Promise<CatalogView> {
  if (target.kind === 'account' || target.kind === 'channel')
    return {
      ...(await request(catalogPath(target), snapshotSchema, { signal })),
      partial: false,
    }
  const value = await request(catalogPath(target), groupSchema, { signal })
  return {
    models: value.models.map((model) => model.id),
    hidden_models: value.hidden_models,
    updated_at: 0,
    server_time: value.server_time,
    known: value.known_accounts > 0 || value.unknown_accounts === 0,
    usable: value.known_accounts > 0 || value.unknown_accounts === 0,
    stale: value.stale_accounts > 0,
    refreshing: value.refreshing,
    refresh_failed: value.refresh_failed,
    retry_after_seconds: 0,
    partial: value.unknown_accounts > 0,
  }
}
export function catalogOptions(
  client: QueryClient,
  target: CatalogTarget,
  userID = client.getQueryData<AuthState>(authKey)?.user?.id ?? 0,
) {
  return queryOptions({
    queryKey: ['model-catalog', userID, target.kind, target.id],
    queryFn: ({ signal }) => readCatalog(target, signal),
    enabled: () => {
      const user = client.getQueryData<AuthState>(authKey)?.user
      return (
        userID > 0 &&
        user?.id === userID &&
        (target.kind === 'key' || user.role === 'admin')
      )
    },
    staleTime: 15_000,
    refetchInterval: (query) =>
      !query.state.error && query.state.data?.refreshing ? 1000 : false,
  })
}
export async function refreshCatalog(
  target: { kind: 'account' | 'channel'; id: string },
  signal?: AbortSignal,
): Promise<CatalogView> {
  return {
    ...(await request(`${catalogPath(target)}/refresh`, snapshotSchema, {
      method: 'POST',
      body: '{}',
      signal,
    })),
    partial: false,
  }
}
