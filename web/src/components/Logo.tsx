export function Logo({ className = 'size-8' }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 256 256"
      fill="currentColor"
      aria-hidden="true"
      className={className}
    >
      <path d="M80 16h136v32a24 24 0 0 1-24 24H88a16 16 0 0 0-16 16v64h-8a48 48 0 0 1-48-48V80a64 64 0 0 1 64-64Z" />
      <path d="M104 104h80a56 56 0 0 1 56 56v24a56 56 0 0 1-56 56H32v-32a24 24 0 0 1 24-24h116a12 12 0 0 0 12-12v-8a12 12 0 0 0-12-12h-68Z" />
    </svg>
  )
}
