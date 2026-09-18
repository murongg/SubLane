import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import { authKey, type AuthState } from './auth'
import { ApiError } from './request'

export async function replaceAuthState(client: QueryClient, state: AuthState) {
  // Cancel first so an older response cannot restore data from the previous session.
  await client.cancelQueries()
  client.setQueryData(authKey, state)
  client.removeQueries({
    predicate: (query) => query.queryKey[0] !== authKey[0],
  })
}

export function createQueryClient() {
  const onError = (error: Error) => {
    if (error instanceof ApiError && error.code === 'unauthorized') {
      void replaceAuthState(client, { initialized: true, user: null })
    }
  }
  const client: QueryClient = new QueryClient({
    queryCache: new QueryCache({ onError }),
    mutationCache: new MutationCache({ onError }),
    defaultOptions: {
      queries: {
        staleTime: 15_000,
        retry: false,
        refetchOnWindowFocus: false,
        // Old page observers can briefly remain mounted while the auth gate redirects.
        enabled: (query) =>
          query.queryKey[0] === authKey[0] ||
          client.getQueryData<AuthState>(authKey)?.user != null,
      },
    },
  })
  return client
}
