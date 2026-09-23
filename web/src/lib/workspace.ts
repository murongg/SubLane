const selectionKey = 'sublane.workspace'

export function selectedWorkspace(): number {
  if (typeof window === 'undefined') return 1
  try {
    const value = Number(window.sessionStorage.getItem(selectionKey))
    return Number.isSafeInteger(value) && value > 0 ? value : 1
  } catch {
    return 1
  }
}

export function selectWorkspace(id: number): void {
  if (!Number.isSafeInteger(id) || id <= 0) return
  window.sessionStorage.setItem(selectionKey, String(id))
}
