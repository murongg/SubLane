import { useEffect, useRef } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { authKey, type AuthState } from '@/lib/auth'
import { revealKey } from '@/lib/keys'

type Disclosure<T> = { secret: string; input: T; isCurrent: () => boolean }

export function useKeySecret<T = void>(
  userID: number,
  keyID: number,
  consume: (value: Disclosure<T>) => Promise<void> | void,
) {
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
  const isCurrentOwner = () =>
    active.current &&
    client.getQueryData<AuthState>(authKey)?.user?.id === userID
  const mutation = useMutation<void, Error, T>({
    gcTime: 0,
    mutationFn: async (input) => {
      if (!isCurrentOwner()) return
      pending.current?.abort()
      const controller = new AbortController()
      pending.current = controller
      const isCurrent = () => !controller.signal.aborted && isCurrentOwner()
      try {
        const result = await revealKey(keyID, controller.signal)
        if (!isCurrent()) return
        // Consumers may hold a value only in their mounted UI; mutation results never contain credentials.
        await consume({ secret: result.secret, input, isCurrent })
      } catch (error) {
        // An old 401 must not sign out the identity that replaced the original requester.
        if (isCurrent()) throw error
      }
    },
  })
  return { mutation, isCurrentOwner }
}
