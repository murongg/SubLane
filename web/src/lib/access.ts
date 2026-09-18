import type { Role } from './auth'

const administratorPaths = ['/accounts', '/members']

export function canAccess(pathname: string, role: Role | undefined) {
  if (role === 'admin') return true
  if (role !== 'member') return false
  let path: string
  try {
    path = decodeURIComponent(pathname).toLowerCase().replace(/\/+$/, '')
  } catch {
    return false
  }
  return !administratorPaths.some(
    (prefix) => path === prefix || path.startsWith(prefix + '/'),
  )
}
