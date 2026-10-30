import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { authKey, type AuthState } from './auth'
import { ApiError, request } from './request'

const securitySchema = z.object({
  totp_enabled: z.boolean(),
  recovery_codes: z.number().int().nonnegative(),
  passkeys: z.array(
    z.object({
      id: z.number().int().positive(),
      name: z.string(),
      created_at: z.number().int(),
      last_used_at: z.number().int().nullable(),
    }),
  ),
})
export type SecurityState = z.infer<typeof securitySchema>
export type Proof = { current_password: string; code: string }
export const setupSchema = z.object({
  secret: z.string(),
  uri: z.string().startsWith('otpauth://'),
  qr: z.string().startsWith('data:image/png;base64,'),
})
export type TOTPSetup = z.infer<typeof setupSchema>
const recoverySchema = z.object({ recovery_codes: z.array(z.string()) })

export function securityOptions(client: QueryClient) {
  const userID = client.getQueryData<AuthState>(authKey)?.user?.id
  return queryOptions({
    queryKey: ['security', userID],
    queryFn: ({ signal }) =>
      request('/api/me/security', securitySchema, { signal }),
    enabled: () =>
      !!userID && client.getQueryData<AuthState>(authKey)?.user?.id === userID,
  })
}
export function beginTOTP(proof: Proof) {
  return request('/api/me/totp', setupSchema, {
    method: 'POST',
    body: JSON.stringify(proof),
  })
}
export function confirmTOTP(code: string) {
  return request('/api/me/totp/confirm', recoverySchema, {
    method: 'POST',
    body: JSON.stringify({ code }),
  })
}
export function disableTOTP(proof: Proof) {
  return request('/api/me/totp', z.undefined(), {
    method: 'DELETE',
    body: JSON.stringify(proof),
  })
}
export function regenerateRecoveryCodes(proof: Proof) {
  return request('/api/me/recovery-codes', recoverySchema, {
    method: 'POST',
    body: JSON.stringify(proof),
  })
}
export function deletePasskey(id: number, proof: Proof) {
  return request(`/api/me/passkeys/${id}`, z.undefined(), {
    method: 'DELETE',
    body: JSON.stringify(proof),
  })
}
export function passkeySupported() {
  return (
    window.isSecureContext &&
    typeof PublicKeyCredential !== 'undefined' &&
    typeof PublicKeyCredential.parseCreationOptionsFromJSON === 'function' &&
    typeof PublicKeyCredential.parseRequestOptionsFromJSON === 'function' &&
    typeof PublicKeyCredential.prototype.toJSON === 'function'
  )
}
// Browser JSON helpers validate and convert WebAuthn's binary fields at the API boundary.
const creationSchema = z.object({
  publicKey: z.custom<PublicKeyCredentialCreationOptionsJSON>(
    (value) =>
      typeof value === 'object' &&
      value !== null &&
      'challenge' in value &&
      'user' in value &&
      'rp' in value &&
      'pubKeyCredParams' in value,
  ),
})
const assertionSchema = z.object({
  publicKey: z.custom<PublicKeyCredentialRequestOptionsJSON>(
    (value) =>
      typeof value === 'object' && value !== null && 'challenge' in value,
  ),
})
export async function addPasskey(name: string, proof: Proof) {
  if (!passkeySupported()) throw new ApiError('passkey_unavailable', 400)
  const options = await request('/api/me/passkeys/begin', creationSchema, {
    method: 'POST',
    body: JSON.stringify(proof),
  })
  const credential = await navigator.credentials.create({
    publicKey: PublicKeyCredential.parseCreationOptionsFromJSON(
      options.publicKey,
    ),
  })
  if (!(credential instanceof PublicKeyCredential))
    throw new ApiError('passkey_failed', 400)
  return request('/api/me/passkeys/finish', z.undefined(), {
    method: 'POST',
    body: JSON.stringify({ name, credential: credential.toJSON() }),
  })
}
export async function passkeyAssertion(turnstileToken?: string) {
  if (!passkeySupported()) throw new ApiError('passkey_unavailable', 400)
  const options = await request('/api/auth/passkey/begin', assertionSchema, {
    method: 'POST',
    body: JSON.stringify(
      turnstileToken ? { turnstile_token: turnstileToken } : {},
    ),
  })
  const credential = await navigator.credentials.get({
    publicKey: PublicKeyCredential.parseRequestOptionsFromJSON(
      options.publicKey,
    ),
  })
  if (!(credential instanceof PublicKeyCredential))
    throw new ApiError('passkey_failed', 400)
  return credential.toJSON()
}
export const securityErrors = {
  current_password_invalid: 'currentPasswordInvalid',
  password_changed: 'passwordChangedRetry',
  two_factor_required: 'factorRequired',
  invalid_factor: 'factorInvalid',
  challenge_expired: 'securityExpired',
  passkey_failed: 'passkeyFailed',
  passkey_unavailable: 'passkeyUnavailable',
  invalid_input: 'securityInvalid',
  rate_limited: 'authRateLimited',
  auth_busy: 'authBusy',
  origin_rejected: 'authOriginError',
  turnstile_failed: 'turnstileFailed',
  turnstile_unavailable: 'turnstileUnavailable',
} as const
export function securityError(error: Error | null) {
  if (
    error instanceof DOMException &&
    (error.name === 'NotAllowedError' || error.name === 'AbortError')
  )
    return 'passkeyCancelled'
  const code = error instanceof ApiError ? error.code : ''
  return Object.hasOwn(securityErrors, code)
    ? securityErrors[code as keyof typeof securityErrors]
    : 'unavailable'
}
