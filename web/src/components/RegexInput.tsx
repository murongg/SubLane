import { useMemo, useRef, type ComponentProps, type ReactNode } from 'react'
import Prism from 'prismjs/components/prism-core'
import type { TokenStream } from 'prismjs'
import 'prismjs/components/prism-regex'
import { cn } from '@/lib/cn'
import { Textarea } from './ui/Textarea'

// Tokenization is local; Prism must never scan or mutate React-owned page content.
Prism.manual = true

const colors: Record<string, string> = {
  'char-class': 'text-success',
  'char-class-punctuation': 'text-success',
  'char-set': 'text-success',
  range: 'text-success',
  'range-punctuation': 'text-info',
  'char-class-negation': 'text-info',
  anchor: 'text-info',
  group: 'text-info',
  alternation: 'text-info',
  quantifier: 'text-warning',
  escape: 'text-violet-700 dark:text-violet-300',
  'special-escape': 'text-violet-700 dark:text-violet-300',
  backreference: 'text-violet-700 dark:text-violet-300',
}

function tokens(value: TokenStream): ReactNode {
  if (typeof value === 'string') return value
  if (Array.isArray(value))
    return value.map((item, index) => <span key={index}>{tokens(item)}</span>)
  // Render strings through React rather than interpolating highlighted HTML.
  return (
    <span data-token={value.type} className={colors[value.type]}>
      {tokens(value.content)}
    </span>
  )
}

type Props = Omit<ComponentProps<'textarea'>, 'value' | 'onChange'> & {
  value: string
  onChange: (value: string) => void
}

export function RegexInput({
  value,
  onChange,
  onScroll,
  className,
  rows = 2,
  maxLength = 1024,
  ...props
}: Props) {
  const mirror = useRef<HTMLPreElement>(null)
  const highlighted = useMemo(
    () => tokens(Prism.tokenize(value, Prism.languages.regex)),
    [value],
  )
  return (
    <div className="relative min-w-0 has-[:disabled]:opacity-50">
      <Textarea
        {...props}
        value={value}
        rows={rows}
        maxLength={maxLength}
        wrap="off"
        autoComplete="off"
        autoCapitalize="off"
        spellCheck={false}
        className={cn(
          'relative block resize-none overflow-auto whitespace-pre font-mono leading-6 text-transparent caret-foreground selection:bg-primary/20 selection:text-transparent disabled:opacity-100 md:leading-6',
          className,
        )}
        onChange={(event) => onChange(event.target.value)}
        onScroll={(event) => {
          if (mirror.current)
            mirror.current.style.transform = `translate(-${event.currentTarget.scrollLeft}px, -${event.currentTarget.scrollTop}px)`
          onScroll?.(event)
        }}
      />
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-px overflow-hidden rounded-md"
      >
        <pre
          ref={mirror}
          className="m-0 min-h-full w-max min-w-full whitespace-pre px-3 py-2 font-mono text-base leading-6 text-foreground md:text-sm md:leading-6"
        >
          {highlighted}
        </pre>
      </div>
    </div>
  )
}
