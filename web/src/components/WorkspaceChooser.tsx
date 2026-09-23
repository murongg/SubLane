import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { tenantsOptions } from '@/lib/tenants'
import { selectWorkspace } from '@/lib/workspace'
import { AuthShell } from './AuthShell'
import { WorkspaceCreate } from './WorkspaceCreate'
import { Button } from './ui/Button'

export function WorkspaceChooser() {
  const { t } = useTranslation()
  const query = useQuery(tenantsOptions)
  const active =
    query.data?.tenants.filter((tenant) => tenant.status === 'active') ?? []
  return (
    <AuthShell>
      <div className="space-y-5">
        <div className="space-y-2">
          <h1 className="text-xl font-semibold">{t('workspaceChooseTitle')}</h1>
          <p className="text-sm leading-6 text-muted-foreground">
            {t('workspaceChooseDescription')}
          </p>
        </div>
        {query.isPending ? (
          <p role="status" className="text-sm text-muted-foreground">
            {t('loading')}
          </p>
        ) : query.isError ? (
          <div role="alert" className="space-y-3">
            <p className="text-sm text-error">{t('unavailable')}</p>
            <Button variant="outline" onClick={() => query.refetch()}>
              {t('reconnect')}
            </Button>
          </div>
        ) : active.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('workspaceNoneActive')}
          </p>
        ) : (
          <div className="grid gap-2">
            {active.map((tenant) => (
              <Button
                key={tenant.id}
                type="button"
                variant="outline"
                className="justify-start"
                onClick={() => {
                  selectWorkspace(tenant.id)
                  window.location.assign('/')
                }}
              >
                {tenant.name}
              </Button>
            ))}
          </div>
        )}
        <WorkspaceCreate />
      </div>
    </AuthShell>
  )
}
