import { z } from 'zod'
import { queryOptions } from '@tanstack/react-query'
import { ApiError, request } from './request'

export const contentModes = ['off', 'observe', 'block'] as const
export const contentKinds = ['text', 'regex'] as const
export const contentModeKeys = {
  off: 'contentOff',
  observe: 'contentObserve',
  block: 'contentBlock',
} as const
export const contentKindKeys = {
  text: 'contentText',
  regex: 'contentRegex',
} as const
const stateSchema = z.object({
  mode: z.enum(contentModes),
  revision: z.number().int().nonnegative(),
  rules: z
    .array(
      z.object({
        id: z.string().max(64),
        name: z.string().max(64),
        kind: z.enum(contentKinds),
        enabled: z.boolean(),
      }),
    )
    .max(50),
})
export type ContentState = z.infer<typeof stateSchema>
export type ContentRule = ContentState['rules'][number] & { pattern?: string }
export type ContentInput = {
  mode: ContentState['mode']
  revision: number
  rules: ContentRule[]
}
export const contentOptions = (userID: number) =>
  queryOptions({
    queryKey: ['content-rules', userID],
    queryFn: ({ signal }) => request('/api/content', stateSchema, { signal }),
  })
export const saveContent = (input: ContentInput, signal: AbortSignal) =>
  request('/api/content', stateSchema, {
    method: 'PUT',
    body: JSON.stringify(input),
    signal,
  })
export const testContent = (
  input: { rule: ContentRule; sample: string },
  signal: AbortSignal,
) =>
  request('/api/content/test', z.object({ matched: z.boolean() }), {
    method: 'POST',
    body: JSON.stringify(input),
    signal,
  })
export function contentError(error: unknown) {
  if (error instanceof ApiError) {
    if (error.code === 'invalid_content_rules') return 'contentInvalid'
    if (error.code === 'content_rules_changed') return 'contentConflict'
    if (error.code === 'demo_read_only') return 'demoReadOnly'
  }
  return 'contentUnavailable'
}
