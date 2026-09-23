import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Building2, Check, ChevronsUpDown, Plus, RotateCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { tenantsOptions } from '@/lib/tenants'
import { selectedWorkspace, selectWorkspace } from '@/lib/workspace'
import { WorkspaceCreate } from './WorkspaceCreate'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'

export function WorkspaceSelect() {
  const { t } = useTranslation()
  const query = useQuery(tenantsOptions)
  const [creating, setCreating] = useState(false)
  const currentID = selectedWorkspace()
  const current = query.data?.tenants.find((tenant) => tenant.id === currentID)
  return (
    <div className="min-w-0 flex-1">
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            aria-label={
              current ? `${t('workspace')}: ${current.name}` : t('workspace')
            }
            className="flex h-10 w-full min-w-0 items-center gap-2 rounded-lg border border-border bg-background px-2.5 text-start text-sm font-medium outline-none transition-colors hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring [@media(pointer:coarse)]:min-h-11"
          >
            <Building2 aria-hidden="true" className="size-4 shrink-0" />
            <span className="min-w-0 flex-1 truncate" title={current?.name}>
              {current?.name ?? t('workspace')}
            </span>
            <ChevronsUpDown
              aria-hidden="true"
              className="size-4 shrink-0 text-muted-foreground"
            />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          sideOffset={8}
          className="w-64 max-w-[calc(100vw-2rem)] p-2"
        >
          <DropdownMenuLabel className="px-2 py-2 text-xs text-muted-foreground">
            {t('workspaces')}
          </DropdownMenuLabel>
          {query.isPending ? (
            <p
              role="status"
              className="px-2 py-3 text-sm text-muted-foreground"
            >
              {t('loading')}
            </p>
          ) : query.isError ? (
            <div className="space-y-1">
              <p role="alert" className="px-2 py-2 text-sm text-error">
                {t('workspaceListFailed')}
              </p>
              <DropdownMenuItem
                onSelect={() => {
                  query.refetch()
                }}
              >
                <RotateCw aria-hidden="true" />
                {t('reconnect')}
              </DropdownMenuItem>
            </div>
          ) : query.data.tenants.length === 0 ? (
            <p className="px-2 py-3 text-sm text-muted-foreground">
              {t('workspaceNoneActive')}
            </p>
          ) : (
            query.data.tenants.map((tenant) => (
              <DropdownMenuItem
                key={tenant.id}
                disabled={tenant.status !== 'active'}
                aria-current={tenant.id === currentID ? 'true' : undefined}
                onSelect={() => {
                  if (tenant.id === currentID) return
                  selectWorkspace(tenant.id)
                  window.location.assign('/')
                }}
              >
                <span className="min-w-0 flex-1 truncate" title={tenant.name}>
                  {tenant.name}
                </span>
                {tenant.id === currentID && (
                  <Check aria-hidden="true" className="size-4" />
                )}
              </DropdownMenuItem>
            ))
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => setCreating(true)}>
            <span className="min-w-0 flex-1">{t('workspaceCreate')}</span>
            <Plus aria-hidden="true" className="size-4" />
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <WorkspaceCreate open={creating} onOpenChange={setCreating} />
    </div>
  )
}
