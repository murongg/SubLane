export type CredentialError = {
  field: 'username' | 'password' | 'confirm'
  message:
    | 'usernameHint'
    | 'passwordRequired'
    | 'passwordHint'
    | 'passwordMismatch'
}

export function validateCredentials(
  input: { username: string; password: string },
  confirmation?: string,
): CredentialError | null {
  if (!/^[a-zA-Z0-9][a-zA-Z0-9_-]{2,31}$/.test(input.username))
    return { field: 'username', message: 'usernameHint' }
  // Match the backend's Unicode character count rather than UTF-16 code units.
  const length = Array.from(input.password).length
  if (!length) return { field: 'password', message: 'passwordRequired' }
  if (confirmation !== undefined) {
    if (length < 8 || length > 20)
      return { field: 'password', message: 'passwordHint' }
    if (input.password !== confirmation)
      return { field: 'confirm', message: 'passwordMismatch' }
  }
  return null
}
