import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { keyOptions, type APIKey } from '@/lib/keys'
import { Button } from './ui/Button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'

export function UsageKey({
  userID,
  value,
  onChange,
}: {
  userID: number
  value: APIKey | null
  onChange: (value: APIKey | null) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [open, setOpen] = useState(false)
  const [page, setPage] = useState(1)
  const options = keyOptions(client, userID, page, true)
  const query = useQuery({
    ...options,
    enabled: open ? options.enabled : false,
  })
  const current = query.data?.keys.find((key) => key.id === value?.id) ?? value
  useEffect(() => {
    // Preserve refreshed metadata when the selected key leaves the current page.
    if (current && current !== value) onChange(current)
  }, [current, value, onChange])
  const label = (key: APIKey) =>
    key.name +
    ' · ' +
    key.prefix +
    (key.revoked_at === null ? '' : ' · ' + t('revoked'))
  const selected = current ? label(current) : t('usageAllKeys')
  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          aria-label={t('usageKey') + ': ' + selected}
          title={selected}
          className="max-w-full sm:max-w-72"
        >
          <span className="min-w-0 truncate">{selected}</span>
          <ChevronDown aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        className="max-w-[calc(100vw-2rem)] sm:max-w-96"
      >
        <DropdownMenuRadioGroup
          value={value ? String(value.id) : 'all'}
          onValueChange={(id) => {
            if (id === 'all') onChange(null)
            else {
              const key = query.data?.keys.find((key) => String(key.id) === id)
              if (key) onChange(key)
            }
          }}
        >
          <DropdownMenuRadioItem value="all">
            {t('usageAllKeys')}
          </DropdownMenuRadioItem>
          {query.isSuccess &&
            query.data.keys.map((key) => (
              <DropdownMenuRadioItem
                key={key.id}
                value={String(key.id)}
                className="items-start break-words"
              >
                {label(key)}
              </DropdownMenuRadioItem>
            ))}
        </DropdownMenuRadioGroup>
        {query.isPending ? (
          <p role="status" className="px-2 py-3 text-sm text-muted-foreground">
            {t('loadingKeys')}
          </p>
        ) : query.isError ? (
          <>
            <p role="alert" className="px-2 py-3 text-sm text-error">
              {t('keysLoadFailed')}
            </p>
            <DropdownMenuItem
              onSelect={(event) => {
                event.preventDefault()
                return query.refetch()
              }}
            >
              {t('reconnect')}
            </DropdownMenuItem>
          </>
        ) : query.data.total === 0 ? (
          <p className="px-2 py-3 text-sm text-muted-foreground">
            {t('noKeys')}
          </p>
        ) : (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">
              {t('paginationPage', {
                page: query.data.page,
                pages: Math.ceil(query.data.total / query.data.page_size),
              })}
            </DropdownMenuLabel>
            <DropdownMenuItem
              disabled={query.isFetching || query.data.page === 1}
              onSelect={(event) => {
                event.preventDefault()
                setPage(query.data.page - 1)
              }}
            >
              {t('previousPage')}
            </DropdownMenuItem>
            <DropdownMenuItem
              disabled={
                query.isFetching ||
                query.data.page * query.data.page_size >= query.data.total
              }
              onSelect={(event) => {
                event.preventDefault()
                setPage(query.data.page + 1)
              }}
            >
              {t('nextPage')}
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
