import { queryOptions } from '@tanstack/react-query'
import { z } from 'zod'
import { ApiError, request } from './request'
import type { en } from '@/locales/en'

const configSchema = z.object({
  scheduled: z.boolean(),
  exhausted: z.boolean(),
  frequency: z.enum(['once', 'daily', 'weekly']),
  time: z.string().max(5),
  date: z.string().max(10),
  weekday: z.number().int().min(0).max(6),
  daily_limit: z.number().int().min(1).max(100),
})
const stateSchema = z.object({
  config: configSchema,
  next_run_at: z.number().int().nonnegative(),
  time_zone: z.string().min(1).max(64),
  used_today: z.number().int().nonnegative(),
  pending: z.boolean(),
  history: z
    .array(
      z.object({
        trigger: z.enum(['manual', 'scheduled', 'exhausted']),
        outcome: z.enum(['pending', 'success', 'skipped', 'failed']),
        reason: z.string().max(64),
        started_at: z.number().int().nonnegative(),
      }),
    )
    .max(20),
})
export type ResetConfig = z.infer<typeof configSchema>
export type ResetState = z.infer<typeof stateSchema>
export function resetOptions(id: string) {
  return queryOptions({
    queryKey: ['accounts', 'resets', id],
    queryFn: ({ signal }) =>
      request(`/api/accounts/${encodeURIComponent(id)}/resets`, stateSchema, {
        signal,
      }),
    staleTime: 30_000,
    refetchOnWindowFocus: false,
    refetchInterval: 60_000,
  })
}
export function saveResetSettings(id: string, config: ResetConfig) {
  return request(
    `/api/accounts/${encodeURIComponent(id)}/resets`,
    stateSchema,
    { method: 'PUT', body: JSON.stringify(config) },
  )
}
export function consumeResetCredit(id: string, operation: 'consume' | 'retry') {
  return request(
    `/api/accounts/${encodeURIComponent(id)}/resets/${operation}`,
    stateSchema,
    { method: 'POST', body: '{}' },
  )
}
export function resetErrorKey(error: unknown): keyof typeof en {
  if (error instanceof ApiError) {
    switch (error.code) {
      case 'invalid_reset_settings':
        return 'resetInvalid'
      case 'reset_busy':
        return 'resetBusy'
      case 'reset_not_pending':
        return 'resetNoPending'
      case 'demo_read_only':
        return 'demoReadOnly'
      case 'account_disabled':
        return 'accountDisabledHint'
      case 'account_reauthorization_required':
        return 'accountReauthorizeHint'
    }
  }
  return 'resetFailed'
}
export const resetReasons: Record<string, keyof typeof en> = {
  reset: 'resetResultSuccess',
  already_redeemed: 'resetResultConfirmed',
  nothing_to_reset: 'resetResultUnneeded',
  no_credit: 'resetResultNoCards',
  confirmation_required: 'resetResultPending',
  disabled: 'resetResultDisabled',
  reauthorize: 'resetResultReauthorize',
  account_changed: 'resetResultChanged',
  missed: 'resetResultMissed',
  daily_limit: 'resetResultLimit',
  usage_unavailable: 'resetResultUsage',
  quota_unknown: 'resetResultUnknown',
  quota_available: 'resetResultAvailable',
}
