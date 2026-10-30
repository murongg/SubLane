import { useRef, useState, type ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'

type Option<Value extends string> = {
  value: Value
  label: string
  searchText?: string
  icon?: ReactNode
}

export function SearchPicker<Value extends string>({
  id,
  label,
  value,
  onChange,
  options,
  searchLabel,
  emptyLabel,
  disabled = false,
}: {
  id?: string
  label: string
  value: Value
  onChange: (value: Value) => void
  options: readonly Option<Value>[]
  searchLabel: string
  emptyLabel: string
  disabled?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const searchInput = useRef<HTMLInputElement>(null)
  const list = useRef<HTMLDivElement>(null)
  const selected =
    options.find((option) => option.value === value) ?? options[0]
  const term = search.trim().toLocaleLowerCase()
  const matches = options.filter((option) =>
    `${option.label} ${option.searchText ?? ''}`
      .toLocaleLowerCase()
      .includes(term),
  )
  const select = (next: Value) => {
    onChange(next)
    setOpen(false)
  }
  return (
    <DropdownMenu
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (next) setSearch('')
      }}
      modal={false}
    >
      <DropdownMenuTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          disabled={disabled}
          aria-label={`${label}: ${selected?.label ?? label}`}
          className="w-full min-w-0 justify-between"
        >
          <span className="flex min-w-0 items-center gap-2">
            {selected?.icon}
            <span className="min-w-0 truncate">{selected?.label ?? label}</span>
          </span>
          <ChevronDown aria-hidden="true" className="size-4 shrink-0" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        aria-label={label}
        aria-labelledby={undefined}
        onFocus={(event) => {
          if (event.target === event.currentTarget) {
            // Redirect the menu's initial focus without intercepting focus on result rows.
            event.preventDefault()
            searchInput.current?.focus()
          }
        }}
        className="w-(--radix-dropdown-menu-trigger-width) max-w-[calc(100vw-2rem)]"
      >
        <Input
          ref={searchInput}
          type="search"
          autoComplete="off"
          aria-label={searchLabel}
          placeholder={searchLabel}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          onKeyDown={(event) => {
            // IME confirmation commits search text instead of selecting a result.
            if (event.nativeEvent.isComposing) {
              event.stopPropagation()
            } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
              event.preventDefault()
              event.stopPropagation()
              const items = list.current?.querySelectorAll<HTMLElement>(
                '[role="menuitemradio"]',
              )
              items
                ?.item(event.key === 'ArrowDown' ? 0 : items.length - 1)
                ?.focus()
            } else if (event.key === 'Enter') {
              event.preventDefault()
              event.stopPropagation()
              if (matches.length) select(matches[0].value)
            } else if (event.key !== 'Escape') {
              // Keep Radix typeahead from moving focus away while the user types a search.
              event.stopPropagation()
            }
          }}
          className="mb-1"
        />
        <div ref={list} className="max-h-52 overflow-y-auto">
          <DropdownMenuRadioGroup
            value={value}
            onValueChange={(next) => select(next as Value)}
          >
            {matches.map((option) => (
              <DropdownMenuRadioItem
                key={option.value}
                value={option.value}
                className="whitespace-normal break-all"
              >
                {option.icon}
                {option.label}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </div>
        {matches.length === 0 && (
          <p role="status" className="px-2 py-3 text-sm text-muted-foreground">
            {emptyLabel}
          </p>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
