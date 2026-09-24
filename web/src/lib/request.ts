import { z } from 'zod'
import { selectedWorkspace } from './workspace'

export class ApiError extends Error {
  constructor(
    public code: string,
    public status: number,
    public retryAfter = 0,
  ) {
    super(code)
  }
}

export async function request<T>(
  path: string,
  schema: z.ZodType<T>,
  init?: RequestInit,
): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: 'same-origin',
    headers: {
      Accept: 'application/json',
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      'X-SubLane-Workspace': String(selectedWorkspace()),
      ...init?.headers,
    },
  })
  if (!response.ok) {
    const body: unknown = await response.json().catch(() => null)
    const parsed = z.object({ error: z.string() }).safeParse(body)
    const fallback = response.status === 401 ? 'unauthorized' : 'unavailable'
    const retry = Number(response.headers.get('Retry-After'))
    throw new ApiError(
      parsed.success ? parsed.data.error : fallback,
      response.status,
      Number.isFinite(retry) ? Math.max(0, retry) : 0,
    )
  }
  const body: unknown =
    response.status === 204
      ? undefined
      : await response.json().catch(() => null)
  const parsed = schema.safeParse(body)
  if (!parsed.success) throw new ApiError('invalid_response', response.status)
  return parsed.data
}
