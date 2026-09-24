import { useEffect, useRef, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { Proxy } from '@/lib/proxies'
import { Button } from './ui/Button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'
import { Input } from './ui/Input'

export function ProxyPicker({
  value,
  onChange,
  proxies,
  disabled = false,
}: {
  value: string
  onChange: (value: string) => void
  proxies: Proxy[]
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const searchInput = useRef<HTMLInputElement>(null)
  const options = useRef<HTMLDivElement>(null)
  const choices = [{ id: '', name: t('directConnection') }, ...proxies]
  const selected =
    choices.find((proxy) => proxy.id === value)?.name ?? t('directConnection')
  const matches = choices.filter((proxy) =>
    proxy.name.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()),
  )
  useEffect(() => {
    if (open) searchInput.current?.focus()
  }, [open])

  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium">{t('accountProxy')}</legend>
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
            type="button"
            variant="outline"
            disabled={disabled}
            aria-label={`${t('accountProxy')}: ${selected}`}
            className="w-full min-w-0 justify-between"
          >
            <span className="min-w-0 truncate">{selected}</span>
            <ChevronDown aria-hidden="true" className="size-4 shrink-0" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          className="w-(--radix-dropdown-menu-trigger-width) max-w-[calc(100vw-2rem)]"
        >
          <Input
            ref={searchInput}
            type="search"
            aria-label={t('proxySearch')}
            placeholder={t('proxySearch')}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
                event.preventDefault()
                event.stopPropagation()
                const items = options.current?.querySelectorAll<HTMLElement>(
                  '[role="menuitemradio"]',
                )
                const index =
                  event.key === 'ArrowDown' ? 0 : (items?.length ?? 0) - 1
                items?.item(index)?.focus()
              } else if (event.key === 'Enter') {
                event.preventDefault()
                event.stopPropagation()
                if (matches.length > 0) {
                  onChange(matches[0].id)
                  setOpen(false)
                }
              } else if (event.key !== 'Escape') {
                // Radix menu typeahead must not take text away from the search field.
                event.stopPropagation()
              }
            }}
            className="mb-1"
          />
          <div ref={options} className="max-h-52 overflow-y-auto">
            <DropdownMenuRadioGroup value={value} onValueChange={onChange}>
              {matches.map((proxy) => (
                <DropdownMenuRadioItem
                  key={proxy.id}
                  value={proxy.id}
                  className="whitespace-normal break-all"
                >
                  {proxy.name}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </div>
          {matches.length === 0 && (
            <p className="px-2 py-3 text-sm text-muted-foreground">
              {t('proxyNoMatch')}
            </p>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </fieldset>
  )
}
