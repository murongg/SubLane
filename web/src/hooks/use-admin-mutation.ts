import { useEffect, useRef } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { authKey, type AuthState } from '@/lib/auth'

export function useAdminMutation<T, Input = void>({
  userID,
  mutationFn,
  onSuccess,
  onError,
}: {
  userID: number
  mutationFn: (input: Input, signal: AbortSignal) => Promise<T>
  onSuccess?: (value: T) => unknown
  onError?: (error: Error) => unknown
}) {
  const client = useQueryClient()
  const active = useRef(false)
  const pending = useRef<AbortController | null>(null)
  useEffect(() => {
    active.current = true
    return () => {
      active.current = false
      pending.current?.abort()
    }
  }, [])
  const current = () => {
    const user = client.getQueryData<AuthState>(authKey)?.user
    return active.current && user?.id === userID && user.role === 'admin'
  }
  return useMutation<T | undefined, Error, Input>({
    gcTime: 0,
    mutationFn: async (input) => {
      if (!current()) return undefined
      pending.current?.abort()
      const controller = new AbortController()
      pending.current = controller
      try {
        const value = await mutationFn(input, controller.signal)
        return current() && !controller.signal.aborted ? value : undefined
      } catch (error) {
        // Old administrator operations cannot publish data or sign out a replacement identity.
        if (current() && !controller.signal.aborted) throw error
        return undefined
      }
    },
    onSuccess: (value) => {
      if (value !== undefined && current()) return onSuccess?.(value)
    },
    onError: (error) => {
      if (current()) return onError?.(error)
    },
  })
}
