const units = [
  [86400, 'day'],
  [3600, 'hour'],
  [60, 'minute'],
  [1, 'second'],
] as const

export function formatDuration(seconds: number, locale: string) {
  let remaining = seconds
  const parts: string[] = []
  for (const [size, unit] of units) {
    const value = Math.floor(remaining / size)
    remaining %= size
    if (value || (unit === 'second' && parts.length === 0)) {
      parts.push(
        new Intl.NumberFormat(locale, {
          style: 'unit',
          unit,
          unitDisplay: 'short',
        }).format(value),
      )
    }
    if (parts.length === 2) break
  }
  return parts.join(' ')
}
