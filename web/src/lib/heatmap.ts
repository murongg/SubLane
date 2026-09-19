export const heatmapTones = [
  'bg-muted',
  'bg-foreground/20',
  'bg-foreground/40',
  'bg-foreground/60',
  'bg-foreground/85',
] as const

export function heatmapTone(value: number | null, maximum: number) {
  if (value === null) return 'bg-muted/40'
  return heatmapTones[
    value === 0 ? 0 : Math.min(4, Math.ceil((value / Math.max(1, maximum)) * 4))
  ]
}
