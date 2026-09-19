import { z } from 'zod'
import { ApiError, request } from './request'

export const providers = ['codex', 'claude', 'antigravity'] as const
export type Provider = (typeof providers)[number]
export const providerLabels: Record<Provider, string> = {
  codex: 'Codex',
  claude: 'Claude',
  antigravity: 'Antigravity (Gemini)',
}
export const callbackURLs: Record<Provider, string> = {
  codex: 'http://localhost:1455/auth/callback',
  claude: 'http://localhost:54545/callback',
  antigravity: 'http://localhost:51121/oauth-callback',
}

export const accountSchema = z.object({
  id: z.string().min(1),
  provider: z.enum(providers).default('codex'),
  max_concurrency: z.number().int().min(1).max(8).default(2),
  name: z.string(),
  email: z.string(),
  plan: z.string(),
  enabled: z.boolean(),
  status: z.enum(['ready', 'unverified', 'reauth_required']),
  expires_at: z.number().int().nonnegative(),
  created_at: z.number().int().nonnegative(),
  updated_at: z.number().int().nonnegative(),
})
export type Account = z.infer<typeof accountSchema>
const pageSchema = z.object({ accounts: z.array(accountSchema).max(100) })
const authorizationSchema = z.object({
  url: z.url().refine((value) => {
    const url = new URL(value)
    return (
      url.protocol === 'https:' &&
      ['auth.openai.com', 'claude.ai', 'accounts.google.com'].includes(
        url.hostname,
      ) &&
      !url.username &&
      !url.password
    )
  }),
  state: z.string().min(1),
  callback_url: z.url().optional(),
  expires_at: z.number().int().positive(),
})
const modelsSchema = z
  .array(
    z.object({
      id: z.string(),
      object: z.literal('model'),
      owned_by: z.string(),
    }),
  )
  .max(512)
export const accountOptions = {
  queryKey: ['accounts'],
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    request('/api/accounts', pageSchema, { signal }),
}
export function importAccount(input: {
  provider?: Provider
  name: string
  auth_json: string
  replace_id?: string
}) {
  return request('/api/accounts/import', accountSchema, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
export function beginAuthorization(input: {
  provider?: Provider
  name: string
  replace_id?: string
}) {
  return request('/api/accounts/oauth', authorizationSchema, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
export function completeAuthorization(input: {
  state: string
  callback_url: string
}) {
  return request('/api/accounts/oauth/complete', accountSchema, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
export function cancelAuthorization(state: string) {
  return request(
    `/api/accounts/oauth/${encodeURIComponent(state)}`,
    z.undefined(),
    { method: 'DELETE', body: '{}' },
  )
}
export function setAccountEnabled(input: { id: string; enabled: boolean }) {
  return request(
    `/api/accounts/${encodeURIComponent(input.id)}`,
    accountSchema,
    { method: 'PATCH', body: JSON.stringify({ enabled: input.enabled }) },
  )
}
export function deleteAccount(id: string) {
  return request(`/api/accounts/${encodeURIComponent(id)}`, z.undefined(), {
    method: 'DELETE',
    body: '{}',
  })
}
export function checkAccount(id: string) {
  return request(
    `/api/accounts/${encodeURIComponent(id)}/check`,
    z.object({ account: accountSchema, models: modelsSchema }),
    { method: 'POST', body: '{}' },
  )
}

const messages = {
  invalid_account_input: 'accountInvalidInput',
  account_exists: 'accountExists',
  account_identity_mismatch: 'accountIdentityMismatch',
  account_limit: 'accountLimitReached',
  account_disabled: 'accountDisabledHint',
  account_reauthorization_required: 'accountReauthorizeHint',
  account_refresh_failed: 'accountRefreshFailed',
  oauth_state_invalid: 'oauthStateInvalid',
  oauth_callback_invalid: 'oauthCallbackInvalid',
  oauth_access_denied: 'oauthAccessDenied',
  oauth_busy: 'accountBusy',
  gateway_busy: 'accountBusy',
  upstream_unavailable: 'accountUpstreamUnavailable',
  upstream_rejected_request: 'accountCheckFailed',
  upstream_rate_limited: 'accountRateLimited',
} as const
export function accountErrorKey(error: unknown) {
  const code = error instanceof ApiError ? error.code : ''
  return Object.hasOwn(messages, code)
    ? messages[code as keyof typeof messages]
    : 'accountActionFailed'
}
